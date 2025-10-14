package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewConfig(t *testing.T) {
	tests := []struct {
		name        string
		configData  string
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid config",
			configData: `server_url = "https://garm.example.com/agent"
token = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJpc19hZ2VudCI6dHJ1ZSwiZm9yZ2VfdHlwZSI6ImdpdGh1YiJ9.dummysignature"
work_dir = "/opt/runner"
log_file = "/var/log/garm-agent.log"
shell = "/bin/bash"
enable_shell = true
runner_cmdline = ["/opt/runner/run.sh"]
state_db_path = "/var/lib/garm-agent/state.db"
`,
			expectError: false,
		},
		{
			name: "missing server_url",
			configData: `token = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJpc19hZ2VudCI6dHJ1ZSwiZm9yZ2VfdHlwZSI6ImdpdGh1YiJ9.dummysignature"
work_dir = "/opt/runner"
runner_cmdline = ["/opt/runner/run.sh"]
`,
			expectError: true,
			errorMsg:    "invalid server_url",
		},
		{
			name: "invalid server_url",
			configData: `server_url = "not-a-valid-url"
token = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJpc19hZ2VudCI6dHJ1ZSwiZm9yZ2VfdHlwZSI6ImdpdGh1YiJ9.dummysignature"
work_dir = "/opt/runner"
runner_cmdline = ["/opt/runner/run.sh"]
`,
			expectError: true,
			errorMsg:    "invalid server_url",
		},
		{
			name: "missing token",
			configData: `server_url = "https://garm.example.com/agent"
work_dir = "/opt/runner"
runner_cmdline = ["/opt/runner/run.sh"]
`,
			expectError: true,
			errorMsg:    "missing token",
		},
		{
			name: "missing runner_cmdline",
			configData: `server_url = "https://garm.example.com/agent"
token = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJpc19hZ2VudCI6dHJ1ZSwiZm9yZ2VfdHlwZSI6ImdpdGh1YiJ9.dummysignature"
work_dir = "/opt/runner"
`,
			expectError: true,
			errorMsg:    "runner_cmdline is not set",
		},
		{
			name: "invalid toml syntax",
			configData: `server_url = "https://garm.example.com/agent"
token = [invalid
`,
			expectError: true,
			errorMsg:    "error decoding toml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			configFile := filepath.Join(tmpDir, "config.toml")

			if err := os.WriteFile(configFile, []byte(tt.configData), 0o600); err != nil {
				t.Fatalf("failed to write test config file: %v", err)
			}

			cfg, err := NewConfig(configFile)

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
				if cfg == nil {
					t.Error("expected non-nil config")
				}
			}
		})
	}
}

