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

// PRBranchTemplate is the Go template used to produce a deterministic head
// branch name for pull/merge requests. It is shared between commitChangesGit
// (which creates the branch) and skipIfPRExists (which checks for it early).
const PRBranchTemplate = "image-updater-{{.TargetKey}}-{{.SHA256}}"

// PullRequestService is implemented by each SCM provider that supports
// opening pull/merge requests.
type PullRequestService interface {
	// create opens a new pull/merge request using the metadata stored in the
	// service at construction time (title, head, base, body).
	create(ctx context.Context) error
	// exists returns true if an open PR from pushBranch → checkOutBranch already
	// exists, preventing duplicate PR creation on repeated reconciliation cycles.
	exists(ctx context.Context, checkOutBranch, pushBranch string) (bool, error)
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
	// labels are applied to the pull/merge request on creation.
	labels []string
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
		title:  title,
		head:   pushBranch,
		base:   checkOutBranch,
		body:   body,
		labels: wbc.PRLabels,
	}, nil
}

// commitChangesPR validates the provider and SCM credentials before pushing the
// branch via commitChangesGit (which also populates wbc.PullRequest), then opens
// a pull/merge request from head → base. The provider and credential checks run
// first so configuration errors are caught before an orphaned branch is pushed.
//
// Before cloning the repository, commitChangesPR attempts to resolve the head
// and base branch names without a git client and queries the SCM provider for
// an existing open PR. When one is found the entire clone/marshal/push cycle
// is skipped, avoiding unnecessary work on every reconciliation while a PR is
// pending review.
func commitChangesPR(ctx context.Context, applicationImages *ApplicationImages, changeList []ChangeEntry, write changeWriter) error {
	logCtx := log.LoggerFromContext(ctx)
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

	// Try to detect an existing open PR before cloning and pushing to avoid
	// unnecessary git clone, override marshalling, commit, push, and API
	// calls on every reconciliation cycle while a PR is pending review.
	checkOutBranch := wbc.GitBranch
	if checkOutBranch == "" {
		checkOutBranch = getWriteBackBranch(ctx, &app, wbc)
	}
	if checkOutBranch != "" && checkOutBranch != "HEAD" {
		if skipped, skipErr := skipIfPRExists(ctx, wbc, tokenProvider, checkOutBranch, app.Namespace, app.Name, changeList); skipErr != nil {
			logCtx.Warnf("could not check for existing PR, proceeding with update: %v", skipErr)
		} else if skipped {
			return nil
		}
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

	case PRProviderGitLab:
		g, err := NewGitLabMRService(ctx, wbc, tokenProvider)
		if err != nil {
			return err
		}

		if err := g.create(ctx); err != nil {
			if errors.Is(err, ErrMRAlreadyExists) {
				return nil
			}
			return err
		}
		return nil

	default:
		return fmt.Errorf("unsupported PR provider: %d", wbc.PRProvider)
	}
}

// skipIfPRExists resolves the push branch name from the template and queries
// the SCM provider for an open PR from pushBranch → checkOutBranch. It returns
// (true, nil) when an open PR is found and the caller should skip the update.
// On any error it returns (false, err) so the caller can log a warning and
// fall through to the normal code path.
func skipIfPRExists(ctx context.Context, wbc *WriteBackConfig, tokenProvider git.SCMTokenProvider, checkOutBranch, appNamespace, appName string, changeList []ChangeEntry) (bool, error) {
	logCtx := log.LoggerFromContext(ctx)

	pushBranch := TemplateBranchName(ctx, PRBranchTemplate, appNamespace, appName, wbc.WriteBackTargetKey(), changeList)
	if pushBranch == "" {
		return false, fmt.Errorf("could not compute push branch name from template")
	}

	var svc PullRequestService
	var svcErr error
	switch wbc.PRProvider {
	case PRProviderGitHub:
		svc, svcErr = NewGithubPRService(ctx, wbc, tokenProvider)
	case PRProviderGitLab:
		svc, svcErr = NewGitLabMRService(ctx, wbc, tokenProvider)
	default:
		return false, fmt.Errorf("unsupported PR provider: %d", wbc.PRProvider)
	}
	if svcErr != nil {
		return false, svcErr
	}

	exists, err := svc.exists(ctx, checkOutBranch, pushBranch)
	if err != nil {
		return false, err
	}
	if exists {
		logCtx.Infof("open PR from %q to %q already exists, skipping update", pushBranch, checkOutBranch)
		return true, nil
	}
	return false, nil
}
