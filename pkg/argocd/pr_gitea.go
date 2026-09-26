package argocd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/argoproj-labs/argocd-image-updater/ext/git"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/log"
)

// giteaListPageSize is the page size requested when listing open pull requests.
// Gitea caps it at its MAX_RESPONSE_ITEMS setting (50 by default), so the
// listing stops on the X-Total-Count header or on an empty page rather than
// on a short one.
const giteaListPageSize = 50

// giteaMaxListPages bounds the open-PR scan in exists. Past that point the
// check gives up and create relies on Gitea's 409 duplicate detection.
const giteaMaxListPages = 20

// GiteaPRService implements PullRequestService for Gitea and Forgejo.
type GiteaPRService struct {
	client *http.Client
	// apiURL is the repository API root, e.g. https://gitea.example.com/api/v1/repos/owner/repo.
	apiURL *url.URL
	token  string
	pr     *PullRequest
}

var _ PullRequestService = (*GiteaPRService)(nil)

// giteaAPIError is a non-2xx answer from the Gitea API.
type giteaAPIError struct {
	statusCode int
	status     string
	message    string
}

func (e *giteaAPIError) Error() string {
	return fmt.Sprintf("Gitea API returned %s: %s", e.status, e.message)
}

// giteaPullRequest holds the fields of a Gitea pull request used by this service.
type giteaPullRequest struct {
	Number  int64  `json:"number"`
	HTMLURL string `json:"html_url"`
	Head    struct {
		Ref    string `json:"ref"`
		RepoID int64  `json:"repo_id"`
	} `json:"head"`
	Base struct {
		Ref    string `json:"ref"`
		RepoID int64  `json:"repo_id"`
	} `json:"base"`
}

// create opens a pull request. Gitea answers 409 Conflict when an open PR for
// the same head → base pair already exists; that case returns ErrPRAlreadyExists
// so the caller can treat it as a no-op.
func (g *GiteaPRService) create(ctx context.Context) error {
	logCtx := log.LoggerFromContext(ctx)

	if g.pr == nil {
		return fmt.Errorf("cannot create PR: pull request metadata is nil")
	}

	// The creation API only accepts label IDs, so labels are applied by name
	// in a follow-up call once the PR exists.
	payload := map[string]any{
		"title": g.pr.title,
		"body":  g.pr.body,
		"head":  g.pr.head,
		"base":  g.pr.base,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("could not encode PR: %w", err)
	}
	var pr giteaPullRequest
	if _, err := g.request(ctx, http.MethodPost, g.apiURL.JoinPath("pulls"), bytes.NewReader(body), &pr); err != nil {
		var apiErr *giteaAPIError
		if errors.As(err, &apiErr) && apiErr.statusCode == http.StatusConflict {
			logCtx.Infof("PR %q → %q already exists, skipping creation", g.pr.head, g.pr.base)
			return ErrPRAlreadyExists
		}
		return fmt.Errorf("could not create PR %q → %q: %w", g.pr.head, g.pr.base, err)
	}
	if pr.Number <= 0 {
		return fmt.Errorf("could not create PR: Gitea response did not contain a valid pull request number")
	}
	logCtx.Infof("created PR #%d %q → %q: %s", pr.Number, g.pr.head, g.pr.base, pr.HTMLURL)

	// A labelling failure must not fail the whole update: the PR exists.
	if len(g.pr.labels) > 0 {
		if err := g.addLabels(ctx, pr.Number, g.pr.labels); err != nil {
			logCtx.Warnf("could not add labels %v to PR #%d: %v", g.pr.labels, pr.Number, err)
		}
	}
	return nil
}

// addLabels applies labels by name to the pull request. Gitea only attaches
// labels that already exist in the repository or its organization.
func (g *GiteaPRService) addLabels(ctx context.Context, number int64, names []string) error {
	body, err := json.Marshal(map[string][]string{"labels": names})
	if err != nil {
		return fmt.Errorf("could not encode PR labels: %w", err)
	}
	endpoint := g.apiURL.JoinPath("issues", strconv.FormatInt(number, 10), "labels")
	var labels json.RawMessage
	_, err = g.request(ctx, http.MethodPost, endpoint, bytes.NewReader(body), &labels)
	return err
}

