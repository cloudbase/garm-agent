package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/cloudbase/garm-agent/config"
	"github.com/cloudbase/garm-agent/service/runner"
	"github.com/cloudbase/garm-agent/state"
	garmWs "github.com/cloudbase/garm-provider-common/util/websocket"
	"github.com/cloudbase/garm/params"
	"github.com/cloudbase/garm/workers/websocket/agent/messaging"
)

var closed = make(chan struct{})

func init() { close(closed) }

func NewService(ctx context.Context, cfg *config.Agent) (*Service, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("failed to validate agent config: %w", err)
	}

	forgeType, err := cfg.ForgeType()
	if err != nil {
		return nil, fmt.Errorf("failed to get forge type for agent: %w", err)
	}

	agentState, err := state.NewManager(cfg.StateDBPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create state manager: %w", err)
	}
	return &Service{
		ctx:        ctx,
		cfg:        cfg,
		done:       closed,
		connecting: make(chan struct{}),
		connected:  closed,
		forgeType:  forgeType,
		sessions:   make(map[string]*ShellSession),
		hasShell:   cfg.EnableShell && HasPTY(),
		agentState: agentState,
	}, nil
}

type Service struct {
	ctx         context.Context
	cfg         *config.Agent
	cli         *garmWs.Reader
	agentState  *state.Manager
	runnerAlive bool
	runnerCmd   runner.Worker
	hasShell    bool

	forgeType params.EndpointType

	mux     sync.Mutex
	cliMux  sync.Mutex
	running bool
	done    chan struct{}

	connecting chan struct{}
	connected  chan struct{}

	sessions map[string]*ShellSession
}

func (s *Service) Done() chan struct{} {
	return s.done
}

func (s *Service) getClient() (*garmWs.Reader, error) {
	s.cliMux.Lock()
	cli := s.cli
	s.cliMux.Unlock()

	if cli == nil {
		return nil, fmt.Errorf("websocket client not connected")
	}
	return cli, nil
}

func (s *Service) writeMessage(msg []byte) error {
	cli, err := s.getClient()
	if err != nil {
		return err
	}

	if err := cli.WriteMessage(websocket.BinaryMessage, msg); err != nil {
		return err
	}
	return nil
}

func (s *Service) sendReadyMessage(sessionID [16]byte, isError byte, message []byte) error {
	readyMsg := messaging.ShellReadyMessage{
		SessionID: sessionID,
		IsError:   isError,
		Message:   message,
	}

	return s.writeMessage(readyMsg.Marshal())
}

func (s *Service) handleCreateShell(agentMsg messaging.AgentMessage) (err error) {
	createShell, err := messaging.Unmarshal[messaging.CreateShellMessage](agentMsg)
	if err != nil {
		return fmt.Errorf("failed to unmarshall create shell message: %w", err)
	}
	slog.InfoContext(s.ctx, "handling create shell message", "session_id", createShell.ID())
	defer func() {
		if err != nil {
			errMsg := fmt.Appendf(nil, "failed to create shell: %q", err)
			if innerErr := s.sendReadyMessage(createShell.SessionID, 1, errMsg); innerErr != nil {
				slog.ErrorContext(s.ctx, "failed to send error message", "error", innerErr)
			}
		}
	}()

	sessionID := createShell.ID()
	if sessionID == "" {
		if msgErr := s.sendReadyMessage(createShell.SessionID, 1, []byte("failed to parse session ID")); msgErr != nil {
			slog.ErrorContext(s.ctx, "failed to send error message", "error", msgErr)
		}
		return fmt.Errorf("failed to parse session ID")
	}
	s.mux.Lock()
	if _, ok := s.sessions[sessionID]; ok {
		s.mux.Unlock()
		if msgErr := s.sendReadyMessage(createShell.SessionID, 1, fmt.Appendf(nil, "session ID %s already exists", sessionID)); msgErr != nil {
			slog.ErrorContext(s.ctx, "failed to send error message", "error", msgErr)
		}
		return fmt.Errorf("session ID %s already exists", sessionID)
	}
	session, err := NewShellSession(s.ctx, createShell, s.writeMessage, s.cfg)
	if err != nil {
		s.mux.Unlock()
		if msgErr := s.sendReadyMessage(createShell.SessionID, 1, fmt.Appendf(nil, "failed to create session: %s", err)); msgErr != nil {
			slog.ErrorContext(s.ctx, "failed to send error message", "error", msgErr)
		}
		return fmt.Errorf("failed to create session: %w", err)
	}
	if err := session.Start(); err != nil {
		s.mux.Unlock()
		if msgErr := s.sendReadyMessage(createShell.SessionID, 1, fmt.Appendf(nil, "failed to start session: %s", err)); msgErr != nil {
			slog.ErrorContext(s.ctx, "failed to send error message", "error", msgErr)
		}
		return fmt.Errorf("failed to start session: %w", err)
	}
	s.sessions[sessionID] = session
	go func(sessionID string) {
		select {
		case <-s.ctx.Done():
		case <-s.done:
		case <-session.Done():
		}
		s.mux.Lock()
		delete(s.sessions, sessionID)
		s.mux.Unlock()
	}(sessionID)
	s.mux.Unlock()
	return nil
}

