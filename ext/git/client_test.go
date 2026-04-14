package git

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sync"
	"testing"

	"github.com/go-git/go-git/v5/plumbing"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

func runCmd(workingDir string, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = workingDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func _createEmptyGitRepo() (string, error) {
	tempDir, err := os.MkdirTemp("", "")
	if err != nil {
		return tempDir, err
	}

	err = runCmd(tempDir, "git", "init")
	if err != nil {
		return tempDir, err
	}

	err = runCmd(tempDir, "git", "commit", "-m", "Initial commit", "--allow-empty")
	return tempDir, err
}

func Test_nativeGitClient_Fetch(t *testing.T) {
	ctx := context.Background()
	tempDir, err := _createEmptyGitRepo()
	require.NoError(t, err)

	client, err := NewClient(fmt.Sprintf("file://%s", tempDir), NopCreds{}, true, false, "")
	require.NoError(t, err)

	err = client.Init(ctx)
	require.NoError(t, err)

	err = client.Fetch(ctx, "")
	assert.NoError(t, err)
}

func Test_nativeGitClient_Fetch_Prune(t *testing.T) {
	ctx := context.Background()

	tempDir, err := _createEmptyGitRepo()
	require.NoError(t, err)

	client, err := NewClient(fmt.Sprintf("file://%s", tempDir), NopCreds{}, true, false, "")
	require.NoError(t, err)

	err = client.Init(ctx)
	require.NoError(t, err)

	err = runCmd(tempDir, "git", "branch", "test/foo")
	require.NoError(t, err)

	err = client.Fetch(ctx, "")
	assert.NoError(t, err)

	err = runCmd(tempDir, "git", "branch", "-d", "test/foo")
	require.NoError(t, err)
	err = runCmd(tempDir, "git", "branch", "test/foo/bar")
	require.NoError(t, err)

	err = client.Fetch(ctx, "")
	assert.NoError(t, err)
}

func Test_IsAnnotatedTag(t *testing.T) {
	ctx := context.Background()

	tempDir := t.TempDir()
	client, err := NewClient(fmt.Sprintf("file://%s", tempDir), NopCreds{}, true, false, "")
	require.NoError(t, err)

	err = client.Init(ctx)
	require.NoError(t, err)

	p := path.Join(client.Root(), "README")
	f, err := os.Create(p)
	require.NoError(t, err)
	_, err = f.WriteString("Hello.")
	require.NoError(t, err)
	err = f.Close()
	require.NoError(t, err)

	err = runCmd(client.Root(), "git", "add", "README")
	require.NoError(t, err)

	err = runCmd(client.Root(), "git", "commit", "-m", "Initial commit", "-a")
	require.NoError(t, err)

	atag := client.IsAnnotatedTag(ctx, "master")
	assert.False(t, atag)

	err = runCmd(client.Root(), "git", "tag", "some-tag", "-a", "-m", "Create annotated tag")
	require.NoError(t, err)
	atag = client.IsAnnotatedTag(ctx, "some-tag")
	assert.True(t, atag)

	// Tag effectually points to HEAD, so it's considered the same
	atag = client.IsAnnotatedTag(ctx, "HEAD")
	assert.True(t, atag)

	err = runCmd(client.Root(), "git", "rm", "README")
	assert.NoError(t, err)
	err = runCmd(client.Root(), "git", "commit", "-m", "remove README", "-a")
	assert.NoError(t, err)

	// We moved on, so tag doesn't point to HEAD anymore
	atag = client.IsAnnotatedTag(ctx, "HEAD")
	assert.False(t, atag)
}

func Test_ChangedFiles(t *testing.T) {
	ctx := context.Background()

	tempDir := t.TempDir()

	client, err := NewClientExt(fmt.Sprintf("file://%s", tempDir), tempDir, NopCreds{}, true, false, "")
	require.NoError(t, err)

	err = client.Init(ctx)
	require.NoError(t, err)

	err = runCmd(client.Root(), "git", "commit", "-m", "Initial commit", "--allow-empty")
	require.NoError(t, err)

	// Create a tag to have a second ref
	err = runCmd(client.Root(), "git", "tag", "some-tag")
	require.NoError(t, err)

	p := path.Join(client.Root(), "README")
	f, err := os.Create(p)
	require.NoError(t, err)
	_, err = f.WriteString("Hello.")
	require.NoError(t, err)
	err = f.Close()
	require.NoError(t, err)

	err = runCmd(client.Root(), "git", "add", "README")
	require.NoError(t, err)

	err = runCmd(client.Root(), "git", "commit", "-m", "Changes", "-a")
	require.NoError(t, err)

	previousSHA, err := client.LsRemote(ctx, "some-tag")
	require.NoError(t, err)

	commitSHA, err := client.LsRemote(ctx, "HEAD")
	require.NoError(t, err)

	// Invalid commits, error
	_, err = client.ChangedFiles(ctx, "0000000000000000000000000000000000000000", "1111111111111111111111111111111111111111")
	require.Error(t, err)

	// Not SHAs, error
	_, err = client.ChangedFiles(ctx, previousSHA, "HEAD")
	require.Error(t, err)

	// Same commit, no changes
	changedFiles, err := client.ChangedFiles(ctx, commitSHA, commitSHA)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{}, changedFiles)

	// Different ref, with changes
	changedFiles, err = client.ChangedFiles(ctx, previousSHA, commitSHA)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"README"}, changedFiles)
}

