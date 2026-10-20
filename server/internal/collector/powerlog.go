package collector

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"smartdisk/internal/model"
)

// powerLogMax bounds how many events are kept on disk (ring buffer).
const powerLogMax = 200

// powerLogStore persists the power-control event log as a JSON array. Events
// are appended newest-last; reads return newest-first for the UI.
type powerLogStore struct {
	path string
	mu   sync.Mutex
	evt  []model.PowerEvent
}

func loadPowerLogStore(path string) *powerLogStore {
	s := &powerLogStore{path: path}
	b, err := os.ReadFile(path)
	if err != nil {
		return s
	}
	var list []model.PowerEvent
	if json.Unmarshal(b, &list) == nil {
		s.evt = list
	}
	return s
}

// add appends an event and persists (capped at powerLogMax, dropping oldest).
func (s *powerLogStore) add(e model.PowerEvent) error {
	if e.Time == 0 {
		e.Time = time.Now().Unix()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	old := append([]model.PowerEvent(nil), s.evt...)
	s.evt = append(s.evt, e)
	if len(s.evt) > powerLogMax {
		s.evt = s.evt[len(s.evt)-powerLogMax:]
	}
	if err := s.saveLocked(); err != nil {
		s.evt = old
		return err
	}
	return nil
}

func (s *powerLogStore) saveLocked() error {
	if s.path == "" {
		return nil
	}
	b, err := json.MarshalIndent(s.evt, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal power log: %w", err)
	}
	if err := writeAtomicFile(s.path, b, 0o600); err != nil {
		return fmt.Errorf("persist power log %s: %w", s.path, err)
	}
	return nil
}

// clear removes every persisted event and empties the ring buffer.
func (s *powerLogStore) clear() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	old := s.evt
	s.evt = nil
	if err := s.saveLocked(); err != nil {
		s.evt = old
		return err
	}
	return nil
}

// list returns up to limit events, newest first. limit<=0 means all.
func (s *powerLogStore) list(limit int) []model.PowerEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := len(s.evt)
	out := make([]model.PowerEvent, 0, n)
	for i := n - 1; i >= 0; i-- {
		out = append(out, s.evt[i])
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}
