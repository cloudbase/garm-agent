package state

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewManager(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	manager, err := NewManager(dbPath)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}
	defer manager.Close()

	if manager.db == nil {
		t.Error("expected non-nil database")
	}
}

func TestNewManagerInvalidPath(t *testing.T) {
	_, err := NewManager("/nonexistent/directory/test.db")
	if err == nil {
		t.Error("expected error for invalid database path")
	}
}

func TestManagerSetAndGetState(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	manager, err := NewManager(dbPath)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}
	defer manager.Close()

	now := time.Now()
	testState := State{
		JobStarted:  true,
		JobFinished: false,
		StartTime:   &now,
		FinishTime:  nil,
	}

	err = manager.SetState(testState)
	if err != nil {
		t.Fatalf("failed to set state: %v", err)
	}

	retrievedState, err := manager.GetState()
	if err != nil {
		t.Fatalf("failed to get state: %v", err)
	}

	if retrievedState.JobStarted != testState.JobStarted {
		t.Errorf("expected JobStarted %v, got %v", testState.JobStarted, retrievedState.JobStarted)
	}

	if retrievedState.JobFinished != testState.JobFinished {
		t.Errorf("expected JobFinished %v, got %v", testState.JobFinished, retrievedState.JobFinished)
	}

	if retrievedState.StartTime == nil {
		t.Error("expected non-nil StartTime")
	} else if !retrievedState.StartTime.Equal(now) {
		t.Errorf("expected StartTime %v, got %v", now, *retrievedState.StartTime)
	}

	if retrievedState.FinishTime != nil {
		t.Errorf("expected nil FinishTime, got %v", retrievedState.FinishTime)
	}
}

func TestManagerGetStateEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	manager, err := NewManager(dbPath)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}
	defer manager.Close()

	state, err := manager.GetState()
	if err != nil {
		t.Fatalf("failed to get empty state: %v", err)
	}

	if state.JobStarted {
		t.Error("expected JobStarted to be false for empty state")
	}

	if state.JobFinished {
		t.Error("expected JobFinished to be false for empty state")
	}

	if state.StartTime != nil {
		t.Error("expected StartTime to be nil for empty state")
	}

	if state.FinishTime != nil {
		t.Error("expected FinishTime to be nil for empty state")
	}
}

func TestManagerSetJobStarted(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	manager, err := NewManager(dbPath)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}
	defer manager.Close()

	before := time.Now()
	err = manager.SetJobStarted()
	after := time.Now()

	if err != nil {
		t.Fatalf("failed to set job started: %v", err)
	}

	state, err := manager.GetState()
	if err != nil {
		t.Fatalf("failed to get state: %v", err)
	}

	if !state.JobStarted {
		t.Error("expected JobStarted to be true")
	}

	if state.StartTime == nil {
		t.Error("expected non-nil StartTime")
	} else {
		if state.StartTime.Before(before) || state.StartTime.After(after) {
			t.Errorf("StartTime %v is outside expected range [%v, %v]", state.StartTime, before, after)
		}
	}
}

func TestManagerSetJobFinished(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	manager, err := NewManager(dbPath)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}
	defer manager.Close()

	before := time.Now()
	err = manager.SetJobFinished()
	after := time.Now()

	if err != nil {
		t.Fatalf("failed to set job finished: %v", err)
	}

	state, err := manager.GetState()
	if err != nil {
		t.Fatalf("failed to get state: %v", err)
	}

	if !state.JobFinished {
		t.Error("expected JobFinished to be true")
	}

	if state.FinishTime == nil {
		t.Error("expected non-nil FinishTime")
	} else {
		if state.FinishTime.Before(before) || state.FinishTime.After(after) {
			t.Errorf("FinishTime %v is outside expected range [%v, %v]", state.FinishTime, before, after)
		}
	}
}

func TestManagerIsJobStarted(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	manager, err := NewManager(dbPath)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}
	defer manager.Close()

	started, err := manager.IsJobStarted()
	if err != nil {
		t.Fatalf("failed to check if job started: %v", err)
	}
	if started {
		t.Error("expected job not started initially")
	}

	err = manager.SetJobStarted()
	if err != nil {
		t.Fatalf("failed to set job started: %v", err)
	}

	started, err = manager.IsJobStarted()
	if err != nil {
		t.Fatalf("failed to check if job started: %v", err)
	}
	if !started {
		t.Error("expected job to be started")
	}
}

