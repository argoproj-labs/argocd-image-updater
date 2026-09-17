package argocd

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	argocdapi "github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/argoproj-labs/argocd-image-updater/ext/git"
)

func Test_NewAzureDevOpsPRService_URL(t *testing.T) {
	for _, tt := range []struct {
		name, repo, want string
	}{
		{"Azure DevOps Services", "https://dev.azure.com/org/project/_git/repo", "https://dev.azure.com/org/project/_apis/git/repositories/repo/pullrequests"},
		{"legacy Services URL", "https://org.visualstudio.com/DefaultCollection/project/_git/repo", "https://org.visualstudio.com/DefaultCollection/project/_apis/git/repositories/repo/pullrequests"},
		{"Azure DevOps Server", "http://server:8080/tfs/collection/project/_git/repo.git", "http://server:8080/tfs/collection/project/_apis/git/repositories/repo.git/pullrequests"},
		{"escaped names and URL credentials", "https://user:password@dev.azure.com/org/my%20project/_git/my%20repo?secret=value#fragment", "https://dev.azure.com/org/my%20project/_apis/git/repositories/my%20repo/pullrequests"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			svc, err := NewAzureDevOpsPRService(context.Background(), &WriteBackConfig{GitRepo: tt.repo}, &mockTokenProvider{token: "pat"})
			require.NoError(t, err)
			assert.Equal(t, tt.want+"?api-version=7.1", svc.apiURL.String())
		})
	}
	for _, repo := range []string{
		"", "git@ssh.dev.azure.com:v3/org/project/repo", "ssh://git@ssh.dev.azure.com/v3/org/project/repo",
		"https:///org/project/_git/repo", "https://dev.azure.com/org/project/repo",
		"https://dev.azure.com/org/project/_git/", "https://dev.azure.com/org/project/_git/repo/extra",
	} {
		t.Run(repo, func(t *testing.T) {
			_, err := NewAzureDevOpsPRService(context.Background(), &WriteBackConfig{GitRepo: repo}, &mockTokenProvider{token: "pat"})
			require.Error(t, err)
		})
	}
}

func Test_NewAzureDevOpsPRService(t *testing.T) {
	for _, tt := range []struct {
		name     string
		provider *mockTokenProvider
		want     string
	}{
		{"token error", &mockTokenProvider{err: errors.New("secret not found")}, "secret not found"},
		{"empty token", &mockTokenProvider{}, "empty SCM token"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewAzureDevOpsPRService(context.Background(), &WriteBackConfig{GitRepo: "https://dev.azure.com/org/project/_git/repo"}, tt.provider)
			require.ErrorContains(t, err, tt.want)
		})
	}
}

func newTestAzureDevOpsPRService(t *testing.T, pr *PullRequest, handler http.HandlerFunc) *AzureDevOpsPRService {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	svc, err := NewAzureDevOpsPRService(context.Background(), &WriteBackConfig{
		GitRepo: server.URL + "/collection/my%20project/_git/my%20repo", PullRequest: pr,
	}, &mockTokenProvider{token: "pat"})
	require.NoError(t, err)
	return svc
}

func Test_AzureDevOpsPRService_create(t *testing.T) {
	for _, tt := range []struct {
		name, head, base string
		labels           []string
	}{
		{"without labels", "image-updater/test", "refs/heads/main", nil},
		{"with labels", "refs/heads/image-updater/test", "main", []string{"image-update", "automated"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			pr := &PullRequest{title: "update images", body: strings.Repeat("界", 4001), head: tt.head, base: tt.base, labels: tt.labels}
			svc := newTestAzureDevOpsPRService(t, pr, func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodPost, r.Method)
				assert.Equal(t, "/collection/my%20project/_apis/git/repositories/my%20repo/pullrequests", r.URL.EscapedPath())
				assert.Equal(t, "7.1", r.URL.Query().Get("api-version"))
				assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
				username, password, ok := r.BasicAuth()
				assert.True(t, ok)
				assert.Empty(t, username)
				assert.Equal(t, "pat", password)
				var body map[string]any
				if assert.NoError(t, json.NewDecoder(r.Body).Decode(&body)) {
					assert.Equal(t, "refs/heads/image-updater/test", body["sourceRefName"])
					assert.Equal(t, "refs/heads/main", body["targetRefName"])
					assert.Equal(t, pr.title, body["title"])
					assert.Equal(t, strings.Repeat("界", 4000), body["description"])
					if len(tt.labels) == 0 {
						assert.NotContains(t, body, "labels")
					} else {
						assert.Equal(t, []any{map[string]any{"name": "image-update"}, map[string]any{"name": "automated"}}, body["labels"])
					}
				}
				w.WriteHeader(http.StatusCreated)
				_, _ = w.Write([]byte(`{"pullRequestId":42,"url":"https://dev.azure.com/org/project/_git/repo/pullrequest/42"}`))
			})
			require.NoError(t, svc.create(context.Background()))
			assert.Equal(t, strings.Repeat("界", 4001), pr.body)
		})
	}
}

