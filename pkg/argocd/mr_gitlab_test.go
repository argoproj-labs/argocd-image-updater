package argocd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gitlab "gitlab.com/gitlab-org/api/client-go"

	"github.com/argoproj-labs/argocd-image-updater/ext/git"
)

func Test_parseGitLabProject(t *testing.T) {
	tests := []struct {
		name        string
		repoURL     string
		wantProject string
		wantErrMsg  string
	}{
		// --- HTTPS ---
		{
			name:        "gitlab.com HTTPS with .git suffix",
			repoURL:     "https://gitlab.com/group/repo.git",
			wantProject: "group/repo",
		},
		{
			name:        "gitlab.com HTTPS without .git suffix",
			repoURL:     "https://gitlab.com/group/repo",
			wantProject: "group/repo",
		},
		{
			name:        "self-managed HTTPS with .git suffix",
			repoURL:     "https://gitlab.example.com/group/repo.git",
			wantProject: "group/repo",
		},
		{
			name:        "self-managed HTTPS without .git suffix",
			repoURL:     "https://gitlab.example.com/group/repo",
			wantProject: "group/repo",
		},
		{
			name:        "self-managed enterprise GitLab HTTPS",
			repoURL:     "https://gitlab.internal.example.com/platform/deploy-config.git",
			wantProject: "platform/deploy-config",
		},
		{
			name:        "nested groups (subgroup) HTTPS",
			repoURL:     "https://gitlab.com/group/subgroup/repo.git",
			wantProject: "group/subgroup/repo",
		},
		{
			name:        "deeply nested groups HTTPS",
			repoURL:     "https://gitlab.com/a/b/c/repo.git",
			wantProject: "a/b/c/repo",
		},

		// --- SCP-style SSH (git@host:group/repo.git) ---
		{
			name:        "gitlab.com SCP-style SSH with .git suffix",
			repoURL:     "git@gitlab.com:group/repo.git",
			wantProject: "group/repo",
		},
		{
			name:        "gitlab.com SCP-style SSH without .git suffix",
			repoURL:     "git@gitlab.com:group/repo",
			wantProject: "group/repo",
		},
		{
			name:        "self-managed SCP-style SSH",
			repoURL:     "git@gitlab.example.com:group/repo.git",
			wantProject: "group/repo",
		},
		{
			name:        "self-managed enterprise SCP-style SSH",
			repoURL:     "git@gitlab.internal.example.com:platform/deploy-config.git",
			wantProject: "platform/deploy-config",
		},
		{
			name:        "nested groups SCP-style SSH",
			repoURL:     "git@gitlab.com:group/subgroup/repo.git",
			wantProject: "group/subgroup/repo",
		},

		// --- ssh:// scheme ---
		{
			name:        "gitlab.com ssh:// URL with .git suffix",
			repoURL:     "ssh://git@gitlab.com/group/repo.git",
			wantProject: "group/repo",
		},
		{
			name:        "self-managed ssh:// URL",
			repoURL:     "ssh://git@gitlab.example.com/group/repo.git",
			wantProject: "group/repo",
		},
		{
			name:        "nested groups ssh:// URL",
			repoURL:     "ssh://git@gitlab.com/group/subgroup/repo.git",
			wantProject: "group/subgroup/repo",
		},

		// --- HTTP (self-managed without TLS) ---
		{
			name:        "self-managed HTTP with .git suffix",
			repoURL:     "http://gitlab.example.com/group/repo.git",
			wantProject: "group/repo",
		},
		{
			name:        "HTTP with IP address and group/repo",
			repoURL:     "http://127.0.0.1:30003/group/repo.git",
			wantProject: "group/repo",
		},

		// --- error cases ---
		{
			name:       "single path segment (no namespace)",
			repoURL:    "https://127.0.0.1:30003/testdata.git",
			wantErrMsg: "does not contain a namespace/project path",
		},
		{
			name:       "HTTPS URL with empty path",
			repoURL:    "https://gitlab.com/",
			wantErrMsg: "does not contain a namespace/project path",
		},
		{
			name:       "HTTPS URL with no path at all",
			repoURL:    "https://gitlab.com",
			wantErrMsg: "does not contain a namespace/project path",
		},
		{
			name:       "empty string",
			repoURL:    "",
			wantErrMsg: "does not contain a namespace/project path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			project, err := parseGitLabProject(tt.repoURL)

			if tt.wantErrMsg != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrMsg)
				assert.Empty(t, project)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantProject, project)
			}
		})
	}
}

