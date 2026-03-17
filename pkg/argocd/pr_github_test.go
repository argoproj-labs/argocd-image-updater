package argocd

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/argoproj-labs/argocd-image-updater/ext/git"
)

// mockTokenProvider implements git.SCMTokenProvider for testing.
type mockTokenProvider struct {
	token string
	err   error
}

func (m *mockTokenProvider) SCMToken(_ context.Context) (string, error) {
	return m.token, m.err
}

// mockTokenAndBaseURLProvider additionally implements git.SCMAPIBaseURLProvider,
// simulating a GitHubAppCreds with an explicit enterprise base URL.
type mockTokenAndBaseURLProvider struct {
	mockTokenProvider
	baseURL string
}

func (m *mockTokenAndBaseURLProvider) SCMAPIBaseURL() string {
	return m.baseURL
}

func Test_parseGitHubOwnerRepo(t *testing.T) {
	tests := []struct {
		name       string
		repoURL    string
		wantOwner  string
		wantRepo   string
		wantErrMsg string
	}{
		// --- HTTPS ---
		{
			name:      "github.com HTTPS with .git suffix",
			repoURL:   "https://github.com/org/repo.git",
			wantOwner: "org",
			wantRepo:  "repo",
		},
		{
			name:      "github.com HTTPS without .git suffix",
			repoURL:   "https://github.com/org/repo",
			wantOwner: "org",
			wantRepo:  "repo",
		},
		{
			name:      "GitHub Enterprise HTTPS with .git suffix",
			repoURL:   "https://github.example.com/org/repo.git",
			wantOwner: "org",
			wantRepo:  "repo",
		},
		{
			name:      "GitHub Enterprise HTTPS without .git suffix",
			repoURL:   "https://github.example.com/org/repo",
			wantOwner: "org",
			wantRepo:  "repo",
		},
		{
			name:      "HTTPS with extra path segments beyond owner/repo",
			repoURL:   "https://github.com/org/repo/extra/path.git",
			wantOwner: "org",
			wantRepo:  "repo",
		},

		// --- SCP-style SSH (git@host:owner/repo.git) ---
		{
			name:      "github.com SCP-style SSH with .git suffix",
			repoURL:   "git@github.com:org/repo.git",
			wantOwner: "org",
			wantRepo:  "repo",
		},
		{
			name:      "github.com SCP-style SSH without .git suffix",
			repoURL:   "git@github.com:org/repo",
			wantOwner: "org",
			wantRepo:  "repo",
		},
		{
			name:      "GitHub Enterprise SCP-style SSH",
			repoURL:   "git@github.example.com:org/repo.git",
			wantOwner: "org",
			wantRepo:  "repo",
		},

		// --- ssh:// scheme ---
		{
			name:      "github.com ssh:// URL with .git suffix",
			repoURL:   "ssh://git@github.com/org/repo.git",
			wantOwner: "org",
			wantRepo:  "repo",
		},
		{
			name:      "GitHub Enterprise ssh:// URL",
			repoURL:   "ssh://git@github.example.com/org/repo.git",
			wantOwner: "org",
			wantRepo:  "repo",
		},

		// --- HTTP (GitHub Enterprise can be configured without TLS) ---
		{
			name:      "GitHub Enterprise HTTP with .git suffix",
			repoURL:   "http://github.example.com/org/repo.git",
			wantOwner: "org",
			wantRepo:  "repo",
		},
		{
			name:      "GitHub Enterprise HTTP without .git suffix",
			repoURL:   "http://github.example.com/org/repo",
			wantOwner: "org",
			wantRepo:  "repo",
		},
		{
			name:      "HTTP with IP address and owner/repo",
			repoURL:   "http://127.0.0.1:30003/org/repo.git",
			wantOwner: "org",
			wantRepo:  "repo",
		},

		// --- local / test registries (no owner) ---
		{
			name:       "local HTTPS registry with only one path segment",
			repoURL:    "https://127.0.0.1:30003/testdata.git",
			wantErrMsg: "does not contain an owner/repo path",
		},
		{
			name:       "local HTTP registry with only one path segment",
			repoURL:    "http://127.0.0.1:30003/testdata.git",
			wantErrMsg: "does not contain an owner/repo path",
		},
		{
			name:       "HTTPS URL with empty path",
			repoURL:    "https://github.com/",
			wantErrMsg: "does not contain an owner/repo path",
		},
		{
			name:       "HTTPS URL with no path at all",
			repoURL:    "https://github.com",
			wantErrMsg: "does not contain an owner/repo path",
		},
		{
			name:       "empty string",
			repoURL:    "",
			wantErrMsg: "does not contain an owner/repo path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			owner, repo, err := parseGitHubOwnerRepo(tt.repoURL)

			if tt.wantErrMsg != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrMsg)
				assert.Empty(t, owner)
				assert.Empty(t, repo)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantOwner, owner)
				assert.Equal(t, tt.wantRepo, repo)
			}
		})
	}
}

