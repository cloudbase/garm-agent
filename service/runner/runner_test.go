package runner

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/cloudbase/garm/params"
)

type mockStateManager struct {
	jobStarted     bool
	jobFinished    bool
	runnerStarted  bool
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

func TestValidateCmdParams(t *testing.T) {
	tests := []struct {
		name        string
		cmdParams   []string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "valid absolute path",
			cmdParams:   []string{"/usr/bin/runner", "arg1", "arg2"},
			expectError: false,
		},
		{
			name:        "empty params",
			cmdParams:   []string{},
			expectError: true,
			errorMsg:    "cmdParams is empty",
		},
		{
			name:        "relative path",
			cmdParams:   []string{"runner", "arg1"},
			expectError: true,
			errorMsg:    "executable path must be absolute",
		},
		{
			name:        "semicolon in argument",
			cmdParams:   []string{"/usr/bin/runner", "arg1;rm -rf /"},
			expectError: true,
			errorMsg:    "contains potentially dangerous characters",
		},
		{
			name:        "pipe in argument",
			cmdParams:   []string{"/usr/bin/runner", "arg1|cat"},
			expectError: true,
			errorMsg:    "contains potentially dangerous characters",
		},
		{
			name:        "ampersand in argument",
			cmdParams:   []string{"/usr/bin/runner", "arg1&"},
			expectError: true,
			errorMsg:    "contains potentially dangerous characters",
		},
		{
			name:        "redirect in argument",
			cmdParams:   []string{"/usr/bin/runner", "arg1>file"},
			expectError: true,
			errorMsg:    "contains potentially dangerous characters",
		},
		{
			name:        "dollar sign in argument",
			cmdParams:   []string{"/usr/bin/runner", "$HOME"},
			expectError: true,
			errorMsg:    "contains potentially dangerous characters",
		},
		{
			name:        "backtick in argument",
			cmdParams:   []string{"/usr/bin/runner", "`whoami`"},
			expectError: true,
			errorMsg:    "contains potentially dangerous characters",
		},
		{
			name:        "valid with dashes and equals",
			cmdParams:   []string{"/usr/bin/runner", "--flag=value", "-v"},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateCmdParams(tt.cmdParams)

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
			}
		})
	}
}

func TestNewRunnerCommand(t *testing.T) {
	tmpDir := t.TempDir()
	workdir := filepath.Join(tmpDir, "workdir")
	err := os.MkdirAll(workdir, 0o755)
	if err != nil {
		t.Fatalf("failed to create workdir: %v", err)
	}

	// Create a mock .runner config file
	runnerConfig := filepath.Join(workdir, ".runner")
	githubConfig := `{"agentId":"123","agentName":"test-runner","ephemeral":"True","serverUrl":"https://github.com"}`
	err = os.WriteFile(runnerConfig, []byte(githubConfig), 0o600)
	if err != nil {
		t.Fatalf("failed to write runner config: %v", err)
	}

	tests := []struct {
		name        string
		cmdParams   []string
		workdir     string
		forgeType   params.EndpointType
		stateManager StateManager
		expectError bool
		errorMsg    string
	}{
		{
			name:         "valid command",
			cmdParams:    []string{"/usr/bin/true"},
			workdir:      workdir,
			forgeType:    params.GithubEndpointType,
			stateManager: &mockStateManager{},
			expectError:  false,
		},
		{
			name:         "invalid workdir",
			cmdParams:    []string{"/usr/bin/true"},
			workdir:      "/nonexistent",
			forgeType:    params.GithubEndpointType,
			stateManager: &mockStateManager{},
			expectError:  true,
			errorMsg:     "failed to read runner config",
		},
		{
			name:         "invalid command params",
			cmdParams:    []string{"relative/path"},
			workdir:      workdir,
			forgeType:    params.GithubEndpointType,
			stateManager: &mockStateManager{},
			expectError:  true,
			errorMsg:     "invalid command parameters",
		},
		{
			name:         "nil state manager",
			cmdParams:    []string{"/usr/bin/true"},
			workdir:      workdir,
			forgeType:    params.GithubEndpointType,
			stateManager: nil,
			expectError:  true,
			errorMsg:     "invalid state manager",
		},
		{
			name:         "workdir is a file not directory",
			cmdParams:    []string{"/usr/bin/true"},
			workdir:      runnerConfig,
			forgeType:    params.GithubEndpointType,
			stateManager: &mockStateManager{},
			expectError:  true,
			errorMsg:     "not a directory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			worker, err := NewRunnerCommand(ctx, tt.cmdParams, tt.workdir, tt.forgeType, tt.stateManager)

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
				if worker == nil {
					t.Error("expected non-nil worker")
				}
			}
		})
	}
}

