package argocd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/argoproj-labs/argocd-image-updater/ext/git"
	gitmock "github.com/argoproj-labs/argocd-image-updater/ext/git/mocks"

	"github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
)

const (
	precheckRepo   = "git@example.com:example"
	precheckBranch = "image-updater/testapp"
	precheckTarget = "apps/testapp/.argocd-source-testapp.yaml"
	sha1           = "1111111111111111111111111111111111111111"
	sha2           = "2222222222222222222222222222222222222222"
	sha3           = "3333333333333333333333333333333333333333"
)

// precheckImages returns a Kustomize application that writes back to
// precheckBranch, with example.com/foo set to tag.
func precheckImages(name string, tag string) *ApplicationImages {
	app := v1alpha1.Application{
		ObjectMeta: v1.ObjectMeta{Name: name},
		Spec: v1alpha1.ApplicationSpec{
			Source: &v1alpha1.ApplicationSource{
				RepoURL:        precheckRepo,
				TargetRevision: "HEAD",
				Path:           "apps/testapp",
				Kustomize: &v1alpha1.ApplicationSourceKustomize{
					Images: v1alpha1.KustomizeImages{v1alpha1.KustomizeImage("example.com/foo:" + tag)},
				},
			},
		},
		Status: v1alpha1.ApplicationStatus{SourceType: v1alpha1.ApplicationSourceTypeKustomize},
	}
	wbc := &WriteBackConfig{
		Method:         WriteBackGit,
		GitRepo:        precheckRepo,
		GitBranch:      "main",
		GitWriteBranch: precheckBranch,
		Target:         "apps/testapp/.argocd-source-" + name + ".yaml",
		GetCreds: func(app *v1alpha1.Application) (git.Creds, error) {
			return git.NopCreds{}, nil
		},
	}
	return &ApplicationImages{Application: app, Images: ImageList{}, WriteBackConfig: wbc}
}

// overrideFor returns the parameter override file that the write-back writes
// for the application name with the given tag.
func overrideFor(t *testing.T, name string, tag string) []byte {
	t.Helper()
	b, err := marshalParamsOverride(context.Background(), precheckImages(name, tag), nil)
	require.NoError(t, err)
	return b
}

// precheckState returns an iteration state whose ls-remote returns the given
// head for the write-back branch, and a counter of ls-remote calls.
func precheckState(cache *GitWriteBackCache, head string) (*SyncIterationState, *atomic.Int32) {
	state := NewSyncIterationState().WithGitWriteBackCache(cache)
	calls := &atomic.Int32{}
	state.remoteRefs.list = func(ctx context.Context, repo string, creds git.Creds) ([]*plumbing.Reference, error) {
		calls.Add(1)
		return []*plumbing.Reference{
			plumbing.NewSymbolicReference(plumbing.HEAD, plumbing.NewBranchReferenceName("main")),
			plumbing.NewHashReference(plumbing.NewBranchReferenceName("main"), plumbing.NewHash(sha1)),
			plumbing.NewHashReference(plumbing.NewBranchReferenceName(precheckBranch), plumbing.NewHash(head)),
		}, nil
	}
	return state, calls
}

// cloneMock returns a git client mock for the full write-back path, with the
// remote target file holding remoteContent and the local head at head.
func cloneMock(t *testing.T, target string, remoteContent []byte, head string) *gitmock.Client {
	t.Helper()
	gitMock, dir, cleanup := mockGit(t)
	t.Cleanup(cleanup)
	gitMock.On("Checkout", precheckBranch, false).Return(nil).Run(func(args mock.Arguments) {
		require.NoError(t, os.MkdirAll(filepath.Join(dir, filepath.Dir(target)), 0700))
		require.NoError(t, os.WriteFile(filepath.Join(dir, target), remoteContent, 0600))
	})
	gitMock.On("CommitSHA").Return(head, nil)
	return gitMock
}

func precheckKey(name string) gitWriteBackKey {
	return gitWriteBackKey{repo: precheckRepo, target: "apps/testapp/.argocd-source-" + name + ".yaml"}
}

func cached(head string, content []byte) gitWriteBackEntry {
	return gitWriteBackEntry{ref: precheckBranch, head: head, content: content}
}

