package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cloudbase/garm-agent/config"
	"github.com/cloudbase/garm/params"
)

func createTestConfig(t *testing.T) (*config.Agent, string) {
	t.Helper()
	tmpDir := t.TempDir()
	stateDB := filepath.Join(tmpDir, "state.db")
	workDir := filepath.Join(tmpDir, "work")
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatalf("failed to create work dir: %v", err)
	}

	// Create a mock .runner config
	runnerConfig := filepath.Join(workDir, ".runner")
	githubConfig := `{"agentId":"123","agentName":"test-runner","ephemeral":"True","serverUrl":"https://github.com"}`
	if err := os.WriteFile(runnerConfig, []byte(githubConfig), 0o600); err != nil {
		t.Fatalf("failed to write runner config: %v", err)
	}

	cfg := &config.Agent{
		ServerURL:      "https://garm.example.com/agent",
		Token:          "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJpc19hZ2VudCI6dHJ1ZSwiZm9yZ2VfdHlwZSI6ImdpdGh1YiJ9.dummysignature",
		WorkDir:        workDir,
		StateDBPath:    stateDB,
		RunnerExecArgs: []string{"/usr/bin/true"},
		EnableShell:    false,
	}
	return cfg, tmpDir
}

func TestNewService(t *testing.T) {
	tests := []struct {
		name        string
		setupConfig func(t *testing.T) *config.Agent
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid config",
			setupConfig: func(t *testing.T) *config.Agent {
				cfg, _ := createTestConfig(t)
				return cfg
			},
			expectError: false,
		},
		{
			name: "invalid config - missing token",
			setupConfig: func(t *testing.T) *config.Agent {
				cfg, _ := createTestConfig(t)
				cfg.Token = ""
				return cfg
			},
			expectError: true,
			errorMsg:    "failed to validate agent config",
		},
		{
			name: "invalid config - bad server URL",
			setupConfig: func(t *testing.T) *config.Agent {
				cfg, _ := createTestConfig(t)
				cfg.ServerURL = "not-a-url"
				return cfg
			},
			expectError: true,
			errorMsg:    "failed to validate agent config",
		},
		{
			name: "invalid forge type",
			setupConfig: func(t *testing.T) *config.Agent {
				cfg, _ := createTestConfig(t)
				cfg.Token = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJpc19hZ2VudCI6dHJ1ZSwiZm9yZ2VfdHlwZSI6ImludmFsaWQifQ.dummysignature"
				return cfg
			},
			expectError: true,
			errorMsg:    "failed to get forge type",
		},
		{
			name: "with shell enabled",
			setupConfig: func(t *testing.T) *config.Agent {
				cfg, _ := createTestConfig(t)
				cfg.EnableShell = true
				return cfg
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			cfg := tt.setupConfig(t)

			service, err := NewService(ctx, cfg)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errorMsg)
				} else if tt.errorMsg != "" && !contains(err.Error(), tt.errorMsg) {
					t.Errorf("expected error containing %q, got %q", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if service == nil {
					t.Error("expected non-nil service")
				} else {
					// Verify service fields are initialized correctly
					if service.cfg != cfg {
						t.Error("service config mismatch")
					}
					if service.agentState == nil {
						t.Error("expected non-nil state manager")
					}
					if service.sessions == nil {
						t.Error("expected non-nil sessions map")
					}
					service.agentState.Close()
				}
			}
		})
	}
}

func TestServiceDone(t *testing.T) {
	ctx := context.Background()
	cfg, _ := createTestConfig(t)

	service, err := NewService(ctx, cfg)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}
	defer service.agentState.Close()

	doneChan := service.Done()
	if doneChan == nil {
		t.Error("expected non-nil done channel")
	}

	// Should be able to read from channel immediately since init() closes it
	select {
	case <-doneChan:
		// Expected - initially closed
	default:
		t.Error("expected done channel to be closed initially")
	}
}

func TestServiceStartStop(t *testing.T) {
	ctx := context.Background()
	cfg, _ := createTestConfig(t)

	service, err := NewService(ctx, cfg)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}
	defer service.agentState.Close()

	// Start service
	if err := service.Start(); err != nil {
		t.Fatalf("failed to start service: %v", err)
	}

	if !service.running {
		t.Error("expected service to be running after Start()")
	}

	// Starting again should be idempotent
	if err := service.Start(); err != nil {
		t.Errorf("second Start() should be idempotent: %v", err)
	}

	// Give goroutines a moment to start
	time.Sleep(100 * time.Millisecond)

	// Stop service
	if err := service.Stop(); err != nil {
		t.Fatalf("failed to stop service: %v", err)
	}

	if service.running {
		t.Error("expected service to not be running after Stop()")
	}

	// Stopping again should be idempotent
	if err := service.Stop(); err != nil {
		t.Errorf("second Stop() should be idempotent: %v", err)
	}
}

func TestServiceGetClient(t *testing.T) {
	ctx := context.Background()
	cfg, _ := createTestConfig(t)

	service, err := NewService(ctx, cfg)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}
	defer service.agentState.Close()

	// Client should be nil initially
	cli, err := service.getClient()
	if err == nil {
		t.Error("expected error when client not connected")
	}
	if cli != nil {
		t.Error("expected nil client when not connected")
	}
	if !contains(err.Error(), "websocket client not connected") {
		t.Errorf("expected websocket error, got: %v", err)
	}
}

