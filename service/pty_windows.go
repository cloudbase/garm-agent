package service

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"unsafe"

	"github.com/charmbracelet/x/conpty"
	"github.com/cloudbase/garm-agent/config"
	"golang.org/x/sys/windows"
)

var _ PTY = &sessionPTY{}

var (
	modkernel32             = syscall.NewLazyDLL("kernel32.dll")
	procCreatePseudoConsole = modkernel32.NewProc("CreatePseudoConsole")
)

const CREATE_SUSPENDED = 0x00000004

func NewSessionPTY(cfg *config.Agent) (PTY, error) {
	defaultShell, err := DefaultShell()
	if err != nil {
		return nil, fmt.Errorf("failed to get default shell: %w", err)
	}

	if cfg.Shell != "" {
		// blindly trust the value in the config
		defaultShell = cfg.Shell
	}

	c, err := conpty.New(120, 80, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}
	cmd := exec.Command(defaultShell)
	s := &sessionPTY{
		ConPty: c,
	}
	if err := s.start(cmd); err != nil {
		c.Close() // Clean up ConPty if start fails
		return nil, fmt.Errorf("failed to start command: %w", err)
	}
	return s, nil
}

type sessionPTY struct {
	*conpty.ConPty

	cmd    *exec.Cmd
	job    windows.Handle
	closed chan struct{}
	once   sync.Once
}

func (c *sessionPTY) start(cmd *exec.Cmd) (err error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return fmt.Errorf("failed to create job object: %w", err)
	}
	defer func() {
		if err != nil {
			windows.CloseHandle(job)
		}
	}()

	c.job = job

	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if ret, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil || ret == 0 {
		return fmt.Errorf("failed to set information job object: %w", err)
	}

	pid, proc, err := c.ConPty.Spawn(cmd.Path, cmd.Args, &syscall.ProcAttr{
		Dir: cmd.Dir,
		Env: cmd.Env,
		Sys: cmd.SysProcAttr,
	})
	if err != nil {
		return fmt.Errorf("failed to spawn process: %w", err)
	}

	cmd.Process, err = os.FindProcess(pid)
	if err != nil {
		if tErr := windows.TerminateProcess(windows.Handle(proc), 1); tErr != nil {
			return fmt.Errorf("failed to terminate process after process not found: %w", tErr)
		}
		return fmt.Errorf("failed to find process after starting: %w", err)
	}

	if err := windows.AssignProcessToJobObject(job, windows.Handle(proc)); err != nil {
		if err := cmd.Process.Kill(); err != nil {
			slog.Error("failed to kill process", "error", err)
		}
		return fmt.Errorf("failed to assign process to job: %w", err)
	}

	c.cmd = cmd
	c.closed = make(chan struct{})

	// Start a goroutine to monitor process exit
	go c.monitorProcess()

	return nil
}

func (c *sessionPTY) monitorProcess() {
	if c.cmd != nil && c.cmd.Process != nil {
		c.cmd.Wait() // Wait for process to exit
		c.Close()    // Automatically cleanup when process exits
	}
}

func (p *sessionPTY) Resize(cols, rows uint16) error {
	if p == nil {
		return nil
	}

	return p.ConPty.Resize(int(cols), int(rows))
}

func (p *sessionPTY) Close() error {
	if p == nil {
		return nil
	}

	var err error
	p.once.Do(func() {
		var errs []error

		// Signal that we're closing
		if p.closed != nil {
			close(p.closed)
		}

		// Close job handle first to terminate child processes
		if p.job != 0 {
			if jobErr := windows.CloseHandle(p.job); jobErr != nil {
				errs = append(errs, fmt.Errorf("failed to close job handle: %w", jobErr))
			}
		}

		// Close the ConPTY (this calls ClosePseudoConsole internally)
		if p.ConPty != nil {
			if conptyErr := p.ConPty.Close(); conptyErr != nil {
				errs = append(errs, conptyErr)
			}
		}

		if len(errs) > 0 {
			err = errors.Join(errs...)
		}
	})

	return err
}

func HasPTY() bool {
	return procCreatePseudoConsole.Find() == nil
}

func DefaultShell() (string, error) {
	return "C:\\Windows\\System32\\cmd.exe", nil
}