func Test_GitWriteBackPrecheck(t *testing.T) {
	ctx := context.Background()

	t.Run("No cached state clones and fills the cache", func(t *testing.T) {
		cache := NewGitWriteBackCache()
		state, lsRemoteCalls := precheckState(cache, sha1)
		ai := precheckImages("testapp", "v1")
		gitMock := cloneMock(t, precheckTarget, overrideFor(t, "testapp", "v0"), sha2)
		gitMock.On("Commit", mock.Anything, mock.Anything).Return(nil)
		gitMock.On("Push", "origin", precheckBranch, true).Return(nil)
		ai.GitClient = gitMock

		require.NoError(t, commitChangesLocked(ctx, ai, state, nil))

		assert.Equal(t, int32(0), lsRemoteCalls.Load(), "ls-remote is not needed without cached state")
		gitMock.AssertCalled(t, "ShallowFetch", precheckBranch)
		gitMock.AssertCalled(t, "Push", "origin", precheckBranch, true)
		entry, ok := cache.get(precheckKey("testapp"))
		require.True(t, ok)
		assert.Equal(t, sha2, entry.head)
		assert.Equal(t, overrideFor(t, "testapp", "v1"), entry.content)
	})

	t.Run("Unchanged head and content skips without cloning", func(t *testing.T) {
		cache := NewGitWriteBackCache()
		cache.set(precheckKey("testapp"), cached(sha1, overrideFor(t, "testapp", "v1")))
		state, lsRemoteCalls := precheckState(cache, sha1)

		for i := 0; i < 3; i++ {
			ai := precheckImages("testapp", "v1")
			gitMock := &gitmock.Client{}
			ai.GitClient = gitMock

			require.NoError(t, commitChangesLocked(ctx, ai, state, nil))
			gitMock.AssertNotCalled(t, "Init")
			gitMock.AssertNotCalled(t, "ShallowFetch", mock.Anything)
			gitMock.AssertNotCalled(t, "Push", mock.Anything, mock.Anything, mock.Anything)
		}
		assert.Equal(t, int32(1), lsRemoteCalls.Load(), "remote refs are listed once per iteration")
	})

	t.Run("Changed head clones and updates the cache", func(t *testing.T) {
		cache := NewGitWriteBackCache()
		cache.set(precheckKey("testapp"), cached(sha1, overrideFor(t, "testapp", "v1")))
		state, lsRemoteCalls := precheckState(cache, sha2)
		ai := precheckImages("testapp", "v1")
		// Someone else committed to the branch; the target file is unchanged,
		// so the full path finds nothing to write either.
		gitMock := cloneMock(t, precheckTarget, overrideFor(t, "testapp", "v1"), sha2)
		ai.GitClient = gitMock

		require.NoError(t, commitChangesLocked(ctx, ai, state, nil))

		assert.Equal(t, int32(1), lsRemoteCalls.Load())
		gitMock.AssertCalled(t, "ShallowFetch", precheckBranch)
		gitMock.AssertNotCalled(t, "Push", mock.Anything, mock.Anything, mock.Anything)
		entry, ok := cache.get(precheckKey("testapp"))
		require.True(t, ok)
		assert.Equal(t, sha2, entry.head)
		assert.Equal(t, overrideFor(t, "testapp", "v1"), entry.content)
	})

	t.Run("Changed content clones, pushes and records the pushed head", func(t *testing.T) {
		cache := NewGitWriteBackCache()
		cache.set(precheckKey("testapp"), cached(sha1, overrideFor(t, "testapp", "v0")))
		state, lsRemoteCalls := precheckState(cache, sha1)
		ai := precheckImages("testapp", "v1")
		gitMock := cloneMock(t, precheckTarget, overrideFor(t, "testapp", "v0"), sha3)
		gitMock.On("Commit", mock.Anything, mock.Anything).Return(nil)
		gitMock.On("Push", "origin", precheckBranch, true).Return(nil)
		ai.GitClient = gitMock

		require.NoError(t, commitChangesLocked(ctx, ai, state, nil))

		gitMock.AssertCalled(t, "ShallowFetch", precheckBranch)
		gitMock.AssertCalled(t, "Push", "origin", precheckBranch, true)
		entry, ok := cache.get(precheckKey("testapp"))
		require.True(t, ok)
		assert.Equal(t, sha3, entry.head)
		assert.Equal(t, overrideFor(t, "testapp", "v1"), entry.content)

		// The push is visible to later write-backs in the same iteration
		// without listing the remote refs again.
		ai = precheckImages("testapp", "v1")
		idle := &gitmock.Client{}
		ai.GitClient = idle
		require.NoError(t, commitChangesLocked(ctx, ai, state, nil))
		idle.AssertNotCalled(t, "ShallowFetch", mock.Anything)
		assert.Equal(t, int32(1), lsRemoteCalls.Load())
	})

	t.Run("Listing failure falls back to cloning", func(t *testing.T) {
		cache := NewGitWriteBackCache()
		cache.set(precheckKey("testapp"), cached(sha1, overrideFor(t, "testapp", "v1")))
		state := NewSyncIterationState().WithGitWriteBackCache(cache)
		state.remoteRefs.list = func(ctx context.Context, repo string, creds git.Creds) ([]*plumbing.Reference, error) {
			return nil, errors.New("connection reset")
		}
		ai := precheckImages("testapp", "v1")
		gitMock := cloneMock(t, precheckTarget, overrideFor(t, "testapp", "v1"), sha1)
		ai.GitClient = gitMock

		require.NoError(t, commitChangesLocked(ctx, ai, state, nil))
		gitMock.AssertCalled(t, "ShallowFetch", precheckBranch)
	})

	t.Run("Listing timeout falls back to cloning once per iteration", func(t *testing.T) {
		cache := NewGitWriteBackCache()
		cache.set(precheckKey("testapp"), cached(sha1, overrideFor(t, "testapp", "v1")))
		state := NewSyncIterationState().WithGitWriteBackCache(cache)
		release := make(chan struct{})
		defer close(release)
		var calls atomic.Int32
		state.remoteRefs.timeout = 50 * time.Millisecond
		state.remoteRefs.list = func(ctx context.Context, repo string, creds git.Creds) ([]*plumbing.Reference, error) {
			calls.Add(1)
			<-release
			return nil, nil
		}
		for i := 0; i < 2; i++ {
			ai := precheckImages("testapp", "v1")
			gitMock := cloneMock(t, precheckTarget, overrideFor(t, "testapp", "v1"), sha1)
			ai.GitClient = gitMock
			start := time.Now()
			require.NoError(t, commitChangesLocked(ctx, ai, state, nil))
			assert.Less(t, time.Since(start), 5*time.Second)
			gitMock.AssertCalled(t, "ShallowFetch", precheckBranch)
		}
		assert.Equal(t, int32(1), calls.Load(), "a failed listing is not retried in the same iteration")
	})

	t.Run("Cached state of another branch clones", func(t *testing.T) {
		cache := NewGitWriteBackCache()
		cache.set(precheckKey("testapp"), gitWriteBackEntry{ref: "image-updater/other", head: sha1, content: overrideFor(t, "testapp", "v1")})
		state, lsRemoteCalls := precheckState(cache, sha1)
		ai := precheckImages("testapp", "v1")
		gitMock := cloneMock(t, precheckTarget, overrideFor(t, "testapp", "v1"), sha1)
		ai.GitClient = gitMock

		require.NoError(t, commitChangesLocked(ctx, ai, state, nil))
		assert.Equal(t, int32(0), lsRemoteCalls.Load())
		gitMock.AssertCalled(t, "ShallowFetch", precheckBranch)
		entry, ok := cache.get(precheckKey("testapp"))
		require.True(t, ok)
		assert.Equal(t, precheckBranch, entry.ref)
	})

	t.Run("Applications sharing a branch", func(t *testing.T) {
		// Two applications write their own files to one branch.
		cache := NewGitWriteBackCache()
		cache.set(precheckKey("testapp"), cached(sha1, overrideFor(t, "testapp", "v0")))
		cache.set(precheckKey("testapp2"), cached(sha1, overrideFor(t, "testapp2", "v1")))
		state, lsRemoteCalls := precheckState(cache, sha1)

		// testapp has a new tag and pushes sha2.
		ai := precheckImages("testapp", "v1")
		gitA := cloneMock(t, precheckTarget, overrideFor(t, "testapp", "v0"), sha2)
		gitA.On("Commit", mock.Anything, mock.Anything).Return(nil)
		gitA.On("Push", "origin", precheckBranch, true).Return(nil)
		ai.GitClient = gitA
		require.NoError(t, commitChangesLocked(ctx, ai, state, nil))
		gitA.AssertCalled(t, "Push", "origin", precheckBranch, true)

		// testapp2 has nothing to write, but its cached head is now behind the
		// branch, so it clones once and records the new head.
		ai = precheckImages("testapp2", "v1")
		gitB := cloneMock(t, ai.Target, overrideFor(t, "testapp2", "v1"), sha2)
		ai.GitClient = gitB
		require.NoError(t, commitChangesLocked(ctx, ai, state, nil))
		gitB.AssertCalled(t, "ShallowFetch", precheckBranch)
		gitB.AssertNotCalled(t, "Push", mock.Anything, mock.Anything, mock.Anything)
		entry, ok := cache.get(precheckKey("testapp2"))
		require.True(t, ok)
		assert.Equal(t, sha2, entry.head)
		assert.Equal(t, int32(1), lsRemoteCalls.Load())

		// Next iteration: both skip without git.
		state, lsRemoteCalls = precheckState(cache, sha2)
		for _, name := range []string{"testapp", "testapp2"} {
			ai := precheckImages(name, "v1")
			idle := &gitmock.Client{}
			ai.GitClient = idle
			require.NoError(t, commitChangesLocked(ctx, ai, state, nil))
			idle.AssertNotCalled(t, "ShallowFetch", mock.Anything)
		}
		assert.Equal(t, int32(1), lsRemoteCalls.Load())
	})

	t.Run("Concurrent write-backs list the refs once", func(t *testing.T) {
		cache := NewGitWriteBackCache()
		const n = 8
		for i := 0; i < n; i++ {
			name := fmt.Sprintf("app%d", i)
			cache.set(precheckKey(name), cached(sha1, overrideFor(t, name, "v1")))
		}
		state, lsRemoteCalls := precheckState(cache, sha1)
		var wg sync.WaitGroup
		errs := make(chan error, n)
		for i := 0; i < n; i++ {
			ai := precheckImages(fmt.Sprintf("app%d", i), "v1")
			ai.GitClient = &gitmock.Client{}
			wg.Add(1)
			go func() {
				defer wg.Done()
				errs <- commitChangesLocked(ctx, ai, state, nil)
			}()
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			assert.NoError(t, err)
		}
		assert.Equal(t, int32(1), lsRemoteCalls.Load())
	})

	t.Run("Disabled without a cache", func(t *testing.T) {
		state := NewSyncIterationState()
		ai := precheckImages("testapp", "v1")
		gitMock := cloneMock(t, precheckTarget, overrideFor(t, "testapp", "v1"), sha1)
		ai.GitClient = gitMock

		require.NoError(t, commitChangesLocked(ctx, ai, state, nil))
		assert.Nil(t, ai.precheck)
		gitMock.AssertCalled(t, "ShallowFetch", precheckBranch)
		gitMock.AssertNotCalled(t, "CommitSHA")
	})

	t.Run("Not used for kustomization write-back", func(t *testing.T) {
		cache := NewGitWriteBackCache()
		state, lsRemoteCalls := precheckState(cache, sha1)
		ai := precheckImages("testapp", "v1")
		ai.KustomizeBase = "apps/testapp"
		gitMock, dir, cleanup := mockGit(t)
		defer cleanup()
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "apps/testapp"), 0700))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "apps/testapp/kustomization.yaml"), []byte("images:\n- name: example.com/foo\n  newTag: v0\n"), 0600))
		gitMock.On("Checkout", precheckBranch, false).Return(nil)
		gitMock.On("Commit", mock.Anything, mock.Anything).Return(nil)
		gitMock.On("Push", mock.Anything, mock.Anything, mock.Anything).Return(nil)
		ai.GitClient = gitMock

		require.NoError(t, commitChangesLocked(ctx, ai, state, nil))
		assert.Nil(t, ai.precheck)
		assert.Equal(t, int32(0), lsRemoteCalls.Load())
	})
}

