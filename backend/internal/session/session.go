// Package session tracks per-device activity for the glance "while
// you were away" auto-surface decision. Each browser/PWA install
// gets a stable opaque id (set as a cookie by the heartbeat
// handler) and the store remembers the wall-clock of its last
// heartbeat. A device whose previous heartbeat is older than the
// configured gap (or who has never heartbeat-ed) is reported as
// "away" on its next Touch.
//
// The store is intentionally in-memory: a server restart re-reports
// every device as away on its next heartbeat, which is acceptable
// for a household glance feature — the worst case is one extra
// auto-surface per device after a restart.
package session

import (
	"sync"
	"time"
)

// pruneAfter is the inactivity threshold for dropping a device from
// the in-memory table. Devices that haven't checked in within this
// window are unlikely to come back without a fresh away verdict
// anyway, so keeping the entry buys nothing.
const pruneAfter = 30 * 24 * time.Hour

// Store is a thread-safe in-memory map of device id → last activity
// time, used to derive a per-device "away" verdict based on a
// configurable inactivity gap.
type Store struct {
	mu   sync.Mutex
	last map[string]time.Time
	gap  time.Duration
}

// New constructs a Store with the supplied inactivity gap. A
// zero/negative gap is allowed (and effectively reports every
// Touch as not-away after the first hit on a device) — the caller
// is responsible for passing a sensible value.
func New(gap time.Duration) *Store {
	return &Store{
		last: map[string]time.Time{},
		gap:  gap,
	}
}

// Touch records activity for deviceID at now and reports whether
// this Touch represents a return from absence. The verdict is:
//
//   - true when the device has no prior activity (first-ever Touch
//     or pruned since), OR when the gap between the previous
//     activity and now exceeds the configured gap.
//   - false otherwise.
//
// Touch also performs lazy pruning of devices whose last activity
// is older than pruneAfter, so a one-shot visitor does not occupy
// memory indefinitely.
func (s *Store) Touch(deviceID string, now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	prev := s.last[deviceID]
	away := prev.IsZero() || now.Sub(prev) > s.gap
	s.last[deviceID] = now

	cutoff := now.Add(-pruneAfter)
	for id, t := range s.last {
		if t.Before(cutoff) {
			delete(s.last, id)
		}
	}
	return away
}
