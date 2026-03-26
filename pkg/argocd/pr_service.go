package argocd

import (
	"context"
	"fmt"

	"github.com/argoproj-labs/argocd-image-updater/ext/git"
)

type PRProvider int

const (
	PRProviderUnsupported PRProvider = iota
	PRProviderGitHub
	PRProviderGitLab
)

// PullRequestService is implemented by each SCM provider that supports
// opening pull/merge requests.
type PullRequestService interface {
	create(ctx context.Context) error
	list(ctx context.Context, checkOutBranch, pushBranch string) error
}

func commitChangesPR(ctx context.Context, applicationImages *ApplicationImages, changeList []ChangeEntry, write changeWriter) error {
	app := applicationImages.Application
	wbc := applicationImages.WriteBackConfig

	creds, err := wbc.GetCreds(&app)
	if err != nil {
		return fmt.Errorf("could not get creds for repo '%s': %v", wbc.GitRepo, err)
	}

	tokenProvider, ok := creds.(git.SCMTokenProvider)
	if !ok {
		return fmt.Errorf("credentials type %T do not support PR creation (use HTTPS or GitHub App credentials)", creds)
	}

	switch wbc.PRProvider {
	case PRProviderGitHub:
		_, err := NewGithubService(ctx, wbc, tokenProvider)
		if err != nil {
			return err
		}
		return fmt.Errorf("PR-based git write-back is not implemented yet")

	// TODO: placeholder for gitlab. Will be implemented in GITOPS-9155
	//case PRProviderGitLab:
	//	return createGitLabMR(ctx, wbc, tokenProvider)
	default:
		return fmt.Errorf("unsupported PR provider: %d", wbc.PRProvider)
	}
}
