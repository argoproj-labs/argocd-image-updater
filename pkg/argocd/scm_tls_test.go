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

// mockInsecureTokenProvider simulates credentials for a repository that is
// configured as insecure.
type mockInsecureTokenProvider struct {
	mockTokenProvider
}

func (m *mockInsecureTokenProvider) SCMInsecure() bool {
	return true
}

// mockInsecureTokenAndBaseURLProvider is mockInsecureTokenProvider with an
// explicit API base URL, like GitHub App credentials for GitHub Enterprise.
type mockInsecureTokenAndBaseURLProvider struct {
	mockTokenAndBaseURLProvider
}

func (m *mockInsecureTokenAndBaseURLProvider) SCMInsecure() bool {
	return true
}

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
// certificates in the Argo CD TLS certificate store and follows the
// repository's insecure setting, the same as the Git transport.
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

	tokenProvider := func(insecure bool) git.SCMTokenProvider {
		if insecure {
			return &mockInsecureTokenProvider{mockTokenProvider{token: "token"}}
		}
		return &mockTokenProvider{token: "token"}
	}

	providers := []struct {
		name string
		call func(t *testing.T, insecure bool) error
	}{
		{
			name: "GitLab",
			call: func(t *testing.T, insecure bool) error {
				svc, err := NewGitLabMRService(ctx, &WriteBackConfig{GitRepo: server.URL + "/group/project.git"}, tokenProvider(insecure))
				require.NoError(t, err)
				_, err = svc.exists(ctx, "main", "branch")
				return err
			},
		},
		{
			name: "GitHub",
			call: func(t *testing.T, insecure bool) error {
				svc, err := NewGithubPRService(ctx, &WriteBackConfig{GitRepo: server.URL + "/owner/repo.git"}, tokenProvider(insecure))
				require.NoError(t, err)
				_, err = svc.exists(ctx, "main", "branch")
				return err
			},
		},
		{
			name: "Azure DevOps",
			call: func(t *testing.T, insecure bool) error {
				svc, err := NewAzureDevOpsPRService(ctx, &WriteBackConfig{GitRepo: server.URL + "/collection/project/_git/repo"}, tokenProvider(insecure))
				require.NoError(t, err)
				_, err = svc.exists(ctx, "main", "branch")
				return err
			},
		},
		{
			name: "GitHub API commit",
			call: func(t *testing.T, insecure bool) error {
				dir := t.TempDir()
				require.NoError(t, os.WriteFile(filepath.Join(dir, "a.yaml"), []byte("a: 1\n"), 0o644))
				gitMock := newAPICommitGitMock(dir, "headsha", []git.WorkingTreeChange{{Path: "a.yaml"}})
				var provider git.SCMTokenProvider = &mockTokenAndBaseURLProvider{
					mockTokenProvider: mockTokenProvider{token: "token"},
					baseURL:           server.URL + "/api/v3",
				}
				if insecure {
					provider = &mockInsecureTokenAndBaseURLProvider{*provider.(*mockTokenAndBaseURLProvider)}
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
				assert.ErrorContains(t, p.call(t, false), "certificate signed by unknown authority")
			})
			t.Run("certificate in Argo CD TLS store is trusted", func(t *testing.T) {
				trustTLSServerCert(t, server)
				assert.NoError(t, p.call(t, false))
			})
			t.Run("insecure repository skips verification", func(t *testing.T) {
				t.Setenv(common.EnvVarTLSDataPath, t.TempDir())
				assert.NoError(t, p.call(t, true))
			})
		})
	}
}
