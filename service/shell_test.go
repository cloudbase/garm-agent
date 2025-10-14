package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/cloudbase/garm/workers/websocket/agent/messaging"
)

func TestNewShellSession(t *testing.T) {
	ctx := context.Background()
	cfg, _ := createTestConfig(t)

	sessionID := [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	shellMsg := messaging.CreateShellMessage{
		SessionID: sessionID,
	}

	var writtenMessages [][]byte
	var msgMux sync.Mutex
	msgWriter := func(msg []byte) error {
		msgMux.Lock()
		defer msgMux.Unlock()
		writtenMessages = append(writtenMessages, msg)
		return nil
	}

	// This will fail on systems without PTY support, but that's expected
	session, err := NewShellSession(ctx, shellMsg, msgWriter, cfg)

	// On systems without PTY support, we expect an error
	if err != nil {
		if !HasPTY() {
			t.Skipf("PTY not available on this system: %v", err)
		}
		t.Fatalf("unexpected error creating shell session: %v", err)
	}

	if session == nil {
		t.Fatal("expected non-nil session")
	}

	if session.SessionID != sessionID {
		t.Errorf("expected session ID %v, got %v", sessionID, session.SessionID)
	}

	if session.shell == nil {
		t.Error("expected non-nil shell")
	}

	if session.writer == nil {
		t.Error("expected non-nil writer")
	}
}

func TestShellSessionWithMockPTY(t *testing.T) {
	ctx := context.Background()
	sessionID := [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}

	var writtenMessages [][]byte
	var msgMux sync.Mutex
	msgWriter := func(msg []byte) error {
		msgMux.Lock()
		defer msgMux.Unlock()
		writtenMessages = append(writtenMessages, msg)
		return nil
	}

	mockPty := &mockPTY{}
	session := &ShellSession{
		ctx:       ctx,
		SessionID: sessionID,
		shell:     mockPty,
		writer:    msgWriter,
		done:      closed,
	}

	// Test Done() channel
	doneChan := session.Done()
	if doneChan == nil {
		t.Error("expected non-nil done channel")
	}

	// Test Start()
	if err := session.Start(); err != nil {
		t.Errorf("unexpected error starting session: %v", err)
	}

	if !session.running {
		t.Error("expected session to be running after Start()")
	}

	// Starting again should be idempotent
	if err := session.Start(); err != nil {
		t.Errorf("second Start() should be idempotent: %v", err)
	}

	// Give time for goroutines to start
	time.Sleep(50 * time.Millisecond)

	// Test Stop()
	if err := session.Stop(); err != nil {
		t.Errorf("unexpected error stopping session: %v", err)
	}

	if session.running {
		t.Error("expected session to not be running after Stop()")
	}

	// Verify PTY was closed
	if !mockPty.closed {
		t.Error("expected PTY to be closed after Stop()")
	}

	// Stopping again should be idempotent
	if err := session.Stop(); err != nil {
		t.Errorf("second Stop() should be idempotent: %v", err)
	}

	// Verify messages were written
	msgMux.Lock()
	messageCount := len(writtenMessages)
	msgMux.Unlock()

	if messageCount < 1 {
		t.Error("expected at least one message to be written (exit message)")
	}
}

func TestShellSessionStartWithoutPTY(t *testing.T) {
	ctx := context.Background()
	sessionID := [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}

	msgWriter := func(msg []byte) error {
		return nil
	}

	session := &ShellSession{
		ctx:       ctx,
		SessionID: sessionID,
		shell:     nil, // No PTY
		writer:    msgWriter,
		done:      closed,
	}

	// Starting without PTY should fail
	err := session.Start()
	if err == nil {
		t.Error("expected error when starting session without PTY")
	}
	if !contains(err.Error(), "PTY not created") {
		t.Errorf("expected PTY error, got: %v", err)
	}
}

func TestShellSessionSendReadyMessage(t *testing.T) {
	ctx := context.Background()
	sessionID := [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}

	var lastMessage []byte
	msgWriter := func(msg []byte) error {
		lastMessage = msg
		return nil
	}

	session := &ShellSession{
		ctx:       ctx,
		SessionID: sessionID,
		writer:    msgWriter,
	}

	// Test success message
	if err := session.sendReadyMessage(0, []byte("ready")); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if lastMessage == nil {
		t.Error("expected message to be written")
	}

	// Test error message
	if err := session.sendReadyMessage(1, []byte("error occurred")); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if lastMessage == nil {
		t.Error("expected error message to be written")
	}
}

func TestShellSessionSendExitMessage(t *testing.T) {
	ctx := context.Background()
	sessionID := [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}

	var exitMessageReceived bool
	msgWriter := func(msg []byte) error {
		exitMessageReceived = true
		return nil
	}

	session := &ShellSession{
		ctx:       ctx,
		SessionID: sessionID,
		writer:    msgWriter,
	}

	if err := session.sendExitMessage(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if !exitMessageReceived {
		t.Error("expected exit message to be sent")
	}
}

func TestShellSessionWriterError(t *testing.T) {
	ctx := context.Background()
	sessionID := [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}

	expectedErr := errors.New("write error")
	msgWriter := func(msg []byte) error {
		return expectedErr
	}

	session := &ShellSession{
		ctx:       ctx,
		SessionID: sessionID,
		writer:    msgWriter,
	}

	// Test that write errors are propagated
	if err := session.sendReadyMessage(0, nil); err != expectedErr {
		t.Errorf("expected error %v, got %v", expectedErr, err)
	}

	if err := session.sendExitMessage(); err != expectedErr {
		t.Errorf("expected error %v, got %v", expectedErr, err)
	}
}

func TestShellSessionContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	sessionID := [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}

	msgWriter := func(msg []byte) error {
		return nil
	}

	mockPty := &mockPTY{}
	session := &ShellSession{
		ctx:       ctx,
		SessionID: sessionID,
		shell:     mockPty,
		writer:    msgWriter,
		done:      closed,
	}

	if err := session.Start(); err != nil {
		t.Fatalf("failed to start session: %v", err)
	}

	// Cancel context
	cancel()

	// Give time for cancellation to propagate
	time.Sleep(100 * time.Millisecond)

	// Session should stop
	select {
	case <-session.Done():
		// Expected
	case <-time.After(1 * time.Second):
		t.Error("session did not stop after context cancellation")
	}
}

func TestMockPTYOperations(t *testing.T) {
	pty := &mockPTY{}

	// Test Write
	data := []byte("test data")
	n, err := pty.Write(data)
	if err != nil {
		t.Errorf("unexpected write error: %v", err)
	}
	if n != len(data) {
		t.Errorf("expected %d bytes written, got %d", len(data), n)
	}

	// Test Read
	buf := make([]byte, 100)
	n, err = pty.Read(buf)
	if err != nil {
		t.Errorf("unexpected read error: %v", err)
	}
	if n != len(data) {
		t.Errorf("expected %d bytes read, got %d", len(data), n)
	}
	if string(buf[:n]) != string(data) {
		t.Errorf("expected %q, got %q", data, buf[:n])
	}

	// Test Resize
	if err := pty.Resize(80, 24); err != nil {
		t.Errorf("unexpected resize error: %v", err)
	}

	// Test Close
	if err := pty.Close(); err != nil {
		t.Errorf("unexpected close error: %v", err)
	}

	// Operations after close should fail
	_, err = pty.Write([]byte("test"))
	if err == nil {
		t.Error("expected error writing to closed PTY")
	}

	_, err = pty.Read(buf)
	if err == nil {
		t.Error("expected error reading from closed PTY")
	}

	err = pty.Resize(80, 24)
	if err == nil {
		t.Error("expected error resizing closed PTY")
	}
}

func TestMessageWriter(t *testing.T) {
	var messages [][]byte
	var mux sync.Mutex

	writer := func(msg []byte) error {
		mux.Lock()
		defer mux.Unlock()
		messages = append(messages, msg)
		return nil
	}

	// Test that writer function works
	testMsg := []byte("test message")
	if err := writer(testMsg); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	mux.Lock()
	if len(messages) != 1 {
		t.Errorf("expected 1 message, got %d", len(messages))
	}
	if string(messages[0]) != string(testMsg) {
		t.Errorf("expected %q, got %q", testMsg, messages[0])
	}
	mux.Unlock()
}