func TestServiceDetermineRunnerState(t *testing.T) {
	ctx := context.Background()
	cfg, _ := createTestConfig(t)

	service, err := NewService(ctx, cfg)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}
	defer service.agentState.Close()

	// Create a mock runner command to set runnerCmd
	workDir := cfg.WorkDir
	runnerCmd, err := createMockRunner(ctx, workDir, cfg)
	if err != nil {
		t.Fatalf("failed to create mock runner: %v", err)
	}
	service.runnerCmd = runnerCmd

	tests := []struct {
		name           string
		runnerAlive    bool
		setupState     func()
		expectedState  params.RunnerStatus
	}{
		{
			name:        "runner offline",
			runnerAlive: false,
			setupState:  func() {},
			expectedState: params.RunnerOffline,
		},
		{
			name:        "runner idle",
			runnerAlive: true,
			setupState:  func() {},
			expectedState: params.RunnerIdle,
		},
		{
			name:        "runner active",
			runnerAlive: true,
			setupState: func() {
				if err := service.agentState.SetJobStarted(); err != nil {
					t.Fatalf("failed to set job started: %v", err)
				}
			},
			expectedState: params.RunnerActive,
		},
		{
			name:        "runner terminated",
			runnerAlive: false,
			setupState: func() {
				if err := service.agentState.SetJobStarted(); err != nil {
					t.Fatalf("failed to set job started: %v", err)
				}
				if err := service.agentState.SetJobFinished(); err != nil {
					t.Fatalf("failed to set job finished: %v", err)
				}
			},
			expectedState: params.RunnerTerminated,
		},
		{
			name:        "runner terminated after job started but offline",
			runnerAlive: false,
			setupState: func() {
				if err := service.agentState.SetJobStarted(); err != nil {
					t.Fatalf("failed to set job started: %v", err)
				}
			},
			expectedState: params.RunnerTerminated,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset state
			tmpDir := t.TempDir()
			stateDB := filepath.Join(tmpDir, "state.db")
			newState, err := createStateManager(stateDB)
			if err != nil {
				t.Fatalf("failed to create state manager: %v", err)
			}
			defer newState.Close()
			service.agentState = newState

			tt.setupState()

			state := service.determineRunnerState(tt.runnerAlive)
			if state != tt.expectedState {
				t.Errorf("expected state %v, got %v", tt.expectedState, state)
			}
		})
	}
}

func TestServiceSleepWithCancel(t *testing.T) {
	tests := []struct {
		name           string
		duration       time.Duration
		cancelAfter    time.Duration
		expectCancel   bool
	}{
		{
			name:         "complete sleep",
			duration:     50 * time.Millisecond,
			expectCancel: false,
		},
		{
			name:         "cancel during sleep",
			duration:     500 * time.Millisecond,
			cancelAfter:  50 * time.Millisecond,
			expectCancel: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			cfg, _ := createTestConfig(t)
			service, err := NewService(ctx, cfg)
			if err != nil {
				t.Fatalf("failed to create service: %v", err)
			}
			defer service.agentState.Close()

			if err := service.Start(); err != nil {
				t.Fatalf("failed to start service: %v", err)
			}
			defer func() {
				if stopErr := service.Stop(); stopErr != nil {
					t.Errorf("failed to stop service: %v", stopErr)
				}
			}()

			if tt.cancelAfter > 0 {
				go func() {
					time.Sleep(tt.cancelAfter)
					cancel()
				}()
			}

			shouldQuit := service.sleepWithCancel(tt.duration)

			if shouldQuit != tt.expectCancel {
				t.Errorf("expected shouldQuit=%v, got %v", tt.expectCancel, shouldQuit)
			}
		})
	}
}

func TestServiceWithInvalidWorkDir(t *testing.T) {
	ctx := context.Background()
	cfg, _ := createTestConfig(t)
	cfg.WorkDir = "/nonexistent/directory"

	service, err := NewService(ctx, cfg)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}
	defer service.agentState.Close()

	// Start should fail when WorkDir doesn't exist
	if err := service.Start(); err == nil {
		t.Error("expected error when starting with invalid work dir")
	}
}

func TestServiceEnableShell(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name         string
		enableShell  bool
		expectedHas  bool
	}{
		{
			name:        "shell disabled",
			enableShell: false,
			expectedHas: false,
		},
		{
			name:        "shell enabled",
			enableShell: true,
			expectedHas: HasPTY(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, _ := createTestConfig(t)
			cfg.EnableShell = tt.enableShell

			service, err := NewService(ctx, cfg)
			if err != nil {
				t.Fatalf("failed to create service: %v", err)
			}
			defer service.agentState.Close()

			if service.hasShell != tt.expectedHas {
				t.Errorf("expected hasShell=%v, got %v", tt.expectedHas, service.hasShell)
			}
		})
	}
}

// Helper functions
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && indexOf(s, substr) >= 0))
}

func indexOf(s, substr string) int {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
