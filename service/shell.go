package service

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/cloudbase/garm-agent/config"
	"github.com/cloudbase/garm/workers/websocket/agent/messaging"
)

type messageWriter func(msg []byte) error

type PTY interface {
	Read([]byte) (int, error)
	Write([]byte) (int, error)
	Resize(cols, rows uint16) error
	Close() error
}

func NewShellSession(ctx context.Context, shellMsg messaging.CreateShellMessage, msgWriter messageWriter, cfg *config.Agent) (*ShellSession, error) {
	pty, err := NewSessionPTY(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create PTY: %w", err)
	}
	sess := &ShellSession{
		ctx:       ctx,
		SessionID: shellMsg.SessionID,
		shell:     pty,
		writer:    msgWriter,
		done:      closed,
	}
	return sess, nil
}

type ShellSession struct {
	ctx       context.Context
	SessionID [16]byte
	shell     PTY

	writer messageWriter
	mux    sync.Mutex

	done    chan struct{}
	running bool
}

func (s *ShellSession) Done() chan struct{} {
	return s.done
}

func (s *ShellSession) Start() error {
	s.mux.Lock()
	defer s.mux.Unlock()

	if s.running {
		return nil
	}

	if s.shell == nil {
		return fmt.Errorf("PTY not created")
	}

	s.running = true
	s.done = make(chan struct{})
	go s.loop()
	go s.handlePTYOutput()
	return nil
}

func (s *ShellSession) Stop() error {
	slog.InfoContext(s.ctx, "stopping; attempting to lock")
	s.mux.Lock()
	defer s.mux.Unlock()

	slog.InfoContext(s.ctx, "stopping; lock acquired")

	if !s.running {
		return nil
	}

	s.running = false
	if s.shell != nil {
		slog.InfoContext(s.ctx, "stopping; closing shell")
		if err := s.shell.Close(); err != nil {
			slog.ErrorContext(s.ctx, "failed to close PTY", "error", err)
		}
	}
	slog.InfoContext(s.ctx, "stopping; closing done")
	close(s.done)
	slog.InfoContext(s.ctx, "stopping; sending exit msg")
	return s.sendExitMessage()
}

func (s *ShellSession) sendExitMessage() error {
	exitMsg := messaging.ShellExitMessage{
		SessionID: s.SessionID,
	}
	return s.writer(exitMsg.Marshal())
}

func (s *ShellSession) sendReadyMessage(isError byte, message []byte) error {
	readyMsg := messaging.ShellReadyMessage{
		SessionID: s.SessionID,
		IsError:   isError,
		Message:   message,
	}

	return s.writer(readyMsg.Marshal())
}

func (s *ShellSession) handlePTYOutput() {
	defer func() {
		if stopErr := s.Stop(); stopErr != nil {
			slog.ErrorContext(s.ctx, "failed to stop shell session", "error", stopErr)
		}
	}()
	if err := s.sendReadyMessage(0, nil); err != nil {
		slog.ErrorContext(s.ctx, "failed to signal shell readyness", "error", err)
		return
	}
	for {
		buf := make([]byte, 1024)
		n, err := s.shell.Read(buf)
		if err != nil {
			slog.ErrorContext(s.ctx, "failed to read shell", "error", err)
			return
		}

		shellMsg := messaging.ShellDataMessage{
			SessionID: s.SessionID,
			Data:      buf[:n],
		}
		if err := s.writer(shellMsg.Marshal()); err != nil {
			slog.ErrorContext(s.ctx, "failed to write message", "error", err)
			break
		}
	}
}

func (s *ShellSession) loop() {
	defer func() {
		if stopErr := s.Stop(); stopErr != nil {
			slog.ErrorContext(s.ctx, "failed to stop shell session", "error", stopErr)
		}
	}()
	for {
		select {
		case <-s.done:
			return
		case <-s.ctx.Done():
			return
		}
	}
}
