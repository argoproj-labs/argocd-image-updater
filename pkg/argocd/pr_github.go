package argocd

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/google/go-github/v69/github"

	"github.com/argoproj-labs/argocd-image-updater/ext/git"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/log"
)

// GithubService implements PullRequestService for GitHub and GitHub Enterprise.
type GithubService struct {
	client *github.Client
	owner  string
	repo   string
}

var _ PullRequestService = (*GithubService)(nil)

// create opens a pull request using the already-configured client.
func (g *GithubService) create(ctx context.Context) error {
	// TODO: implement PR creation using g.client, g.owner, g.repo
	return nil
}

func (g *GithubService) list(ctx context.Context, checkOutBranch, pushBranch string) error {
	// TODO: implement PR listing for idempotency check
	return nil
}

// NewGithubService builds a GithubService from the resolved write-back config
// and the credential token provider. It obtains a token, resolves the API base
// URL, and parses the owner/repo from the repo URL.
func NewGithubService(ctx context.Context, wbc *WriteBackConfig, tokenProvider git.SCMTokenProvider) (*GithubService, error) {
	log := log.LoggerFromContext(ctx)

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
		u, _ := url.Parse(apiBaseURL)
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

	log.Infof("GitHub PR service initialised for %s/%s", owner, repoName)
	return &GithubService{
		client: client,
		owner:  owner,
		repo:   repoName,
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
