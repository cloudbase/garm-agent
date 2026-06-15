package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/cloudbase/garm-agent/config"
	"github.com/cloudbase/garm-agent/service/runner"
	"github.com/cloudbase/garm-agent/state"
)

// createMockRunner creates a minimal runner.Worker for testing
func createMockRunner(ctx context.Context, workDir string, cfg *config.Agent) (runner.Worker, error) {
	forgeType, err := cfg.ForgeType()
	if err != nil {
		return nil, err
	}

	stateManager := &mockStateManager{}
	return runner.NewRunnerCommand(ctx, cfg.RunnerExecArgs, workDir, forgeType, stateManager)
}

// createStateManager creates a state manager for testing
func createStateManager(dbPath string) (*state.Manager, error) {
	return state.NewManager(dbPath)
}

// mockStateManager implements runner.StateManager for testing
type mockStateManager struct {
	runnerStarted bool
	jobStarted    bool
	jobFinished   bool
}

func (m *mockStateManager) SetRunnerStarted(st bool) {
	m.runnerStarted = st
}

func (m *mockStateManager) SetJobStarted() {
	m.jobStarted = true
}

func (m *mockStateManager) SetJobFinished() {
	m.jobFinished = true
}

// mockPTY implements the PTY interface for testing. Its methods are
// goroutine-safe to match the real *os.File-backed sessionPTY, whose
// Read/Write/Close are safe for concurrent use and where Close unblocks a
// pending Read. Without this, the race detector flags accesses that are
// actually safe in production.
type mockPTY struct {
	mu     sync.Mutex
	data   []byte
	closed bool
}

func (m *mockPTY) Read(p []byte) (int, error) {
	for {
		m.mu.Lock()
		if m.closed {
			m.mu.Unlock()
			return 0, fmt.Errorf("PTY is closed")
		}
		if len(m.data) > 0 {
			n := copy(p, m.data)
			m.data = m.data[n:]
			m.mu.Unlock()
			return n, nil
		}
		m.mu.Unlock()
		// Block until data is written or the PTY is closed, like a real PTY.
		time.Sleep(time.Millisecond)
	}
}

func (m *mockPTY) Write(p []byte) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return 0, fmt.Errorf("PTY is closed")
	}
	m.data = append(m.data, p...)
	return len(p), nil
}

func (m *mockPTY) Resize(cols, rows uint16) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return fmt.Errorf("PTY is closed")
	}
	return nil
}

func (m *mockPTY) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	return nil
}
