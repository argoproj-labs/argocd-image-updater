package argocd

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	gitlab "gitlab.com/gitlab-org/api/client-go"

	"github.com/argoproj-labs/argocd-image-updater/ext/git"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/log"
)

// ErrMRAlreadyExists is returned by create when GitLab reports that an open
// MR for the same source → target pair already exists. The caller can treat
// this as a successful no-op rather than a reconciliation failure.
var ErrMRAlreadyExists = errors.New("MR already exists")

// GitLabMRService implements PullRequestService for GitLab and self-managed GitLab instances.
type GitLabMRService struct {
	client    *gitlab.Client
	projectID string // namespace/project path (e.g. "group/subgroup/repo")
	pr        *PullRequest
}

var _ PullRequestService = (*GitLabMRService)(nil)

// create opens a merge request using the already-configured client.
// If GitLab reports that an open MR for the same source → target pair already
// exists (HTTP 409), ErrMRAlreadyExists is returned so the caller can treat
// the situation as a no-op rather than a reconciliation failure.
func (g *GitLabMRService) create(ctx context.Context) error {
	logCtx := log.LoggerFromContext(ctx)

	if g.pr == nil {
		return fmt.Errorf("cannot create MR: merge request metadata is nil")
	}

	opts := &gitlab.CreateMergeRequestOptions{
		Title:        gitlab.Ptr(g.pr.title),
		SourceBranch: gitlab.Ptr(g.pr.head),
		TargetBranch: gitlab.Ptr(g.pr.base),
		Description:  gitlab.Ptr(g.pr.body),
	}

	mr, resp, err := g.client.MergeRequests.CreateMergeRequest(g.projectID, opts, gitlab.WithContext(ctx))
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusConflict {
			logCtx.Infof("MR %q → %q already exists, skipping creation", g.pr.head, g.pr.base)
			return ErrMRAlreadyExists
		}
		return fmt.Errorf("could not create MR %q → %q: %w", g.pr.head, g.pr.base, err)
	}

	logCtx.Infof("created MR !%d %q → %q: %s", mr.IID, g.pr.head, g.pr.base, mr.WebURL)
	return nil
}

// list returns true if there is already an open MR from pushBranch into
// checkOutBranch.
func (g *GitLabMRService) list(ctx context.Context, checkOutBranch, pushBranch string) (bool, error) {
	// TODO: implement MR listing for idempotency check
	return false, nil
}

// NewGitLabMRService builds a GitLabMRService from the resolved write-back config
// and the credential token provider. It obtains a token, resolves the API base
// URL, and parses the project path from the repo URL.
func NewGitLabMRService(ctx context.Context, wbc *WriteBackConfig, tokenProvider git.SCMTokenProvider) (*GitLabMRService, error) {
	logCtx := log.LoggerFromContext(ctx)

	token, err := tokenProvider.SCMToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("could not obtain SCM token: %w", err)
	}
	if token == "" {
		return nil, fmt.Errorf("empty SCM token: GitLab MR creation requires authentication")
	}

	// SCMAPIBaseURLProvider carries an explicit base URL.
	// Otherwise derive from the repo URL for self-managed GitLab instances.
	// For gitlab.com, apiBaseURL stays empty and the default https://gitlab.com is used.
	apiBaseURL := ""
	if urlProvider, ok := tokenProvider.(git.SCMAPIBaseURLProvider); ok {
		apiBaseURL = urlProvider.SCMAPIBaseURL()
	} else if isSSH, _ := git.IsSSHURL(wbc.GitRepo); !isSSH || strings.HasPrefix(wbc.GitRepo, "ssh://") {
		// SCP-style SSH URLs (git@host:path) are not valid for url.Parse —
		// skip them and fall through with the default gitlab.com base URL.
		// Only HTTPS and ssh:// scheme URLs can be parsed to derive the host.
		u, parseErr := url.Parse(wbc.GitRepo)
		if parseErr != nil {
			return nil, fmt.Errorf("could not parse repo URL %q: %w", wbc.GitRepo, parseErr)
		}
		if u.Host != "gitlab.com" {
			apiBaseURL = u.Scheme + "://" + u.Host
		}
	}

	var opts []gitlab.ClientOptionFunc
	if apiBaseURL != "" {
		opts = append(opts, gitlab.WithBaseURL(apiBaseURL))
	}

	client, err := gitlab.NewClient(token, opts...)
	if err != nil {
		return nil, fmt.Errorf("could not create GitLab client: %w", err)
	}

	projectPath, err := parseGitLabProject(wbc.GitRepo)
	if err != nil {
		return nil, fmt.Errorf("could not parse project path from %q: %w", wbc.GitRepo, err)
	}

	logCtx.Infof("GitLab MR service initialised for %s", projectPath)
	return &GitLabMRService{
		client:    client,
		projectID: projectPath,
		pr:        wbc.PullRequest,
	}, nil
}

// parseGitLabProject extracts the full project path (namespace/project) from a
// Git URL. Unlike GitHub (always owner/repo), GitLab supports nested groups so
// the returned path may contain more than two segments
// (e.g. "group/subgroup/repo").
// Handles SCP-style SSH  (git@gitlab.com:group/repo.git),
// ssh:// URLs           (ssh://git@gitlab.com/group/repo.git),
// and HTTPS/HTTP URLs   (https://gitlab.com/group/repo.git).
func parseGitLabProject(repoURL string) (string, error) {
	var pathStr string

	if isSSH, _ := git.IsSSHURL(repoURL); isSSH {
		if !strings.HasPrefix(repoURL, "ssh://") {
			// SCP-style: git@gitlab.com:group/repo.git
			idx := strings.Index(repoURL, ":")
			if idx < 0 {
				return "", fmt.Errorf("malformed SSH repo URL %q: missing colon separator", repoURL)
			}
			pathStr = repoURL[idx+1:]
		} else {
			// ssh://git@gitlab.com/group/repo.git
			u, parseErr := url.Parse(repoURL)
			if parseErr != nil {
				return "", fmt.Errorf("invalid SSH repo URL %q: %w", repoURL, parseErr)
			}
			pathStr = strings.TrimPrefix(u.Path, "/")
		}
	} else {
		// HTTPS or HTTP
		u, parseErr := url.Parse(repoURL)
		if parseErr != nil {
			return "", fmt.Errorf("invalid repo URL %q: %w", repoURL, parseErr)
		}
		pathStr = strings.TrimPrefix(u.Path, "/")
	}

	pathStr = strings.TrimSuffix(pathStr, ".git")

	// GitLab requires at least namespace/project (two segments).
	parts := strings.SplitN(pathStr, "/", 2)
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return "", fmt.Errorf("repo URL %q does not contain a namespace/project path", repoURL)
	}

	return pathStr, nil
}
