package argocd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	argocdapi "github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
	gogithub "github.com/google/go-github/v69/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/argoproj-labs/argocd-image-updater/ext/git"
)

// --- mock types shared by commitChangesPR tests ---

// mockGitClient stubs all git.Client methods. Set initErr to simulate a
// failure at the earliest point inside commitChangesGit.
type mockGitClient struct {
	root    string
	initErr error
}

func (m *mockGitClient) Root() string                                          { return m.root }
func (m *mockGitClient) Init(_ context.Context) error                          { return m.initErr }
func (m *mockGitClient) Fetch(_ context.Context, _ string) error               { return nil }
func (m *mockGitClient) ShallowFetch(_ context.Context, _ string, _ int) error { return nil }
func (m *mockGitClient) Submodule(_ context.Context) error                     { return nil }
func (m *mockGitClient) Checkout(_ context.Context, _ string, _ bool) error    { return nil }
func (m *mockGitClient) LsRefs(_ context.Context) (*git.Refs, error)           { return &git.Refs{}, nil }
func (m *mockGitClient) LsRemote(_ context.Context, _ string) (string, error)  { return "", nil }
func (m *mockGitClient) LsFiles(_ context.Context, _ string, _ bool) ([]string, error) {
	return nil, nil
}
func (m *mockGitClient) LsLargeFiles(_ context.Context) ([]string, error) { return nil, nil }
func (m *mockGitClient) CommitSHA(_ context.Context) (string, error)      { return "", nil }
func (m *mockGitClient) RevisionMetadata(_ context.Context, _ string) (*git.RevisionMetadata, error) {
	return nil, nil
}
func (m *mockGitClient) VerifyCommitSignature(_ context.Context, _ string) (string, error) {
	return "", nil
}
func (m *mockGitClient) IsAnnotatedTag(_ context.Context, _ string) bool { return false }
func (m *mockGitClient) ChangedFiles(_ context.Context, _, _ string) ([]string, error) {
	return nil, nil
}
func (m *mockGitClient) Commit(_ context.Context, _ string, _ *git.CommitOptions) error { return nil }
func (m *mockGitClient) Branch(_ context.Context, _, _ string) error                    { return nil }
func (m *mockGitClient) Push(_ context.Context, _, _ string, _ bool) error              { return nil }
func (m *mockGitClient) Add(_ context.Context, _ string) error                          { return nil }
func (m *mockGitClient) SymRefToBranch(_ context.Context, _ string) (string, error) {
	return "main", nil
}
func (m *mockGitClient) Config(_ context.Context, _, _ string) error { return nil }

// mockGitAndSCMCreds implements both git.Creds (required return type of
// GetCreds) and git.SCMTokenProvider (required by commitChangesPR).
type mockGitAndSCMCreds struct {
	token string
}

func (m *mockGitAndSCMCreds) Environ(_ context.Context) (io.Closer, []string, error) {
	return io.NopCloser(bytes.NewReader(nil)), nil, nil
}

func (m *mockGitAndSCMCreds) SCMToken(_ context.Context) (string, error) {
	return m.token, nil
}

// noopWriter is a changeWriter that skips the commit/push step.
// commitChangesGit returns nil immediately after calling write when skip=true,
// but only after buildPullRequest has already populated wbc.PullRequest.
func noopWriter(_ context.Context, _ *ApplicationImages, _ git.Client) (error, bool) {
	return nil, true
}

// makeTestAppImages builds a minimal ApplicationImages for commitChangesPR tests.
func makeTestAppImages(wbc *WriteBackConfig) *ApplicationImages {
	var app argocdapi.Application
	app.Name = "test-app"
	app.Namespace = "test-ns"
	return &ApplicationImages{
		Application:     app,
		WriteBackConfig: wbc,
	}
}

// --- Test_buildPullRequest ---