// exists reports whether an open PR from pushBranch into checkOutBranch exists
// in the repository. The list endpoint does not filter on the head branch in
// every Gitea and Forgejo version, so the match is done on the client side.
func (g *GiteaPRService) exists(ctx context.Context, checkOutBranch, pushBranch string) (bool, error) {
	seen := 0
	for page := 1; page <= giteaMaxListPages; page++ {
		u := *g.apiURL.JoinPath("pulls")
		query := url.Values{}
		query.Set("state", "open")
		// Narrows the listing on Gitea versions that support it; ignored otherwise.
		query.Set("base_branch", checkOutBranch)
		query.Set("page", strconv.Itoa(page))
		query.Set("limit", strconv.Itoa(giteaListPageSize))
		u.RawQuery = query.Encode()

		var prs []giteaPullRequest
		header, err := g.request(ctx, http.MethodGet, &u, nil, &prs)
		if err != nil {
			return false, fmt.Errorf("could not list Gitea PRs: %w", err)
		}
		if len(prs) == 0 {
			return false, nil
		}
		for _, pr := range prs {
			// Same-repository PRs only: a fork may carry a branch with the same name.
			if pr.Head.Ref == pushBranch && pr.Base.Ref == checkOutBranch && pr.Head.RepoID == pr.Base.RepoID {
				return true, nil
			}
		}
		// Saves the request for an empty page when the total is known.
		seen += len(prs)
		if total, err := strconv.Atoi(header.Get("X-Total-Count")); err == nil && seen >= total {
			return false, nil
		}
	}
	return false, fmt.Errorf("more than %d pages of open PRs, giving up the scan", giteaMaxListPages)
}

// request calls the Gitea REST API using the token from the Git credentials
// and returns the response headers.
func (g *GiteaPRService) request(ctx context.Context, method string, endpoint *url.URL, body io.Reader, result any) (http.Header, error) {
	req, err := http.NewRequestWithContext(ctx, method, endpoint.String(), body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "token "+g.token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := g.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		var apiErr struct {
			Message string `json:"message"`
		}
		// Non-JSON error responses still report their HTTP status.
		_ = json.NewDecoder(resp.Body).Decode(&apiErr)
		return nil, &giteaAPIError{statusCode: resp.StatusCode, status: resp.Status, message: apiErr.Message}
	}
	if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
		return nil, fmt.Errorf("could not decode Gitea response: %w", err)
	}
	return resp.Header, nil
}

// NewGiteaPRService builds a service from the repository URL and an access token.
// The API URL is derived from the repository URL, keeping any sub-path the
// instance is served under (e.g. https://example.com/gitea/owner/repo.git).
func NewGiteaPRService(ctx context.Context, wbc *WriteBackConfig, tokenProvider git.SCMTokenProvider) (*GiteaPRService, error) {
	token, err := tokenProvider.SCMToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("could not obtain SCM token: %w", err)
	}
	if token == "" {
		return nil, fmt.Errorf("empty SCM token: Gitea PR creation requires authentication")
	}
	apiURL, owner, repo, err := giteaRepoAPIURL(wbc.GitRepo)
	if err != nil {
		return nil, err
	}
	log.LoggerFromContext(ctx).Infof("Gitea PR service initialised for %s/%s", owner, repo)
	return &GiteaPRService{
		client: &http.Client{
			Timeout: 30 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				// The token travels in a header: never follow a redirect to
				// another host, nor from HTTPS down to plain HTTP.
				if !strings.EqualFold(req.URL.Host, via[0].URL.Host) || (via[0].URL.Scheme == "https" && req.URL.Scheme != "https") {
					return fmt.Errorf("refusing Gitea redirect to a different host or non-HTTPS URL")
				}
				// A 301, 302 or 303 turns a POST into a body-less GET, which
				// would surface as a misleading decoding error.
				if req.Method != via[0].Method {
					return fmt.Errorf("refusing Gitea redirect that turns %s into %s", via[0].Method, req.Method)
				}
				if len(via) >= 10 {
					return fmt.Errorf("stopped after 10 redirects")
				}
				return nil
			},
		},
		apiURL: apiURL,
		token:  token,
		pr:     wbc.PullRequest,
	}, nil
}

// giteaRepoAPIURL derives the repository API root from an HTTP(S) clone URL:
// https://host[/sub/path]/owner/repo[.git] → https://host[/sub/path]/api/v1/repos/owner/repo.
func giteaRepoAPIURL(repoURL string) (*url.URL, string, string, error) {
	u, err := url.Parse(repoURL)
	if err != nil {
		return nil, "", "", fmt.Errorf("could not parse Gitea repo URL: %w", err)
	}
	if (u.Scheme != "https" && u.Scheme != "http") || u.Hostname() == "" {
		return nil, "", "", fmt.Errorf("Gitea PR creation requires an HTTP(S) repository URL with a host")
	}
	segments := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(segments) < 2 {
		return nil, "", "", fmt.Errorf("Gitea repository URL must end with /{owner}/{repository}")
	}
	owner := segments[len(segments)-2]
	repo := strings.TrimSuffix(segments[len(segments)-1], ".git")
	if owner == "" || repo == "" {
		return nil, "", "", fmt.Errorf("Gitea repository URL must end with /{owner}/{repository}")
	}
	api := &url.URL{Scheme: u.Scheme, Host: u.Host}
	api = api.JoinPath(slices.Concat(segments[:len(segments)-2], []string{"api", "v1", "repos", owner, repo})...)
	return api, owner, repo, nil
}
