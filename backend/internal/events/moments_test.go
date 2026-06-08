package events

import (
	"reflect"
	"testing"
	"time"
)

// mkItem builds a minimal Item for moment tests; t0 is the started_at
// offset added to a fixed reference instant, dur is the event duration
// (negative ⇒ no ended_at), and score is taken as a pointer when
// nonzero — pass NaN-style sentinel via the ptr variant when nil is the
// intended value.
func mkItem(id, cam, label string, kind Kind, t0Sec, durSec int64, score *float64, hasClip bool) Item {
	ref := time.Date(2026, 6, 4, 12, 0, 0, 0, time.UTC)
	started := ref.Add(time.Duration(t0Sec) * time.Second).UTC().Format(time.RFC3339)
	it := Item{
		ID:        id,
		CamID:     cam,
		StartedAt: started,
		Label:     label,
		Kind:      kind,
		Score:     score,
		HasClip:   hasClip,
	}
	if durSec >= 0 {
		ended := time.Date(2026, 6, 4, 12, 0, 0, 0, time.UTC).
			Add(time.Duration(t0Sec+durSec) * time.Second).UTC().Format(time.RFC3339)
		it.EndedAt = &ended
		d := durSec
		it.DurationSeconds = &d
	}
	return it
}

func fptr(v float64) *float64 { return &v }

func TestGroupMoments_SingleEvent(t *testing.T) {
	items := []Item{mkItem("e1", "cam1", "person", KindPerson, 0, 10, fptr(0.9), true)}
	got := GroupMoments(items, time.Time{})
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	m := got[0]
	if m.CamID != "cam1" {
		t.Errorf("cam_id = %q", m.CamID)
	}
	if m.EventCount != 1 {
		t.Errorf("event_count = %d", m.EventCount)
	}
	if m.RepresentativeEventID != "e1" {
		t.Errorf("rep id = %q", m.RepresentativeEventID)
	}
	if !m.RepresentativeHasClip {
		t.Error("rep has_clip should be true")
	}
	if !reflect.DeepEqual(m.Kinds, []Kind{KindPerson}) {
		t.Errorf("kinds = %v", m.Kinds)
	}
	if !reflect.DeepEqual(m.Labels, []string{"person"}) {
		t.Errorf("labels = %v", m.Labels)
	}
	if m.EndedAt == nil {
		t.Error("ended_at should be non-nil for a finished event")
	}
	if len(m.Events) != 1 {
		t.Fatalf("events len = %d, want 1", len(m.Events))
	}
	if m.Events[0].ID != "e1" {
		t.Errorf("events[0].id = %q, want e1", m.Events[0].ID)
	}
}

func TestGroupMoments_WithinGapMerges(t *testing.T) {
	items := []Item{
		mkItem("e1", "cam1", "person", KindPerson, 0, 30, fptr(0.5), false),
		mkItem("e2", "cam1", "car", KindVehicle, 60, 30, fptr(0.8), true), // +60s
	}
	got := GroupMoments(items, time.Time{})
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1 (events within gap merge)", len(got))
	}
	m := got[0]
	if m.EventCount != 2 {
		t.Errorf("event_count = %d, want 2", m.EventCount)
	}
	wantKinds := []Kind{KindPerson, KindVehicle}
	if !reflect.DeepEqual(m.Kinds, wantKinds) {
		t.Errorf("kinds = %v, want %v", m.Kinds, wantKinds)
	}
	wantLabels := []string{"car", "person"} // sorted ascending
	if !reflect.DeepEqual(m.Labels, wantLabels) {
		t.Errorf("labels = %v, want %v", m.Labels, wantLabels)
	}
	if m.RepresentativeEventID != "e2" {
		t.Errorf("rep id = %q, want e2 (higher score)", m.RepresentativeEventID)
	}
	if !m.RepresentativeHasClip {
		t.Error("rep has_clip should be true (from e2)")
	}
	if len(m.Events) != m.EventCount {
		t.Fatalf("events len = %d, want %d (== event_count)", len(m.Events), m.EventCount)
	}
	// Newest detection first: e2 started 60s after e1.
	wantOrder := []string{"e2", "e1"}
	for i, w := range wantOrder {
		if m.Events[i].ID != w {
			t.Errorf("events[%d].id = %q, want %q (newest first)",
				i, m.Events[i].ID, w)
		}
	}
}

func TestGroupMoments_BeyondGapSplits(t *testing.T) {
	gap := int64(MomentGap / time.Second)
	items := []Item{
		mkItem("e1", "cam1", "person", KindPerson, 0, 10, fptr(0.5), false),
		mkItem("e2", "cam1", "person", KindPerson, gap+1, 10, fptr(0.7), false), // gap+1 second later
	}
	got := GroupMoments(items, time.Time{})
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2 (gap exceeded splits)", len(got))
	}
	// Most recent moment first.
	if got[0].RepresentativeEventID != "e2" {
		t.Errorf("got[0].rep = %q, want e2", got[0].RepresentativeEventID)
	}
	if got[1].RepresentativeEventID != "e1" {
		t.Errorf("got[1].rep = %q, want e1", got[1].RepresentativeEventID)
	}
}

func TestGroupMoments_TwoCamerasNeverMerge(t *testing.T) {
	items := []Item{
		mkItem("a", "cam1", "person", KindPerson, 0, 10, fptr(0.5), false),
		mkItem("b", "cam2", "person", KindPerson, 30, 10, fptr(0.6), false),
	}
	got := GroupMoments(items, time.Time{})
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2 (per-camera grouping)", len(got))
	}
	cams := map[string]bool{got[0].CamID: true, got[1].CamID: true}
	if !cams["cam1"] || !cams["cam2"] {
		t.Errorf("camera ids = %v, want both cam1 and cam2", cams)
	}
}

