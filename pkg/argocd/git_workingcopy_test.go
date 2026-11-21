package argocd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/argoproj-labs/argocd-image-updater/ext/git"
)

const workingCopyFiles = 60

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %s: %s", strings.Join(args, " "), out)
	return strings.TrimSpace(string(out))
}

// workingCopyRemote creates a bare repository with one commit on main holding
// values.yaml and workingCopyFiles other files, and a clone to push from.
func workingCopyRemote(t *testing.T) (remoteURL string, seed string) {
	t.Helper()
	root := t.TempDir()
	t.Setenv("GIT_AUTHOR_NAME", "test")
	t.Setenv("GIT_AUTHOR_EMAIL", "test@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "test")
	t.Setenv("GIT_COMMITTER_EMAIL", "test@example.com")
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(root, "gitconfig"))
	remote := filepath.Join(root, "remote.git")
	runGit(t, root, "init", "-q", "--bare", "-b", "main", remote)
	seed = filepath.Join(root, "seed")
	runGit(t, root, "clone", "-q", remote, seed)
	runGit(t, seed, "checkout", "-q", "-b", "main")
	require.NoError(t, os.WriteFile(filepath.Join(seed, "values.yaml"), []byte("image:\n  tag: v1\n"), 0o600))
	for i := 0; i < workingCopyFiles; i++ {
		content := fmt.Sprintf("tenant: %d\n%s", i, strings.Repeat(strconv.Itoa(i), 512))
		require.NoError(t, os.WriteFile(filepath.Join(seed, fmt.Sprintf("tenant-%02d.yaml", i)), []byte(content), 0o600))
	}
	runGit(t, seed, "add", "-A")
	runGit(t, seed, "commit", "-q", "-m", "seed")
	runGit(t, seed, "push", "-q", "origin", "main")
	return "file://" + remote, seed
}

func pushExternalChange(t *testing.T, seed, name string) string {
	t.Helper()
	runGit(t, seed, "pull", "-q", "--ff-only", "origin", "main")
	require.NoError(t, os.WriteFile(filepath.Join(seed, name), []byte(name+"\n"), 0o600))
	runGit(t, seed, "add", name)
	runGit(t, seed, "commit", "-q", "-m", "external "+name)
	runGit(t, seed, "push", "-q", "origin", "main")
	return runGit(t, seed, "rev-parse", "HEAD")
}

func workingCopyApp(remoteURL, dir string) *ApplicationImages {
	return &ApplicationImages{
		Application: v1alpha1.Application{ObjectMeta: v1.ObjectMeta{Name: "testapp"}},
		WriteBackConfig: &WriteBackConfig{
			Method:            WriteBackGit,
			GitBranch:         "main",
			GitRepo:           remoteURL,
			GitCommitUser:     "image-updater",
			GitCommitEmail:    "image-updater@example.com",
			GitWorkingCopyDir: dir,
			GetCreds: func(app *v1alpha1.Application) (git.Creds, error) {
				return git.NopCreds{}, nil
			},
		},
	}
}

// unchangedTarget is the no-op cycle of #1939: the target already holds the tag.
func unchangedTarget(ctx context.Context, ai *ApplicationImages, gitC git.Client) (error, bool) {
	return nil, true
}

func setTag(tag string) changeWriter {
	return func(ctx context.Context, ai *ApplicationImages, gitC git.Client) (error, bool) {
		return os.WriteFile(filepath.Join(gitC.Root(), "values.yaml"), []byte("image:\n  tag: "+tag+"\n"), 0o600), false
	}
}

func writeBack(app *ApplicationImages, write changeWriter) error {
	_, err := commitChangesGit(context.Background(), app, nil, write)
	return err
}

// onlyWorkingCopy returns the single working copy kept under dir.
func onlyWorkingCopy(t *testing.T, dir string) string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 1, "one working copy per repository is kept under %s", dir)
	return filepath.Join(dir, entries[0].Name())
}

// storedObjects counts every object in the working copy, loose and packed, so
// a fetch that re-sends objects the copy already holds raises it.
func storedObjects(t *testing.T, workingCopy string) int {
	t.Helper()
	total := 0
	for _, line := range strings.Split(runGit(t, workingCopy, "count-objects", "-v"), "\n") {
		key, value, _ := strings.Cut(line, ": ")
		if key == "count" || key == "in-pack" {
			n, err := strconv.Atoi(value)
			require.NoError(t, err)
			total += n
		}
	}
	return total
}

