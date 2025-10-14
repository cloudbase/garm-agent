package runner

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cloudbase/garm/params"
)

func TestNewRunnerConfig(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name        string
		forgeType   params.EndpointType
		configData  string
		expectError bool
		errorMsg    string
	}{
		{
			name:       "valid github config",
			forgeType:  params.GithubEndpointType,
			configData: `{"agentId":"123","agentName":"test-runner","ephemeral":"True","serverUrl":"https://github.com"}`,
			expectError: false,
		},
		{
			name:       "valid gitea config",
			forgeType:  params.GiteaEndpointType,
			configData: `{"id":456,"name":"gitea-runner","ephemeral":true,"address":"https://gitea.com"}`,
			expectError: false,
		},
		{
			name:        "invalid json",
			forgeType:   params.GithubEndpointType,
			configData:  `{"agentId":"123",invalid}`,
			expectError: true,
			errorMsg:    "failed to unmarshal",
		},
		{
			name:        "unsupported forge type",
			forgeType:   "unsupported",
			configData:  `{"agentId":"123"}`,
			expectError: true,
			errorMsg:    "unknown forge type",
		},
		{
			name:        "file not found",
			forgeType:   params.GithubEndpointType,
			configData:  "",
			expectError: true,
			errorMsg:    "failed to read runner config",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var configFile string
			if tt.configData != "" {
				configFile = filepath.Join(tmpDir, tt.name+".runner")
				if err := os.WriteFile(configFile, []byte(tt.configData), 0o600); err != nil {
					t.Fatalf("failed to write config file: %v", err)
				}
			} else {
				configFile = "/nonexistent/config"
			}

			cfg, err := NewRunnerConfig(configFile, tt.forgeType)

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

func TestGitHubRunnerConfig(t *testing.T) {
	tests := []struct {
		name            string
		config          GitHubRunnerConfig
		expectedAgentID uint
		expectedName    string
		expectedEphem   bool
		expectedURL     string
	}{
		{
			name: "valid config",
			config: GitHubRunnerConfig{
				AgentID:   "123",
				AgentName: "test-runner",
				Ephemeral: "True",
				ServerURL: "https://github.com",
			},
			expectedAgentID: 123,
			expectedName:    "test-runner",
			expectedEphem:   true,
			expectedURL:     "https://github.com",
		},
		{
			name: "non-ephemeral",
			config: GitHubRunnerConfig{
				AgentID:   "456",
				AgentName: "persistent-runner",
				Ephemeral: "False",
				ServerURL: "https://github.com",
			},
			expectedAgentID: 456,
			expectedName:    "persistent-runner",
			expectedEphem:   false,
			expectedURL:     "https://github.com",
		},
		{
			name: "invalid agent id",
			config: GitHubRunnerConfig{
				AgentID:   "invalid",
				AgentName: "test-runner",
				Ephemeral: "True",
				ServerURL: "https://github.com",
			},
			expectedAgentID: 0,
			expectedName:    "test-runner",
			expectedEphem:   true,
			expectedURL:     "https://github.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if id := tt.config.GetAgentID(); id != tt.expectedAgentID {
				t.Errorf("expected AgentID %d, got %d", tt.expectedAgentID, id)
			}

			if name := tt.config.GetAgentName(); name != tt.expectedName {
				t.Errorf("expected AgentName %q, got %q", tt.expectedName, name)
			}

			if ephem := tt.config.IsEphemeral(); ephem != tt.expectedEphem {
				t.Errorf("expected Ephemeral %v, got %v", tt.expectedEphem, ephem)
			}

			if url := tt.config.GetServerURL(); url != tt.expectedURL {
				t.Errorf("expected ServerURL %q, got %q", tt.expectedURL, url)
			}
		})
	}
}

func TestGiteaRunnerConfig(t *testing.T) {
	tests := []struct {
		name            string
		config          GiteaRunnerConfig
		expectedAgentID uint
		expectedName    string
		expectedEphem   bool
		expectedURL     string
	}{
		{
			name: "valid config",
			config: GiteaRunnerConfig{
				AgentID:   789,
				AgentName: "gitea-runner",
				Ephemeral: true,
				ServerURL: "https://gitea.com",
			},
			expectedAgentID: 789,
			expectedName:    "gitea-runner",
			expectedEphem:   true,
			expectedURL:     "https://gitea.com",
		},
		{
			name: "non-ephemeral",
			config: GiteaRunnerConfig{
				AgentID:   321,
				AgentName: "persistent-gitea",
				Ephemeral: false,
				ServerURL: "https://gitea.example.com",
			},
			expectedAgentID: 321,
			expectedName:    "persistent-gitea",
			expectedEphem:   false,
			expectedURL:     "https://gitea.example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if id := tt.config.GetAgentID(); id != tt.expectedAgentID {
				t.Errorf("expected AgentID %d, got %d", tt.expectedAgentID, id)
			}

			if name := tt.config.GetAgentName(); name != tt.expectedName {
				t.Errorf("expected AgentName %q, got %q", tt.expectedName, name)
			}

			if ephem := tt.config.IsEphemeral(); ephem != tt.expectedEphem {
				t.Errorf("expected Ephemeral %v, got %v", tt.expectedEphem, ephem)
			}

			if url := tt.config.GetServerURL(); url != tt.expectedURL {
				t.Errorf("expected ServerURL %q, got %q", tt.expectedURL, url)
			}
		})
	}
}

