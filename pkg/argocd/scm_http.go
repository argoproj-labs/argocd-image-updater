package argocd

import (
	"context"
	"net/http"
	"time"

	"github.com/argoproj-labs/argocd-image-updater/ext/git"
)

// newSCMAPIHTTPClient builds the HTTP client a PullRequestService uses for its
// REST API calls, so that those calls share the TLS configuration already
// applied to Git operations against the same repository: custom root CAs taken
// from the Argo CD TLS certificate store, and a TLS client certificate when the
// credentials carry one.
//
// Without this, the API client falls back to http.DefaultTransport and only the
// system trust store. Self-hosted SCM instances are commonly served with a
// private CA, which would make write-back fail halfway: commitChangesGit pushes
// the head branch successfully, then the PR call fails with an
// unknown-authority error, leaving an orphaned branch behind on every cycle.
//
// creds may be nil, and may be any git.Creds implementation; the TLS client
// certificate is only picked up from credentials implementing
// git.GenericHTTPSCreds.
//
// The insecure and proxy parameters of git.GetRepoHTTPClient are passed as
// false and "" because neither is reachable from a git.Creds value (both fields
// are unexported and have no accessor). This matches what getGitClient already
// does for the Git side, so the two paths stay consistent.
//
// timeout and checkRedirect are supplied by the caller so each provider keeps
// its own request budget and redirect policy.
func newSCMAPIHTTPClient(ctx context.Context, repoURL string, creds git.Creds, timeout time.Duration, checkRedirect func(req *http.Request, via []*http.Request) error) *http.Client {
	client := git.GetRepoHTTPClient(ctx, repoURL, false, creds, "")
	client.Timeout = timeout
	client.CheckRedirect = checkRedirect
	return client
}
