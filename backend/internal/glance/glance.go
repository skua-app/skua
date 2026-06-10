// Package glance persists the household "while you were away" state for
// the glance feature: a scope-keyed seen-id set plus a seen_through
// watermark. A moment is "seen" when its representative event id
// appears in the household scope's seen set, or when its newest event
// start is at or before the scope's seen_through watermark.
//
// The store is intentionally minimal: it knows nothing about events,
// HTTP, or moment grouping. It only holds and persists the per-scope
// state, pruning seen entries older than the retention window so the
// file stays bounded. The API handler composes it with the moment
// grouping to build the GET /api/glance payload.
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

// scopeState carries one scope's persisted state: the seen-id set
// (map of event id → seen-at unix seconds) and a seen_through
// watermark in unix seconds (zero when unset). Moments whose newest
// event start is at or before seen_through are surfaced with
// seen=true by the API handler (they are NOT dropped).
type scopeState struct {
	Seen        map[string]int64 `json:"seen"`
	SeenThrough int64            `json:"seen_through,omitempty"`
}

// state is the JSON shape persisted at the store's path: a map of
// scope name → scopeState.
type state map[string]*scopeState

// Store is a thread-safe, file-backed seen-state store keyed by scope.
type Store struct {
	path string
	mu   sync.RWMutex
	cur  state
}

// New loads the seen-state from path. A missing file means an empty
// store. A parse error, the round-2 scope→{id:ts} shape, the
// pre-Model-B last_seen shape, and the pre-rename {"cleared_at": ...}
// shape are all logged and start the store empty — best-effort
// recency state must not block startup, and there is no automatic
// migration off the old cleared_at watermark (the new tag is
// seen_through; old files lose the watermark once on upgrade, which
// is acceptable). Entries older than the retention window are pruned
// on load.
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
	cleaned := state{}
	for scope, sc := range loaded {
		if sc == nil {
			continue
		}
		if sc.Seen == nil {
			sc.Seen = map[string]int64{}
		}
		cleaned[scope] = sc
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
	sc := s.cur[scope]
	if sc == nil {
		return map[string]struct{}{}
	}
	out := make(map[string]struct{}, len(sc.Seen))
	for id := range sc.Seen {
		out[id] = struct{}{}
	}
	return out
}

// SeenThrough returns the scope's seen_through watermark as a
// time.Time (zero value when unset) under a read lock.
func (s *Store) SeenThrough(scope string) time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sc := s.cur[scope]
	if sc == nil || sc.SeenThrough == 0 {
		return time.Time{}
	}
	return time.Unix(sc.SeenThrough, 0).UTC()
}

// MarkSeen adds each id to the scope's seen set with at's unix seconds,
// prunes entries older than the retention window, and atomically
// persists the result. Re-marking an id is idempotent and refreshes
// its timestamp. An empty ids slice is a no-op with no write.
func (s *Store) MarkSeen(scope string, ids []string, at time.Time) error {
	if len(ids) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	sc := s.cur[scope]
	if sc == nil {
		sc = &scopeState{Seen: map[string]int64{}}
		s.cur[scope] = sc
	}
	if sc.Seen == nil {
		sc.Seen = map[string]int64{}
	}
	ts := at.Unix()
	for _, id := range ids {
		if id == "" {
			continue
		}
		sc.Seen[id] = ts
	}
	s.pruneLocked(at)
	return s.atomicWrite()
}

// MarkAllSeen sets the scope's seen_through watermark to at.Unix(),
// prunes seen ids older than the retention window, and atomically
// persists the result. Moments whose newest event start sits at or
// before the watermark are reported as seen by GET /api/glance —
// they are NOT removed from the list.
func (s *Store) MarkAllSeen(scope string, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	sc := s.cur[scope]
	if sc == nil {
		sc = &scopeState{Seen: map[string]int64{}}
		s.cur[scope] = sc
	}
	if sc.Seen == nil {
		sc.Seen = map[string]int64{}
	}
	sc.SeenThrough = at.Unix()
	s.pruneLocked(at)
	return s.atomicWrite()
}

// pruneLocked drops seen entries whose seen-at is older than now minus
// glanceRetention. Caller must hold s.mu (write or read+write upgrade).
// A scope is only removed when both its seen set is empty AND its
// seen_through watermark is zero — the watermark must survive an
// empty seen set so that mark-all-seen persists across pruning runs.
func (s *Store) pruneLocked(now time.Time) {
	cutoff := now.Add(-glanceRetention).Unix()
	for scope, sc := range s.cur {
		if sc == nil {
			delete(s.cur, scope)
			continue
		}
		for id, ts := range sc.Seen {
			if ts < cutoff {
				delete(sc.Seen, id)
			}
		}
		if len(sc.Seen) == 0 && sc.SeenThrough == 0 {
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
