package argocd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_NewGiteaPRService_URL(t *testing.T) {
	for _, tt := range []struct {
		name, repo, want string
	}{
		{"HTTPS clone URL", "https://gitea.example.com/owner/repo.git", "https://gitea.example.com/api/v1/repos/owner/repo"},
		{"without .git suffix", "https://gitea.example.com/owner/repo", "https://gitea.example.com/api/v1/repos/owner/repo"},
		{"trailing slash", "https://gitea.example.com/owner/repo/", "https://gitea.example.com/api/v1/repos/owner/repo"},
		{"plain HTTP with port", "http://gitea-http.gitea.svc:3000/owner/repo.git", "http://gitea-http.gitea.svc:3000/api/v1/repos/owner/repo"},
		{"served under a sub-path", "https://example.com/git/gitea/owner/repo.git", "https://example.com/git/gitea/api/v1/repos/owner/repo"},
		{"escaped names and URL credentials", "https://user:password@gitea.example.com/my%20org/my%20repo.git?secret=value#fragment", "https://gitea.example.com/api/v1/repos/my%20org/my%20repo"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			svc, err := NewGiteaPRService(context.Background(), &WriteBackConfig{GitRepo: tt.repo}, &mockTokenProvider{token: "token"})
			require.NoError(t, err)
			assert.Equal(t, tt.want, svc.apiURL.String())
		})
	}
	for _, repo := range []string{
		"", "git@gitea.example.com:owner/repo.git", "ssh://git@gitea.example.com/owner/repo.git",
		"https:///owner/repo", "https://gitea.example.com/repo.git", "https://gitea.example.com/",
		"https://gitea.example.com/owner/.git", "https://bad host/owner/repo", "https://gitea.example.com:invalid/owner/repo",
	} {
		t.Run(repo, func(t *testing.T) {
			_, err := NewGiteaPRService(context.Background(), &WriteBackConfig{GitRepo: repo}, &mockTokenProvider{token: "token"})
			require.Error(t, err)
		})
	}
}

func Test_NewGiteaPRService(t *testing.T) {
	for _, tt := range []struct {
		name     string
		provider *mockTokenProvider
		want     string
	}{
		{"token error", &mockTokenProvider{err: errors.New("secret not found")}, "secret not found"},
		{"empty token", &mockTokenProvider{}, "empty SCM token"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewGiteaPRService(context.Background(), &WriteBackConfig{GitRepo: "https://gitea.example.com/owner/repo.git"}, tt.provider)
			require.ErrorContains(t, err, tt.want)
		})
	}
}

func newTestGiteaPRService(t *testing.T, pr *PullRequest, handler http.HandlerFunc) *GiteaPRService {
	t.Helper()
	server := httptest.NewTLSServer(handler)
	t.Cleanup(server.Close)
	svc, err := NewGiteaPRService(context.Background(), &WriteBackConfig{
		GitRepo: server.URL + "/my%20org/repo.git", PullRequest: pr,
	}, &mockTokenProvider{token: "token"})
	require.NoError(t, err)
	svc.client.Transport = server.Client().Transport
	return svc
}

type giteaRoundTripper func(*http.Request) (*http.Response, error)

