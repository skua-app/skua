// Package glance persists the household "seen-state" for the glance
// feature: a scope-keyed set of viewed event ids. A moment is "seen"
// when its representative event id appears in the household scope's
// set. The on-disk shape is an object mapping a scope name to an object
// mapping an event id to a seen-at unix-seconds integer.
//
// The store is intentionally minimal: it knows nothing about events,
// HTTP, or moment grouping. It only holds and persists the set,
// pruning entries older than the retention window so the file stays
// bounded. The API handler composes it with the moment grouping to
// build the GET /api/glance "while you were away" payload.
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

// ScopeHousehold is the v1 seen-state scope: a single set shared by
// every client on the LAN-only deployment. The scope field is
// reserved for a future per-user split; today every read and write
// uses this constant.
const ScopeHousehold = "household"

// glanceRetention bounds how long a seen entry survives. Anything
// older than now minus this window is pruned on load and after every
// write so the file stays bounded for a long-running household
// install.
const glanceRetention = 30 * 24 * time.Hour

// state is the JSON shape persisted at the store's path: a map of
// scope name → map of event id → seen-at unix seconds.
type state map[string]map[string]int64

// Store is a thread-safe, file-backed seen-id store keyed by scope.
type Store struct {
	path string
	mu   sync.RWMutex
	cur  state
}

// New loads the seen-state from path. A missing file means an empty
// store. A parse error, or an old last_seen-shaped file from before
// the Model B migration, is logged and the store starts empty —
// best-effort recency state must not block startup. Entries older
// than the retention window are pruned on load.
func New(path string) (*Store, error) {
	s := &Store{path: path, cur: state{}}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return s, nil
		}
		return nil, fmt.Errorf("glance: read %s: %w", path, err)
	}
	var loaded state
	if err := json.Unmarshal(data, &loaded); err != nil {
		slog.Warn("glance: parse failed, starting from empty seen-set", "path", path, "error", err)
		return s, nil
	}
	// Reject the old { "last_seen": ... } shape: it unmarshals into a
	// map whose values are not map[string]int64. json.Unmarshal silently
	// produces an empty loaded in that case (top-level last_seen is a
	// string, not an object), but be defensive against any non-set
	// scope value as well.
	cleaned := state{}
	for scope, ids := range loaded {
		if ids == nil {
			continue
		}
		cleaned[scope] = ids
	}
	s.cur = cleaned
	s.pruneLocked(time.Now())
	return s, nil
}

// SeenSet returns a copy of the given scope's id set under a read
// lock. An absent scope yields an empty set. The returned map is
// safe for the caller to mutate.
func (s *Store) SeenSet(scope string) map[string]struct{} {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]struct{}, len(s.cur[scope]))
	for id := range s.cur[scope] {
		out[id] = struct{}{}
	}
	return out
}

// MarkSeen adds each id to the scope's set with at's unix seconds,
// prunes entries older than the retention window, and atomically
// persists the result. Re-marking an id is idempotent and refreshes
// its timestamp. An empty ids slice is a no-op with no write.
func (s *Store) MarkSeen(scope string, ids []string, at time.Time) error {
	if len(ids) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cur[scope] == nil {
		s.cur[scope] = map[string]int64{}
	}
	ts := at.Unix()
	for _, id := range ids {
		if id == "" {
			continue
		}
		s.cur[scope][id] = ts
	}
	s.pruneLocked(at)
	return s.atomicWrite()
}

// pruneLocked drops entries whose seen-at is older than now minus
// glanceRetention. Caller must hold s.mu (write or read+write upgrade).
// Empty scopes after pruning are removed so the persisted file does
// not accumulate dead keys.
func (s *Store) pruneLocked(now time.Time) {
	cutoff := now.Add(-glanceRetention).Unix()
	for scope, ids := range s.cur {
		for id, ts := range ids {
			if ts < cutoff {
				delete(ids, id)
			}
		}
		if len(ids) == 0 {
			delete(s.cur, scope)
		}
	}
}

func (s *Store) atomicWrite() error {
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("glance: mkdir %s: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, "glance-*.tmp")
	if err != nil {
		return fmt.Errorf("glance: create temp: %w", err)
	}
	tmpPath := tmp.Name()

	if err := json.NewEncoder(tmp).Encode(s.cur); err != nil {
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
