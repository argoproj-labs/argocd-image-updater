package argocd

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/go-github/v69/github"

	"github.com/argoproj-labs/argocd-image-updater/ext/git"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/log"
)

// ErrPRAlreadyExists is returned by create when GitHub reports that an open
// PR for the same head → base pair already exists. The caller can treat this
// as a successful no-op rather than a reconciliation failure.
var ErrPRAlreadyExists = errors.New("PR already exists")

// GithubPRService implements PullRequestService for GitHub and GitHub Enterprise.
type GithubPRService struct {
	client *github.Client
	owner  string
	repo   string
	pr     *PullRequest
}

var _ PullRequestService = (*GithubPRService)(nil)

// create opens a pull request using the already-configured client.
// If GitHub reports that an open PR for the same head → base pair already
// exists (HTTP 422), ErrPRAlreadyExists is returned so the caller can treat
// the situation as a no-op rather than a reconciliation failure.
func (g *GithubPRService) create(ctx context.Context) error {
	logCtx := log.LoggerFromContext(ctx)

	if g.pr == nil {
		return fmt.Errorf("cannot create PR: pull request metadata is nil")
	}

	newPR := &github.NewPullRequest{
		Title: github.Ptr(g.pr.title),
		Head:  github.Ptr(g.pr.head),
		Base:  github.Ptr(g.pr.base),
		Body:  github.Ptr(g.pr.body),
	}
	githubPullRequest, _, err := g.client.PullRequests.Create(ctx, g.owner, g.repo, newPR)
	if err != nil {
		if isAlreadyExistsError(err) {
			logCtx.Infof("PR %q → %q already exists, skipping creation", g.pr.head, g.pr.base)
			return ErrPRAlreadyExists
		}
		return fmt.Errorf("could not create PR %q → %q: %w", g.pr.head, g.pr.base, err)
	}
	logCtx.Infof("created PR #%d %q → %q: %s", githubPullRequest.GetNumber(), g.pr.head, g.pr.base, githubPullRequest.GetHTMLURL())

	return nil
}

// upsert creates or updates a GitHub pull request for the configured head → base pair.
func (g *GithubPRService) upsert(ctx context.Context, reopenClosed bool) error {
	logCtx := log.LoggerFromContext(ctx)

	if g.pr == nil {
		return fmt.Errorf("cannot upsert PR: pull request metadata is nil")
	}

	prs, err := g.listMatching(ctx)
	if err != nil {
		return err
	}

	for _, pr := range prs {
		if pr.GetState() == "open" {
			body := g.pr.body
			if commitMessages, err := g.listCommitMessages(ctx, pr.GetNumber()); err != nil {
				logCtx.Warnf("could not build PR body from commits for PR #%d: %v", pr.GetNumber(), err)
			} else {
				body = buildUpsertPullRequestBody(commitMessages, g.pr.head, g.pr.base)
			}
			updated, _, err := g.client.PullRequests.Edit(ctx, g.owner, g.repo, pr.GetNumber(), &github.PullRequest{
				Title: github.Ptr(g.pr.title),
				Body:  github.Ptr(body),
			})
			if err != nil {
				return fmt.Errorf("could not update PR #%d %q → %q: %w", pr.GetNumber(), g.pr.head, g.pr.base, err)
			}
			logCtx.Infof("updated PR #%d %q → %q: %s", updated.GetNumber(), g.pr.head, g.pr.base, updated.GetHTMLURL())
			return nil
		}
	}

	for _, pr := range prs {
		if pr.GetState() == "closed" && pr.MergedAt == nil {
			if reopenClosed {
				body := g.pr.body
				if commitMessages, err := g.listCommitMessages(ctx, pr.GetNumber()); err != nil {
					logCtx.Warnf("could not build PR body from commits for PR #%d: %v", pr.GetNumber(), err)
				} else {
					body = buildUpsertPullRequestBody(commitMessages, g.pr.head, g.pr.base)
				}
				updated, _, err := g.client.PullRequests.Edit(ctx, g.owner, g.repo, pr.GetNumber(), &github.PullRequest{
					Title: github.Ptr(g.pr.title),
					Body:  github.Ptr(body),
					State: github.Ptr("open"),
				})
				if err != nil {
					return fmt.Errorf("could not reopen PR #%d %q → %q: %w", pr.GetNumber(), g.pr.head, g.pr.base, err)
				}
				logCtx.Infof("reopened PR #%d %q → %q: %s", updated.GetNumber(), g.pr.head, g.pr.base, updated.GetHTMLURL())
				return nil
			}

			err := g.create(ctx)
			if err != nil {
				return fmt.Errorf("closed unmerged PR #%d already exists for %q → %q and reopenClosed is false; creating a new PR failed: %w", pr.GetNumber(), g.pr.head, g.pr.base, err)
			}
			return nil
		}
	}

	return g.create(ctx)
}

