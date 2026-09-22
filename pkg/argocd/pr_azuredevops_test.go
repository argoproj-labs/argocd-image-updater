package argocd

import (
	"context"
	"encoding/json"
	"errors"
	"io"
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
		{"Azure DevOps Server", "https://server:8443/tfs/collection/project/_git/repo.git", "https://server:8443/tfs/collection/project/_apis/git/repositories/repo.git/pullrequests"},
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
		"http://dev.azure.com/org/project/_git/repo", "https://:443/org/project/_git/repo",
		"https://bad host/org/project/_git/repo", "https://dev.azure.com:invalid/org/project/_git/repo",
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
	server := httptest.NewTLSServer(handler)
	t.Cleanup(server.Close)
	svc, err := NewAzureDevOpsPRService(context.Background(), &WriteBackConfig{
		GitRepo: server.URL + "/collection/my%20project/_git/my%20repo", PullRequest: pr,
	}, &mockTokenProvider{token: "pat"})
	require.NoError(t, err)
	svc.client.Transport = server.Client().Transport
	return svc
}

type azureDevOpsRoundTripper func(*http.Request) (*http.Response, error)

func (f azureDevOpsRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func Test_AzureDevOpsPRService_redirects(t *testing.T) {
	for _, tt := range []struct {
		name, location, wantErr string
		wantRequests            int
	}{
		{"same host", "https://dev.azure.com/redirected", "", 2},
		{"relative URL", "/redirected", "", 2},
		{"HTTP downgrade", "http://dev.azure.com/redirected", "refusing Azure DevOps redirect", 1},
		{"different host", "https://example.com/redirected", "refusing Azure DevOps redirect", 1},
		{"subdomain", "https://sub.dev.azure.com/redirected", "refusing Azure DevOps redirect", 1},
		{"different port", "https://dev.azure.com:444/redirected", "refusing Azure DevOps redirect", 1},
		{"redirect loop", "/loop", "stopped after 10 redirects", 10},
	} {
		t.Run(tt.name, func(t *testing.T) {
			svc, err := NewAzureDevOpsPRService(context.Background(), &WriteBackConfig{
				GitRepo: "https://dev.azure.com/org/project/_git/repo",
			}, &mockTokenProvider{token: "pat"})
			require.NoError(t, err)
			requests := 0
			svc.client.Transport = azureDevOpsRoundTripper(func(req *http.Request) (*http.Response, error) {
				requests++
				_, password, ok := req.BasicAuth()
				assert.True(t, ok)
				assert.Equal(t, "pat", password)
				status := http.StatusTemporaryRedirect
				if req.URL.Path == "/redirected" {
					status = http.StatusOK
				}
				return &http.Response{
					StatusCode: status,
					Header:     http.Header{"Location": {tt.location}},
					Body:       io.NopCloser(strings.NewReader(`{"value":[]}`)),
					Request:    req,
				}, nil
			})
			_, err = svc.exists(context.Background(), "main", "update")
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.wantRequests, requests)
		})
	}
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
		name        string
		labels      []string
		failedLabel string
		wantLabels  []string
	}{
		{"apply labels after creation", []string{"image-update", "automated"}, "", []string{"image-update", "automated"}},
		{"no labels configured", nil, "", nil},
		{"continue after a label fails", []string{"image-update", "automated"}, "image-update", []string{"automated"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			pr := &PullRequest{title: "chore: update images", head: "update", base: "main", labels: tt.labels}
			var appliedLabels []string
			var attemptedLabels []string
			created := false
			createRequests := 0
			svc := newTestAzureDevOpsPRService(t, pr, func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodPost, r.Method)
				assert.Equal(t, "7.1", r.URL.Query().Get("api-version"))
				assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
				username, password, ok := r.BasicAuth()
				assert.True(t, ok)
				assert.Empty(t, username)
				assert.Equal(t, "pat", password)
				var body map[string]any
				if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&body)) {
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				switch r.URL.EscapedPath() {
				case "/collection/my%20project/_apis/git/repositories/my%20repo/pullrequests":
					createRequests++
					assert.NotContains(t, body, "labels")
					created = true
					w.WriteHeader(http.StatusCreated)
					// The labels endpoint must use the configured repository URL, not this response URL.
					_, _ = w.Write([]byte(`{"pullRequestId":42,"url":"https://example.com/untrusted"}`))
				case "/collection/my%20project/_apis/git/repositories/my%20repo/pullrequests/42/labels":
					assert.True(t, created, "labels must be applied after the PR exists")
					name, ok := body["name"].(string)
					assert.True(t, ok)
					assert.Len(t, body, 1)
					attemptedLabels = append(attemptedLabels, name)
					if name == tt.failedLabel {
						w.WriteHeader(http.StatusForbidden)
						_, _ = w.Write([]byte(`{"message":"Access denied."}`))
						return
					}
					appliedLabels = append(appliedLabels, name)
					_, _ = w.Write([]byte(`{"id":"label-id","name":"` + name + `","active":true}`))
				default:
					t.Errorf("unexpected request: %s", r.URL)
					w.WriteHeader(http.StatusNotFound)
				}
			})
			require.NoError(t, svc.create(context.Background()))
			assert.Equal(t, 1, createRequests)
			assert.Equal(t, tt.labels, attemptedLabels)
			assert.Equal(t, tt.wantLabels, appliedLabels)
		})
	}
}

func Test_AzureDevOpsPRService_create_labels_creationFailed(t *testing.T) {
	for _, tt := range []struct {
		name, body string
		status     int
	}{
		{"duplicate", `{"typeKey":"GitPullRequestExistsException"}`, http.StatusConflict},
		{"forbidden", `{"message":"Access denied."}`, http.StatusForbidden},
		{"missing PR ID", `{}`, http.StatusCreated},
	} {
		t.Run(tt.name, func(t *testing.T) {
			requests := 0
			svc := newTestAzureDevOpsPRService(t, &PullRequest{labels: []string{"image-update"}}, func(w http.ResponseWriter, r *http.Request) {
				requests++
				assert.Equal(t, "/collection/my%20project/_apis/git/repositories/my%20repo/pullrequests", r.URL.EscapedPath())
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			})
			require.Error(t, svc.create(context.Background()))
			assert.Equal(t, 1, requests, "labels must not be applied without a created PR")
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
