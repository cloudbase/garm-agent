package runner

import (
	"fmt"
	"log/slog"
	"os/exec"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// platformExecutor handles Windows-specific process management using job objects
type platformExecutor struct {
	cmd  *exec.Cmd
	job  windows.Handle
	once sync.Once

	// wait() is the single owner of cmd.Wait(); waitOnce guards it and waitErr
	// caches its result. Mirrors the Unix executor so the shared executeCommand
	// can call executor.wait() on every platform.
	waitOnce sync.Once
	waitErr  error
	exited   chan struct{}
}

// setupCommand configures the command for Windows with job object support
func setupCommand(cmd *exec.Cmd) (*platformExecutor, error) {
	// Create a job object to manage child processes
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create job object: %w", err)
	}

	// Configure the job to kill all processes when the job handle is closed
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if ret, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil || ret == 0 {
		windows.CloseHandle(job)
		return nil, fmt.Errorf("failed to set information job object: %w", err)
	}

	// Configure the command to run in a new process group
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags = windows.CREATE_NEW_PROCESS_GROUP

	return &platformExecutor{
		cmd:    cmd,
		job:    job,
		exited: make(chan struct{}),
	}, nil
}

// wait reaps the process exactly once and signals its exit. It is the single
// owner of cmd.Wait(), mirroring the Unix executor so the shared executeCommand
// can call executor.wait() regardless of platform.
func (e *platformExecutor) wait() error {
	e.waitOnce.Do(func() {
		e.waitErr = e.cmd.Wait()
		close(e.exited)
	})
	return e.waitErr
}

// startCommand starts the process and assigns it to the job object
func (e *platformExecutor) startCommand() error {
	if err := e.cmd.Start(); err != nil {
		windows.CloseHandle(e.job)
		return fmt.Errorf("failed to start process: %w", err)
	}

	// Get the process handle - on Windows, Process.Pid is actually the process ID
	// We need to open a handle to the process
	processHandle, err := windows.OpenProcess(windows.PROCESS_ALL_ACCESS, false, uint32(e.cmd.Process.Pid))
	if err != nil {
		if killErr := e.cmd.Process.Kill(); killErr != nil {
			slog.Error("failed to kill process after handle open failure", "error", killErr)
		}
		windows.CloseHandle(e.job)
		return fmt.Errorf("failed to open process handle: %w", err)
	}
	defer windows.CloseHandle(processHandle)

	// Assign the process to the job object
	if err := windows.AssignProcessToJobObject(e.job, processHandle); err != nil {
		// If we can't assign to job, kill the process
		if killErr := e.cmd.Process.Kill(); killErr != nil {
			slog.Error("failed to kill process after job assignment failure", "error", killErr)
		}
		windows.CloseHandle(e.job)
		return fmt.Errorf("failed to assign process to job: %w", err)
	}

	return nil
}

// cleanup terminates all processes in the job and cleans up resources
func (e *platformExecutor) cleanup() error {
	var err error
	e.once.Do(func() {
		if e.job == 0 {
			return
		}

		// Close the job handle - this will terminate all processes in the job
		// due to JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE flag
		if jobErr := windows.CloseHandle(e.job); jobErr != nil {
			err = fmt.Errorf("failed to close job handle: %w", jobErr)
		}
	})
	return err
}