func TestGitHubRunnerConfigParsing(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, ".runner")

	configData := `{
		"agentId": "987",
		"agentName": "integration-test-runner",
		"ephemeral": "True",
		"serverUrl": "https://api.github.com"
	}`

	if err := os.WriteFile(configFile, []byte(configData), 0o600); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	cfg, err := NewRunnerConfig(configFile, params.GithubEndpointType)
	if err != nil {
		t.Fatalf("failed to parse config: %v", err)
	}

	if cfg.GetAgentID() != 987 {
		t.Errorf("expected AgentID 987, got %d", cfg.GetAgentID())
	}

	if cfg.GetAgentName() != "integration-test-runner" {
		t.Errorf("expected AgentName 'integration-test-runner', got %q", cfg.GetAgentName())
	}

	if !cfg.IsEphemeral() {
		t.Error("expected Ephemeral to be true")
	}

	if cfg.GetServerURL() != "https://api.github.com" {
		t.Errorf("expected ServerURL 'https://api.github.com', got %q", cfg.GetServerURL())
	}
}

func TestGiteaRunnerConfigParsing(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, ".runner")

	configData := `{
		"id": 654,
		"name": "gitea-integration-runner",
		"ephemeral": false,
		"address": "https://gitea.internal"
	}`

	if err := os.WriteFile(configFile, []byte(configData), 0o600); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	cfg, err := NewRunnerConfig(configFile, params.GiteaEndpointType)
	if err != nil {
		t.Fatalf("failed to parse config: %v", err)
	}

	if cfg.GetAgentID() != 654 {
		t.Errorf("expected AgentID 654, got %d", cfg.GetAgentID())
	}

	if cfg.GetAgentName() != "gitea-integration-runner" {
		t.Errorf("expected AgentName 'gitea-integration-runner', got %q", cfg.GetAgentName())
	}

	if cfg.IsEphemeral() {
		t.Error("expected Ephemeral to be false")
	}

	if cfg.GetServerURL() != "https://gitea.internal" {
		t.Errorf("expected ServerURL 'https://gitea.internal', got %q", cfg.GetServerURL())
	}
}

func TestJobStartedRegex(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		expected bool
	}{
		{
			name:     "github job started pattern",
			line:     "2024-01-01 12:00:00 Running job: Build and Test",
			expected: true,
		},
		{
			name:     "github invalid pattern",
			line:     "Listening for Jobs",
			expected: false,
		},
		{
			name:     "gitea job started pattern",
			line:     "task 42 repo is owner/repository",
			expected: true,
		},
		{
			name:     "gitea invalid pattern",
			line:     "runner: test, declare successfully",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var result bool
			if contains(tt.name, "github") {
				result = githubJobStartedRegex.MatchString(tt.line)
			} else {
				result = giteaJobStartedRegex.MatchString(tt.line)
			}

			if result != tt.expected {
				t.Errorf("expected %v for line %q, got %v", tt.expected, tt.line, result)
			}
		})
	}
}

func TestListenForJobsRegex(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		expected bool
	}{
		{
			name:     "github listen pattern",
			line:     "Listening for Jobs",
			expected: true,
		},
		{
			name:     "github invalid pattern",
			line:     "Starting runner",
			expected: false,
		},
		{
			name:     "gitea listen pattern",
			line:     "runner: my-runner, declare successfully",
			expected: true,
		},
		{
			name:     "gitea invalid pattern",
			line:     "task 1 repo is test",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var result bool
			if contains(tt.name, "github") {
				result = githubListenForJobs.MatchString(tt.line)
			} else {
				result = giteaListenForJobs.MatchString(tt.line)
			}

			if result != tt.expected {
				t.Errorf("expected %v for line %q, got %v", tt.expected, tt.line, result)
			}
		})
	}
}
