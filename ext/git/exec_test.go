package git

import (
	"context"
	"io"
	"os/exec"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/log"
	executil "github.com/argoproj/argo-cd/v3/util/exec"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// logHook captures logrus entries during tests. It mirrors the minimal API of
// github.com/sirupsen/logrus/hooks/test without requiring that package to be
// vendored.
type logHook struct {
	mu      sync.RWMutex
	Entries []logrus.Entry
}

func (h *logHook) Fire(e *logrus.Entry) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.Entries = append(h.Entries, *e)
	return nil
}

func (h *logHook) Levels() []logrus.Level { return logrus.AllLevels }

// newTestContext returns a context whose logger writes to a hook so that
// log entries can be inspected in tests. The logger discards actual output.
func newTestContext(t *testing.T) (context.Context, *logHook) {
	t.Helper()
	logger := logrus.New()
	logger.SetOutput(io.Discard)
	logger.SetLevel(logrus.TraceLevel)
	hook := &logHook{}
	logger.AddHook(hook)
	entry := logrus.NewEntry(logger)
	ctx := log.ContextWithLogger(context.Background(), entry)
	return ctx, hook
}

// newTestContextWithFields adds extra structured fields to the logger so we can
// verify they propagate into command log entries.
func newTestContextWithFields(t *testing.T, fields logrus.Fields) (context.Context, *logHook) {
	t.Helper()
	ctx, hook := newTestContext(t)
	entry := log.LoggerFromContext(ctx).WithFields(fields)
	return log.ContextWithLogger(ctx, entry), hook
}

// --- RunCommandExt tests -------------------------------------------------

func TestRunCommandExt_Success(t *testing.T) {
	ctx, _ := newTestContext(t)
	cmd := exec.CommandContext(ctx, "echo", "hello")
	out, err := RunCommandExt(ctx, cmd, executil.CmdOpts{})
	require.NoError(t, err)
	assert.Equal(t, "hello", out)
}

// Port of ArgoCD's TestRunInDir: cmd.Dir is respected.
func TestRunCommandExt_RunInDir(t *testing.T) {
	ctx, _ := newTestContext(t)
	cmd := exec.CommandContext(ctx, "pwd")
	cmd.Dir = "/"
	out, err := RunCommandExt(ctx, cmd, executil.CmdOpts{})
	require.NoError(t, err)
	assert.Equal(t, "/", out)
}

func TestRunCommandExt_TrimmedOutput(t *testing.T) {
	ctx, _ := newTestContext(t)
	cmd := exec.CommandContext(ctx, "printf", "hello world")
	out, err := RunCommandExt(ctx, cmd, executil.CmdOpts{})
	require.NoError(t, err)
	assert.Equal(t, "hello world", out)
}

// TestRunCommandExt_LogFields verifies that every command log entry carries
// execID and dir (info) / duration (debug), and that fields stored on the
// context logger propagate into the entries — the core reason this file exists.
func TestRunCommandExt_LogFields(t *testing.T) {
	ctx, hook := newTestContextWithFields(t, logrus.Fields{
		"application": "test-app",
		"namespace":   "test-ns",
	})

	cmd := exec.CommandContext(ctx, "echo", "hi")
	_, err := RunCommandExt(ctx, cmd, executil.CmdOpts{})
	require.NoError(t, err)
	require.Len(t, hook.Entries, 2)

	info := hook.Entries[0]
	assert.Equal(t, logrus.InfoLevel, info.Level)
	assert.Contains(t, info.Data, "execID")
	assert.Contains(t, info.Data, "dir")
	assert.Equal(t, "test-app", info.Data["application"])
	assert.Equal(t, "test-ns", info.Data["namespace"])

	debug := hook.Entries[1]
	assert.Equal(t, logrus.DebugLevel, debug.Level)
	assert.Contains(t, debug.Data, "execID")
	assert.Contains(t, debug.Data, "duration")
	assert.Equal(t, "test-app", debug.Data["application"])
}