func Test_nativeGitClient_Submodule(t *testing.T) {
	ctx := context.Background()

	tempDir, err := os.MkdirTemp("", "")
	require.NoError(t, err)

	foo := filepath.Join(tempDir, "foo")
	err = os.Mkdir(foo, 0755)
	require.NoError(t, err)

	err = runCmd(foo, "git", "init")
	require.NoError(t, err)

	bar := filepath.Join(tempDir, "bar")
	err = os.Mkdir(bar, 0755)
	require.NoError(t, err)

	err = runCmd(bar, "git", "init")
	require.NoError(t, err)

	err = runCmd(bar, "git", "commit", "-m", "Initial commit", "--allow-empty")
	require.NoError(t, err)

	// Embed repository bar into repository foo
	t.Setenv("GIT_ALLOW_PROTOCOL", "file")
	err = runCmd(foo, "git", "submodule", "add", bar)
	require.NoError(t, err)

	err = runCmd(foo, "git", "commit", "-m", "Initial commit")
	require.NoError(t, err)

	tempDir, err = os.MkdirTemp("", "")
	require.NoError(t, err)

	// Clone foo
	err = runCmd(tempDir, "git", "clone", foo)
	require.NoError(t, err)

	client, err := NewClient(fmt.Sprintf("file://%s", foo), NopCreds{}, true, false, "")
	require.NoError(t, err)

	err = client.Init(ctx)
	require.NoError(t, err)

	err = client.Fetch(ctx, "")
	assert.NoError(t, err)

	commitSHA, err := client.LsRemote(ctx, "HEAD")
	assert.NoError(t, err)

	// Call Checkout() with submoduleEnabled=false.
	err = client.Checkout(ctx, commitSHA, false)
	assert.NoError(t, err)

	// Check if submodule url does not exist in .git/config
	err = runCmd(client.Root(), "git", "config", "submodule.bar.url")
	assert.Error(t, err)

	// Call Submodule() via Checkout() with submoduleEnabled=true.
	err = client.Checkout(ctx, commitSHA, true)
	assert.NoError(t, err)

	// Check if the .gitmodule URL is reflected in .git/config
	cmd := exec.Command("git", "config", "submodule.bar.url")
	cmd.Dir = client.Root()
	result, err := cmd.Output()
	assert.NoError(t, err)
	assert.Equal(t, bar+"\n", string(result))

	// Change URL of submodule bar
	err = runCmd(client.Root(), "git", "config", "--file=.gitmodules", "submodule.bar.url", bar+"baz")
	require.NoError(t, err)

	// Call Submodule()
	err = client.Submodule(ctx)
	assert.NoError(t, err)

	// Check if the URL change in .gitmodule is reflected in .git/config
	cmd = exec.Command("git", "config", "submodule.bar.url")
	cmd.Dir = client.Root()
	result, err = cmd.Output()
	assert.NoError(t, err)
	assert.Equal(t, bar+"baz\n", string(result))
}

