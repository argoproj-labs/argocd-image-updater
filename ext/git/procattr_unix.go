//go:build !windows

package git

import (
	"os/exec"
	"syscall"

	executil "github.com/argoproj/argo-cd/v3/util/exec"
)

// setSysProcAttr configures the command to run in its own process group so that
// child processes (e.g. git-remote-https) can be cleaned up when the parent is
// killed on timeout or context cancellation.
func setSysProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// killProcessGroup kills the entire process group to clean up any orphaned
// child processes such as git-remote-https. The negative PID denotes the
// process group, which was set via Setpgid above.
func killProcessGroup(cmd *exec.Cmd) {
	if cmd.Process != nil {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}

// newTimeoutBehavior returns the platform-specific timeout behavior.
func newTimeoutBehavior() executil.TimeoutBehavior {
	return executil.TimeoutBehavior{
		Signal:     syscall.SIGTERM,
		ShouldWait: true,
	}
}