func TestRunCommandExt_Redactor(t *testing.T) {
	ctx, hook := newTestContext(t)
	redactor := executil.Redact([]string{"secret"})

	cmd := exec.CommandContext(ctx, "echo", "secret")
	out, err := RunCommandExt(ctx, cmd, executil.CmdOpts{Redactor: redactor})
	require.NoError(t, err)
	assert.Equal(t, "secret", out) // stdout is not redacted
	require.NotEmpty(t, hook.Entries)
	assert.NotContains(t, hook.Entries[0].Message, "secret")
	assert.Contains(t, hook.Entries[0].Message, "******")
}

func TestRunCommandExt_ExitError(t *testing.T) {
	ctx, hook := newTestContext(t)
	cmd := exec.CommandContext(ctx, "sh", "-c", "echo hi && echo boom >&2 && exit 1")
	out, err := RunCommandExt(ctx, cmd, executil.CmdOpts{})
	assert.Equal(t, "hi", out)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exit status 1")
	assert.Contains(t, err.Error(), "boom")

	// info (command), debug (stdout), error (failure)
	require.Len(t, hook.Entries, 3)
	assert.Equal(t, logrus.ErrorLevel, hook.Entries[2].Level)
}

func TestRunCommandExt_SkipErrorLogging(t *testing.T) {
	ctx, hook := newTestContext(t)
	cmd := exec.CommandContext(ctx, "sh", "-c", "exit 1")
	_, err := RunCommandExt(ctx, cmd, executil.CmdOpts{SkipErrorLogging: true})
	require.Error(t, err)
	for _, e := range hook.Entries {
		assert.NotEqual(t, logrus.ErrorLevel, e.Level, "error must not be logged when SkipErrorLogging is set")
	}
}

func TestRunCommandExt_CaptureStderr(t *testing.T) {
	ctx, _ := newTestContext(t)
	cmd := exec.CommandContext(ctx, "sh", "-c", "echo hello && echo world >&2")
	out, err := RunCommandExt(ctx, cmd, executil.CmdOpts{CaptureStderr: true})
	require.NoError(t, err)
	assert.Equal(t, "hello\nworld", out)
}

// TestRunCommandExt_Timeout tests that a slow command is terminated and a
// timeout error is returned. sleep is used directly (no sh wrapper) so it has
// no child processes that could inherit stdout and delay cmd.Wait().
func TestRunCommandExt_Timeout(t *testing.T) {
	ctx, _ := newTestContext(t)
	cmd := exec.CommandContext(ctx, "sleep", "10")
	_, err := RunCommandExt(ctx, cmd, executil.CmdOpts{
		Timeout: 200 * time.Millisecond,
		TimeoutBehavior: executil.TimeoutBehavior{
			Signal:     syscall.SIGTERM,
			ShouldWait: true,
		},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "timeout after 200ms")
}

// TestRunCommandExt_FatalTimeout tests that a process ignoring SIGTERM is
// killed by SIGKILL after the fatal timeout. The busy-loop (no subprocesses)
// avoids stdout-pipe inheritance so cmd.Wait() returns promptly after SIGKILL.
func TestRunCommandExt_FatalTimeout(t *testing.T) {
	ctx, _ := newTestContext(t)
	cmd := exec.CommandContext(ctx, "sh", "-c", "trap '' TERM; while :; do :; done")
	_, err := RunCommandExt(ctx, cmd, executil.CmdOpts{
		Timeout:      200 * time.Millisecond,
		FatalTimeout: 100 * time.Millisecond,
		TimeoutBehavior: executil.TimeoutBehavior{
			Signal:     syscall.SIGTERM,
			ShouldWait: true,
		},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fatal timeout after 300ms")
}

// --- RunWithExecRunOpts tests --------------------------------------------

func TestRunWithExecRunOpts_Success(t *testing.T) {
	ctx, _ := newTestContext(t)
	cmd := exec.CommandContext(ctx, "echo", "hello")
	out, err := RunWithExecRunOpts(ctx, cmd, executil.ExecRunOpts{})
	require.NoError(t, err)
	assert.Equal(t, "hello", out)
}

func TestRunWithExecRunOpts_CaptureStderr(t *testing.T) {
	ctx, _ := newTestContext(t)
	cmd := exec.CommandContext(ctx, "sh", "-c", "echo hello && echo world >&2")
	out, err := RunWithExecRunOpts(ctx, cmd, executil.ExecRunOpts{CaptureStderr: true})
	require.NoError(t, err)
	assert.Equal(t, "hello\nworld", out)
}
