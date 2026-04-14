package git

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newWriterTestRepo initialises a git repository suitable for writer tests:
// user identity is configured in local config and an initial empty commit is
// created so that subsequent operations have a valid HEAD.
func newWriterTestRepo(t *testing.T) (dir string, client Client) {
	t.Helper()
	dir = t.TempDir()
	require.NoError(t, runCmd(dir, "git", "init", "-b", "master"))
	require.NoError(t, runCmd(dir, "git", "config", "user.email", "test@example.com"))
	require.NoError(t, runCmd(dir, "git", "config", "user.name", "Test User"))
	require.NoError(t, runCmd(dir, "git", "commit", "--allow-empty", "-m", "initial"))
	var err error
	client, err = NewClientExt("", dir, NopCreds{}, false, false, "")
	require.NoError(t, err)
	return dir, client
}

// ---------------------------------------------------------------------------
// Config
// ---------------------------------------------------------------------------

func TestConfig(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	require.NoError(t, runCmd(dir, "git", "init"))

	client, err := NewClientExt("", dir, NopCreds{}, false, false, "")
	require.NoError(t, err)

	err = client.Config(ctx, "Jane Doe", "jane@example.com")
	assert.NoError(t, err)

	name, err := runCmdOut(dir, "git", "config", "user.name")
	assert.NoError(t, err)
	assert.Equal(t, "Jane Doe", name)

	email, err := runCmdOut(dir, "git", "config", "user.email")
	assert.NoError(t, err)
	assert.Equal(t, "jane@example.com", email)
}

// ---------------------------------------------------------------------------
// Add
// ---------------------------------------------------------------------------

func TestAdd(t *testing.T) {
	ctx := context.Background()
	dir, client := newWriterTestRepo(t)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("hello"), 0644))

	err := client.Add(ctx, "hello.txt")
	assert.NoError(t, err)

	// File must appear as staged ("A ") in short status.
	status, err := runCmdOut(dir, "git", "status", "--short")
	assert.NoError(t, err)
	assert.Contains(t, status, "A  hello.txt")
}

func TestAdd_NonExistentFile(t *testing.T) {
	ctx := context.Background()
	_, client := newWriterTestRepo(t)

	err := client.Add(ctx, "does-not-exist.txt")
	assert.Error(t, err)
}

// ---------------------------------------------------------------------------
// Commit
// ---------------------------------------------------------------------------

