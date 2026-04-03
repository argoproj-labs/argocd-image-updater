package argocd

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/argoproj-labs/argocd-image-updater/ext/git"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/log"
)

// PRProvider identifies which SCM provider is used to open pull/merge requests.
type PRProvider int

const (
	// PRProviderUnsupported is the zero value; no PR provider has been configured.
	PRProviderUnsupported PRProvider = iota
	// PRProviderGitHub opens pull requests via the GitHub REST API.
	PRProviderGitHub
	// PRProviderGitLab opens merge requests via the GitLab REST API.
	PRProviderGitLab
)

// PullRequestService is implemented by each SCM provider that supports
// opening pull/merge requests.
type PullRequestService interface {
	// create opens a new pull/merge request using the metadata stored in the
	// service at construction time (title, head, base, body).
	create(ctx context.Context) error
	// list returns true if an open PR from pushBranch → checkOutBranch already
	// exists, preventing duplicate PR creation on repeated reconciliation cycles.
	list(ctx context.Context, checkOutBranch, pushBranch string) (bool, error)
}

// PullRequest holds the metadata required to open a pull/merge request.
type PullRequest struct {
	// title is the single-line summary shown in the SCM UI.
	title string
	// body is the optional multi-line description rendered in the PR description.
	body string
	// head is the branch carrying the image update commits (PR source).
	head string
	// base is the branch the PR will be merged into (PR target, e.g. "main").
	base string
}

// buildPullRequest derives the PR title, body, head and base from the
// write-back config, the application identity, and the resolved branch names.
//
// Title / body derivation rules:
//   - If GitCommitMessage is set, its first line becomes the title and
//     everything after the first newline becomes the body.
//   - A single-line GitCommitMessage produces an empty body.
//   - An empty GitCommitMessage generates a default title and body that
//     include the application namespace and name for reviewer context.
func buildPullRequest(ctx context.Context, wbc *WriteBackConfig, appNamespace, appName, checkOutBranch, pushBranch string) (*PullRequest, error) {
	logCtx := log.LoggerFromContext(ctx)

	title := fmt.Sprintf("chore: update images for %s/%s", appNamespace, appName)
	body := fmt.Sprintf("This pull request was created automatically by argocd-image-updater for application %s/%s.", appNamespace, appName)

	if wbc.GitCommitMessage != "" {
		parts := strings.SplitN(wbc.GitCommitMessage, "\n", 2)
		if trimmed := strings.TrimSpace(parts[0]); trimmed != "" {
			title = trimmed
		}
		if len(parts) == 2 {
			body = strings.TrimSpace(parts[1])
		} else {
			body = ""
		}
	}

	if utf8.RuneCountInString(title) > 255 {
		title = string([]rune(title)[:255])
		logCtx.Warnf("PR title exceeded 255 characters and was truncated: %s", title)
	}
	if utf8.RuneCountInString(body) > 65536 {
		body = string([]rune(body)[:65536])
		logCtx.Warnf("PR body exceeded 65536 characters and was truncated")
	}

	return &PullRequest{
		title: title,
		head:  pushBranch,
		base:  checkOutBranch,
		body:  body,
	}, nil
}

// commitChangesPR validates the provider and SCM credentials before pushing the
// branch via commitChangesGit (which also populates wbc.PullRequest), then opens
// a pull/merge request from head → base. The provider and credential checks run
// first so configuration errors are caught before an orphaned branch is pushed.
func commitChangesPR(ctx context.Context, applicationImages *ApplicationImages, changeList []ChangeEntry, write changeWriter) error {
	app := applicationImages.Application
	wbc := applicationImages.WriteBackConfig

	// GetCreds is called again here (also called inside commitChangesGit).
	// This is safe: GitHubAppCreds tokens are cached by ghinstallation;
	// HTTPSCreds return a plain string. No redundant network calls occur.
	creds, err := wbc.GetCreds(&app)
	if err != nil {
		return fmt.Errorf("could not get creds for repo '%s': %v", wbc.GitRepo, err)
	}

	tokenProvider, ok := creds.(git.SCMTokenProvider)
	if !ok {
		return fmt.Errorf("credentials type %T do not support PR creation (use HTTPS or GitHub App credentials)", creds)
	}

	// Push the image update commit to the head branch first.
	err = commitChangesGit(ctx, applicationImages, changeList, write)
	if err != nil {
		return err
	}

	if wbc.PullRequest == nil {
		return fmt.Errorf("pull request structure is not initialized")
	}

	switch wbc.PRProvider {
	case PRProviderGitHub:
		g, err := NewGithubPRService(ctx, wbc, tokenProvider)
		if err != nil {
			return err
		}

		if err := g.create(ctx); err != nil {
			if errors.Is(err, ErrPRAlreadyExists) {
				return nil
			}
			return err
		}
		return nil

	// TODO: placeholder for gitlab. Will be implemented in GITOPS-9155
	//case PRProviderGitLab:
	//	return createGitLabService(ctx, wbc, tokenProvider)
	default:
		return fmt.Errorf("unsupported PR provider: %d", wbc.PRProvider)
	}
}