// newTestGitLabMRService creates a GitLabMRService whose client is pointed at
// the provided httptest.Server instead of the real GitLab API.
func newTestGitLabMRService(server *httptest.Server, pr *PullRequest) *GitLabMRService {
	client, err := gitlab.NewClient("test-token", gitlab.WithBaseURL(server.URL))
	if err != nil {
		panic(fmt.Sprintf("newTestGitLabMRService: NewClient failed: %v", err))
	}
	return &GitLabMRService{
		client:    client,
		projectID: "group/repo",
		pr:        pr,
	}
}

func Test_GitLabMRService_create(t *testing.T) {
	ctx := context.Background()

	pr := &PullRequest{
		title: "chore: update images",
		head:  "image-updater-branch",
		base:  "main",
		body:  "automated update",
	}

	tests := []struct {
		name       string
		handler    http.HandlerFunc
		wantErr    error  // exact sentinel to check with errors.Is
		wantErrMsg string // substring match for non-sentinel errors
	}{
		{
			name: "success — GitLab returns 201 with MR IID and URL",
			handler: func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodPost, r.Method)
				assert.Contains(t, r.URL.Path, "/merge_requests")

				w.WriteHeader(http.StatusCreated)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"iid":     42,
					"web_url": "https://gitlab.com/group/repo/-/merge_requests/42",
				})
			},
		},
		{
			name: "MR already exists — GitLab returns 409 Conflict — no error",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusConflict)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"message": []string{"Another open merge request already exists for this source branch: !1"},
				})
			},
			wantErr: ErrMRAlreadyExists,
		},
		{
			name: "API error — GitLab returns 422 Unprocessable Entity",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusUnprocessableEntity)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"message": "some validation error",
				})
			},
			wantErrMsg: "could not create MR",
		},
		{
			name: "API error — GitLab returns 500 internal server error",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			},
			wantErrMsg: "could not create MR",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(tt.handler)
			defer server.Close()

			svc := newTestGitLabMRService(server, pr)
			err := svc.create(ctx)

			switch {
			case tt.wantErr != nil:
				require.ErrorIs(t, err, tt.wantErr)
			case tt.wantErrMsg != "":
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrMsg)
			default:
				require.NoError(t, err)
			}
		})
	}
}

