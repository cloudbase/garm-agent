package runner

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/cloudbase/garm/params"
)

var doneChan = make(chan struct{})

func init() {
	close(doneChan)
}

// validateCmdParams validates command parameters to prevent command injection
func validateCmdParams(cmdParams []string) error {
	if len(cmdParams) == 0 {
		return fmt.Errorf("cmdParams is empty")
	}

	// Validate the executable path must be absolute
	execPath := cmdParams[0]
	if !filepath.IsAbs(execPath) {
		return fmt.Errorf("executable path must be absolute: %s", execPath)
	}

	// Validate arguments don't contain dangerous shell metacharacters
	for i, arg := range cmdParams {
		if strings.ContainsAny(arg, ";|&<>$`") {
			return fmt.Errorf("argument %d contains potentially dangerous characters: %s", i, arg)
		}
	}

	return nil
}

func NewRunnerCommand(ctx context.Context, cmdParams []string, workdir string, forgeType params.EndpointType, st StateManager) (Worker, error) {
	runnerCfg := filepath.Join(workdir, ".runner")
	runCfg, err := NewRunnerConfig(runnerCfg, forgeType)
	if err != nil {
		return nil, fmt.Errorf("failed to read runner config: %w", err)
	}

	mode, err := os.Stat(workdir)
	if err != nil {
		return nil, fmt.Errorf("failed to access workdir: %w", err)
	}
	if !mode.IsDir() {
		return nil, fmt.Errorf("workdir %s is not a folder", workdir)
	}

	if err := os.Chdir(workdir); err != nil {
		return nil, fmt.Errorf("failed to chdir to %s: %w", workdir, err)
	}

	if st == nil {
		return nil, fmt.Errorf("invalid state manager")
	}

	// Validate command parameters to prevent command injection
	if err := validateCmdParams(cmdParams); err != nil {
		return nil, fmt.Errorf("invalid command parameters: %w", err)
	}

	// #nosec G204 - cmdParams validated above for security
	command := exec.Command(cmdParams[0], cmdParams[1:]...)

	// Set up platform-specific process management
	executor, err := setupCommand(command)
	if err != nil {
		return nil, fmt.Errorf("failed to setup command: %w", err)
	}

	ret := &runnerCmd{
		ctx:       ctx,
		forgeType: forgeType,
		workdir:   workdir,
		cmd:       command,
		executor:  executor,
		runnerCfg: runCfg,
		done:      doneChan,
		st:        st,
		cmdParams: cmdParams, // Store for recreating command on restart
	}

	return ret, nil
}

type runnerCmd struct {
	ctx       context.Context
	forgeType params.EndpointType
	workdir   string
	runnerCfg Config
	st        StateManager

	done      chan struct{}
	running   bool
	mux       sync.Mutex
	executor  *platformExecutor
	cmdParams []string // Store command parameters for recreating the command

	cmd    *exec.Cmd
	cmdErr error
}

func (r *runnerCmd) AgentID() uint {
	return r.runnerCfg.GetAgentID()
}

func (r *runnerCmd) Error() error {
	return r.cmdErr
}

func (r *runnerCmd) Wait() <-chan struct{} {
	return r.done
}

func (r *runnerCmd) Start() error {
	r.mux.Lock()
	defer r.mux.Unlock()

	if r.running {
		return nil
	}

	// Recreate the command to avoid "Stdout already set" errors on retry
	// #nosec G204 - cmdParams validated during NewRunnerCommand
	r.cmd = exec.Command(r.cmdParams[0], r.cmdParams[1:]...)

	// Set up platform-specific process management
	executor, err := setupCommand(r.cmd)
	if err != nil {
		return fmt.Errorf("failed to setup command: %w", err)
	}
	r.executor = executor

	r.done = make(chan struct{})
	r.running = true

	go r.loop()
	go r.executeCommand()
	return nil
}