func TestRunnerCmdAgentID(t *testing.T) {
	tmpDir := t.TempDir()
	workdir := filepath.Join(tmpDir, "workdir")
	err := os.MkdirAll(workdir, 0o755)
	if err != nil {
		t.Fatalf("failed to create workdir: %v", err)
	}

	runnerConfig := filepath.Join(workdir, ".runner")
	githubConfig := `{"agentId":"456","agentName":"test-runner","ephemeral":"True","serverUrl":"https://github.com"}`
	err = os.WriteFile(runnerConfig, []byte(githubConfig), 0o600)
	if err != nil {
		t.Fatalf("failed to write runner config: %v", err)
	}

	ctx := context.Background()
	worker, err := NewRunnerCommand(ctx, []string{"/usr/bin/true"}, workdir, params.GithubEndpointType, &mockStateManager{})
	if err != nil {
		t.Fatalf("failed to create worker: %v", err)
	}

	agentID := worker.AgentID()
	if agentID != 456 {
		t.Errorf("expected agent ID 456, got %d", agentID)
	}
}

func TestRunnerCmdWait(t *testing.T) {
	tmpDir := t.TempDir()
	workdir := filepath.Join(tmpDir, "workdir")
	err := os.MkdirAll(workdir, 0o755)
	if err != nil {
		t.Fatalf("failed to create workdir: %v", err)
	}

	runnerConfig := filepath.Join(workdir, ".runner")
	githubConfig := `{"agentId":"123","agentName":"test-runner","ephemeral":"True","serverUrl":"https://github.com"}`
	err = os.WriteFile(runnerConfig, []byte(githubConfig), 0o600)
	if err != nil {
		t.Fatalf("failed to write runner config: %v", err)
	}

	ctx := context.Background()
	worker, err := NewRunnerCommand(ctx, []string{"/usr/bin/true"}, workdir, params.GithubEndpointType, &mockStateManager{})
	if err != nil {
		t.Fatalf("failed to create worker: %v", err)
	}

	doneChan := worker.Wait()
	if doneChan == nil {
		t.Error("expected non-nil done channel")
	}

	// Should be able to read from channel immediately since init() closes it
	select {
	case <-doneChan:
		// Expected
	default:
		t.Error("expected done channel to be closed initially")
	}
}

func TestRunnerCmdError(t *testing.T) {
	tmpDir := t.TempDir()
	workdir := filepath.Join(tmpDir, "workdir")
	err := os.MkdirAll(workdir, 0o755)
	if err != nil {
		t.Fatalf("failed to create workdir: %v", err)
	}

	runnerConfig := filepath.Join(workdir, ".runner")
	githubConfig := `{"agentId":"123","agentName":"test-runner","ephemeral":"True","serverUrl":"https://github.com"}`
	err = os.WriteFile(runnerConfig, []byte(githubConfig), 0o600)
	if err != nil {
		t.Fatalf("failed to write runner config: %v", err)
	}

	ctx := context.Background()
	worker, err := NewRunnerCommand(ctx, []string{"/usr/bin/true"}, workdir, params.GithubEndpointType, &mockStateManager{})
	if err != nil {
		t.Fatalf("failed to create worker: %v", err)
	}

	// Initially should have no error
	if worker.Error() != nil {
		t.Errorf("expected nil error initially, got %v", worker.Error())
	}
}

func TestIsJobStartedLine(t *testing.T) {
	tests := []struct {
		name      string
		forgeType params.EndpointType
		line      string
		expected  bool
	}{
		{
			name:      "github job started",
			forgeType: params.GithubEndpointType,
			line:      "2024-01-01 12:00:00Z: Running job: test-job",
			expected:  true,
		},
		{
			name:      "github job not started",
			forgeType: params.GithubEndpointType,
			line:      "Listening for Jobs",
			expected:  false,
		},
		{
			name:      "gitea job started",
			forgeType: params.GiteaEndpointType,
			line:      "task 123 repo is example/repo",
			expected:  true,
		},
		{
			name:      "gitea job not started",
			forgeType: params.GiteaEndpointType,
			line:      "runner: test-runner, declare successfully",
			expected:  false,
		},
		{
			name:      "invalid forge type",
			forgeType: "invalid",
			line:      "Running job: test-job",
			expected:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &runnerCmd{forgeType: tt.forgeType}
			result := r.isJobStartedLine([]byte(tt.line))
			if result != tt.expected {
				t.Errorf("expected %v, got %v for line: %s", tt.expected, result, tt.line)
			}
		})
	}
}

func TestIsRunnerStartedLine(t *testing.T) {
	tests := []struct {
		name      string
		forgeType params.EndpointType
		line      string
		expected  bool
	}{
		{
			name:      "github runner started",
			forgeType: params.GithubEndpointType,
			line:      "Listening for Jobs",
			expected:  true,
		},
		{
			name:      "github runner not started",
			forgeType: params.GithubEndpointType,
			line:      "Starting runner...",
			expected:  false,
		},
		{
			name:      "gitea runner started",
			forgeType: params.GiteaEndpointType,
			line:      "runner: test-runner, declare successfully",
			expected:  true,
		},
		{
			name:      "gitea runner not started",
			forgeType: params.GiteaEndpointType,
			line:      "starting runner",
			expected:  false,
		},
		{
			name:      "invalid forge type",
			forgeType: "invalid",
			line:      "Listening for Jobs",
			expected:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &runnerCmd{forgeType: tt.forgeType}
			result := r.isRunnerStartedLine([]byte(tt.line))
			if result != tt.expected {
				t.Errorf("expected %v, got %v for line: %s", tt.expected, result, tt.line)
			}
		})
	}
}

// Helper function
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