func Test_buildPullRequest(t *testing.T) {
	ctx := context.Background()
	const (
		ns         = "my-namespace"
		name       = "my-app"
		baseBranch = "main"
		headBranch = "image-updater-my-namespace-my-app"
	)

	tests := []struct {
		name          string
		commitMessage string
		wantTitle     string
		wantBody      string
		wantHead      string
		wantBase      string
	}{
		{
			name:      "empty commit message produces default title and body",
			wantTitle: "chore: update images for my-namespace/my-app",
			wantBody:  "This pull request was created automatically by argocd-image-updater for application my-namespace/my-app.",
			wantHead:  headBranch,
			wantBase:  baseBranch,
		},
		{
			name:          "single-line message sets title, body is empty",
			commitMessage: "build: update nginx to 1.5",
			wantTitle:     "build: update nginx to 1.5",
			wantBody:      "",
			wantHead:      headBranch,
			wantBase:      baseBranch,
		},
		{
			name:          "multi-line message splits into title and body",
			commitMessage: "build: update nginx\n\nupdates nginx tag '1.0' → '1.5'",
			wantTitle:     "build: update nginx",
			wantBody:      "updates nginx tag '1.0' → '1.5'",
			wantHead:      headBranch,
			wantBase:      baseBranch,
		},
		{
			name:          "leading/trailing whitespace is trimmed from title and body",
			commitMessage: "  trimmed title  \n  trimmed body  ",
			wantTitle:     "trimmed title",
			wantBody:      "trimmed body",
			wantHead:      headBranch,
			wantBase:      baseBranch,
		},
		{
			name:          "title over 255 characters is truncated",
			commitMessage: strings.Repeat("x", 300),
			wantTitle:     strings.Repeat("x", 255),
			wantBody:      "",
			wantHead:      headBranch,
			wantBase:      baseBranch,
		},
		{
			name:          "body over 65536 characters is truncated",
			commitMessage: "title\n" + strings.Repeat("y", 70000),
			wantTitle:     "title",
			wantBody:      strings.Repeat("y", 65536),
			wantHead:      headBranch,
			wantBase:      baseBranch,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wbc := &WriteBackConfig{GitCommitMessage: tt.commitMessage}
			pr, err := buildPullRequest(ctx, wbc, ns, name, baseBranch, headBranch)
			require.NoError(t, err)
			assert.Equal(t, tt.wantTitle, pr.title)
			assert.Equal(t, tt.wantBody, pr.body)
			assert.Equal(t, tt.wantHead, pr.head)
			assert.Equal(t, tt.wantBase, pr.base)
		})
	}
}

// --- Test_commitChangesPR ---