func Test_NewGithubService(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name          string
		gitRepo       string
		tokenProvider git.SCMTokenProvider
		wantOwner     string
		wantRepo      string
		wantBaseURL   string
		wantUploadURL string
		wantErrMsg    string
	}{
		// --- error cases ---
		{
			name:          "token provider returns error",
			gitRepo:       "https://github.com/org/repo.git",
			tokenProvider: &mockTokenProvider{err: fmt.Errorf("secret not found")},
			wantErrMsg:    "could not obtain SCM token",
		},
		{
			name:          "token is empty",
			gitRepo:       "https://github.com/org/repo.git",
			tokenProvider: &mockTokenProvider{token: ""},
			wantErrMsg:    "empty SCM token",
		},
		{
			name:          "repo URL has no owner/repo path",
			gitRepo:       "https://127.0.0.1:30003/testdata.git",
			tokenProvider: &mockTokenProvider{token: "ghp_token"},
			wantErrMsg:    "could not parse owner/repo",
		},

		// --- HTTPS PAT (mockTokenProvider has no SCMAPIBaseURLProvider) ---
		{
			name:          "HTTPS PAT against github.com",
			gitRepo:       "https://github.com/org/repo.git",
			tokenProvider: &mockTokenProvider{token: "ghp_token"},
			wantOwner:     "org",
			wantRepo:      "repo",
			wantBaseURL:   "https://api.github.com/",
			wantUploadURL: "https://uploads.github.com/",
		},
		{
			// The key assertion: uploadURL must be /api/uploads/, NOT /api/v3/api/uploads/.
			// apiBaseURL is derived as https://github.example.com/api/v3 from the repo URL,
			// then uploadURL is stripped to https://github.example.com so that
			// WithEnterpriseURLs can append /api/uploads/ cleanly.
			name:          "HTTPS PAT against GitHub Enterprise — uploadURL is /api/uploads/ not /api/v3/api/uploads/",
			gitRepo:       "https://github.example.com/org/repo.git",
			tokenProvider: &mockTokenProvider{token: "ghp_token"},
			wantOwner:     "org",
			wantRepo:      "repo",
			wantBaseURL:   "https://github.example.com/api/v3/",
			wantUploadURL: "https://github.example.com/api/uploads/",
		},
		{
			name:          "HTTPS PAT against GitHub Enterprise HTTP",
			gitRepo:       "http://github.example.com/org/repo.git",
			tokenProvider: &mockTokenProvider{token: "ghp_token"},
			wantOwner:     "org",
			wantRepo:      "repo",
			wantBaseURL:   "http://github.example.com/api/v3/",
			wantUploadURL: "http://github.example.com/api/uploads/",
		},

		// --- GitHub App (mockTokenAndBaseURLProvider also implements SCMAPIBaseURLProvider) ---
		{
			name:    "GitHub App against github.com — empty base URL",
			gitRepo: "https://github.com/org/repo.git",
			tokenProvider: &mockTokenAndBaseURLProvider{
				mockTokenProvider: mockTokenProvider{token: "ghs_installation_token"},
				baseURL:           "",
			},
			wantOwner:     "org",
			wantRepo:      "repo",
			wantBaseURL:   "https://api.github.com/",
			wantUploadURL: "https://uploads.github.com/",
		},
		{
			// The key assertion: credentials carry baseURL with /api/v3 already in the path.
			// uploadURL must still be /api/uploads/, not /api/v3/api/uploads/.
			name:    "GitHub App against GitHub Enterprise — uploadURL is /api/uploads/ not /api/v3/api/uploads/",
			gitRepo: "https://github.example.com/org/repo.git",
			tokenProvider: &mockTokenAndBaseURLProvider{
				mockTokenProvider: mockTokenProvider{token: "ghs_installation_token"},
				baseURL:           "https://github.example.com/api/v3",
			},
			wantOwner:     "org",
			wantRepo:      "repo",
			wantBaseURL:   "https://github.example.com/api/v3/",
			wantUploadURL: "https://github.example.com/api/uploads/",
		},
		{
			// GitHub App can be used with SSH repo URLs; the API base URL still
			// comes from the credentials, not from wbc.GitRepo.
			name:    "GitHub App with SSH repo URL — base URL comes from credentials, not repo URL",
			gitRepo: "git@github.example.com:org/repo.git",
			tokenProvider: &mockTokenAndBaseURLProvider{
				mockTokenProvider: mockTokenProvider{token: "ghs_installation_token"},
				baseURL:           "https://github.example.com/api/v3",
			},
			wantOwner:     "org",
			wantRepo:      "repo",
			wantBaseURL:   "https://github.example.com/api/v3/",
			wantUploadURL: "https://github.example.com/api/uploads/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wbc := &WriteBackConfig{GitRepo: tt.gitRepo}

			svc, err := NewGithubService(ctx, wbc, tt.tokenProvider)

			if tt.wantErrMsg != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrMsg)
				assert.Nil(t, svc)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, svc)
			assert.Equal(t, tt.wantOwner, svc.owner)
			assert.Equal(t, tt.wantRepo, svc.repo)
			require.NotNil(t, svc.client)
			assert.Equal(t, tt.wantBaseURL, svc.client.BaseURL.String())
			assert.Equal(t, tt.wantUploadURL, svc.client.UploadURL.String())
		})
	}
}
