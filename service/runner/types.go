package runner

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/cloudbase/garm/params"
)

// These patterns match state-transition lines in the runner's stdout/stderr.
// They are intentionally unanchored (no ^/$) because the exact log-line prefix
// differs across runner versions and may include timestamps. The leading/trailing
// .* that were here previously were redundant — Go's regexp does substring
// matching already. A false positive (e.g. a job step echoing "Running job:")
// is theoretically possible but unlikely in practice since the runner does not
// forward job-step output on its own stdout.
var (
	giteaJobStartedRegex  = regexp.MustCompile(`task [0-9]+ repo is `)
	githubJobStartedRegex = regexp.MustCompile(` Running job: `)

	githubListenForJobs = regexp.MustCompile(`Listening for Jobs`)
	giteaListenForJobs  = regexp.MustCompile(`runner: .*, declare successfully`)
)

type Worker interface {
	AgentID() uint
	Start() error
	Stop() error
	Wait() <-chan struct{}
	Error() error
}

type StateManager interface {
	SetRunnerStarted(st bool)
	SetJobStarted()
	SetJobFinished()
}

type Config interface {
	GetAgentID() uint
	GetAgentName() string
	IsEphemeral() bool
	GetServerURL() string
}

func (r *runnerCmd) isJobStartedLine(msg []byte) bool {
	switch r.forgeType {
	case params.GiteaEndpointType:
		return giteaJobStartedRegex.Match(msg)
	case params.GithubEndpointType:
		return githubJobStartedRegex.Match(msg)
	default:
		return false
	}
}

func (r *runnerCmd) isRunnerStartedLine(msg []byte) bool {
	switch r.forgeType {
	case params.GiteaEndpointType:
		return giteaListenForJobs.Match(msg)
	case params.GithubEndpointType:
		return githubListenForJobs.Match(msg)
	default:
		return false
	}
}

func NewRunnerConfig(cfg string, forgeType params.EndpointType) (Config, error) {
	data, err := os.ReadFile(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to read runner config: %w", err)
	}
	var runCfg Config
	switch forgeType {
	case params.GiteaEndpointType:
		var giteaCfg GiteaRunnerConfig
		if err = json.Unmarshal(data, &giteaCfg); err != nil {
			return nil, fmt.Errorf("failed to unmarshal gitea runner config: %w", err)
		}
		runCfg = giteaCfg
	case params.GithubEndpointType:
		var githubCfg GitHubRunnerConfig
		if err = json.Unmarshal(data, &githubCfg); err != nil {
			return nil, fmt.Errorf("failed to unmarshal github runner config: %w", err)
		}
		runCfg = githubCfg
	default:
		return nil, fmt.Errorf("unknown forge type %s", forgeType)
	}
	if runCfg.GetAgentID() == 0 {
		return nil, fmt.Errorf("runner config has no valid agent ID (parsed as 0)")
	}
	return runCfg, nil
}

type GitHubRunnerConfig struct {
	AgentID   string `json:"agentId"`
	AgentName string `json:"agentName"`
	Ephemeral string `json:"ephemeral"`
	ServerURL string `json:"serverUrl"`
}

func (r GitHubRunnerConfig) GetAgentID() uint {
	asUint, err := strconv.ParseUint(r.AgentID, 10, 32)
	if err != nil {
		return 0
	}
	return uint(asUint)
}

func (r GitHubRunnerConfig) GetAgentName() string {
	return r.AgentName
}

func (r GitHubRunnerConfig) IsEphemeral() bool {
	return strings.EqualFold(r.Ephemeral, "true")
}

func (r GitHubRunnerConfig) GetServerURL() string {
	return r.ServerURL
}

type GiteaRunnerConfig struct {
	AgentID   uint   `json:"id"`
	AgentName string `json:"name"`
	Ephemeral bool   `json:"ephemeral"`
	ServerURL string `json:"address"`
}

func (r GiteaRunnerConfig) GetAgentID() uint {
	return r.AgentID
}

func (r GiteaRunnerConfig) GetAgentName() string {
	return r.AgentName
}

func (r GiteaRunnerConfig) IsEphemeral() bool {
	return r.Ephemeral
}

func (r GiteaRunnerConfig) GetServerURL() string {
	return r.ServerURL
}
