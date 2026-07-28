package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/log"

	executil "github.com/argoproj/argo-cd/v3/util/exec"
	"github.com/argoproj/argo-cd/v3/util/rand"
	"github.com/sirupsen/logrus"
)

var (
	execTimeout      time.Duration
	execFatalTimeout time.Duration
)

// init mirrors the timeout initialisation from executil (vendor/github.com/argoproj/argo-cd/v3/util/exec/exec.go)
// so that both code paths honour the same ARGOCD_EXEC_TIMEOUT / ARGOCD_EXEC_FATAL_TIMEOUT env vars.
func init() {
	var err error
	execTimeout, err = time.ParseDuration(os.Getenv("ARGOCD_EXEC_TIMEOUT"))
	if err != nil {
		execTimeout = 90 * time.Second
	}
	execFatalTimeout, err = time.ParseDuration(os.Getenv("ARGOCD_EXEC_FATAL_TIMEOUT"))
	if err != nil {
		execFatalTimeout = 10 * time.Second
	}
}

// newCmdError constructs an executil.CmdError. executil.newCmdError is
// unexported, so we provide a local wrapper around the exported CmdError struct.
func newCmdError(args string, cause error, stderr string) *executil.CmdError {
	return &executil.CmdError{Args: args, Stderr: stderr, Cause: cause}
}

// RunWithExecRunOpts is a context-aware replacement for executil.RunWithExecRunOpts.
// It forwards timeout and redactor settings to RunCommandExt, which logs every
// git command with the structured fields carried in ctx.
func RunWithExecRunOpts(ctx context.Context, cmd *exec.Cmd, opts executil.ExecRunOpts) (string, error) {
	cmdOpts := executil.CmdOpts{Timeout: execTimeout, FatalTimeout: execFatalTimeout, Redactor: opts.Redactor, TimeoutBehavior: opts.TimeoutBehavior, SkipErrorLogging: opts.SkipErrorLogging, CaptureStderr: opts.CaptureStderr}
	return RunCommandExt(ctx, cmd, cmdOpts)
}

// RunCommandExt is a context-aware replacement for executil.RunCommandExt
// (vendor/github.com/argoproj/argo-cd/v3/util/exec/exec.go). The only
// difference from the original is that it accepts ctx and derives the logger
// from it via log.LoggerFromContext, so every command log line inherits the
// reconcile fields (application, controller, namespace, etc.) from the caller.
func RunCommandExt(ctx context.Context, cmd *exec.Cmd, opts executil.CmdOpts) (string, error) {
	baseLogger := log.LoggerFromContext(ctx)

	execId, err := rand.RandHex(5)
	if err != nil {
		return "", err
	}
	logCtx := baseLogger.WithFields(logrus.Fields{"execID": execId})

	redactor := executil.DefaultCmdOpts.Redactor
	if opts.Redactor != nil {
		redactor = opts.Redactor
	}

	// log in a way we can copy-and-paste into a terminal
	args := strings.Join(cmd.Args, " ")
	logCtx.WithFields(logrus.Fields{"dir": cmd.Dir}).Infof("%s", redactor(args))

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	err = cmd.Start()
	if err != nil {
		return "", err
	}

	done := make(chan error)
	go func() { done <- cmd.Wait() }()

	// Start timers for timeout
	timeout := executil.DefaultCmdOpts.Timeout
	fatalTimeout := executil.DefaultCmdOpts.FatalTimeout

	if opts.Timeout != time.Duration(0) {
		timeout = opts.Timeout
	}

	if opts.FatalTimeout != time.Duration(0) {
		fatalTimeout = opts.FatalTimeout
	}

	var timoutCh <-chan time.Time
	if timeout != 0 {
		timoutCh = time.NewTimer(timeout).C
	}

	var fatalTimeoutCh <-chan time.Time
	if fatalTimeout != 0 {
		fatalTimeoutCh = time.NewTimer(timeout + fatalTimeout).C
	}

	timeoutBehavior := executil.DefaultCmdOpts.TimeoutBehavior
	fatalTimeoutBehaviour := syscall.SIGKILL
	if opts.TimeoutBehavior.Signal != syscall.Signal(0) {
		timeoutBehavior = opts.TimeoutBehavior
	}

	select {
	// noinspection ALL
	case <-timoutCh:
		// send timeout signal
		_ = cmd.Process.Signal(timeoutBehavior.Signal)
		// wait on timeout signal and fallback to fatal timeout signal
		if timeoutBehavior.ShouldWait {
			select {
			case <-done:
			case <-fatalTimeoutCh:
				// upgrades to SIGKILL if cmd does not respect SIGTERM
				_ = cmd.Process.Signal(fatalTimeoutBehaviour)
				// now original cmd should exit immediately after SIGKILL
				<-done
				// return error with a marker indicating that cmd exited only after fatal SIGKILL
				output := stdout.String()
				if opts.CaptureStderr {
					output += stderr.String()
				}
				logCtx.WithFields(logrus.Fields{"duration": time.Since(start)}).Debugf("%s", redactor(output))
				err = newCmdError(redactor(args), fmt.Errorf("fatal timeout after %v", timeout+fatalTimeout), "")
				logCtx.Errorf("%s", err.Error())
				return strings.TrimSuffix(output, "\n"), err
			}
		}
		// either did not wait for timeout or cmd did respect SIGTERM
		output := stdout.String()
		if opts.CaptureStderr {
			output += stderr.String()
		}
		logCtx.WithFields(logrus.Fields{"duration": time.Since(start)}).Debugf("%s", redactor(output))
		err = newCmdError(redactor(args), fmt.Errorf("timeout after %v", timeout), "")
		logCtx.Errorf("%s", err.Error())
		return strings.TrimSuffix(output, "\n"), err
	case err := <-done:
		if err != nil {
			output := stdout.String()
			if opts.CaptureStderr {
				output += stderr.String()
			}
			logCtx.WithFields(logrus.Fields{"duration": time.Since(start)}).Debugf("%s", redactor(output))
			err := newCmdError(redactor(args), errors.New(redactor(err.Error())), strings.TrimSpace(redactor(stderr.String())))
			if !opts.SkipErrorLogging {
				logCtx.Errorf("%s", err.Error())
			}
			return strings.TrimSuffix(output, "\n"), err
		}
	}
	output := stdout.String()
	if opts.CaptureStderr {
		output += stderr.String()
	}
	logCtx.WithFields(logrus.Fields{"duration": time.Since(start)}).Debugf("%s", redactor(output))

	return strings.TrimSuffix(output, "\n"), nil
}
