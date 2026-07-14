package argocd

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/argoproj-labs/argocd-image-updater/ext/git"
	gitmock "github.com/argoproj-labs/argocd-image-updater/ext/git/mocks"
)

func Test_graphQLEndpoint(t *testing.T) {
	assert.Equal(t, "https://api.github.com/graphql", graphQLEndpoint(""))
	assert.Equal(t, "https://api.github.com/graphql", graphQLEndpoint("https://api.github.com"))
	assert.Equal(t, "https://ghe.example.com/api/graphql", graphQLEndpoint("https://ghe.example.com/api/v3"))
	assert.Equal(t, "https://ghe.example.com/api/graphql", graphQLEndpoint("https://ghe.example.com/api/v3/"))
}

func Test_splitCommitMessage(t *testing.T) {
	h, b := splitCommitMessage("build: update image\n\nfoo updated to v2\nbar updated to v3")
	assert.Equal(t, "build: update image", h)
	assert.Equal(t, "foo updated to v2\nbar updated to v3", b)

	h, b = splitCommitMessage("single line")
	assert.Equal(t, "single line", h)
	assert.Empty(t, b)

	h, b = splitCommitMessage("")
	assert.Equal(t, "Update container image versions", h)
	assert.Empty(t, b)
}

func Test_createCommitOnBranch_Success(t *testing.T) {
	var gotAuth string
	var gotBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		require.NoError(t, json.NewDecoder(r.Body).Decode(&gotBody))
		fmt.Fprint(w, `{"data":{"createCommitOnBranch":{"commit":{"oid":"abc123"}}}}`)
	}))
	defer srv.Close()

	input := &commitOnBranchInput{}
	input.Branch.RepositoryNameWithOwner = "example/repo"
	input.Branch.BranchName = "main"
	input.ExpectedHeadOID = "headsha"
	input.Message.Headline = "build: update image"

	oid, err := createCommitOnBranch(context.Background(), srv.URL, "test-token", input)
	require.NoError(t, err)
	assert.Equal(t, "abc123", oid)
	assert.Equal(t, "Bearer test-token", gotAuth)

	vars := gotBody["variables"].(map[string]interface{})["input"].(map[string]interface{})
	assert.Equal(t, "headsha", vars["expectedHeadOid"])
	branch := vars["branch"].(map[string]interface{})
	assert.Equal(t, "example/repo", branch["repositoryNameWithOwner"])
	assert.Equal(t, "main", branch["branchName"])
	assert.Equal(t, "build: update image", vars["message"].(map[string]interface{})["headline"])
}

// Test_createCommitOnBranch_OmitsEmptyFileChangeLists pins that nil
// additions/deletions slices are omitted from the JSON payload entirely. A
// marshaled `"deletions": null` passes GraphQL schema validation but crashes
// GitHub's createCommitOnBranch resolver with the opaque "Something went
// wrong while executing your query" error.
func Test_createCommitOnBranch_OmitsEmptyFileChangeLists(t *testing.T) {
	var gotBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&gotBody))
		fmt.Fprint(w, `{"data":{"createCommitOnBranch":{"commit":{"oid":"abc123"}}}}`)
	}))
	defer srv.Close()

	input := &commitOnBranchInput{}
	input.Branch.RepositoryNameWithOwner = "example/repo"
	input.Branch.BranchName = "main"
	input.ExpectedHeadOID = "headsha"
	input.Message.Headline = "build: update image"
	input.FileChanges.Additions = []graphQLFileAddition{{Path: "a.yaml", Contents: "eA=="}}

	_, err := createCommitOnBranch(context.Background(), srv.URL, "t", input)
	require.NoError(t, err)

	fc := gotBody["variables"].(map[string]interface{})["input"].(map[string]interface{})["fileChanges"].(map[string]interface{})
	assert.Contains(t, fc, "additions")
	assert.NotContains(t, fc, "deletions", "nil deletions must be omitted, not marshaled as JSON null")
}