func TestGroupMoments_SinceFiltersStrictlyOlder(t *testing.T) {
	ref := time.Date(2026, 6, 4, 12, 0, 0, 0, time.UTC)
	items := []Item{
		mkItem("old", "cam1", "person", KindPerson, 0, 10, fptr(0.5), false),
		mkItem("at-since", "cam1", "person", KindPerson, 30, 10, fptr(0.6), false), // == since
		mkItem("new", "cam1", "person", KindPerson, 60, 10, fptr(0.7), false),
	}
	since := ref.Add(30 * time.Second) // matches at-since event exactly
	got := GroupMoments(items, since)
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1 (only strictly-newer events survive)", len(got))
	}
	if got[0].EventCount != 1 || got[0].RepresentativeEventID != "new" {
		t.Errorf("kept events = %d, rep = %q; want only 'new' to survive",
			got[0].EventCount, got[0].RepresentativeEventID)
	}
}

func TestGroupMoments_RepresentativePicksHighestScore(t *testing.T) {
	items := []Item{
		mkItem("low", "cam1", "person", KindPerson, 0, 10, fptr(0.3), false),
		mkItem("high", "cam1", "person", KindPerson, 30, 10, fptr(0.9), true),
		mkItem("mid", "cam1", "person", KindPerson, 60, 10, fptr(0.6), false),
	}
	got := GroupMoments(items, time.Time{})
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if got[0].RepresentativeEventID != "high" {
		t.Errorf("rep = %q, want 'high'", got[0].RepresentativeEventID)
	}
	if !got[0].RepresentativeHasClip {
		t.Error("rep has_clip should match the 'high' event")
	}
}

func TestGroupMoments_NilScoresFallback(t *testing.T) {
	items := []Item{
		mkItem("nil1", "cam1", "person", KindPerson, 0, 10, nil, false),
		mkItem("nil2", "cam1", "person", KindPerson, 30, 10, nil, true),
	}
	got := GroupMoments(items, time.Time{})
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	// All nil scores ⇒ tie ⇒ most recent wins.
	if got[0].RepresentativeEventID != "nil2" {
		t.Errorf("all-nil tiebreak rep = %q, want 'nil2' (most recent)",
			got[0].RepresentativeEventID)
	}

	mixed := []Item{
		mkItem("nil", "cam1", "person", KindPerson, 0, 10, nil, false),
		mkItem("real", "cam1", "person", KindPerson, 30, 10, fptr(0.1), false),
	}
	got = GroupMoments(mixed, time.Time{})
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if got[0].RepresentativeEventID != "real" {
		t.Errorf("nil-vs-real rep = %q, want 'real' (any score > nil)",
			got[0].RepresentativeEventID)
	}
}

func TestGroupMoments_RepresentativeScoreTieBreaksByRecency(t *testing.T) {
	items := []Item{
		mkItem("early", "cam1", "person", KindPerson, 0, 10, fptr(0.8), false),
		mkItem("late", "cam1", "person", KindPerson, 60, 10, fptr(0.8), false),
	}
	got := GroupMoments(items, time.Time{})
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if got[0].RepresentativeEventID != "late" {
		t.Errorf("score-tie rep = %q, want 'late'", got[0].RepresentativeEventID)
	}
}

func TestGroupMoments_NilEndedAtPropagates(t *testing.T) {
	items := []Item{
		mkItem("done", "cam1", "person", KindPerson, 0, 10, fptr(0.5), false),
		mkItem("ongoing", "cam1", "person", KindPerson, 30, -1, fptr(0.6), false),
	}
	got := GroupMoments(items, time.Time{})
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if got[0].EndedAt != nil {
		t.Errorf("ended_at = %v, want nil when any event is in-progress", *got[0].EndedAt)
	}
}

func TestGroupMoments_OrderingNewestFirst(t *testing.T) {
	gap := int64(MomentGap / time.Second)
	items := []Item{
		mkItem("c-old", "camC", "person", KindPerson, 0, 10, fptr(0.5), false),
		mkItem("a-mid", "camA", "person", KindPerson, gap+10, 10, fptr(0.5), false),
		mkItem("b-new", "camB", "person", KindPerson, 2*gap+20, 10, fptr(0.5), false),
	}
	got := GroupMoments(items, time.Time{})
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	want := []string{"camB", "camA", "camC"}
	for i, w := range want {
		if got[i].CamID != w {
			t.Errorf("got[%d].cam_id = %q, want %q", i, got[i].CamID, w)
		}
	}
}

func TestGroupMoments_UnsortedInputStillGroups(t *testing.T) {
	items := []Item{
		mkItem("e2", "cam1", "car", KindVehicle, 60, 30, fptr(0.8), true),
		mkItem("e1", "cam1", "person", KindPerson, 0, 30, fptr(0.5), false),
	}
	got := GroupMoments(items, time.Time{})
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1 (must sort internally)", len(got))
	}
	// Kinds in encounter order after sorting ⇒ person, then vehicle.
	want := []Kind{KindPerson, KindVehicle}
	if !reflect.DeepEqual(got[0].Kinds, want) {
		t.Errorf("kinds = %v, want %v", got[0].Kinds, want)
	}
}

func TestGroupMoments_EmptyInput(t *testing.T) {
	got := GroupMoments(nil, time.Time{})
	if len(got) != 0 {
		t.Errorf("len = %d, want 0", len(got))
	}
}