func TestNewClient_invalidSSHURL(t *testing.T) {
	client, err := NewClient("ssh://bitbucket.org:org/repo", NopCreds{}, false, false, "")
	assert.Nil(t, client)
	assert.ErrorIs(t, err, ErrInvalidRepoURL)
}

// ---------------------------------------------------------------------------
// Helpers shared by newAuth and getRefs tests
// ---------------------------------------------------------------------------

// generateSSHPrivateKeyPEM creates an ed25519 private key encoded in the
// OpenSSH PEM format that ssh.ParsePrivateKey accepts.
func generateSSHPrivateKeyPEM(t *testing.T) []byte {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	block, err := ssh.MarshalPrivateKey(priv, "")
	require.NoError(t, err)
	return pem.EncodeToMemory(block)
}

// memoryRefCache is a simple in-memory implementation of gitRefCache used in
// tests. GetOrLockGitReferences returns a cache hit when refs have already been
// stored, or returns the caller's own lockId (making the caller the lock owner)
// when the cache is empty.
type memoryRefCache struct {
	mu          sync.Mutex
	refs        map[string][]*plumbing.Reference
	setCount    int
	unlockCount int
}

func newMemoryRefCache() *memoryRefCache {
	return &memoryRefCache{refs: make(map[string][]*plumbing.Reference)}
}

func (c *memoryRefCache) GetOrLockGitReferences(repo, myLockId string, out *[]*plumbing.Reference) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if refs, ok := c.refs[repo]; ok {
		*out = refs
		return "", nil // cache hit — caller is not the lock owner
	}
	return myLockId, nil // cache miss — caller becomes the lock owner
}

func (c *memoryRefCache) SetGitReferences(repo string, refs []*plumbing.Reference) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.refs[repo] = refs
	c.setCount++
	return nil
}

func (c *memoryRefCache) UnlockGitReferences(_ string, _ string) error {
	c.unlockCount++
	return nil
}

// ---------------------------------------------------------------------------
// newAuth tests
// ---------------------------------------------------------------------------

func TestNewAuth_NopCreds(t *testing.T) {
	auth, err := newAuth(context.Background(), "https://github.com/org/repo", NopCreds{})
	require.NoError(t, err)
	assert.Nil(t, auth, "NopCreds must produce nil auth")
}

func TestNewAuth_HTTPSCreds(t *testing.T) {
	ctx := context.Background()

	t.Run("WithUsername", func(t *testing.T) {
		creds := NewHTTPSCreds("alice", "secret", "", "", false, "", &NoopCredsStore{}, false)
		auth, err := newAuth(ctx, "https://github.com/org/repo", creds)
		require.NoError(t, err)
		basic, ok := auth.(*githttp.BasicAuth)
		require.True(t, ok, "expected *githttp.BasicAuth")
		assert.Equal(t, "alice", basic.Username)
		assert.Equal(t, "secret", basic.Password)
	})

	t.Run("EmptyUsername_defaultsToXAccessToken", func(t *testing.T) {
		creds := NewHTTPSCreds("", "mytoken", "", "", false, "", &NoopCredsStore{}, false)
		auth, err := newAuth(ctx, "https://github.com/org/repo", creds)
		require.NoError(t, err)
		basic, ok := auth.(*githttp.BasicAuth)
		require.True(t, ok)
		assert.Equal(t, "x-access-token", basic.Username)
		assert.Equal(t, "mytoken", basic.Password)
	})
}