func (s *Service) handleShellResize(agentMsg messaging.AgentMessage) error {
	resizeMsg, err := messaging.Unmarshal[messaging.ShellResizeMessage](agentMsg)
	if err != nil {
		return fmt.Errorf("failed to unmarshall shell resize message: %w", err)
	}
	if !s.hasShell {
		if msgErr := s.sendReadyMessage(resizeMsg.SessionID, 1, []byte("shell is disabled")); msgErr != nil {
			slog.ErrorContext(s.ctx, "failed to send error message", "error", msgErr)
		}
		return nil
	}

	s.mux.Lock()
	session, ok := s.sessions[resizeMsg.ID()]
	if !ok {
		s.mux.Unlock()
		return nil
	}
	if err := session.shell.Resize(resizeMsg.Cols, resizeMsg.Rows); err != nil {
		s.mux.Unlock()
		return fmt.Errorf("failed to resize shell: %w", err)
	}
	s.mux.Unlock()
	return nil
}

func (s *Service) handleClientShellClosed(agentMsg messaging.AgentMessage) error {
	closedMsg, err := messaging.Unmarshal[messaging.ClientShellClosedMessage](agentMsg)
	if err != nil {
		return fmt.Errorf("failed to unmarshall shell closed message: %w", err)
	}
	if !s.hasShell {
		if msgErr := s.sendReadyMessage(closedMsg.SessionID, 1, []byte("shell is disabled")); msgErr != nil {
			slog.ErrorContext(s.ctx, "failed to send error message", "error", msgErr)
		}
		return nil
	}

	slog.InfoContext(s.ctx, "handling close shell message", "session_id", closedMsg.ID())
	s.mux.Lock()
	session, ok := s.sessions[closedMsg.ID()]
	if !ok {
		s.mux.Unlock()
		return nil
	}
	if err := session.Stop(); err != nil {
		s.mux.Unlock()
		return fmt.Errorf("failed to close session: %w", err)
	}
	s.mux.Unlock()
	return nil
}

func (s *Service) handleShellData(agentMsg messaging.AgentMessage) error {
	shellData, err := messaging.Unmarshal[messaging.ShellDataMessage](agentMsg)
	if err != nil {
		return fmt.Errorf("failed to unmarshall shell data message: %w", err)
	}
	if !s.hasShell {
		if msgErr := s.sendReadyMessage(shellData.SessionID, 1, []byte("shell is disabled")); msgErr != nil {
			slog.ErrorContext(s.ctx, "failed to send error message", "error", msgErr)
		}
		return nil
	}

	s.mux.Lock()
	session, ok := s.sessions[shellData.ID()]
	if !ok {
		s.mux.Unlock()
		return nil
	}
	if _, err := session.shell.Write(shellData.Data); err != nil {
		slog.ErrorContext(s.ctx, "failed to write shell data; stopping session", "error", err, "session_id", shellData.ID())
		if err := session.Stop(); err != nil {
			s.mux.Unlock()
			return fmt.Errorf("failed to stop session %s", shellData.ID())
		}
	}
	s.mux.Unlock()
	return nil
}