func (f giteaRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func Test_GiteaPRService_redirects(t *testing.T) {
	for _, tt := range []struct {
		name, repo, location, wantErr string
		wantRequests                  int
	}{
		{"same host", "https://gitea.example.com/owner/repo", "https://gitea.example.com/redirected", "", 2},
		{"relative URL", "https://gitea.example.com/owner/repo", "/redirected", "", 2},
		{"HTTP downgrade", "https://gitea.example.com/owner/repo", "http://gitea.example.com/redirected", "refusing Gitea redirect", 1},
		{"HTTP to HTTPS", "http://gitea.example.com/owner/repo", "https://gitea.example.com/redirected", "", 2},
		{"different host", "https://gitea.example.com/owner/repo", "https://example.com/redirected", "refusing Gitea redirect", 1},
		{"different port", "https://gitea.example.com/owner/repo", "https://gitea.example.com:444/redirected", "refusing Gitea redirect", 1},
		{"redirect loop", "https://gitea.example.com/owner/repo", "/loop", "stopped after 10 redirects", 10},
	} {
		t.Run(tt.name, func(t *testing.T) {
			svc, err := NewGiteaPRService(context.Background(), &WriteBackConfig{GitRepo: tt.repo}, &mockTokenProvider{token: "token"})
			require.NoError(t, err)
			requests := 0
			svc.client.Transport = giteaRoundTripper(func(req *http.Request) (*http.Response, error) {
				requests++
				assert.Equal(t, "token token", req.Header.Get("Authorization"))
				status := http.StatusTemporaryRedirect
				if req.URL.Path == "/redirected" {
					status = http.StatusOK
				}
				return &http.Response{
					StatusCode: status,
					Header:     http.Header{"Location": {tt.location}},
					Body:       io.NopCloser(strings.NewReader(`[]`)),
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

	for _, tt := range []struct {
		name         string
		status       int
		wantErr      string
		wantRequests int
	}{
		{"307 keeps the POST", http.StatusTemporaryRedirect, "", 2},
		{"308 keeps the POST", http.StatusPermanentRedirect, "", 2},
		{"302 turns the POST into a GET", http.StatusFound, "turns POST into GET", 1},
		{"301 turns the POST into a GET", http.StatusMovedPermanently, "turns POST into GET", 1},
	} {
		t.Run("create: "+tt.name, func(t *testing.T) {
			pr := &PullRequest{title: "t", head: "h", base: "b"}
			svc, err := NewGiteaPRService(context.Background(), &WriteBackConfig{GitRepo: "https://gitea.example.com/owner/repo", PullRequest: pr}, &mockTokenProvider{token: "token"})
			require.NoError(t, err)
			requests := 0
			svc.client.Transport = giteaRoundTripper(func(req *http.Request) (*http.Response, error) {
				requests++
				if req.URL.Path == "/redirected" {
					assert.Equal(t, http.MethodPost, req.Method)
					return &http.Response{StatusCode: http.StatusCreated, Body: io.NopCloser(strings.NewReader(`{"number":1}`)), Request: req}, nil
				}
				return &http.Response{
					StatusCode: tt.status,
					Header:     http.Header{"Location": {"/redirected"}},
					Body:       io.NopCloser(strings.NewReader("")),
					Request:    req,
				}, nil
			})
			err = svc.create(context.Background())
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.wantRequests, requests)
		})
	}
}

func Test_GiteaPRService_create(t *testing.T) {
	pr := &PullRequest{title: "chore: update images", body: "automated update", head: "image-updater-test", base: "main"}
	tests := []struct {
		name, body string
		status     int
		wantErr    error
		wantErrMsg string
	}{
		{"success", `{"number":42,"html_url":"https://gitea.example.com/owner/repo/pulls/42"}`, http.StatusCreated, nil, ""},
		{"duplicate", `{"message":"pull request already exists for these targets"}`, http.StatusConflict, ErrPRAlreadyExists, ""},
		{"validation error", `{"message":"Invalid PullRequest: There are no changes between the head and the base"}`, http.StatusUnprocessableEntity, nil, "no changes between the head and the base"},
		{"branch not found", `{"message":"The target couldn't be found."}`, http.StatusNotFound, nil, "404 Not Found"},
		{"unauthorized", `{"message":"token is required"}`, http.StatusUnauthorized, nil, "token is required"},
		{"server error", "Internal server error", http.StatusInternalServerError, nil, "500 Internal Server Error"},
		{"invalid response", `{"number":0}`, http.StatusCreated, nil, "valid pull request number"},
		{"malformed response", `{`, http.StatusCreated, nil, "could not decode Gitea response"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newTestGiteaPRService(t, pr, func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodPost, r.Method)
				assert.Equal(t, "/api/v1/repos/my%20org/repo/pulls", r.URL.EscapedPath())
				assert.Equal(t, "token token", r.Header.Get("Authorization"))
				assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
				var payload map[string]any
				require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
				assert.Equal(t, map[string]any{
					"title": "chore: update images",
					"body":  "automated update",
					"head":  "image-updater-test",
					"base":  "main",
				}, payload)
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			})

			err := svc.create(context.Background())
			switch {
			case tt.wantErr != nil:
				require.ErrorIs(t, err, tt.wantErr)
			case tt.wantErrMsg != "":
				require.ErrorContains(t, err, tt.wantErrMsg)
			default:
				require.NoError(t, err)
			}
		})
	}

	t.Run("nil pull request", func(t *testing.T) {
		svc := newTestGiteaPRService(t, nil, func(http.ResponseWriter, *http.Request) {
			t.Error("no request expected")
		})
		require.ErrorContains(t, svc.create(context.Background()), "pull request metadata is nil")
	})
}

func Test_GiteaPRService_create_labels(t *testing.T) {
	for _, tt := range []struct {
		name        string
		labels      []string
		labelStatus int
		wantLabels  bool
	}{
		{"no labels", nil, http.StatusOK, false},
		{"labels applied", []string{"image-update", "automated"}, http.StatusOK, true},
		{"labelling failure does not fail the update", []string{"image-update"}, http.StatusForbidden, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			pr := &PullRequest{title: "t", head: "h", base: "b", labels: tt.labels}
			labelled := false
			svc := newTestGiteaPRService(t, pr, func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.EscapedPath() {
				case "/api/v1/repos/my%20org/repo/pulls":
					w.WriteHeader(http.StatusCreated)
					_, _ = w.Write([]byte(`{"number":7}`))
				case "/api/v1/repos/my%20org/repo/issues/7/labels":
					labelled = true
					assert.Equal(t, http.MethodPost, r.Method)
					var payload map[string][]string
					require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
					assert.Equal(t, tt.labels, payload["labels"])
					w.WriteHeader(tt.labelStatus)
					_, _ = w.Write([]byte(`[]`))
				default:
					t.Errorf("unexpected request %s %s", r.Method, r.URL)
				}
			})
			require.NoError(t, svc.create(context.Background()))
			assert.Equal(t, tt.wantLabels, labelled)
		})
	}
}

func Test_GiteaPRService_exists(t *testing.T) {
	pr := func(head, base string, headRepo, baseRepo int64) string {
		return fmt.Sprintf(`{"number":1,"head":{"ref":%q,"repo_id":%d},"base":{"ref":%q,"repo_id":%d}}`, head, headRepo, base, baseRepo)
	}
	for _, tt := range []struct {
		name    string
		pages   []string
		total   string
		status  int
		want    bool
		wantErr string
	}{
		{"no open PR", []string{`[]`}, "", http.StatusOK, false, ""},
		{"matching PR", []string{"[" + pr("update", "main", 1, 1) + "]"}, "", http.StatusOK, true, ""},
		{"other head branch", []string{"[" + pr("other", "main", 1, 1) + "]", `[]`}, "", http.StatusOK, false, ""},
		{"other base branch", []string{"[" + pr("update", "develop", 1, 1) + "]", `[]`}, "", http.StatusOK, false, ""},
		{"same branch from a fork", []string{"[" + pr("update", "main", 2, 1) + "]", `[]`}, "", http.StatusOK, false, ""},
		{"match on second page", []string{"[" + pr("other", "main", 1, 1) + "]", "[" + pr("update", "main", 1, 1) + "]"}, "", http.StatusOK, true, ""},
		{"total count ends the scan", []string{"[" + pr("other", "main", 1, 1) + "]"}, "1", http.StatusOK, false, ""},
		{"total count beyond the first page", []string{"[" + pr("other", "main", 1, 1) + "]", "[" + pr("update", "main", 1, 1) + "]"}, "2", http.StatusOK, true, ""},
		{"invalid total count is ignored", []string{"[" + pr("other", "main", 1, 1) + "]", `[]`}, "many", http.StatusOK, false, ""},
		{"API error", nil, "", http.StatusForbidden, false, "could not list Gitea PRs"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			requests := 0
			svc := newTestGiteaPRService(t, nil, func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodGet, r.Method)
				assert.Equal(t, "/api/v1/repos/my%20org/repo/pulls", r.URL.EscapedPath())
				q := r.URL.Query()
				assert.Equal(t, "open", q.Get("state"))
				assert.Equal(t, "main", q.Get("base_branch"))
				assert.Equal(t, fmt.Sprint(requests+1), q.Get("page"))
				if tt.status != http.StatusOK {
					w.WriteHeader(tt.status)
					_, _ = w.Write([]byte(`{"message":"forbidden"}`))
					return
				}
				require.Less(t, requests, len(tt.pages), "unexpected extra page request")
				if tt.total != "" {
					w.Header().Set("X-Total-Count", tt.total)
				}
				_, _ = w.Write([]byte(tt.pages[requests]))
				requests++
			})
			got, err := svc.exists(context.Background(), "main", "update")
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}

	t.Run("scan is bounded", func(t *testing.T) {
		requests := 0
		svc := newTestGiteaPRService(t, nil, func(w http.ResponseWriter, _ *http.Request) {
			requests++
			_, _ = w.Write([]byte("[" + pr("other", "main", 1, 1) + "]"))
		})
		_, err := svc.exists(context.Background(), "main", "update")
		require.ErrorContains(t, err, "giving up the scan")
		assert.Equal(t, giteaMaxListPages, requests)
	})
}