func TestManagerIsJobFinished(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	manager, err := NewManager(dbPath)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}
	defer manager.Close()

	finished, err := manager.IsJobFinished()
	if err != nil {
		t.Fatalf("failed to check if job finished: %v", err)
	}
	if finished {
		t.Error("expected job not finished initially")
	}

	err = manager.SetJobFinished()
	if err != nil {
		t.Fatalf("failed to set job finished: %v", err)
	}

	finished, err = manager.IsJobFinished()
	if err != nil {
		t.Fatalf("failed to check if job finished: %v", err)
	}
	if !finished {
		t.Error("expected job to be finished")
	}
}

func TestManagerIsJobRunning(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	manager, err := NewManager(dbPath)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}
	defer manager.Close()

	// Initially not running
	running, err := manager.IsJobRunning()
	if err != nil {
		t.Fatalf("failed to check if job running: %v", err)
	}
	if running {
		t.Error("expected job not running initially")
	}

	// Start job - should be running
	err = manager.SetJobStarted()
	if err != nil {
		t.Fatalf("failed to set job started: %v", err)
	}

	running, err = manager.IsJobRunning()
	if err != nil {
		t.Fatalf("failed to check if job running: %v", err)
	}
	if !running {
		t.Error("expected job to be running after start")
	}

	// Finish job - should not be running
	err = manager.SetJobFinished()
	if err != nil {
		t.Fatalf("failed to set job finished: %v", err)
	}

	running, err = manager.IsJobRunning()
	if err != nil {
		t.Fatalf("failed to check if job running: %v", err)
	}
	if running {
		t.Error("expected job not running after finish")
	}
}

func TestManagerGetJobDuration(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	manager, err := NewManager(dbPath)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}
	defer manager.Close()

	// Duration when job not started should error
	_, err = manager.GetJobDuration()
	if err == nil {
		t.Error("expected error when getting duration of non-started job")
	}

	// Start job
	err = manager.SetJobStarted()
	if err != nil {
		t.Fatalf("failed to set job started: %v", err)
	}

	// Sleep to allow some duration
	time.Sleep(100 * time.Millisecond)

	// Duration while running should be positive
	duration, err := manager.GetJobDuration()
	if err != nil {
		t.Fatalf("failed to get job duration: %v", err)
	}
	if duration <= 0 {
		t.Errorf("expected positive duration, got %v", duration)
	}
	if duration < 100*time.Millisecond {
		t.Errorf("expected duration >= 100ms, got %v", duration)
	}

	// Finish job
	time.Sleep(50 * time.Millisecond)
	err = manager.SetJobFinished()
	if err != nil {
		t.Fatalf("failed to set job finished: %v", err)
	}

	// Duration after finish should be fixed
	duration1, err := manager.GetJobDuration()
	if err != nil {
		t.Fatalf("failed to get job duration: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	duration2, err := manager.GetJobDuration()
	if err != nil {
		t.Fatalf("failed to get job duration: %v", err)
	}

	// Duration should be the same after job is finished
	if duration1 != duration2 {
		t.Errorf("expected consistent duration after job finish: %v vs %v", duration1, duration2)
	}
}

func TestManagerStatePersistence(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	// Create manager and set state
	manager1, err := NewManager(dbPath)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	err = manager1.SetJobStarted()
	if err != nil {
		t.Fatalf("failed to set job started: %v", err)
	}

	err = manager1.Close()
	if err != nil {
		t.Fatalf("failed to close manager: %v", err)
	}

	// Create new manager with same database
	manager2, err := NewManager(dbPath)
	if err != nil {
		t.Fatalf("failed to create second manager: %v", err)
	}
	defer manager2.Close()

	// State should persist
	started, err := manager2.IsJobStarted()
	if err != nil {
		t.Fatalf("failed to check if job started: %v", err)
	}
	if !started {
		t.Error("expected job started state to persist")
	}
}

func TestManagerClose(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	manager, err := NewManager(dbPath)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	err = manager.Close()
	if err != nil {
		t.Errorf("unexpected error closing manager: %v", err)
	}

	// Verify database file exists
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Error("expected database file to exist after close")
	}
}

func TestManagerConcurrentAccess(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	manager, err := NewManager(dbPath)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}
	defer manager.Close()

	done := make(chan bool)

	// Concurrent writes
	go func() {
		for i := 0; i < 10; i++ {
			_ = manager.SetJobStarted()
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 10; i++ {
			_ = manager.SetJobFinished()
		}
		done <- true
	}()

	// Concurrent reads
	go func() {
		for i := 0; i < 10; i++ {
			_, _ = manager.IsJobRunning()
		}
		done <- true
	}()

	// Wait for all goroutines
	for i := 0; i < 3; i++ {
		<-done
	}

	// Verify we can still read state
	_, err = manager.GetState()
	if err != nil {
		t.Errorf("failed to get state after concurrent access: %v", err)
	}
}