func (s *Service) handleMessage(msgType int, msg []byte) (err error) {
	if msgType != websocket.BinaryMessage && msgType != websocket.TextMessage {
		slog.InfoContext(s.ctx, "ignoring invalid message type", "message_type", msgType)
		return nil
	}

	agentMsg, err := messaging.UnmarshalAgentMessage(msg)
	if err != nil {
		slog.ErrorContext(s.ctx, "failed to unmarshal agent message", "error", err)
		return fmt.Errorf("failed to unmarshal agent message")
	}

	switch agentMsg.Type {
	case messaging.MessageTypeCreateShell:
		return s.handleCreateShell(agentMsg)
	case messaging.MessageTypeShellResize:
		return s.handleShellResize(agentMsg)
	case messaging.MessageTypeClientShellClosed:
		return s.handleClientShellClosed(agentMsg)
	case messaging.MessageTypeShellData:
		return s.handleShellData(agentMsg)
	}
	return nil
}

func (s *Service) Start() error {
	s.mux.Lock()
	defer s.mux.Unlock()

	if s.running {
		return nil
	}

	if s.cfg.WorkDir != "" {
		if mode, err := os.Stat(s.cfg.WorkDir); err == nil {
			if mode.IsDir() {
				if chdirErr := os.Chdir(s.cfg.WorkDir); chdirErr != nil {
					slog.ErrorContext(s.ctx, "failed to change directory", "work_dir", s.cfg.WorkDir, "error", chdirErr)
				}
			}
		} else {
			slog.ErrorContext(s.ctx, "failed to access work_dir", "work_dir", s.cfg.WorkDir, "error", err)
			return err
		}
	}

	s.running = true
	s.done = make(chan struct{})
	go s.keepAliveLoop()
	go s.loop()
	go s.keepRunnerAlive()

	return nil
}

func (s *Service) Stop() error {
	s.mux.Lock()
	defer s.mux.Unlock()

	if !s.running {
		return nil
	}

	close(s.done)
	s.running = false
	if s.cli != nil {
		s.cli.Stop()
	}
	return nil
}

func (s *Service) determineRunnerState(runnerAlive bool) params.RunnerStatus {
	state := params.RunnerOffline
	if runnerAlive {
		state = params.RunnerIdle
	}

	st, err := s.agentState.GetState()
	if err != nil {
		slog.ErrorContext(s.ctx, "failed to get state", "error", err)
		return state
	}
	if st.JobStarted {
		if !runnerAlive {
			// We're coming back online and for some reason, we didn't record
			// that the job was finished, but we did record that the job was started.
			// If the job was started but the runner is offline, then the job was either
			// finished, or canceled.
			state = params.RunnerTerminated
		} else {
			state = params.RunnerActive
		}
	}

	if st.JobFinished {
		state = params.RunnerTerminated
	}

	return state
}

// snapshot reads the runner's agent ID and liveness under s.mux. ok is false
// when the runner command has not been created yet (s.runnerCmd is nil), in
// which case there is no identity to report and callers should skip sending
// status/heartbeats. Safe to call without holding s.mux; callers that already
// hold s.mux must read the fields directly, since sync.Mutex is not reentrant.
func (s *Service) snapshot() (id uint, runnerAlive bool, ok bool) {
	s.mux.Lock()
	defer s.mux.Unlock()
	if s.runnerCmd == nil {
		return 0, s.runnerAlive, false
	}
	return s.runnerCmd.AgentID(), s.runnerAlive, true
}

// isRunnerAlive reports whether the runner is currently up, read under s.mux.
func (s *Service) isRunnerAlive() bool {
	s.mux.Lock()
	defer s.mux.Unlock()
	return s.runnerAlive
}

func (s *Service) sendRunnerStatus(agentID uint, runnerAlive bool) {
	state := s.determineRunnerState(runnerAlive)
	status := params.InstanceUpdateMessage{
		Status:  state,
		Message: fmt.Sprintf("Agent update status to: %s", state),
	}
	if err := s.sendRunnerStatusMessage(agentID, status); err != nil {
		slog.ErrorContext(s.ctx, "failed to send status", "error", err)
	}
}