func Test_AzureDevOpsPRService_createErrors(t *testing.T) {
	for _, tt := range []struct {
		name      string
		status    int
		body      string
		duplicate bool
	}{
		{"duplicate", http.StatusConflict, `{"typeKey":"GitPullRequestExistsException","message":"A pull request already exists."}`, true},
		{"other conflict", http.StatusConflict, `{"typeKey":"OtherException","message":"Another conflict."}`, false},
		{"unauthorized", http.StatusUnauthorized, `{"message":"Access denied."}`, false},
		{"non-JSON error", http.StatusBadGateway, "Bad gateway", false},
		{"invalid response", http.StatusCreated, "invalid JSON", false},
		{"missing PR ID", http.StatusCreated, `{}`, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			svc := newTestAzureDevOpsPRService(t, &PullRequest{head: "update", base: "main"}, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			})
			err := svc.create(context.Background())
			require.Error(t, err)
			assert.Equal(t, tt.duplicate, errors.Is(err, ErrPRAlreadyExists))
		})
	}
}

func Test_AzureDevOpsPRService_exists(t *testing.T) {
	for _, tt := range []struct {
		name, body string
		status     int
		want       bool
		wantErr    bool
	}{
		{"exists", `{"count":1,"value":[{"pullRequestId":42}]}`, http.StatusOK, true, false},
		{"absent", `{"count":0,"value":[]}`, http.StatusOK, false, false},
		{"API error", `{"message":"Access denied."}`, http.StatusForbidden, false, true},
		{"invalid response", "invalid JSON", http.StatusOK, false, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			svc := newTestAzureDevOpsPRService(t, nil, func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodGet, r.Method)
				assert.Equal(t, url.Values{
					"api-version": {"7.1"}, "$top": {"1"}, "searchCriteria.status": {"active"},
					"searchCriteria.sourceRefName": {"refs/heads/image-updater/test"},
					"searchCriteria.targetRefName": {"refs/heads/main"},
				}, r.URL.Query())
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			})
			exists, err := svc.exists(context.Background(), "main", "refs/heads/image-updater/test")
			assert.Equal(t, tt.want, exists)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func Test_commitChangesPR_AzureDevOps(t *testing.T) {
	for _, tt := range []struct {
		name      string
		exists    bool
		duplicate bool
	}{
		{"existing PR skips git", true, false},
		{"creates PR", false, false},
		{"concurrent duplicate is a no-op", false, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var mu sync.Mutex
			var requests []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				requests = append(requests, r.Method)
				if r.Method == http.MethodGet {
					if tt.exists {
						_, _ = w.Write([]byte(`{"value":[{"pullRequestId":42}]}`))
					} else {
						_, _ = w.Write([]byte(`{"value":[]}`))
					}
					return
				}
				if tt.duplicate {
					w.WriteHeader(http.StatusConflict)
					_, _ = w.Write([]byte(`{"typeKey":"GitPullRequestExistsException"}`))
				} else {
					w.WriteHeader(http.StatusCreated)
					_, _ = w.Write([]byte(`{"pullRequestId":42}`))
				}
			}))
			defer server.Close()
			gitClient := &mockGitClient{}
			if tt.exists {
				gitClient.initErr = errors.New("git must not be initialized when the PR exists")
			}
			wbc := &WriteBackConfig{
				GitRepo: server.URL + "/org/project/_git/repo", GitBranch: "main", PRProvider: PRProviderAzureDevOps,
				GitClient: gitClient,
				GetCreds: func(_ *argocdapi.Application) (git.Creds, error) {
					return &mockGitAndSCMCreds{token: "pat"}, nil
				},
			}
			require.NoError(t, commitChangesPR(context.Background(), makeTestAppImages(wbc), nil, noopWriter))
			mu.Lock()
			defer mu.Unlock()
			if tt.exists {
				assert.Equal(t, []string{http.MethodGet}, requests)
			} else {
				assert.Equal(t, []string{http.MethodGet, http.MethodPost}, requests)
			}
		})
	}
}