func TestNewAuth_SSHCreds(t *testing.T) {
	ctx := context.Background()

	t.Run("ValidKey_insecure", func(t *testing.T) {
		keyPEM := generateSSHPrivateKeyPEM(t)
		creds := NewSSHCreds(string(keyPEM), "", true, &NoopCredsStore{}, "")
		auth, err := newAuth(ctx, "git@github.com:org/repo.git", creds)
		require.NoError(t, err)
		pk, ok := auth.(*PublicKeysWithOptions)
		require.True(t, ok, "expected *PublicKeysWithOptions")
		assert.NotNil(t, pk.Signer)
		// Insecure: host key callback must be the permissive one (non-nil).
		assert.NotNil(t, pk.HostKeyCallback)
	})

	t.Run("ValidKey_SSHURLExtractsUser", func(t *testing.T) {
		keyPEM := generateSSHPrivateKeyPEM(t)
		creds := NewSSHCreds(string(keyPEM), "", true, &NoopCredsStore{}, "")
		auth, err := newAuth(ctx, "ssh://bob@github.com/org/repo", creds)
		require.NoError(t, err)
		pk, ok := auth.(*PublicKeysWithOptions)
		require.True(t, ok)
		assert.Equal(t, "bob", pk.User)
	})

	t.Run("InvalidKey_returnsError", func(t *testing.T) {
		creds := NewSSHCreds("not-a-valid-pem-key", "", true, &NoopCredsStore{}, "")
		_, err := newAuth(ctx, "git@github.com:org/repo.git", creds)
		assert.Error(t, err)
	})
}

func TestNewAuth_GitHubAppCreds(t *testing.T) {
	ctx := context.Background()

	key := generateRSAPrivateKeyPEM(t)
	server, _ := githubAppMockServer(t, "ghs_newauth_token")
	creds := newTestGitHubAppCreds(t, key, server.URL, &NoopCredsStore{})

	auth, err := newAuth(ctx, "https://github.com/org/repo", creds)
	require.NoError(t, err)
	basic, ok := auth.(*githttp.BasicAuth)
	require.True(t, ok, "expected *githttp.BasicAuth")
	assert.Equal(t, "x-access-token", basic.Username)
	assert.Equal(t, "ghs_newauth_token", basic.Password)
}

func TestNewAuth_GoogleCloudCreds(t *testing.T) {
	ctx := context.Background()

	t.Run("ValidCreds", func(t *testing.T) {
		gcpCreds := GoogleCloudCreds{
			creds: &google.Credentials{
				ProjectID:   "my-google-project",
				TokenSource: oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "gcp-test-token"}),
				JSON:        []byte(gcpServiceAccountKeyJSON),
			},
			store: &NoopCredsStore{},
		}
		auth, err := newAuth(ctx, "https://source.developers.google.com/p/project/r/repo", gcpCreds)
		require.NoError(t, err)
		basic, ok := auth.(*githttp.BasicAuth)
		require.True(t, ok, "expected *githttp.BasicAuth")
		assert.Equal(t, "argocd-service-account@my-google-project.iam.gserviceaccount.com", basic.Username)
		assert.NotEmpty(t, basic.Password)
	})

	t.Run("NilCreds_returnsError", func(t *testing.T) {
		gcpCreds := GoogleCloudCreds{creds: nil, store: &NoopCredsStore{}}
		_, err := newAuth(ctx, "https://source.developers.google.com/p/project/r/repo", gcpCreds)
		assert.Error(t, err)
	})
}

// ---------------------------------------------------------------------------
// getRefs tests
// ---------------------------------------------------------------------------

func TestGetRefs_Basic(t *testing.T) {
	ctx := context.Background()
	repoURL, _ := setupLocalRemoteRepo(t)

	c, err := NewClient(repoURL, NopCreds{}, true, false, "")
	require.NoError(t, err)

	refs, err := c.(*nativeGitClient).getRefs(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, refs, "at least one ref (the initial branch) must be returned")

	var branchNames []string
	for _, r := range refs {
		if r.Name().IsBranch() {
			branchNames = append(branchNames, r.Name().Short())
		}
	}
	assert.Contains(t, branchNames, "master")
}