func (s *Service) sendRunnerStatusMessage(agentID uint, status params.InstanceUpdateMessage) error {
	// GARM's InstanceUpdateMessage carries the agent ID as a signed *int64, so
	// guard the unsigned->signed conversion against overflow. Real agent IDs
	// fit comfortably; an out-of-range value indicates a malformed ID.
	if uint64(agentID) > math.MaxInt64 {
		return fmt.Errorf("agent ID %d does not fit in int64", agentID)
	}
	signedID := int64(agentID)
	status.AgentID = &signedID

	asJs, err := json.Marshal(status)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}
	msg := messaging.RunnerUpdateMessage{
		AgentID: uint64(agentID),
		Payload: asJs,
	}

	cli, err := s.getClient()
	if err != nil {
		return err
	}

	if err := cli.WriteMessage(websocket.BinaryMessage, msg.Marshal()); err != nil {
		return fmt.Errorf("failed to send runner status: %w", err)
	}
	return nil
}

func (s *Service) SetRunnerStarted(st bool) {
	s.mux.Lock()
	defer s.mux.Unlock()

	s.runnerAlive = st
	if s.runnerCmd == nil {
		return
	}
	s.sendRunnerStatus(s.runnerCmd.AgentID(), s.runnerAlive)
}

func (s *Service) SetJobStarted() {
	s.mux.Lock()
	defer s.mux.Unlock()
	if err := s.agentState.SetJobStarted(); err != nil {
		slog.ErrorContext(s.ctx, "failed to set job started", "error", err)
	}
	if s.runnerCmd == nil {
		return
	}
	status := params.InstanceUpdateMessage{
		Status:  params.RunnerActive,
		Message: "runner is now executing a job",
	}
	if err := s.sendRunnerStatusMessage(s.runnerCmd.AgentID(), status); err != nil {
		slog.ErrorContext(s.ctx, "failed to send status", "error", err)
	}
}

func (s *Service) SetJobFinished() {
	s.mux.Lock()
	defer s.mux.Unlock()
	if err := s.agentState.SetJobFinished(); err != nil {
		slog.ErrorContext(s.ctx, "failed to set job finished", "error", err)
	}

	if s.runnerCmd == nil {
		return
	}
	status := params.InstanceUpdateMessage{
		Status:  params.RunnerTerminated,
		Message: "Job execution has finished",
	}
	if err := s.sendRunnerStatusMessage(s.runnerCmd.AgentID(), status); err != nil {
		slog.ErrorContext(s.ctx, "failed to send status", "error", err)
	}
}

func (s *Service) sendHeartbeat(agentID uint) error {
	msg := messaging.RunnerHeartbetMessage{
		AgentID: uint64(agentID),
	}

	agentCap := params.AgentCapabilities{
		Shell: s.hasShell,
	}
	asJs, err := json.Marshal(agentCap)
	if err != nil {
		return fmt.Errorf("failed to marshal capabilities: %w", err)
	}
	msg.Payload = asJs

	cli, err := s.getClient()
	if err != nil {
		return err
	}

	if err := cli.WriteMessage(websocket.BinaryMessage, msg.Marshal()); err != nil {
		return fmt.Errorf("failed to send heartbeat: %w", err)
	}
	return nil
}

func (s *Service) sleepWithCancel(d time.Duration) (shouldQuit bool) {
	sleepTicker := time.NewTicker(d)
	defer sleepTicker.Stop()

	select {
	case <-sleepTicker.C:
		return false
	case <-s.done:
	case <-s.ctx.Done():
	}
	return true
}

