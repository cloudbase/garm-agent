package config

import (
	"errors"
	"fmt"
	"net/url"

	"github.com/BurntSushi/toml"
	jwt "github.com/golang-jwt/jwt/v5"

	"github.com/cloudbase/garm/auth"
	"github.com/cloudbase/garm/params"
)

// NewConfig returns a new Config
func NewConfig(cfgFile string) (*Agent, error) {
	var config Agent
	if _, err := toml.DecodeFile(cfgFile, &config); err != nil {
		return nil, fmt.Errorf("error decoding toml: %w", err)
	}
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("error validating config: %w", err)
	}
	return &config, nil
}

type Agent struct {
	// ServerURL is the agent endpoint on the GARM server that the agent needs to connect to.
	// Typically whis will be https://garm.example.com/agent.
	ServerURL string `toml:"server_url"`
	Token     string `toml:"token"`
	// WorkDir is the folder in which the runner will execute workflows.
	// In the case of github, it is expected that the WorkDir is also the installation
	// dir of the runner.
	WorkDir     string `toml:"work_dir"`
	LogFile     string `toml:"log_file"`
	Shell       string `toml:"shell"`
	EnableShell bool   `toml:"enable_shell"`
	// RunnerExecArgs is a list of command line parameters needed to launch the runner.
	// This must include the executable as the first arg and any needed subsequent parametes.
	// If the runner is started via a wrapper script, the fist argument must be the proper interpreter
	// that can launch the wrapper script. For example, if the runner is launched by a bash script, the
	// RunnerExecArgs will need to include: ["/bin/bash", "-C", "/path/to/script.sh", "any", "other", "args"].
	// If the runner is an ELF or a Windows executable, the first arg can be the runner itself.
	RunnerExecArgs []string `toml:"runner_cmdline"`
	// StateDBPath is the path on disk to the bbold DB file where the agent saves some state. Currently
	// the agent uses this file to store whether or not it has detected that a job was run. A runner may
	// pick up a job and run it while the connection between GARM and the agent was down. In which case,
	// the agent might not have notified GARM that it needs to be removed. We save state in this file for 2
	// reasond:
	//   * If a job runs and the agent gets restarted, it must not attempt to start the runner again
	//   * It needs to let GARM know that it can no longer be used and needs to be removed.
	//
	// The parent folder must allow tha agent process write access and expected to persist across reboots.
	StateDBPath string `toml:"state_db_path"`
}

func (a *Agent) Validate() error {
	if _, err := url.ParseRequestURI(a.ServerURL); err != nil {
		return fmt.Errorf("invalid server_url: %w", err)
	}

	if a.Token == "" {
		return fmt.Errorf("missing token")
	}

	if _, err := a.TokenClaims(); err != nil {
		return fmt.Errorf("failed to parse token: %w", err)
	}

	if len(a.RunnerExecArgs) == 0 {
		return fmt.Errorf("runner_cmdline is not set")
	}

	return nil
}

func (a *Agent) TokenClaims() (auth.InstanceJWTClaims, error) {
	claims := auth.InstanceJWTClaims{}
	_, err := jwt.ParseWithClaims(a.Token, &claims, nil)
	if err != nil && !errors.Is(err, jwt.ErrTokenUnverifiable) {
		return auth.InstanceJWTClaims{}, fmt.Errorf("failed to parse JWT token: %w", err)
	}

	if !claims.IsAgent {
		return auth.InstanceJWTClaims{}, fmt.Errorf("token is not agent scoped")
	}
	return claims, nil
}

func (a *Agent) ForgeType() (params.EndpointType, error) {
	claims, err := a.TokenClaims()
	if err != nil {
		return "", fmt.Errorf("failed to get token claims: %w", err)
	}
	forgeType := params.EndpointType(claims.ForgeType)
	switch forgeType {
	case params.GithubEndpointType, params.GiteaEndpointType:
	default:
		return "", fmt.Errorf("invalid forge type: %s", forgeType)
	}
	return forgeType, nil
}