func Test_NewGitLabMRService(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name          string
		gitRepo       string
		tokenProvider git.SCMTokenProvider
		wantProject   string
		wantBaseURL   string
		wantErrMsg    string
	}{
		// --- error cases ---
		{
			name:          "token provider returns error",
			gitRepo:       "https://gitlab.com/group/repo.git",
			tokenProvider: &mockTokenProvider{err: fmt.Errorf("secret not found")},
			wantErrMsg:    "could not obtain SCM token",
		},
		{
			name:          "token is empty",
			gitRepo:       "https://gitlab.com/group/repo.git",
			tokenProvider: &mockTokenProvider{token: ""},
			wantErrMsg:    "empty SCM token",
		},
		{
			name:          "repo URL has no namespace/project path",
			gitRepo:       "https://127.0.0.1:30003/testdata.git",
			tokenProvider: &mockTokenProvider{token: "glpat-token"},
			wantErrMsg:    "could not parse project path",
		},

		// --- HTTPS PAT (mockTokenProvider has no SCMAPIBaseURLProvider) ---
		{
			name:          "HTTPS PAT against gitlab.com",
			gitRepo:       "https://gitlab.com/group/repo.git",
			tokenProvider: &mockTokenProvider{token: "glpat-token"},
			wantProject:   "group/repo",
			wantBaseURL:   "https://gitlab.com/api/v4",
		},
		{
			name:          "HTTPS PAT against self-managed GitLab",
			gitRepo:       "https://gitlab.example.com/group/repo.git",
			tokenProvider: &mockTokenProvider{token: "glpat-token"},
			wantProject:   "group/repo",
			wantBaseURL:   "https://gitlab.example.com/api/v4",
		},
		{
			name:          "HTTPS PAT against self-managed enterprise GitLab",
			gitRepo:       "https://gitlab.internal.example.com/platform/deploy-config.git",
			tokenProvider: &mockTokenProvider{token: "glpat-token"},
			wantProject:   "platform/deploy-config",
			wantBaseURL:   "https://gitlab.internal.example.com/api/v4",
		},
		{
			name:          "HTTPS PAT against self-managed GitLab HTTP",
			gitRepo:       "http://gitlab.example.com/group/repo.git",
			tokenProvider: &mockTokenProvider{token: "glpat-token"},
			wantProject:   "group/repo",
			wantBaseURL:   "http://gitlab.example.com/api/v4",
		},
		{
			name:          "HTTPS PAT with nested groups",
			gitRepo:       "https://gitlab.com/group/subgroup/repo.git",
			tokenProvider: &mockTokenProvider{token: "glpat-token"},
			wantProject:   "group/subgroup/repo",
			wantBaseURL:   "https://gitlab.com/api/v4",
		},

		// --- SCP-style SSH without SCMAPIBaseURLProvider ---
		{
			// SCP-style SSH URLs can't be parsed by url.Parse to derive a host.
			// Without SCMAPIBaseURLProvider the code falls back to the gitlab.com
			// default, which is correct for gitlab.com repos and a safe fallback
			// for self-managed (SSH creds can't reach this path at runtime anyway).
			name:          "SCP-style SSH without base URL provider — falls back to gitlab.com default",
			gitRepo:       "git@gitlab.example.com:group/repo.git",
			tokenProvider: &mockTokenProvider{token: "glpat-token"},
			wantProject:   "group/repo",
			wantBaseURL:   "https://gitlab.com/api/v4",
		},

		// --- SCMAPIBaseURLProvider (e.g. app-based credentials with explicit base URL) ---
		{
			name:    "base URL provider against gitlab.com — empty base URL",
			gitRepo: "https://gitlab.com/group/repo.git",
			tokenProvider: &mockTokenAndBaseURLProvider{
				mockTokenProvider: mockTokenProvider{token: "glpat-token"},
				baseURL:           "",
			},
			wantProject: "group/repo",
			wantBaseURL: "https://gitlab.com/api/v4",
		},
		{
			name:    "base URL provider against self-managed GitLab",
			gitRepo: "https://gitlab.example.com/group/repo.git",
			tokenProvider: &mockTokenAndBaseURLProvider{
				mockTokenProvider: mockTokenProvider{token: "glpat-token"},
				baseURL:           "https://gitlab.example.com",
			},
			wantProject: "group/repo",
			wantBaseURL: "https://gitlab.example.com/api/v4",
		},
		{
			name:    "base URL provider with SSH repo URL — base URL comes from credentials",
			gitRepo: "git@gitlab.example.com:group/repo.git",
			tokenProvider: &mockTokenAndBaseURLProvider{
				mockTokenProvider: mockTokenProvider{token: "glpat-token"},
				baseURL:           "https://gitlab.example.com",
			},
			wantProject: "group/repo",
			wantBaseURL: "https://gitlab.example.com/api/v4",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wbc := &WriteBackConfig{GitRepo: tt.gitRepo}

			svc, err := NewGitLabMRService(ctx, wbc, tt.tokenProvider)

			if tt.wantErrMsg != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrMsg)
				assert.Nil(t, svc)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, svc)
			assert.Equal(t, tt.wantProject, svc.projectID)
			require.NotNil(t, svc.client)
			assert.Contains(t, svc.client.BaseURL().String(), tt.wantBaseURL)
		})
	}
}
