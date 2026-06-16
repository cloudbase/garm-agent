package state

import (
	"encoding/json"
	"fmt"
	"time"

	bolt "go.etcd.io/bbolt"
)

type State struct {
	JobStarted  bool       `json:"jobStarted"`
	JobFinished bool       `json:"jobFinished"`
	StartTime   *time.Time `json:"startTime,omitempty"`
	FinishTime  *time.Time `json:"finishTime,omitempty"`
}

type Manager struct {
	db     *bolt.DB
	bucket []byte
}

func NewManager(dbPath string) (*Manager, error) {
	db, err := bolt.Open(dbPath, 0o600, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	bucket := []byte("state")

	// Create bucket if it doesn't exist
	err = db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(bucket)
		return err
	})
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create bucket: %w", err)
	}

	return &Manager{
		db:     db,
		bucket: bucket,
	}, nil
}

func (sm *Manager) Close() error {
	return sm.db.Close()
}

func (sm *Manager) SetState(state State) error {
	return sm.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(sm.bucket)

		data, err := json.Marshal(state)
		if err != nil {
			return fmt.Errorf("failed to marshal state: %w", err)
		}

		return b.Put([]byte("current"), data)
	})
}

func (sm *Manager) GetState() (State, error) {
	var state State

	err := sm.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(sm.bucket)
		data := b.Get([]byte("current"))

		if data == nil {
			// Return zero value if no state exists
			return nil
		}

		return json.Unmarshal(data, &state)
	})

	return state, err
}

func (sm *Manager) SetJobStarted() error {
	return sm.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(sm.bucket)
		var state State
		if data := b.Get([]byte("current")); data != nil {
			if err := json.Unmarshal(data, &state); err != nil {
				return fmt.Errorf("failed to unmarshal state: %w", err)
			}
		}
		now := time.Now()
		state.JobStarted = true
		state.StartTime = &now
		data, err := json.Marshal(state)
		if err != nil {
			return fmt.Errorf("failed to marshal state: %w", err)
		}
		return b.Put([]byte("current"), data)
	})
}

func (sm *Manager) SetJobFinished() error {
	return sm.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(sm.bucket)
		var state State
		if data := b.Get([]byte("current")); data != nil {
			if err := json.Unmarshal(data, &state); err != nil {
				return fmt.Errorf("failed to unmarshal state: %w", err)
			}
		}
		now := time.Now()
		state.JobFinished = true
		state.FinishTime = &now
		data, err := json.Marshal(state)
		if err != nil {
			return fmt.Errorf("failed to marshal state: %w", err)
		}
		return b.Put([]byte("current"), data)
	})
}

func (sm *Manager) IsJobStarted() (bool, error) {
	state, err := sm.GetState()
	return state.JobStarted, err
}

func (sm *Manager) IsJobFinished() (bool, error) {
	state, err := sm.GetState()
	return state.JobFinished, err
}

func (sm *Manager) IsJobRunning() (bool, error) {
	state, err := sm.GetState()
	if err != nil {
		return false, err
	}
	return state.JobStarted && !state.JobFinished, nil
}

func (sm *Manager) GetJobDuration() (time.Duration, error) {
	state, err := sm.GetState()
	if err != nil {
		return 0, err
	}

	if state.StartTime == nil {
		return 0, fmt.Errorf("job not started")
	}

	endTime := time.Now()
	if state.FinishTime != nil {
		endTime = *state.FinishTime
	}

	return endTime.Sub(*state.StartTime), nil
}