func (r *runnerCmd) Stop() error {
	r.mux.Lock()
	defer r.mux.Unlock()

	slog.Info("stopping runner command")
	if !r.running {
		slog.Info("runner command not started; returning")
		return nil
	}

	close(r.done)
	r.running = false

	// Use platform-specific cleanup to terminate all child processes
	slog.Info("cleaning up processes")
	if err := r.executor.cleanup(); err != nil {
		slog.ErrorContext(r.ctx, "failed to cleanup process group/job", "error", err)
		return err
	}

	return nil
}

func (r *runnerCmd) executeCommand() {
	var err error
	var stdout, stderr io.ReadCloser
	var jobStarted bool
	var jobMux sync.Mutex
	var scannerWg sync.WaitGroup

	defer func() {
		r.cmdErr = err
		if stopErr := r.Stop(); stopErr != nil {
			slog.ErrorContext(r.ctx, "failed to stop runner", "error", stopErr)
		}
		// Wait for scanner goroutines to drain any buffered output before
		// marking the runner as offline. This ensures that if the process
		// wrote "Listening for Jobs" just before exiting (or before a child
		// process was killed), the idle signal reaches GARM before offline.
		scannerWg.Wait()
		r.st.SetRunnerStarted(false)

		jobMux.Lock()
		started := jobStarted
		jobMux.Unlock()

		if started {
			// Job was started and the runner exited. This means that the job reached a conclusion
			// and we need to remove the runner.
			slog.InfoContext(r.ctx, "runner has finished the job")
			r.st.SetJobFinished()
		}
	}()
	// Create pipes for stdout and stderr
	stdout, err = r.cmd.StdoutPipe()
	if err != nil {
		slog.ErrorContext(r.ctx, "failed to create stdout pipe", "error", err)
		return
	}

	stderr, err = r.cmd.StderrPipe()
	if err != nil {
		slog.ErrorContext(r.ctx, "failed to create stderr pipe", "error", err)
		return
	}

	scannerWg.Add(2)
	go func() {
		defer scannerWg.Done()
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Bytes()
			slog.InfoContext(r.ctx, string(line))
			if r.isJobStartedLine(line) {
				slog.InfoContext(r.ctx, "runner is active")
				r.st.SetJobStarted()
				jobMux.Lock()
				jobStarted = true
				jobMux.Unlock()
				continue
			}

			if r.isRunnerStartedLine(line) {
				slog.InfoContext(r.ctx, "runner is online and idle")
				r.st.SetRunnerStarted(true)
				continue
			}
		}
		if err := scanner.Err(); err != nil {
			slog.ErrorContext(r.ctx, "error reading stdout", "error", err)
		}
	}()

	go func() {
		defer scannerWg.Done()
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			line := scanner.Bytes()
			slog.InfoContext(r.ctx, string(line))
			if r.isJobStartedLine(line) {
				slog.InfoContext(r.ctx, "runner is active")
				r.st.SetJobStarted()
				jobMux.Lock()
				jobStarted = true
				jobMux.Unlock()
				continue
			}

			if r.isRunnerStartedLine(line) {
				slog.InfoContext(r.ctx, "runner is online and idle")
				r.st.SetRunnerStarted(true)
				continue
			}
		}
		if err := scanner.Err(); err != nil {
			slog.ErrorContext(r.ctx, "error reading stderr", "error", err)
		}
	}()

	// Start the command with platform-specific process management
	err = r.executor.startCommand()
	if err != nil {
		slog.ErrorContext(r.ctx, "failed to start command", "error", err)
		return
	}

	// wait() is the single owner of cmd.Wait(); it reaps the process and signals
	// cleanup() (via the executor's exited channel) that the process is gone.
	err = r.executor.wait()
	if err != nil {
		slog.ErrorContext(r.ctx, "command failed", "error", err)
		return
	}
}

func (r *runnerCmd) loop() {
	defer func() {
		if stopErr := r.Stop(); stopErr != nil {
			slog.ErrorContext(r.ctx, "failed to stop runner", "error", stopErr)
		}
	}()

	for {
		select {
		case <-r.done:
			return
		case <-r.ctx.Done():
			return
		}
	}
}
