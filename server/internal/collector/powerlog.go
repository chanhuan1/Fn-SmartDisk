package collector

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
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
func (s *powerLogStore) add(e model.PowerEvent) {
	if e.Time == 0 {
		e.Time = time.Now().Unix()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.evt = append(s.evt, e)
	if len(s.evt) > powerLogMax {
		s.evt = s.evt[len(s.evt)-powerLogMax:]
	}
	s.saveLocked()
}

func (s *powerLogStore) saveLocked() {
	if s.path == "" {
		return
	}
	b, err := json.MarshalIndent(s.evt, "", "  ")
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(s.path), 0o755)
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		log.Printf("[powerlog] write temp events %s: %v", tmp, err)
		return
	}
	if err := os.Rename(tmp, s.path); err != nil {
		log.Printf("[powerlog] rename events %s -> %s: %v", tmp, s.path, err)
	}
}

// clear removes every persisted event and empties the ring buffer.
func (s *powerLogStore) clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.evt = nil
	s.saveLocked()
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
