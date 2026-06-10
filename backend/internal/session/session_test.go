package session_test

import (
	"testing"
	"time"

	"github.com/skua-app/skua/internal/session"
)

func TestTouch_UnknownDevice_AwayTrue(t *testing.T) {
	s := session.New(30 * time.Minute)
	now := time.Date(2026, 6, 4, 12, 0, 0, 0, time.UTC)
	if away := s.Touch("dev-a", now); !away {
		t.Errorf("first Touch on unknown device: away=false, want true")
	}
}

func TestTouch_SecondTouchWithinGap_AwayFalse(t *testing.T) {
	s := session.New(30 * time.Minute)
	t0 := time.Date(2026, 6, 4, 12, 0, 0, 0, time.UTC)
	if away := s.Touch("dev-a", t0); !away {
		t.Fatalf("seed Touch: away=false, want true")
	}
	if away := s.Touch("dev-a", t0.Add(5*time.Minute)); away {
		t.Errorf("Touch within gap: away=true, want false")
	}
}

func TestTouch_TouchAfterGap_AwayTrue(t *testing.T) {
	s := session.New(30 * time.Minute)
	t0 := time.Date(2026, 6, 4, 12, 0, 0, 0, time.UTC)
	if away := s.Touch("dev-a", t0); !away {
		t.Fatalf("seed Touch: away=false, want true")
	}
	// gap + 1ns is strictly greater than the gap.
	if away := s.Touch("dev-a", t0.Add(30*time.Minute+time.Nanosecond)); !away {
		t.Errorf("Touch after gap: away=false, want true")
	}
}

func TestTouch_PruneDropsStaleEntries(t *testing.T) {
	s := session.New(30 * time.Minute)
	t0 := time.Date(2026, 6, 4, 12, 0, 0, 0, time.UTC)
	if away := s.Touch("dev-stale", t0); !away {
		t.Fatalf("seed Touch: away=false, want true")
	}
	// Way beyond the 30-day pruneAfter horizon — the prune sweep
	// inside the next Touch must drop dev-stale, so a follow-up
	// Touch on dev-stale at this new "now" reports away=true again
	// (as if it had never been seen).
	tLate := t0.Add(40 * 24 * time.Hour)
	if away := s.Touch("dev-other", tLate); !away {
		t.Fatalf("Touch on fresh device: away=false, want true")
	}
	if away := s.Touch("dev-stale", tLate.Add(time.Second)); !away {
		t.Errorf("post-prune Touch on dev-stale: away=false, want true (entry should have been pruned)")
	}
}
