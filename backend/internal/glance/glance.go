// Package glance persists the household "seen-state" for the glance
// feature: a single last_seen timestamp shared across the household.
// The on-disk shape is an object with one field, last_seen, holding
// either an RFC3339 string or null when nothing has been seen yet.
//
// The store is intentionally minimal: it knows nothing about events,
// HTTP, or grouping. It only holds and persists the timestamp. The
// API handler composes it with the Phase 1 moment grouping to build
// the GET /api/glance "while you were away" payload.
package glance

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// state is the JSON shape persisted at the store's path. last_seen is
// a pointer so the absent / "never-seen" case round-trips as JSON null
// rather than the zero RFC3339 string.
type state struct {
	LastSeen *string `json:"last_seen"`
}

// Store is a thread-safe, file-backed last_seen store.
type Store struct {
	path string
	mu   sync.RWMutex
	cur  time.Time // zero ⇒ never-seen
}

// New loads the last_seen timestamp from path. A missing file means
// "never-seen" (zero time). A parse error or an unparseable last_seen
// string is logged and the store starts from zero — a corrupt
// best-effort state file must not block startup.
func New(path string) (*Store, error) {
	s := &Store{path: path}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return s, nil
		}
		return nil, fmt.Errorf("glance: read %s: %w", path, err)
	}
	var st state
	if err := json.Unmarshal(data, &st); err != nil {
		slog.Warn("glance: parse failed, starting from never-seen", "path", path, "error", err)
		return s, nil
	}
	if st.LastSeen != nil && *st.LastSeen != "" {
		t, perr := time.Parse(time.RFC3339, *st.LastSeen)
		if perr != nil {
			slog.Warn("glance: last_seen unparseable, starting from never-seen", "path", path, "value", *st.LastSeen, "error", perr)
			return s, nil
		}
		s.cur = t
	}
	return s, nil
}

// LastSeen returns the current last_seen timestamp under a read lock.
// The zero time.Time means "never-seen".
func (s *Store) LastSeen() time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cur
}

// Ack advances last_seen to seenThrough when it is strictly after the
// current value, persists the change atomically, and returns the
// resulting current value. When seenThrough is equal to or older than
// the current value, Ack is a no-op (no disk write) and returns the
// existing value — the household last_seen is monotonic.
func (s *Store) Ack(seenThrough time.Time) (time.Time, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !seenThrough.After(s.cur) {
		return s.cur, nil
	}
	if err := s.atomicWrite(seenThrough); err != nil {
		return s.cur, err
	}
	s.cur = seenThrough
	return s.cur, nil
}

func (s *Store) atomicWrite(t time.Time) error {
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("glance: mkdir %s: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, "glance-*.tmp")
	if err != nil {
		return fmt.Errorf("glance: create temp: %w", err)
	}
	tmpPath := tmp.Name()

	formatted := t.UTC().Format(time.RFC3339)
	payload := state{LastSeen: &formatted}
	if err := json.NewEncoder(tmp).Encode(payload); err != nil {
		if cerr := tmp.Close(); cerr != nil {
			return fmt.Errorf("glance: encode: %w; close temp: %v", err, cerr)
		}
		if rerr := os.Remove(tmpPath); rerr != nil {
			return fmt.Errorf("glance: encode: %w; remove temp: %v", err, rerr)
		}
		return fmt.Errorf("glance: encode: %w", err)
	}
	if err := tmp.Close(); err != nil {
		if rerr := os.Remove(tmpPath); rerr != nil {
			return fmt.Errorf("glance: close temp: %w; remove temp: %v", err, rerr)
		}
		return fmt.Errorf("glance: close temp: %w", err)
	}
	if err := os.Rename(tmpPath, s.path); err != nil {
		if rerr := os.Remove(tmpPath); rerr != nil {
			return fmt.Errorf("glance: rename: %w; remove temp: %v", err, rerr)
		}
		return fmt.Errorf("glance: rename: %w", err)
	}
	return nil
}