func (s *Service) keepRunnerAlive() {
retryCreate:
	state := s.determineRunnerState(s.isRunnerAlive())
	if state == params.RunnerTerminated {
		// no need for this goroutine.
		return
	}
	runnerCommand, err := runner.NewRunnerCommand(s.ctx, s.cfg.RunnerExecArgs, s.cfg.WorkDir, s.forgeType, s)
	if err != nil {
		slog.ErrorContext(s.ctx, "failed to create runner command", "error", err)
		if s.sleepWithCancel(5 * time.Second) {
			return
		}
		goto retryCreate
	}
	s.mux.Lock()
	s.runnerCmd = runnerCommand
	s.mux.Unlock()
	defer func() {
		if stopErr := runnerCommand.Stop(); stopErr != nil {
			slog.ErrorContext(s.ctx, "failed to stop runner command", "error", stopErr)
		}
		select {
		case <-time.After(2 * time.Second):
			return
		case <-runnerCommand.Wait():
			return
		}
	}()

	retryCount := 0

retryStart:
	if retryCount > 5 {
		slog.WarnContext(s.ctx, "max retry reached", "max_retries", 5)
		return
	}
	runnerState := s.determineRunnerState(s.isRunnerAlive())
	if runnerState == params.RunnerTerminated {
		// we only attepmt to start the runner if we need to. A runner that has already run a job,
		// should not be started again, even if the agent is still online.
		return
	}
	if err := runnerCommand.Start(); err != nil {
		slog.ErrorContext(s.ctx, "failed to start runner", "error", err)
		retryCount++
		runnerState := s.determineRunnerState(s.isRunnerAlive())
		if runnerState == params.RunnerOffline {
			// The runner did not run a job as far as we know, but it's failing to start. Send a failed message to GARM.
			status := params.InstanceUpdateMessage{
				Status:  params.RunnerFailed,
				Message: fmt.Sprintf("Runner failed to start: %s", runnerCommand.Error()),
			}
			if statusErr := s.sendRunnerStatusMessage(runnerCommand.AgentID(), status); statusErr != nil {
				slog.ErrorContext(s.ctx, "failed to send runner status", "error", statusErr)
			}
		}
		if s.sleepWithCancel(5 * time.Second) {
			return
		}
		goto retryStart
	}
	retryCount = 0

	for {
		select {
		case <-s.done:
			return
		case <-s.ctx.Done():
			return
		case <-runnerCommand.Wait():
			if s.determineRunnerState(s.isRunnerAlive()) == params.RunnerTerminated {
				return
			}
			if s.sleepWithCancel(5 * time.Second) {
				return
			}
			goto retryStart
		}
	}
}

func (s *Service) keepAliveLoop() {
	var sleepTime time.Duration
retryConnecting:
	if sleepTime > 0 {
		if s.sleepWithCancel(sleepTime) {
			return
		}
	}
	for {
		select {
		case <-s.done:
			return
		case <-s.ctx.Done():
			return
		case <-s.connected:
			slog.InfoContext(s.ctx, "attempting to connect to GARM server", "server", s.cfg.ServerURL)
			sleepTime = 5 * time.Second
			parsed, err := url.ParseRequestURI(s.cfg.ServerURL)
			if err != nil {
				slog.ErrorContext(s.ctx, "server url is invalid", "error", err)
				return
			}
			baseURL := fmt.Sprintf("%s://%s", parsed.Scheme, parsed.Host)
			cli, err := garmWs.NewReader(s.ctx, baseURL, parsed.Path, s.cfg.Token, s.handleMessage)
			if err != nil {
				slog.WarnContext(s.ctx, "failed to create websocket client", "error", err)
				goto retryConnecting
			}

			s.cliMux.Lock()
			s.cli = cli
			s.cliMux.Unlock()

			if err := s.cli.Start(); err != nil {
				slog.WarnContext(s.ctx, "failed to start websocket connection", "error", err)
				goto retryConnecting
			}
			slog.InfoContext(s.ctx, "successfully connected to GARM", "server", s.cfg.ServerURL)
			s.connected = make(chan struct{})
			close(s.connecting)
		}
	}
}

func (s *Service) loop() {
	heartbeatTicker := time.NewTicker(30 * time.Second)
	defer func() {
		slog.InfoContext(s.ctx, "daemon is shutting down")
		if stopErr := s.Stop(); stopErr != nil {
			slog.ErrorContext(s.ctx, "failed to stop service", "error", stopErr)
		}
		heartbeatTicker.Stop()
	}()

connecting:
	select {
	case <-s.done:
		return
	case <-s.ctx.Done():
		return
	case <-s.connecting:
	}
	// send initial heartbeat
	if id, alive, ok := s.snapshot(); ok {
		if err := s.sendHeartbeat(id); err != nil {
			slog.ErrorContext(s.ctx, "failed to send heartbeat", "error", err)
		}
		s.sendRunnerStatus(id, alive)
	}

	for {
		select {
		case <-s.done:
			return
		case <-s.ctx.Done():
			return
		case <-s.cli.Done():
			slog.InfoContext(s.ctx, "remote host closed WS connection")
			s.connecting = make(chan struct{})
			close(s.connected)
			goto connecting
		case <-heartbeatTicker.C:
			// send heartbeat
			if id, _, ok := s.snapshot(); ok {
				if err := s.sendHeartbeat(id); err != nil {
					slog.ErrorContext(s.ctx, "failed to send heartbeat", "error", err)
				}
			}
		}
	}
}