func TestCommit(t *testing.T) {
	ctx := context.Background()

	// stageFile is a helper that writes a file and stages it.
	stageFile := func(t *testing.T, dir, name, content string) {
		t.Helper()
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0644))
		require.NoError(t, runCmd(dir, "git", "add", name))
	}

	t.Run("MessageText", func(t *testing.T) {
		dir, client := newWriterTestRepo(t)
		stageFile(t, dir, "f.txt", "content")

		err := client.Commit(ctx, "*", &CommitOptions{CommitMessageText: "custom message"})
		assert.NoError(t, err)

		subject, err := runCmdOut(dir, "git", "log", "-1", "--format=%s")
		assert.NoError(t, err)
		assert.Equal(t, "custom message", subject)
	})

	t.Run("MessageFile", func(t *testing.T) {
		dir, client := newWriterTestRepo(t)
		msgFile := filepath.Join(dir, "commit-msg.txt")
		require.NoError(t, os.WriteFile(msgFile, []byte("message from file"), 0644))
		stageFile(t, dir, "f.txt", "content")

		err := client.Commit(ctx, "*", &CommitOptions{CommitMessagePath: msgFile})
		assert.NoError(t, err)

		subject, err := runCmdOut(dir, "git", "log", "-1", "--format=%s")
		assert.NoError(t, err)
		assert.Equal(t, "message from file", subject)
	})

	t.Run("DefaultMessage", func(t *testing.T) {
		dir, client := newWriterTestRepo(t)
		stageFile(t, dir, "f.txt", "content")

		err := client.Commit(ctx, "*", &CommitOptions{})
		assert.NoError(t, err)

		subject, err := runCmdOut(dir, "git", "log", "-1", "--format=%s")
		assert.NoError(t, err)
		assert.Equal(t, "Update parameters", subject)
	})

	t.Run("EmptyPathSpecCommitsAll", func(t *testing.T) {
		dir, client := newWriterTestRepo(t)
		// Modify an already-tracked file (git commit -a picks up tracked modifications).
		stageFile(t, dir, "tracked.txt", "original")
		require.NoError(t, runCmd(dir, "git", "commit", "-m", "add tracked file"))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "tracked.txt"), []byte("modified"), 0644))

		err := client.Commit(ctx, "", &CommitOptions{CommitMessageText: "all changes"})
		assert.NoError(t, err)

		content, err := runCmdOut(dir, "git", "show", "HEAD:tracked.txt")
		assert.NoError(t, err)
		assert.Equal(t, "modified", content)
	})

	t.Run("WildcardPathSpecCommitsAll", func(t *testing.T) {
		dir, client := newWriterTestRepo(t)
		stageFile(t, dir, "tracked.txt", "original")
		require.NoError(t, runCmd(dir, "git", "commit", "-m", "add tracked file"))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "tracked.txt"), []byte("modified again"), 0644))

		err := client.Commit(ctx, "*", &CommitOptions{CommitMessageText: "wildcard"})
		assert.NoError(t, err)

		content, err := runCmdOut(dir, "git", "show", "HEAD:tracked.txt")
		assert.NoError(t, err)
		assert.Equal(t, "modified again", content)
	})

	t.Run("SignOff", func(t *testing.T) {
		dir, client := newWriterTestRepo(t)
		stageFile(t, dir, "f.txt", "content")

		err := client.Commit(ctx, "*", &CommitOptions{
			CommitMessageText: "signed off",
			SignOff:           true,
		})
		assert.NoError(t, err)

		body, err := runCmdOut(dir, "git", "log", "-1", "--format=%B")
		assert.NoError(t, err)
		assert.Contains(t, body, "Signed-off-by:")
	})

	t.Run("SSHSigning", func(t *testing.T) {
		// Reuse setupRepoWithSignedCommit which already configures SSH signing
		// in the local repo config (gpg.format, user.signingkey, allowed signers).
		repoPath, _, _ := setupRepoWithSignedCommit(t)

		client, err := NewClientExt("", repoPath, NopCreds{}, false, false, "")
		require.NoError(t, err)

		// Retrieve the signing key path that setupRepoWithSignedCommit stored
		// in local config so we can pass it to CommitOptions.
		signingKey, err := runCmdOut(repoPath, "git", "config", "user.signingkey")
		require.NoError(t, err)

		// Stage a new file.
		require.NoError(t, os.WriteFile(filepath.Join(repoPath, "signed.txt"), []byte("data"), 0644))
		require.NoError(t, runCmd(repoPath, "git", "add", "signed.txt"))

		err = client.Commit(ctx, "*", &CommitOptions{
			CommitMessageText: "ssh signed commit",
			SigningKey:        signingKey,
			SigningMethod:     "ssh",
		})
		assert.NoError(t, err)

		// git log --format=%GK prints the signing key fingerprint for signed commits.
		keyFP, err := runCmdOut(repoPath, "git", "log", "-1", "--format=%GK")
		assert.NoError(t, err)
		assert.NotEmpty(t, keyFP, "commit should carry a signature")
	})
}

// ---------------------------------------------------------------------------
// Branch
// ---------------------------------------------------------------------------

func TestBranch(t *testing.T) {
	ctx := context.Background()

	t.Run("FromCurrentHead", func(t *testing.T) {
		dir, client := newWriterTestRepo(t)

		err := client.Branch(ctx, "", "feature")
		assert.NoError(t, err)

		branches, err := runCmdOut(dir, "git", "branch")
		assert.NoError(t, err)
		assert.Contains(t, branches, "feature")
	})

	t.Run("FromSourceBranch", func(t *testing.T) {
		dir, client := newWriterTestRepo(t)

		// Create a diverging branch "develop" with its own commit.
		require.NoError(t, runCmd(dir, "git", "checkout", "-b", "develop"))
		require.NoError(t, runCmd(dir, "git", "commit", "--allow-empty", "-m", "develop commit"))
		require.NoError(t, runCmd(dir, "git", "checkout", "master"))

		err := client.Branch(ctx, "develop", "feature-from-develop")
		assert.NoError(t, err)

		developSHA, err := runCmdOut(dir, "git", "rev-parse", "develop")
		require.NoError(t, err)
		featureSHA, err := runCmdOut(dir, "git", "rev-parse", "feature-from-develop")
		require.NoError(t, err)
		assert.Equal(t, developSHA, featureSHA)
	})

	t.Run("ErrorOnDuplicateBranch", func(t *testing.T) {
		_, client := newWriterTestRepo(t)

		err := client.Branch(ctx, "", "master")
		assert.Error(t, err)
	})

	t.Run("ErrorOnMissingSourceBranch", func(t *testing.T) {
		_, client := newWriterTestRepo(t)

		err := client.Branch(ctx, "nonexistent", "new-branch")
		assert.Error(t, err)
	})
}