func TestAgentValidate(t *testing.T) {
	tests := []struct {
		name        string
		config      Agent
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid config",
			config: Agent{
				ServerURL:      "https://garm.example.com/agent",
				Token:          "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJpc19hZ2VudCI6dHJ1ZSwiZm9yZ2VfdHlwZSI6ImdpdGh1YiJ9.dummysignature",
				WorkDir:        "/opt/runner",
				RunnerExecArgs: []string{"/opt/runner/run.sh"},
			},
			expectError: false,
		},
		{
			name: "invalid url",
			config: Agent{
				ServerURL:      "not a url",
				Token:          "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJpc19hZ2VudCI6dHJ1ZSwiZm9yZ2VfdHlwZSI6ImdpdGh1YiJ9.dummysignature",
				RunnerExecArgs: []string{"/opt/runner/run.sh"},
			},
			expectError: true,
			errorMsg:    "invalid server_url",
		},
		{
			name: "empty token",
			config: Agent{
				ServerURL:      "https://garm.example.com/agent",
				Token:          "",
				RunnerExecArgs: []string{"/opt/runner/run.sh"},
			},
			expectError: true,
			errorMsg:    "missing token",
		},
		{
			name: "empty runner exec args",
			config: Agent{
				ServerURL: "https://garm.example.com/agent",
				Token:     "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJpc19hZ2VudCI6dHJ1ZSwiZm9yZ2VfdHlwZSI6ImdpdGh1YiJ9.dummysignature",
			},
			expectError: true,
			errorMsg:    "runner_cmdline is not set",
		},
		{
			name: "non-agent token",
			config: Agent{
				ServerURL:      "https://garm.example.com/agent",
				Token:          "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJpc19hZ2VudCI6ZmFsc2UsImZvcmdlX3R5cGUiOiJnaXRodWIifQ.dummysignature",
				RunnerExecArgs: []string{"/opt/runner/run.sh"},
			},
			expectError: true,
			errorMsg:    "token is not agent scoped",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errorMsg)
				} else if !contains(err.Error(), tt.errorMsg) {
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

func TestAgentTokenClaims(t *testing.T) {
	tests := []struct {
		name        string
		token       string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "valid agent token",
			token:       "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJpc19hZ2VudCI6dHJ1ZSwiZm9yZ2VfdHlwZSI6ImdpdGh1YiJ9.dummysignature",
			expectError: false,
		},
		{
			name:        "non-agent token",
			token:       "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJpc19hZ2VudCI6ZmFsc2UsImZvcmdlX3R5cGUiOiJnaXRodWIifQ.dummysignature",
			expectError: true,
			errorMsg:    "token is not agent scoped",
		},
		{
			name:        "invalid token format",
			token:       "invalid.token",
			expectError: true,
			errorMsg:    "failed to parse JWT token",
		},
		{
			name:        "empty token",
			token:       "",
			expectError: true,
			errorMsg:    "failed to parse JWT token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agent := Agent{Token: tt.token}
			claims, err := agent.TokenClaims()

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errorMsg)
				} else if !contains(err.Error(), tt.errorMsg) {
					t.Errorf("expected error containing %q, got %q", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if !claims.IsAgent {
					t.Error("expected IsAgent to be true")
				}
			}
		})
	}
}

func TestAgentForgeType(t *testing.T) {
	tests := []struct {
		name        string
		token       string
		expectType  string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "github forge type",
			token:       "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJpc19hZ2VudCI6dHJ1ZSwiZm9yZ2VfdHlwZSI6ImdpdGh1YiJ9.dummysignature",
			expectType:  "github",
			expectError: false,
		},
		{
			name:        "gitea forge type",
			token:       "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJpc19hZ2VudCI6dHJ1ZSwiZm9yZ2VfdHlwZSI6ImdpdGVhIn0.dummysignature",
			expectType:  "gitea",
			expectError: false,
		},
		{
			name:        "invalid forge type",
			token:       "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJpc19hZ2VudCI6dHJ1ZSwiZm9yZ2VfdHlwZSI6ImludmFsaWQifQ.dummysignature",
			expectError: true,
			errorMsg:    "invalid forge type",
		},
		{
			name:        "non-agent token",
			token:       "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJpc19hZ2VudCI6ZmFsc2UsImZvcmdlX3R5cGUiOiJnaXRodWIifQ.dummysignature",
			expectError: true,
			errorMsg:    "token is not agent scoped",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agent := Agent{Token: tt.token}
			forgeType, err := agent.ForgeType()

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errorMsg)
				} else if !contains(err.Error(), tt.errorMsg) {
					t.Errorf("expected error containing %q, got %q", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if string(forgeType) != tt.expectType {
					t.Errorf("expected forge type %q, got %q", tt.expectType, forgeType)
				}
			}
		})
	}
}

func TestNewConfigFileNotFound(t *testing.T) {
	_, err := NewConfig("/nonexistent/config.toml")
	if err == nil {
		t.Error("expected error for non-existent config file")
	}
	if !contains(err.Error(), "error decoding toml") {
		t.Errorf("expected error about toml decoding, got: %v", err)
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
