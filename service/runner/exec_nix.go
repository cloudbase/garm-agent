//go:build !windows
// +build !windows

package runner

import (
	"fmt"
	"log/slog"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

// platformExecutor handles Unix/Linux-specific process management using process groups
type platformExecutor struct {
	cmd  *exec.Cmd
	once sync.Once

	// wait() is the single owner of cmd.Wait(). waitOnce guards it, waitErr
	// caches its result, and exited is closed once the process has been reaped
	// so cleanup() can wait on the real exit instead of probing the PID.
	waitOnce sync.Once
	waitErr  error
	exited   chan struct{}
}

// setupCommand configures the command for Unix/Linux with process group support
func setupCommand(cmd *exec.Cmd) (*platformExecutor, error) {
	// Set up the command to create a new process group
	// This allows us to send signals to all child processes
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	// Create a new process group with the process as the leader
	cmd.SysProcAttr.Setpgid = true
	cmd.SysProcAttr.Pgid = 0

	return &platformExecutor{
		cmd:    cmd,
		exited: make(chan struct{}),
	}, nil
}

// startCommand starts the process with process group configuration
func (e *platformExecutor) startCommand() error {
	if err := e.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start process: %w", err)
	}
	return nil
}

// wait reaps the process exactly once and signals its exit. It is the single
// owner of cmd.Wait(): executeCommand calls it, and cleanup() blocks on the
// exited channel it closes. Calling cmd.Wait() from more than one place returns
// an immediate "Wait was already called" error and races the reaper.
func (e *platformExecutor) wait() error {
	e.waitOnce.Do(func() {
		e.waitErr = e.cmd.Wait()
		close(e.exited)
	})
	return e.waitErr
}

// cleanup terminates all processes in the process group
func (e *platformExecutor) cleanup() error {
	var err error
	e.once.Do(func() {
		slog.Info("checking process state")
		if e.cmd == nil || e.cmd.Process == nil {
			slog.Info("process is nil nothing to do")
			return
		}

		pid := e.cmd.Process.Pid

		// If the process has already been reaped (the common path — cleanup
		// runs from executeCommand's defer after wait() returned), skip
		// signalling entirely. Sending signals to a reaped PID is harmless
		// but noisy, and the PID could theoretically have been recycled.
		select {
		case <-e.exited:
			return
		default:
		}

		// Process is still alive; ask it to shut down gracefully.
		if killErr := syscall.Kill(-pid, syscall.SIGTERM); killErr != nil {
			slog.Warn("failed to send SIGTERM to process group", "error", killErr, "pid", pid)
			if killErr := syscall.Kill(pid, syscall.SIGTERM); killErr != nil {
				slog.Warn("failed to send SIGTERM to process", "error", killErr, "pid", pid)
			}
		}

		// Wait for graceful exit, keying off the real process exit (exited is
		// closed by wait(), the single cmd.Wait() owner) rather than probing the
		// PID — a reaped PID can be recycled.
		select {
		case <-e.exited:
			return
		case <-time.After(2000 * time.Millisecond):
		}

		// Grace period elapsed and the process is still running — exited is not
		// closed, so its PID cannot have been reused. Force kill the whole group.
		if killErr := syscall.Kill(-pid, syscall.SIGKILL); killErr != nil {
			slog.Warn("failed to send SIGKILL to process group", "error", killErr, "pid", pid)
			// If process group kill fails, try killing just the process
			if killErr := syscall.Kill(pid, syscall.SIGKILL); killErr != nil {
				err = fmt.Errorf("failed to send SIGKILL to process: %w", killErr)
			}
		}
	})
	return err
}
