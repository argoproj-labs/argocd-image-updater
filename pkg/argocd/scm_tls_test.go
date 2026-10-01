package argocd

import (
	"context"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/argoproj/argo-cd/v3/common"

	"github.com/argoproj-labs/argocd-image-updater/ext/git"
)

// trustTLSServerCert registers the certificate of server in a temporary Argo CD
// TLS certificate store, the same way `argocd cert add-tls` does.
func trustTLSServerCert(t *testing.T, server *httptest.Server) {
	t.Helper()
	u, err := url.Parse(server.URL)
	require.NoError(t, err)
	dir := t.TempDir()
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})
	require.NoError(t, os.WriteFile(filepath.Join(dir, u.Hostname()), certPEM, 0o600))
	t.Setenv(common.EnvVarTLSDataPath, dir)
}

// Test_SCMAPIClients_CustomCA checks that every SCM API client trusts the
// certificates in the Argo CD TLS certificate store, like the Git transport.
func Test_SCMAPIClients_CustomCA(t *testing.T) {
	ctx := context.Background()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "/_apis/git/repositories/"):
			fmt.Fprint(w, `{"value":[]}`)
		case r.URL.Path == "/api/v3/repos/owner/repo/git/refs":
			w.WriteHeader(http.StatusCreated)
			fmt.Fprint(w, `{"ref":"refs/heads/branch","object":{"sha":"headsha"}}`)
		case r.URL.Path == "/api/graphql":
			fmt.Fprint(w, `{"data":{"createCommitOnBranch":{"commit":{"oid":"newsha"}}}}`)
		default:
			fmt.Fprint(w, `[]`)
		}
	}))
	defer server.Close()

	// Write-back secrets without an "insecure" key get insecure=true for
	// backwards compatibility (see parseLegacyInsecure). That must not turn off
	// TLS verification for API calls, which carry the token.
	legacyInsecureCreds := git.NewHTTPSCreds("user", "token", "", "", true, "", &git.NoopCredsStore{}, false).(git.SCMTokenProvider)

	providers := []struct {
		name string
		call func(t *testing.T, tokenProvider git.SCMTokenProvider) error
		// usesRepoCreds is false for providers that take the API base URL from the credentials.
		usesRepoCreds bool
	}{
		{
			name:          "GitLab",
			usesRepoCreds: true,
			call: func(t *testing.T, tokenProvider git.SCMTokenProvider) error {
				svc, err := NewGitLabMRService(ctx, &WriteBackConfig{GitRepo: server.URL + "/group/project.git"}, tokenProvider)
				require.NoError(t, err)
				_, err = svc.exists(ctx, "main", "branch")
				return err
			},
		},
		{
			name:          "GitHub",
			usesRepoCreds: true,
			call: func(t *testing.T, tokenProvider git.SCMTokenProvider) error {
				svc, err := NewGithubPRService(ctx, &WriteBackConfig{GitRepo: server.URL + "/owner/repo.git"}, tokenProvider)
				require.NoError(t, err)
				_, err = svc.exists(ctx, "main", "branch")
				return err
			},
		},
		{
			name:          "Azure DevOps",
			usesRepoCreds: true,
			call: func(t *testing.T, tokenProvider git.SCMTokenProvider) error {
				svc, err := NewAzureDevOpsPRService(ctx, &WriteBackConfig{GitRepo: server.URL + "/collection/project/_git/repo"}, tokenProvider)
				require.NoError(t, err)
				_, err = svc.exists(ctx, "main", "branch")
				return err
			},
		},
		{
			name: "GitHub API commit",
			call: func(t *testing.T, _ git.SCMTokenProvider) error {
				dir := t.TempDir()
				require.NoError(t, os.WriteFile(filepath.Join(dir, "a.yaml"), []byte("a: 1\n"), 0o644))
				gitMock := newAPICommitGitMock(dir, "headsha", []git.WorkingTreeChange{{Path: "a.yaml"}})
				provider := &mockTokenAndBaseURLProvider{
					mockTokenProvider: mockTokenProvider{token: "token"},
					baseURL:           server.URL + "/api/v3",
				}
				wbc := &WriteBackConfig{GitRepo: "https://github.com/owner/repo.git", GitCommitMessage: "msg"}
				// branchCreated=true covers the REST ref creation and the GraphQL commit.
				return commitChangesGithubAPI(ctx, wbc, gitMock, provider, "branch", true)
			},
		},
	}

	for _, p := range providers {
		t.Run(p.name, func(t *testing.T) {
			t.Run("untrusted certificate is rejected", func(t *testing.T) {
				t.Setenv(common.EnvVarTLSDataPath, t.TempDir())
				assert.ErrorContains(t, p.call(t, &mockTokenProvider{token: "token"}), "certificate signed by unknown authority")
			})
			t.Run("certificate in Argo CD TLS store is trusted", func(t *testing.T) {
				trustTLSServerCert(t, server)
				assert.NoError(t, p.call(t, &mockTokenProvider{token: "token"}))
			})
			if p.usesRepoCreds {
				t.Run("insecure credentials do not disable verification", func(t *testing.T) {
					t.Setenv(common.EnvVarTLSDataPath, t.TempDir())
					assert.ErrorContains(t, p.call(t, legacyInsecureCreds), "certificate signed by unknown authority")
				})
			}
		})
	}
}
