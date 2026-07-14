package git

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkingTreeChanges(t *testing.T) {
	ctx := context.Background()
	dir, client := newWriterTestRepo(t)

	// Seed tracked files, including a path with spaces: the -z porcelain
	// format is what keeps such paths unquoted, so this pins that flag.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.yaml"), []byte("a: 1\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.yaml"), []byte("b: 1\n"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "sub dir"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "sub dir", "a b.yaml"), []byte("s: 1\n"), 0o644))
	require.NoError(t, runCmd(dir, "git", "add", "."))
	require.NoError(t, runCmd(dir, "git", "commit", "-m", "seed"))

	// Modify two (one with spaces in its path), add a new untracked file
	// inside a new directory, delete one.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.yaml"), []byte("a: 2\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "sub dir", "a b.yaml"), []byte("s: 2\n"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "sub"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "sub", "new.yaml"), []byte("n: 1\n"), 0o644))
	require.NoError(t, os.Remove(filepath.Join(dir, "b.yaml")))

	changes, err := client.WorkingTreeChanges(ctx)
	require.NoError(t, err)
	assert.ElementsMatch(t, []WorkingTreeChange{
		{Path: "a.yaml"},
		{Path: "sub dir/a b.yaml"},
		{Path: "sub/new.yaml"},
		{Path: "b.yaml", Deleted: true},
	}, changes)
}

// TestWorkingTreeChanges_StagedRename exercises the porcelain rename entry
// (`R  new\0old`), whose origin path arrives as a separate NUL field: the
// parser must consume it as the deletion and keep the new path as changed.
func TestWorkingTreeChanges_StagedRename(t *testing.T) {
	ctx := context.Background()
	dir, client := newWriterTestRepo(t)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.yaml"), []byte("a: 1\n"), 0o644))
	require.NoError(t, runCmd(dir, "git", "add", "."))
	require.NoError(t, runCmd(dir, "git", "commit", "-m", "seed"))

	require.NoError(t, runCmd(dir, "git", "mv", "a.yaml", "c.yaml"))

	changes, err := client.WorkingTreeChanges(ctx)
	require.NoError(t, err)
	assert.ElementsMatch(t, []WorkingTreeChange{
		{Path: "c.yaml"},
		{Path: "a.yaml", Deleted: true},
	}, changes)
}

func TestWorkingTreeChanges_CleanTree(t *testing.T) {
	ctx := context.Background()
	_, client := newWriterTestRepo(t)

	changes, err := client.WorkingTreeChanges(ctx)
	require.NoError(t, err)
	assert.Empty(t, changes)
}