func (g *GithubPRService) listCommitMessages(ctx context.Context, number int) ([]string, error) {
	var messages []string
	opts := &github.ListOptions{PerPage: 100}
	for {
		commits, resp, err := g.client.PullRequests.ListCommits(ctx, g.owner, g.repo, number, opts)
		if err != nil {
			return nil, err
		}
		for _, commit := range commits {
			if commit.Commit != nil && commit.Commit.Message != nil {
				messages = append(messages, commit.Commit.GetMessage())
			}
		}
		if resp == nil || resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return messages, nil
}

func (g *GithubPRService) listMatching(ctx context.Context) ([]*github.PullRequest, error) {
	opts := &github.PullRequestListOptions{
		State: "all",
		Head:  fmt.Sprintf("%s:%s", g.owner, g.pr.head),
		Base:  g.pr.base,
		ListOptions: github.ListOptions{
			PerPage: 100,
		},
	}

	var pulls []*github.PullRequest
	for {
		page, resp, err := g.client.PullRequests.List(ctx, g.owner, g.repo, opts)
		if err != nil {
			return nil, fmt.Errorf("could not list PRs for %q → %q: %w", g.pr.head, g.pr.base, err)
		}
		pulls = append(pulls, page...)
		if resp == nil || resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return pulls, nil
}

// isAlreadyExistsError reports whether err is a GitHub 422 response whose
// error list contains an "A pull request already exists" message.
func isAlreadyExistsError(err error) bool {
	var ghErr *github.ErrorResponse
	if !errors.As(err, &ghErr) {
		return false
	}
	if ghErr.Response == nil || ghErr.Response.StatusCode != http.StatusUnprocessableEntity {
		return false
	}
	for _, e := range ghErr.Errors {
		if strings.Contains(e.Message, "A pull request already exists") {
			return true
		}
	}
	return false
}

// list returns true if there is already an open PR from pushBranch into
// checkOutBranch.
func (g *GithubPRService) list(ctx context.Context, checkOutBranch, pushBranch string) (bool, error) {
	if g.pr == nil {
		g.pr = &PullRequest{base: checkOutBranch, head: pushBranch}
	}
	prs, err := g.listMatching(ctx)
	if err != nil {
		return false, err
	}
	for _, pr := range prs {
		if pr.GetState() == "open" {
			return true, nil
		}
	}
	return false, nil
}

// NewGithubPRService builds a GithubPRService from the resolved write-back config
// and the credential token provider. It obtains a token, resolves the API base
// URL, and parses the owner/repo from the repo URL.
func NewGithubPRService(ctx context.Context, wbc *WriteBackConfig, tokenProvider git.SCMTokenProvider) (*GithubPRService, error) {
	logCtx := log.LoggerFromContext(ctx)

	token, err := tokenProvider.SCMToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("could not obtain SCM token: %w", err)
	}
	if token == "" {
		return nil, fmt.Errorf("empty SCM token: GitHub PR creation requires authentication")
	}

	// GitHub App carries its own enterprise base URL via SCMAPIBaseURLProvider.
	// HTTPS PAT does not — for GHE instances derive the API base URL from the
	// repo URL (e.g. https://github.example.com/org/repo.git → https://github.example.com/api/v3).
	// For github.com, apiBaseURL stays empty and the default api.github.com is used.
	apiBaseURL := ""
	if urlProvider, ok := tokenProvider.(git.SCMAPIBaseURLProvider); ok {
		apiBaseURL = urlProvider.SCMAPIBaseURL()
	} else {
		u, parseErr := url.Parse(wbc.GitRepo)
		if parseErr != nil {
			return nil, fmt.Errorf("could not parse repo URL %q: %w", wbc.GitRepo, parseErr)
		}
		if u.Host != "github.com" {
			apiBaseURL = u.Scheme + "://" + u.Host + "/api/v3"
		}
	}

	var client *github.Client
	if apiBaseURL == "" {
		// github.com: no enterprise URLs needed, nil uses http.DefaultClient
		client = github.NewClient(nil).WithAuthToken(token)
	} else {
		// uploadURL must be scheme+host only so WithEnterpriseURLs appends
		// /api/uploads/ correctly — passing apiBaseURL for both would produce
		// /api/v3/api/uploads/ when apiBaseURL already contains /api/v3.
		u, parseErr := url.Parse(apiBaseURL)
		if parseErr != nil || u == nil || u.Scheme == "" || u.Host == "" {
			return nil, fmt.Errorf("invalid GitHub API base URL %q: %w", apiBaseURL, parseErr)
		}
		uploadURL := u.Scheme + "://" + u.Host
		client, err = github.NewClient(nil).WithAuthToken(token).WithEnterpriseURLs(apiBaseURL, uploadURL)
		if err != nil {
			return nil, fmt.Errorf("could not create GitHub enterprise client for %q: %w", apiBaseURL, err)
		}
	}

	owner, repoName, err := parseGitHubOwnerRepo(wbc.GitRepo)
	if err != nil {
		return nil, fmt.Errorf("could not parse owner/repo from %q: %w", wbc.GitRepo, err)
	}

	logCtx.Infof("GitHub PR service initialised for %s/%s", owner, repoName)
	return &GithubPRService{
		client: client,
		owner:  owner,
		repo:   repoName,
		pr:     wbc.PullRequest,
	}, nil
}

// parseGitHubOwnerRepo extracts the owner and repository name from a Git URL.
// Handles SCP-style SSH  (git@github.com:owner/repo.git),
// ssh:// URLs           (ssh://git@github.com/owner/repo.git),
// and HTTPS/HTTP URLs   (https://github.com/owner/repo.git).
// Returns an error for URLs that do not carry an owner/repo path segment
// (e.g. local test registries like https://127.0.0.1:30003/testdata.git).
func parseGitHubOwnerRepo(repoURL string) (owner, repo string, err error) {
	var pathStr string

	if isSSH, _ := git.IsSSHURL(repoURL); isSSH {
		if !strings.HasPrefix(repoURL, "ssh://") {
			// SCP-style: git@github.com:owner/repo.git
			// The colon separates the host from the path.
			idx := strings.Index(repoURL, ":")
			if idx < 0 {
				return "", "", fmt.Errorf("malformed SSH repo URL %q: missing colon separator", repoURL)
			}
			pathStr = repoURL[idx+1:]
		} else {
			// ssh://git@github.com/owner/repo.git
			u, parseErr := url.Parse(repoURL)
			if parseErr != nil {
				return "", "", fmt.Errorf("invalid SSH repo URL %q: %w", repoURL, parseErr)
			}
			pathStr = strings.TrimPrefix(u.Path, "/")
		}
	} else {
		// HTTPS or HTTP
		u, parseErr := url.Parse(repoURL)
		if parseErr != nil {
			return "", "", fmt.Errorf("invalid repo URL %q: %w", repoURL, parseErr)
		}
		pathStr = strings.TrimPrefix(u.Path, "/")
	}

	pathStr = strings.TrimSuffix(pathStr, ".git")
	parts := strings.SplitN(pathStr, "/", 3)
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("repo URL %q does not contain an owner/repo path", repoURL)
	}
	return parts[0], parts[1], nil
}