// ---------------------------------------------------------------------------
// Push
// ---------------------------------------------------------------------------

func TestPush(t *testing.T) {
	ctx := context.Background()

	// setupPushEnv creates a working directory with one commit and a bare remote.
	setupPushEnv := func(t *testing.T) (workDir, bareDir string, client Client) {
		t.Helper()
		workDir = t.TempDir()
		bareDir = t.TempDir()

		require.NoError(t, runCmd(workDir, "git", "init", "-b", "master"))
		require.NoError(t, runCmd(workDir, "git", "config", "user.email", "test@example.com"))
		require.NoError(t, runCmd(workDir, "git", "config", "user.name", "Test User"))
		require.NoError(t, runCmd(workDir, "git", "commit", "--allow-empty", "-m", "initial"))
		require.NoError(t, runCmd(bareDir, "git", "init", "--bare"))
		require.NoError(t, runCmd(workDir, "git", "remote", "add", "origin", bareDir))

		var err error
		client, err = NewClientExt(fmt.Sprintf("file://%s", bareDir), workDir, NopCreds{}, false, false, "")
		require.NoError(t, err)
		return workDir, bareDir, client
	}

	t.Run("Normal", func(t *testing.T) {
		_, bareDir, client := setupPushEnv(t)

		err := client.Push(ctx, "origin", "master", false)
		assert.NoError(t, err)

		sha, err := runCmdOut(bareDir, "git", "rev-parse", "master")
		assert.NoError(t, err)
		assert.NotEmpty(t, sha)
	})

	t.Run("Force", func(t *testing.T) {
		workDir, bareDir, client := setupPushEnv(t)

		// Push once to establish history in the remote.
		require.NoError(t, runCmd(workDir, "git", "push", "origin", "master"))

		// Rewrite the last commit so local and remote diverge.
		require.NoError(t, runCmd(workDir, "git", "commit", "--allow-empty", "--amend", "-m", "amended"))

		err := client.Push(ctx, "origin", "master", true)
		assert.NoError(t, err)

		remoteMsg, err := runCmdOut(bareDir, "git", "log", "-1", "--format=%s")
		assert.NoError(t, err)
		assert.Equal(t, "amended", remoteMsg)
	})

	t.Run("ErrorOnNonFastForward", func(t *testing.T) {
		workDir, _, client := setupPushEnv(t)

		// Push once, then amend without force – must fail.
		require.NoError(t, runCmd(workDir, "git", "push", "origin", "master"))
		require.NoError(t, runCmd(workDir, "git", "commit", "--allow-empty", "--amend", "-m", "diverged"))

		err := client.Push(ctx, "origin", "master", false)
		assert.Error(t, err)
	})
}

// ---------------------------------------------------------------------------
// SymRefToBranch
// ---------------------------------------------------------------------------

func TestSymRefToBranch(t *testing.T) {
	ctx := context.Background()

	// Build a bare remote whose HEAD points to "main", then set up a clone.
	workDir := t.TempDir()
	bareDir := t.TempDir()

	require.NoError(t, runCmd(workDir, "git", "init", "-b", "main"))
	require.NoError(t, runCmd(workDir, "git", "config", "user.email", "test@example.com"))
	require.NoError(t, runCmd(workDir, "git", "config", "user.name", "Test User"))
	require.NoError(t, runCmd(workDir, "git", "commit", "--allow-empty", "-m", "initial"))
	require.NoError(t, runCmd(bareDir, "git", "init", "--bare"))
	require.NoError(t, runCmd(workDir, "git", "remote", "add", "origin", bareDir))
	require.NoError(t, runCmd(workDir, "git", "push", "origin", "main"))
	require.NoError(t, runCmd(bareDir, "git", "symbolic-ref", "HEAD", "refs/heads/main"))

	cloneDir := t.TempDir()
	client, err := NewClientExt(fmt.Sprintf("file://%s", bareDir), cloneDir, NopCreds{}, false, false, "")
	require.NoError(t, err)

	require.NoError(t, client.Init(ctx))
	require.NoError(t, client.Fetch(ctx, ""))

	branch, err := client.SymRefToBranch(ctx, "HEAD")
	assert.NoError(t, err)
	assert.Equal(t, "main", branch)
}

func TestSymRefToBranch_NoRemote(t *testing.T) {
	ctx := context.Background()

	// A repo with no remote configured must return an error.
	dir, client := newWriterTestRepo(t)
	_ = dir

	_, err := client.SymRefToBranch(ctx, "HEAD")
	assert.Error(t, err)
}