func Test_commitChangesPR(t *testing.T) {
	ctx := context.Background()

	// --- precondition checks (fail before commitChangesGit is called) ---

	t.Run("unsupported PR provider", func(t *testing.T) {
		wbc := &WriteBackConfig{
			GitRepo:    "https://github.com/org/repo.git",
			GitBranch:  "main",
			PRProvider: PRProvider(99), // truly unsupported
			GitClient:  &mockGitClient{},
			GetCreds: func(_ *argocdapi.Application) (git.Creds, error) {
				return &mockGitAndSCMCreds{token: "token"}, nil
			},
		}
		err := commitChangesPR(ctx, makeTestAppImages(wbc), nil, noopWriter)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported PR provider")
	})

	t.Run("GetCreds fails — rejected before git push", func(t *testing.T) {
		// No GitClient needed: GetCreds is called before commitChangesGit.
		wbc := &WriteBackConfig{
			GitRepo:    "https://github.com/org/repo.git",
			PRProvider: PRProviderGitHub,
			GetCreds: func(_ *argocdapi.Application) (git.Creds, error) {
				return nil, fmt.Errorf("secret not found")
			},
		}
		err := commitChangesPR(ctx, makeTestAppImages(wbc), nil, noopWriter)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "could not get creds")
	})

	t.Run("creds do not implement SCMTokenProvider — rejected before git push", func(t *testing.T) {
		// No GitClient needed: the type-assertion fires before commitChangesGit.
		wbc := &WriteBackConfig{
			GitRepo:    "https://github.com/org/repo.git",
			PRProvider: PRProviderGitHub,
			GetCreds: func(_ *argocdapi.Application) (git.Creds, error) {
				// NopCreds satisfies git.Creds but NOT git.SCMTokenProvider.
				return git.NopCreds{}, nil
			},
		}
		err := commitChangesPR(ctx, makeTestAppImages(wbc), nil, noopWriter)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "do not support PR creation")
	})

	// --- git push phase ---

	t.Run("commitChangesGit fails: git init error", func(t *testing.T) {
		wbc := &WriteBackConfig{
			GitRepo:    "https://github.com/org/repo.git",
			GitBranch:  "main",
			PRProvider: PRProviderGitHub,
			GitClient:  &mockGitClient{initErr: fmt.Errorf("init failed")},
			GetCreds: func(_ *argocdapi.Application) (git.Creds, error) {
				return &mockGitAndSCMCreds{token: "token"}, nil
			},
		}
		err := commitChangesPR(ctx, makeTestAppImages(wbc), nil, noopWriter)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "init failed")
	})

	// --- GitHub API phase ---

	t.Run("GitHub: PR created successfully", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, http.MethodPost, r.Method)
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(gogithub.PullRequest{
				Number:  gogithub.Ptr(1),
				HTMLURL: gogithub.Ptr("http://example.com/pull/1"),
			})
		}))
		defer server.Close()

		wbc := &WriteBackConfig{
			GitRepo:    server.URL + "/org/repo.git",
			GitBranch:  "main",
			PRProvider: PRProviderGitHub,
			GitClient:  &mockGitClient{},
			GetCreds: func(_ *argocdapi.Application) (git.Creds, error) {
				return &mockGitAndSCMCreds{token: "github-token"}, nil
			},
		}
		err := commitChangesPR(ctx, makeTestAppImages(wbc), nil, noopWriter)
		require.NoError(t, err)
	})

	t.Run("GitHub: PR already exists — treated as no-op, no error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnprocessableEntity)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"message": "Validation Failed",
				"errors":  []map[string]string{{"message": "A pull request already exists for org:image-updater-branch."}},
			})
		}))
		defer server.Close()

		wbc := &WriteBackConfig{
			GitRepo:    server.URL + "/org/repo.git",
			GitBranch:  "main",
			PRProvider: PRProviderGitHub,
			GitClient:  &mockGitClient{},
			GetCreds: func(_ *argocdapi.Application) (git.Creds, error) {
				return &mockGitAndSCMCreds{token: "github-token"}, nil
			},
		}
		err := commitChangesPR(ctx, makeTestAppImages(wbc), nil, noopWriter)
		require.NoError(t, err)
	})

	t.Run("GitHub: create fails with 422", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnprocessableEntity)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"message": "Validation Failed",
				"errors":  []map[string]string{{"message": "some error"}},
			})
		}))
		defer server.Close()

		wbc := &WriteBackConfig{
			GitRepo:    server.URL + "/org/repo.git",
			GitBranch:  "main",
			PRProvider: PRProviderGitHub,
			GitClient:  &mockGitClient{},
			GetCreds: func(_ *argocdapi.Application) (git.Creds, error) {
				return &mockGitAndSCMCreds{token: "github-token"}, nil
			},
		}
		err := commitChangesPR(ctx, makeTestAppImages(wbc), nil, noopWriter)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "could not create PR")
	})

	// --- GitLab API phase ---

	t.Run("GitLab: MR created successfully", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, http.MethodPost, r.Method)
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"iid":     1,
				"web_url": "http://example.com/-/merge_requests/1",
			})
		}))
		defer server.Close()

		wbc := &WriteBackConfig{
			GitRepo:    server.URL + "/group/repo.git",
			GitBranch:  "main",
			PRProvider: PRProviderGitLab,
			GitClient:  &mockGitClient{},
			GetCreds: func(_ *argocdapi.Application) (git.Creds, error) {
				return &mockGitAndSCMCreds{token: "gitlab-token"}, nil
			},
		}
		err := commitChangesPR(ctx, makeTestAppImages(wbc), nil, noopWriter)
		require.NoError(t, err)
	})

	t.Run("GitLab: MR already exists — treated as no-op, no error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusConflict)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"message": []string{"Another open merge request already exists for this source branch: !1"},
			})
		}))
		defer server.Close()

		wbc := &WriteBackConfig{
			GitRepo:    server.URL + "/group/repo.git",
			GitBranch:  "main",
			PRProvider: PRProviderGitLab,
			GitClient:  &mockGitClient{},
			GetCreds: func(_ *argocdapi.Application) (git.Creds, error) {
				return &mockGitAndSCMCreds{token: "gitlab-token"}, nil
			},
		}
		err := commitChangesPR(ctx, makeTestAppImages(wbc), nil, noopWriter)
		require.NoError(t, err)
	})

	t.Run("GitLab: create fails with 422", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnprocessableEntity)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"message": "some validation error",
			})
		}))
		defer server.Close()

		wbc := &WriteBackConfig{
			GitRepo:    server.URL + "/group/repo.git",
			GitBranch:  "main",
			PRProvider: PRProviderGitLab,
			GitClient:  &mockGitClient{},
			GetCreds: func(_ *argocdapi.Application) (git.Creds, error) {
				return &mockGitAndSCMCreds{token: "gitlab-token"}, nil
			},
		}
		err := commitChangesPR(ctx, makeTestAppImages(wbc), nil, noopWriter)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "could not create MR")
	})
}