func Test_createCommitOnBranch_GraphQLError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"data":null,"errors":[{"message":"Expected head OID did not match"}]}`)
	}))
	defer srv.Close()

	_, err := createCommitOnBranch(context.Background(), srv.URL, "t", &commitOnBranchInput{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Expected head OID did not match")
}

func Test_createCommitOnBranch_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad credentials", http.StatusUnauthorized)
	}))
	defer srv.Close()

	_, err := createCommitOnBranch(context.Background(), srv.URL, "t", &commitOnBranchInput{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "HTTP 401")
}

// newAPICommitGitMock builds a git client mock over dir with the given
// working-tree changes and head SHA.
func newAPICommitGitMock(dir, headSHA string, changes []git.WorkingTreeChange) *gitmock.Client {
	m := &gitmock.Client{}
	// The generated mock drops the ctx arg from Called() for context-only
	// methods, so expectations are registered with no argument matchers.
	m.On("WorkingTreeChanges").Return(changes, nil)
	m.On("CommitSHA").Return(headSHA, nil)
	m.On("Root").Return(dir)
	return m
}

func Test_commitChangesGithubAPI_ExistingBranch(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "overlays"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "overlays", "kustomization.yaml"), []byte("images: []\n"), 0o644))

	gitMock := newAPICommitGitMock(dir, "headsha123", []git.WorkingTreeChange{
		{Path: "overlays/kustomization.yaml"},
		{Path: "old.yaml", Deleted: true},
	})

	var gotBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/graphql":
			assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))
			require.NoError(t, json.NewDecoder(r.Body).Decode(&gotBody))
			fmt.Fprint(w, `{"data":{"createCommitOnBranch":{"commit":{"oid":"newsha456"}}}}`)
		default:
			t.Errorf("unexpected request path %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	provider := &mockTokenAndBaseURLProvider{
		mockTokenProvider: mockTokenProvider{token: "test-token"},
		baseURL:           srv.URL + "/api/v3",
	}
	wbc := &WriteBackConfig{
		GitRepo:          "https://github.com/example/repo.git",
		GitCommitMessage: "build: update image\n\nfoo updated to v2",
	}

	err := commitChangesGithubAPI(context.Background(), wbc, gitMock, provider, "main", false)
	require.NoError(t, err)

	vars := gotBody["variables"].(map[string]interface{})["input"].(map[string]interface{})
	assert.Equal(t, "headsha123", vars["expectedHeadOid"])
	assert.Equal(t, "main", vars["branch"].(map[string]interface{})["branchName"])
	assert.Equal(t, "example/repo", vars["branch"].(map[string]interface{})["repositoryNameWithOwner"])
	assert.Equal(t, "build: update image", vars["message"].(map[string]interface{})["headline"])
	fc := vars["fileChanges"].(map[string]interface{})
	additions := fc["additions"].([]interface{})
	require.Len(t, additions, 1)
	assert.Equal(t, "overlays/kustomization.yaml", additions[0].(map[string]interface{})["path"])
	assert.Equal(t, base64.StdEncoding.EncodeToString([]byte("images: []\n")), additions[0].(map[string]interface{})["contents"])
	deletions := fc["deletions"].([]interface{})
	require.Len(t, deletions, 1)
	assert.Equal(t, "old.yaml", deletions[0].(map[string]interface{})["path"])
}

func Test_commitChangesGithubAPI_NewBranchCreatesRef(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.yaml"), []byte("a: 1\n"), 0o644))

	gitMock := newAPICommitGitMock(dir, "basesha", []git.WorkingTreeChange{{Path: "a.yaml"}})

	refCreated := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v3/repos/example/repo/git/refs" && r.Method == http.MethodPost:
			var body map[string]interface{}
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			assert.Equal(t, "refs/heads/image-updater-branch", body["ref"])
			assert.Equal(t, "basesha", body["sha"])
			refCreated = true
			w.WriteHeader(http.StatusCreated)
			fmt.Fprint(w, `{"ref":"refs/heads/image-updater-branch","object":{"sha":"basesha"}}`)
		case r.URL.Path == "/api/graphql":
			fmt.Fprint(w, `{"data":{"createCommitOnBranch":{"commit":{"oid":"newsha"}}}}`)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	provider := &mockTokenAndBaseURLProvider{
		mockTokenProvider: mockTokenProvider{token: "test-token"},
		baseURL:           srv.URL + "/api/v3",
	}
	wbc := &WriteBackConfig{GitRepo: "https://github.com/example/repo.git", GitCommitMessage: "msg"}

	err := commitChangesGithubAPI(context.Background(), wbc, gitMock, provider, "image-updater-branch", true)
	require.NoError(t, err)
	assert.True(t, refCreated)
}

// Test_commitChangesGithubAPI_NewBranchRefAlreadyExists pins the idempotency
// guard: when the remote branch was already created by a previous (partially
// failed) cycle, the 422 "Reference already exists" response is tolerated and
// the commit proceeds.
func Test_commitChangesGithubAPI_NewBranchRefAlreadyExists(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.yaml"), []byte("a: 1\n"), 0o644))

	gitMock := newAPICommitGitMock(dir, "basesha", []git.WorkingTreeChange{{Path: "a.yaml"}})

	commitCreated := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v3/repos/example/repo/git/refs" && r.Method == http.MethodPost:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnprocessableEntity)
			fmt.Fprint(w, `{"message":"Reference already exists","documentation_url":"https://docs.github.com"}`)
		case r.URL.Path == "/api/graphql":
			commitCreated = true
			fmt.Fprint(w, `{"data":{"createCommitOnBranch":{"commit":{"oid":"newsha"}}}}`)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	provider := &mockTokenAndBaseURLProvider{
		mockTokenProvider: mockTokenProvider{token: "test-token"},
		baseURL:           srv.URL + "/api/v3",
	}
	wbc := &WriteBackConfig{GitRepo: "https://github.com/example/repo.git", GitCommitMessage: "msg"}

	err := commitChangesGithubAPI(context.Background(), wbc, gitMock, provider, "image-updater-branch", true)
	require.NoError(t, err)
	assert.True(t, commitCreated, "commit must proceed when the branch ref already exists")
}

func Test_commitChangesGithubAPI_NoChangesErrors(t *testing.T) {
	gitMock := newAPICommitGitMock(t.TempDir(), "sha", nil)
	// A server that fails the test if anything calls it.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
	}))
	defer srv.Close()

	provider := &mockTokenAndBaseURLProvider{
		mockTokenProvider: mockTokenProvider{token: "t"},
		baseURL:           srv.URL + "/api/v3",
	}
	wbc := &WriteBackConfig{GitRepo: "https://github.com/example/repo.git"}

	err := commitChangesGithubAPI(context.Background(), wbc, gitMock, provider, "main", false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no file changes")
}

func Test_githubAppCredsProvider(t *testing.T) {
	appCreds := git.NewGitHubAppCreds(1, 2, "key", "", "https://github.com/example/repo.git", "", "", false, "", git.NoopCredsStore{})
	p, ok := githubAppCredsProvider(appCreds)
	assert.True(t, ok)
	assert.NotNil(t, p)

	p, ok = githubAppCredsProvider(git.NopCreds{})
	assert.False(t, ok)
	assert.Nil(t, p)
}
