//go:build windows

package git

import (
	"os/exec"
	"syscall"

	executil "github.com/argoproj/argo-cd/v3/util/exec"
)

// setSysProcAttr is a no-op on Windows because Setpgid is not supported.
func setSysProcAttr(_ *exec.Cmd) {}

// killProcessGroup is a no-op on Windows because syscall.Kill with negative
// PIDs (process groups) is not supported.
func killProcessGroup(_ *exec.Cmd) {}

// newTimeoutBehavior returns the platform-specific timeout behavior.
// On Windows, SIGKILL is the only reliable signal, and we don't wait.
func newTimeoutBehavior() executil.TimeoutBehavior {
	return executil.TimeoutBehavior{
		Signal:     syscall.SIGKILL,
		ShouldWait: false,
	}
}
