package argocd

import (
	"context"
	"net/http"
	"time"

	"github.com/argoproj-labs/argocd-image-updater/ext/git"
)

// Request budgets for the SCM API clients that have no other deadline.
const (
	githubAPITimeout      = 30 * time.Second
	gitlabAPITimeout      = 30 * time.Second
	azureDevOpsAPITimeout = 30 * time.Second
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
// The proxy and no-proxy settings of the repository are taken from the
// credentials too, so API calls leave through the same proxy as Git operations.
//
// The insecure parameter of git.GetRepoHTTPClient is always false: API calls
// carry the access token, and write-back secrets default insecure to true for
// backward compatibility on the Git side only (see parseLegacyInsecure).
//
// timeout and checkRedirect are supplied by the caller so each provider keeps
// its own request budget and redirect policy.
func newSCMAPIHTTPClient(ctx context.Context, repoURL string, creds git.Creds, timeout time.Duration, checkRedirect func(req *http.Request, via []*http.Request) error) *http.Client {
	proxy, noProxy := repoProxyOptions(creds)
	client := git.GetRepoHTTPClient(ctx, repoURL, false, creds, proxy, noProxy)
	client.Timeout = timeout
	client.CheckRedirect = checkRedirect
	return client
}