func remoteFile(t *testing.T, seed, name string) string {
	t.Helper()
	runGit(t, seed, "fetch", "-q", "origin", "main")
	return runGit(t, seed, "show", "origin/main:"+name)
}

func Test_commitChangesGit_WorkingCopyKeptAcrossNoOpCycles(t *testing.T) {
	remoteURL, _ := workingCopyRemote(t)
	dir := t.TempDir()
	app := workingCopyApp(remoteURL, dir)

	require.NoError(t, writeBack(app, unchangedTarget))
	workingCopy := onlyWorkingCopy(t, dir)
	head := runGit(t, workingCopy, "rev-parse", "HEAD")
	before := storedObjects(t, workingCopy)
	require.GreaterOrEqual(t, before, workingCopyFiles)

	require.NoError(t, writeBack(app, unchangedTarget))
	assert.Equal(t, workingCopy, onlyWorkingCopy(t, dir))
	assert.Equal(t, head, runGit(t, workingCopy, "rev-parse", "HEAD"))
	assert.Equal(t, before, storedObjects(t, workingCopy), "a cycle with nothing new on the remote fetched objects again")
}

func Test_commitChangesGit_WorkingCopyCommitsOnTheRemoteHead(t *testing.T) {
	remoteURL, seed := workingCopyRemote(t)
	dir := t.TempDir()
	app := workingCopyApp(remoteURL, dir)

	require.NoError(t, writeBack(app, unchangedTarget))
	workingCopy := onlyWorkingCopy(t, dir)
	before := storedObjects(t, workingCopy)

	external := pushExternalChange(t, seed, "other.yaml")
	require.NoError(t, writeBack(app, setTag("v2")))
	assert.Equal(t, "image:\n  tag: v2", remoteFile(t, seed, "values.yaml"))
	assert.Equal(t, external, runGit(t, seed, "rev-parse", "origin/main^"))
	// The external commit, its tree and blob, then this cycle's commit, tree and blob.
	assert.LessOrEqual(t, storedObjects(t, workingCopy)-before, 6, "the fetch re-sent objects the working copy holds")

	// The copy's own earlier push must not become the base of the next commit.
	external = pushExternalChange(t, seed, "third.yaml")
	require.NoError(t, writeBack(app, setTag("v3")))
	assert.Equal(t, "image:\n  tag: v3", remoteFile(t, seed, "values.yaml"))
	assert.Equal(t, external, runGit(t, seed, "rev-parse", "origin/main^"))
}

func Test_commitChangesGit_WorkingCopyDiscardsAFailedCyclesEdits(t *testing.T) {
	remoteURL, _ := workingCopyRemote(t)
	dir := t.TempDir()
	app := workingCopyApp(remoteURL, dir)

	failing := func(ctx context.Context, ai *ApplicationImages, gitC git.Client) (error, bool) {
		require.NoError(t, os.WriteFile(filepath.Join(gitC.Root(), "values.yaml"), []byte("half written"), 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(gitC.Root(), "stray.yaml"), []byte("stray"), 0o600))
		return fmt.Errorf("write failed"), false
	}
	require.Error(t, writeBack(app, failing))

	var seen string
	var stray error
	reader := func(ctx context.Context, ai *ApplicationImages, gitC git.Client) (error, bool) {
		data, err := os.ReadFile(filepath.Join(gitC.Root(), "values.yaml"))
		seen = string(data)
		_, stray = os.Stat(filepath.Join(gitC.Root(), "stray.yaml"))
		return err, true
	}
	require.NoError(t, writeBack(app, reader))
	assert.Equal(t, "image:\n  tag: v1\n", seen)
	assert.True(t, os.IsNotExist(stray))
}

func Test_commitChangesGit_WriteBranchKeepsNoWorkingCopy(t *testing.T) {
	remoteURL, seed := workingCopyRemote(t)
	dir := t.TempDir()
	app := workingCopyApp(remoteURL, dir)
	app.WriteBackConfig.GitWriteBranch = "image-updater"

	require.NoError(t, writeBack(app, setTag("v2")))
	runGit(t, seed, "fetch", "-q", "origin", "image-updater")
	assert.Equal(t, "image:\n  tag: v2", runGit(t, seed, "show", "FETCH_HEAD:values.yaml"))
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, entries)
}
