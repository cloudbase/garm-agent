package service

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/creack/pty"
	"golang.org/x/sys/unix"

	"github.com/cloudbase/garm-agent/config"
)

var _ PTY = &sessionPTY{}

func NewSessionPTY(cfg *config.Agent) (PTY, error) {
	defaultShell, err := DefaultShell()
	if err != nil {
		return nil, fmt.Errorf("failed to get default shell: %w", err)
	}
	if cfg.Shell != "" {
		// blindly trust the value in the config
		defaultShell = cfg.Shell
	}

	cmd := exec.Command(defaultShell)
	ptyFile, err := pty.Start(cmd)
	if err != nil {
		return nil, fmt.Errorf("failed to start PTY: %w", err)
	}

	return &sessionPTY{
		PTY: ptyFile,
		cmd: cmd,
	}, nil
}

type sessionPTY struct {
	PTY *os.File
	cmd *exec.Cmd
}

func (p *sessionPTY) Read(b []byte) (int, error) {
	if p == nil {
		return 0, os.ErrInvalid
	}
	return p.PTY.Read(b)
}

func (p *sessionPTY) Write(b []byte) (int, error) {
	if p == nil {
		return 0, os.ErrInvalid
	}
	return p.PTY.Write(b)
}

func (p *sessionPTY) Resize(cols, rows uint16) error {
	if p == nil {
		return nil
	}
	if cols <= 0 || rows <= 0 {
		return nil
	}
	return pty.Setsize(p.PTY, &pty.Winsize{Cols: cols, Rows: rows})
}

func (p *sessionPTY) Close() error {
	if p == nil {
		return nil
	}

	// Close PTY first to break any blocking reads
	ptyErr := p.PTY.Close()

	if p.cmd != nil && p.cmd.Process != nil {
		// Send SIGTERM to the process group for graceful shutdown
		if err := syscall.Kill(-p.cmd.Process.Pid, syscall.SIGTERM); err != nil {
			// If process group kill fails, try killing just the process
			if killErr := syscall.Kill(p.cmd.Process.Pid, syscall.SIGTERM); killErr != nil {
				slog.Warn("failed to send SIGTERM to process", "error", killErr, "pid", p.cmd.Process.Pid)
			}
		}

		// Wait briefly for graceful exit
		done := make(chan error, 1)
		go func() {
			done <- p.cmd.Wait()
		}()

		select {
		case <-done:
			// Process exited gracefully
		case <-time.After(100 * time.Millisecond):
			// Force kill if it didn't exit gracefully
			if err := syscall.Kill(-p.cmd.Process.Pid, syscall.SIGKILL); err != nil {
				if killErr := syscall.Kill(p.cmd.Process.Pid, syscall.SIGKILL); killErr != nil {
					slog.Warn("failed to send SIGKILL to process", "error", killErr, "pid", p.cmd.Process.Pid)
				}
			}
			<-done // Wait for the process to actually exit
		}
	}

	return ptyErr
}

func HasPTY() bool {
	fd, err := unix.Open("/dev/ptmx", unix.O_RDWR|unix.O_CLOEXEC, 0)
	if err != nil {
		return false
	}
	unix.Close(fd)
	return true
}

func DefaultShell() (string, error) {
	return "/bin/bash", nil
}
