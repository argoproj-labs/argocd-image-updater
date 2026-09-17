package argocd

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	pr := &PullRequest{title: "chore: update images", body: "automated update", head: "image-updater/test", base: "refs/heads/main"}
	tests := []struct {
		name, body string
		status     int
		wantErr    error
		wantErrMsg string
	}{
		{"success", `{"pullRequestId":42,"url":"https://dev.azure.com/org/project/_git/repo/pullrequest/42"}`, http.StatusCreated, nil, ""},
		{"duplicate", `{"typeKey":"GitPullRequestExistsException"}`, http.StatusConflict, ErrPRAlreadyExists, ""},
		{"other conflict", `{"typeKey":"OtherException","message":"Another conflict."}`, http.StatusConflict, nil, "Another conflict."},
		{"validation error", `{"message":"Invalid branch."}`, http.StatusBadRequest, nil, "Invalid branch."},
		{"unauthorized", `{"message":"Access denied."}`, http.StatusUnauthorized, nil, "Access denied."},
		{"server error", "Internal server error", http.StatusInternalServerError, nil, "500 Internal Server Error"},
		{"invalid response", "invalid JSON", http.StatusCreated, nil, "could not decode Azure DevOps response"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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
					assert.Equal(t, pr.body, body["description"])
				}
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			})
			err := svc.create(context.Background())
			switch {
			case tt.wantErr != nil:
				require.ErrorIs(t, err, tt.wantErr)
			case tt.wantErrMsg != "":
				require.ErrorContains(t, err, "could not create PR")
				assert.Contains(t, err.Error(), tt.wantErrMsg)
				assert.NotErrorIs(t, err, ErrPRAlreadyExists)
			default:
				require.NoError(t, err)
			}
		})
	}
}

func Test_AzureDevOpsPRService_create_labels(t *testing.T) {
	for _, tt := range []struct {
		name   string
		labels []string
	}{
		{"labels are sent with the create request", []string{"image-update", "automated"}},
		{"no labels field is sent when none are configured", nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			pr := &PullRequest{title: "chore: update images", head: "update", base: "main", labels: tt.labels}
			svc := newTestAzureDevOpsPRService(t, pr, func(w http.ResponseWriter, r *http.Request) {
				var body map[string]any
				if assert.NoError(t, json.NewDecoder(r.Body).Decode(&body)) {
					if len(tt.labels) == 0 {
						assert.NotContains(t, body, "labels")
					} else {
						assert.Equal(t, []any{map[string]any{"name": "image-update"}, map[string]any{"name": "automated"}}, body["labels"])
					}
				}
				w.WriteHeader(http.StatusCreated)
				_, _ = w.Write([]byte(`{"pullRequestId":42}`))
			})
			require.NoError(t, svc.create(context.Background()))
		})
	}
}

func Test_AzureDevOpsPRService_create_body(t *testing.T) {
	pr := &PullRequest{title: "chore: update images", body: strings.Repeat("é", 4001), head: "update", base: "main"}
	svc := newTestAzureDevOpsPRService(t, pr, func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if assert.NoError(t, json.NewDecoder(r.Body).Decode(&body)) {
			assert.Equal(t, strings.Repeat("é", 4000), body["description"])
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"pullRequestId":42}`))
	})
	require.NoError(t, svc.create(context.Background()))
	assert.Equal(t, strings.Repeat("é", 4001), pr.body)
}

func Test_AzureDevOpsPRService_exists(t *testing.T) {
	for _, tt := range []struct {
		name, body string
		status     int
		wantExists bool
		wantErrMsg string
	}{
		{"no open PRs", `{"count":0,"value":[]}`, http.StatusOK, false, ""},
		{"one open PR", `{"count":1,"value":[{"pullRequestId":42}]}`, http.StatusOK, true, ""},
		{"multiple open PRs", `{"count":2,"value":[{"pullRequestId":42},{"pullRequestId":43}]}`, http.StatusOK, true, ""},
		{"API error", `{"message":"Access denied."}`, http.StatusForbidden, false, "Access denied."},
		{"server error", "Internal server error", http.StatusInternalServerError, false, "500 Internal Server Error"},
		{"invalid response", "invalid JSON", http.StatusOK, false, "could not decode Azure DevOps response"},
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
			assert.Equal(t, tt.wantExists, exists)
			if tt.wantErrMsg != "" {
				require.ErrorContains(t, err, "could not list Azure DevOps PRs")
				assert.Contains(t, err.Error(), tt.wantErrMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
