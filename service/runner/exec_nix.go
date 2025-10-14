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
		cmd: cmd,
	}, nil
}

// startCommand starts the process with process group configuration
func (e *platformExecutor) startCommand() error {
	if err := e.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start process: %w", err)
	}
	return nil
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

		// Send SIGTERM to the process group for graceful shutdown
		// Use negative PID to signal the entire process group
		if killErr := syscall.Kill(-pid, syscall.SIGTERM); killErr != nil {
			slog.Warn("failed to send SIGTERM to process group", "error", killErr, "pid", pid)
			// If process group kill fails, try killing just the process
			if killErr := syscall.Kill(pid, syscall.SIGTERM); killErr != nil {
				slog.Warn("failed to send SIGTERM to process", "error", killErr, "pid", pid)
			}
		}

		// Wait briefly for graceful exit
		done := make(chan error, 1)
		go func() {
			done <- e.cmd.Wait()
		}()

		select {
		case <-done:
			// Process exited gracefully
		case <-time.After(2000 * time.Millisecond):
			// Force kill if it didn't exit gracefully
			// Send SIGKILL to the process group
			if killErr := syscall.Kill(-pid, syscall.SIGKILL); killErr != nil {
				slog.Warn("failed to send SIGKILL to process group", "error", killErr, "pid", pid)
				// If process group kill fails, try killing just the process
				if killErr := syscall.Kill(pid, syscall.SIGKILL); killErr != nil {
					err = fmt.Errorf("failed to send SIGKILL to process: %w", killErr)
				}
			}
			<-done // Wait for the process to actually exit
		}
	})
	return err
}