func TestGetRefs_OnLsRemoteCallback(t *testing.T) {
	ctx := context.Background()
	repoURL, _ := setupLocalRemoteRepo(t)

	called := false
	doneCalled := false
	handlers := EventHandlers{
		OnLsRemote: func(repo string) func() {
			assert.Equal(t, repoURL, repo)
			called = true
			return func() { doneCalled = true }
		},
	}

	c, err := NewClient(repoURL, NopCreds{}, true, false, "", WithEventHandlers(handlers))
	require.NoError(t, err)

	_, err = c.(*nativeGitClient).getRefs(ctx)
	require.NoError(t, err)
	assert.True(t, called, "OnLsRemote must be invoked")
	assert.True(t, doneCalled, "OnLsRemote done callback must be invoked")
}

func TestGetRefs_CacheHit(t *testing.T) {
	ctx := context.Background()
	repoURL, _ := setupLocalRemoteRepo(t)

	// Pre-populate the cache with a sentinel reference so we can tell whether
	// getRefs returned from cache or from the remote.
	sentinel := plumbing.NewHashReference(
		plumbing.NewBranchReferenceName("cached-sentinel"),
		plumbing.NewHash("0000000000000000000000000000000000000000"),
	)
	cache := newMemoryRefCache()
	cache.refs[repoURL] = []*plumbing.Reference{sentinel}

	c, err := NewClient(repoURL, NopCreds{}, true, false, "", WithCache(cache, true))
	require.NoError(t, err)

	refs, err := c.(*nativeGitClient).getRefs(ctx)
	require.NoError(t, err)
	require.Len(t, refs, 1)
	assert.Equal(t, "cached-sentinel", refs[0].Name().Short())

	// Cache must not have been written again (it was a hit).
	assert.Equal(t, 0, cache.setCount, "SetGitReferences must not be called on a cache hit")
}

func TestGetRefs_CacheMiss_StoresResult(t *testing.T) {
	ctx := context.Background()
	repoURL, _ := setupLocalRemoteRepo(t)

	cache := newMemoryRefCache() // empty — triggers a cache miss

	c, err := NewClient(repoURL, NopCreds{}, true, false, "", WithCache(cache, true))
	require.NoError(t, err)

	refs, err := c.(*nativeGitClient).getRefs(ctx)
	require.NoError(t, err)
	assert.NotEmpty(t, refs)

	// Fetched refs must have been stored in the cache.
	assert.Equal(t, 1, cache.setCount, "SetGitReferences must be called exactly once on a cache miss")
	assert.NotEmpty(t, cache.refs[repoURL])
}

func TestGetRefs_WithoutCacheLoading(t *testing.T) {
	ctx := context.Background()
	repoURL, _ := setupLocalRemoteRepo(t)

	// Cache is set but loadRefFromCache=false: getRefs must skip GetOrLockGitReferences
	// but still store the result via SetGitReferences.
	sentinel := plumbing.NewHashReference(
		plumbing.NewBranchReferenceName("should-not-be-returned"),
		plumbing.NewHash("0000000000000000000000000000000000000000"),
	)
	cache := newMemoryRefCache()
	cache.refs[repoURL] = []*plumbing.Reference{sentinel}

	c, err := NewClient(repoURL, NopCreds{}, true, false, "", WithCache(cache, false))
	require.NoError(t, err)

	refs, err := c.(*nativeGitClient).getRefs(ctx)
	require.NoError(t, err)

	// Must have fetched from remote (not returned the sentinel).
	var names []string
	for _, r := range refs {
		names = append(names, r.Name().Short())
	}
	assert.NotContains(t, names, "should-not-be-returned",
		"without loadRefFromCache, stale cache must be bypassed")

	// Result must still be stored.
	assert.Equal(t, 1, cache.setCount)
}