func Test_RemoteRefsHead(t *testing.T) {
	ctx := context.Background()
	refs := newRemoteRefs()
	calls := 0
	refs.list = func(ctx context.Context, repo string, creds git.Creds) ([]*plumbing.Reference, error) {
		calls++
		return []*plumbing.Reference{
			plumbing.NewSymbolicReference(plumbing.HEAD, plumbing.NewBranchReferenceName("main")),
			plumbing.NewHashReference(plumbing.NewBranchReferenceName("main"), plumbing.NewHash(sha1)),
			plumbing.NewHashReference(plumbing.NewTagReferenceName("main-tag"), plumbing.NewHash(sha3)),
		}, nil
	}
	noCreds := func() (git.Creds, error) { return git.NopCreds{}, nil }

	head, err := refs.head(ctx, "repo", "HEAD", noCreds)
	require.NoError(t, err)
	assert.Equal(t, sha1, head, "symbolic HEAD resolves to its branch")

	head, err = refs.head(ctx, "repo", "main", noCreds)
	require.NoError(t, err)
	assert.Equal(t, sha1, head)

	_, err = refs.head(ctx, "repo", "main-tag", noCreds)
	assert.Error(t, err, "only branches are write-back refs")

	_, err = refs.head(ctx, "repo", "missing", noCreds)
	assert.Error(t, err)

	refs.setHead("repo", "main", sha2)
	head, err = refs.head(ctx, "repo", "HEAD", noCreds)
	require.NoError(t, err)
	assert.Equal(t, sha2, head, "a push to the default branch moves HEAD")

	refs.setHead("other", "main", sha2)
	head, err = refs.head(ctx, "other", "main", noCreds)
	require.NoError(t, err)
	assert.Equal(t, sha1, head, "a push before the listing is not recorded")
	assert.Equal(t, 2, calls, "one listing per repository")

	_, err = refs.head(ctx, "broken", "main", func() (git.Creds, error) { return nil, errors.New("no creds") })
	assert.Error(t, err)
}

