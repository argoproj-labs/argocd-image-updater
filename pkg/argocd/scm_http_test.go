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
	"time"

	"github.com/stretchr/testify/require"

	"github.com/argoproj-labs/argocd-image-updater/ext/git"
)

// trustServerViaCertStore registers the test server's certificate in a
// temporary Argo CD TLS certificate store keyed by the server's hostname, the
// way an operator registers the private CA of a self-hosted SCM instance.
func trustServerViaCertStore(t *testing.T, server *httptest.Server) {
	t.Helper()
	u, err := url.Parse(server.URL)
	require.NoError(t, err)
	dir := t.TempDir()
	pemData := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})
	require.NotNil(t, pemData)
	require.NoError(t, os.WriteFile(filepath.Join(dir, u.Hostname()), pemData, 0o600))
	t.Setenv("ARGOCD_TLS_DATA_PATH", dir)
}

func Test_newSCMAPIHTTPClient(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()
	repoURL := server.URL + "/owner/repo.git"

	t.Run("certificate not registered", func(t *testing.T) {
		t.Setenv("ARGOCD_TLS_DATA_PATH", t.TempDir())
		client := newSCMAPIHTTPClient(context.Background(), repoURL, nil, 5*time.Second, nil)
		// The request fails during the TLS handshake, so there is no body.
		_, err := client.Get(server.URL)
		require.ErrorContains(t, err, "certificate signed by unknown authority")
	})

	t.Run("certificate registered in the Argo CD cert store", func(t *testing.T) {
		trustServerViaCertStore(t, server)
		client := newSCMAPIHTTPClient(context.Background(), repoURL, nil, 5*time.Second, nil)
		resp, err := client.Get(server.URL)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("timeout and redirect policy come from the caller", func(t *testing.T) {
		checkRedirect := func(_ *http.Request, _ []*http.Request) error { return nil }
		client := newSCMAPIHTTPClient(context.Background(), repoURL, nil, 7*time.Second, checkRedirect)
		require.Equal(t, 7*time.Second, client.Timeout)
		require.NotNil(t, client.CheckRedirect)
	})
}

// Test_SCMAPIClients_UseCertStore runs each provider's API client through its
// real constructor against a TLS server, so a provider that stops using
// newSCMAPIHTTPClient fails here.
func Test_SCMAPIClients_UseCertStore(t *testing.T) {
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
			t.Run("certificate not registered", func(t *testing.T) {
				t.Setenv("ARGOCD_TLS_DATA_PATH", t.TempDir())
				require.ErrorContains(t, p.call(t, &mockTokenProvider{token: "token"}), "certificate signed by unknown authority")
			})
			t.Run("certificate registered in the Argo CD cert store", func(t *testing.T) {
				trustServerViaCertStore(t, server)
				require.NoError(t, p.call(t, &mockTokenProvider{token: "token"}))
			})
			if p.usesRepoCreds {
				t.Run("insecure credentials do not disable verification", func(t *testing.T) {
					t.Setenv("ARGOCD_TLS_DATA_PATH", t.TempDir())
					require.ErrorContains(t, p.call(t, legacyInsecureCreds), "certificate signed by unknown authority")
				})
			}
		})
	}
}