func Test_WriteBackRef(t *testing.T) {
	ctx := context.Background()
	ai := precheckImages("testapp", "v1")
	app := &ai.Application
	assert.Equal(t, "branch", writeBackRef(ctx, app, &WriteBackConfig{GitBranch: "main", GitWriteBranch: "tpl"}, "branch"))
	assert.Equal(t, "main", writeBackRef(ctx, app, &WriteBackConfig{GitBranch: "main"}, ""))
	assert.Equal(t, "HEAD", writeBackRef(ctx, app, &WriteBackConfig{}, ""))
	app.Spec.Source.TargetRevision = ""
	assert.Equal(t, "HEAD", writeBackRef(ctx, app, &WriteBackConfig{}, ""))
	app.Spec.Source.TargetRevision = "release"
	assert.Equal(t, "release", writeBackRef(ctx, app, &WriteBackConfig{}, ""))
}

// Test_GitWriteBackPrecheck_LocalRemote runs the write-back with the real git
// client against a local bare repository across several iterations.
func Test_GitWriteBackPrecheck_LocalRemote(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}
	ctx := context.Background()
	tmp := t.TempDir()
	remote := filepath.Join(tmp, "remote.git")
	seed := filepath.Join(tmp, "seed")
	run := func(dir string, args ...string) string {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-c", "user.name=test", "-c", "user.email=test@example.com", "-c", "init.defaultBranch=main"}, args...)...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, string(out))
		return strings.TrimSpace(string(out))
	}
	run(tmp, "init", "--bare", remote)
	run(tmp, "clone", remote, seed)
	require.NoError(t, os.MkdirAll(filepath.Join(seed, "apps/testapp"), 0700))
	require.NoError(t, os.WriteFile(filepath.Join(seed, precheckTarget), overrideFor(t, "testapp", "v0"), 0600))
	run(seed, "add", ".")
	run(seed, "commit", "-m", "seed")
	run(seed, "push", "origin", "HEAD:main")
	remoteHead := func() string { return run(remote, "rev-parse", "refs/heads/"+precheckBranch) }

	repoURL := "file://" + remote
	cache := NewGitWriteBackCache()
	var fetches, lsRemotes atomic.Int32
	newState := func() *SyncIterationState {
		state := NewSyncIterationState().WithGitWriteBackCache(cache)
		state.remoteRefs.list = func(ctx context.Context, repo string, creds git.Creds) ([]*plumbing.Reference, error) {
			lsRemotes.Add(1)
			return listRemoteRefs(ctx, repo, creds)
		}
		return state
	}
	writeBack := func(state *SyncIterationState, tag string) {
		t.Helper()
		ai := precheckImages("testapp", tag)
		ai.Spec.Source.RepoURL = repoURL
		ai.GitRepo = repoURL
		ai.GitCommitUser = "argocd-image-updater"
		ai.GitCommitEmail = "noreply@example.com"
		root, err := os.MkdirTemp(tmp, "wb")
		require.NoError(t, err)
		ai.GitClient, err = git.NewClientExt(repoURL, root, git.NopCreds{}, false, false, "", git.WithEventHandlers(git.EventHandlers{
			OnFetch: func(repo string) func() { fetches.Add(1); return func() {} },
		}))
		require.NoError(t, err)
		require.NoError(t, commitChangesLocked(ctx, ai, state, nil))
	}

	// Iteration 1: nothing cached, the branch does not exist yet. The
	// write-back creates it from main.
	writeBack(newState(), "v1")
	assert.Equal(t, int32(2), fetches.Load(), "fetch of the missing branch, then of main")
	assert.Equal(t, int32(0), lsRemotes.Load())
	h1 := remoteHead()

	// Iteration 2: nothing to write. Only the remote refs are listed.
	fetches.Store(0)
	writeBack(newState(), "v1")
	assert.Equal(t, int32(0), fetches.Load())
	assert.Equal(t, int32(1), lsRemotes.Load())
	assert.Equal(t, h1, remoteHead())

	// Someone else commits another file to the branch.
	run(seed, "fetch", "origin", precheckBranch)
	run(seed, "checkout", "-B", precheckBranch, "FETCH_HEAD")
	require.NoError(t, os.WriteFile(filepath.Join(seed, "apps/testapp/other.yaml"), []byte("a: b\n"), 0600))
	run(seed, "add", ".")
	run(seed, "commit", "-m", "other")
	run(seed, "push", "origin", precheckBranch)
	h2 := remoteHead()

	// Iteration 3: the head moved, so the write-back clones once, finds
	// nothing to write and remembers the new head.
	fetches.Store(0)
	writeBack(newState(), "v1")
	assert.Equal(t, int32(1), fetches.Load())
	assert.Equal(t, h2, remoteHead())
	entry, ok := cache.get(gitWriteBackKey{repo: repoURL, target: precheckTarget})
	require.True(t, ok)
	assert.Equal(t, h2, entry.head)

	// Iteration 4: a new tag is written, and a second write-back in the same
	// iteration sees the pushed head without cloning or listing again.
	fetches.Store(0)
	lsRemotes.Store(0)
	state := newState()
	writeBack(state, "v2")
	assert.Equal(t, int32(1), fetches.Load())
	assert.NotEqual(t, h2, remoteHead())
	writeBack(state, "v2")
	assert.Equal(t, int32(1), fetches.Load())
	assert.Equal(t, int32(1), lsRemotes.Load())

	run(seed, "fetch", "origin", precheckBranch)
	content := run(seed, "show", "FETCH_HEAD:"+precheckTarget)
	assert.Equal(t, strings.TrimSpace(string(overrideFor(t, "testapp", "v2"))), content)

	// Iteration 5: the head recorded after the push is the real remote head,
	// so a fresh listing matches it and nothing is cloned.
	fetches.Store(0)
	lsRemotes.Store(0)
	writeBack(newState(), "v2")
	assert.Equal(t, int32(0), fetches.Load())
	assert.Equal(t, int32(1), lsRemotes.Load())
}
