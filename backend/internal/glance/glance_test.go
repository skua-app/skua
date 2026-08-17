// Timestamps in this file must be derived from time.Now(), never from a
// hardcoded calendar date. New prunes entries older than the retention
// window against the real wall clock (glance.go: pruneLocked(time.Now())),
// and drops a scope entirely once its seen set empties and its
// seen_through watermark is zero. A fixed past date therefore ages out on
// its own: the in-memory assertions still pass, the file is still written
// correctly, and then the reload comes back empty — so any test that
// stamps a fixed date and later reloads through New starts failing on a
// calendar boundary rather than on a code change. Moving such a date
// forward only rearms the same trap with a longer fuse.
//
// Offsets that need to straddle the cutoff come from glance.Retention
// (see export_test.go) so they keep exercising the real window if that
// constant is ever changed.
package glance_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/skua-app/skua/internal/glance"
)

func TestNew_MissingFile_StartsEmpty(t *testing.T) {
	dir := t.TempDir()
	store, err := glance.New(filepath.Join(dir, "glance.json"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := store.SeenSet(glance.ScopeHousehold); len(got) != 0 {
		t.Errorf("SeenSet = %v, want empty", got)
	}
}

func TestNew_CorruptFile_StartsEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "glance.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := glance.New(path)
	if err != nil {
		t.Fatalf("New must not error on corrupt file, got: %v", err)
	}
	if got := store.SeenSet(glance.ScopeHousehold); len(got) != 0 {
		t.Errorf("SeenSet = %v, want empty after corrupt file", got)
	}
}

func TestNew_OldLastSeenShape_StartsEmpty(t *testing.T) {
	// The pre-Model-B file shape: { "last_seen": "<RFC3339>" }. New
	// must accept it without error and start with an empty seen-set.
	// The date in the payload is inert: the string value fails to
	// unmarshal into scopeState, so it never becomes a seen-at stamp
	// and retention never looks at it.
	dir := t.TempDir()
	path := filepath.Join(dir, "glance.json")
	if err := os.WriteFile(path, []byte(`{"last_seen":"2026-05-20T22:00:00Z"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := glance.New(path)
	if err != nil {
		t.Fatalf("New must not error on old shape, got: %v", err)
	}
	if got := store.SeenSet(glance.ScopeHousehold); len(got) != 0 {
		t.Errorf("SeenSet = %v, want empty (no migration from last_seen)", got)
	}
}

func TestMarkSeen_PersistsAndReloadsRoundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "glance.json")
	store, err := glance.New(path)
	if err != nil {
		t.Fatal(err)
	}
	// Well inside the retention window so the reload below keeps both ids.
	now := time.Now().Add(-time.Hour)
	if err := store.MarkSeen(glance.ScopeHousehold, []string{"a", "b"}, now); err != nil {
		t.Fatalf("MarkSeen: %v", err)
	}

	got := store.SeenSet(glance.ScopeHousehold)
	if _, ok := got["a"]; !ok {
		t.Errorf("SeenSet missing %q", "a")
	}
	if _, ok := got["b"]; !ok {
		t.Errorf("SeenSet missing %q", "b")
	}

	reloaded, err := glance.New(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	got2 := reloaded.SeenSet(glance.ScopeHousehold)
	if _, ok := got2["a"]; !ok {
		t.Errorf("reloaded SeenSet missing %q", "a")
	}
	if _, ok := got2["b"]; !ok {
		t.Errorf("reloaded SeenSet missing %q", "b")
	}
}

func TestMarkSeen_Idempotent_RefreshesTimestamp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "glance.json")
	store, err := glance.New(path)
	if err != nil {
		t.Fatal(err)
	}
	// Fixed instants are safe here, and wanted: the assertion below is on
	// the exact serialized unix value, and this test reads the file
	// directly rather than reloading through New, so the wall-clock prune
	// on load never sees these stamps. Pruning inside MarkSeen uses the
	// caller's `at`, against which both stamps are current.
	first := time.Date(2026, 6, 4, 12, 0, 0, 0, time.UTC)
	if err := store.MarkSeen(glance.ScopeHousehold, []string{"a"}, first); err != nil {
		t.Fatalf("first MarkSeen: %v", err)
	}
	second := first.Add(2 * time.Hour)
	if err := store.MarkSeen(glance.ScopeHousehold, []string{"a"}, second); err != nil {
		t.Fatalf("second MarkSeen: %v", err)
	}

	// The on-disk file must hold the refreshed timestamp, not the
	// older one. Re-marking is idempotent on the id and "wins" with
	// the newer at value.
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var decoded map[string]struct {
		Seen        map[string]int64 `json:"seen"`
		SeenThrough int64            `json:"seen_through"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got := decoded[glance.ScopeHousehold].Seen["a"]; got != second.Unix() {
		t.Errorf("a ts = %d, want %d (refreshed)", got, second.Unix())
	}
}

func TestMarkSeen_ScopeIsolation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "glance.json")
	store, err := glance.New(path)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().Add(-time.Hour)
	if err := store.MarkSeen(glance.ScopeHousehold, []string{"x"}, now); err != nil {
		t.Fatalf("MarkSeen: %v", err)
	}
	if got := store.SeenSet("other-scope"); len(got) != 0 {
		t.Errorf("other-scope SeenSet = %v, want empty", got)
	}
	if _, ok := store.SeenSet(glance.ScopeHousehold)["x"]; !ok {
		t.Errorf("household SeenSet should still contain x")
	}
}

func TestMarkSeen_EmptyIDs_NoWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "glance.json")
	store, err := glance.New(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.MarkSeen(glance.ScopeHousehold, nil, time.Now()); err != nil {
		t.Fatalf("MarkSeen nil: %v", err)
	}
	if err := store.MarkSeen(glance.ScopeHousehold, []string{}, time.Now()); err != nil {
		t.Fatalf("MarkSeen empty: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("file exists after empty MarkSeen, want missing (err=%v)", err)
	}
}

func TestMarkSeen_PrunesOldEntriesAfterWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "glance.json")
	store, err := glance.New(path)
	if err != nil {
		t.Fatal(err)
	}
	// Seed "old" far enough back that it sits clearly outside the
	// retention window as of the later write, then re-mark a different
	// id at that later point. The stale entry must be pruned in the
	// same write. Both offsets derive from the retention constant, so
	// this keeps straddling the real cutoff if the window changes.
	tLate := time.Now()
	t0 := tLate.Add(-glance.Retention - time.Hour)
	if err := store.MarkSeen(glance.ScopeHousehold, []string{"old"}, t0); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := store.MarkSeen(glance.ScopeHousehold, []string{"fresh"}, tLate); err != nil {
		t.Fatalf("late MarkSeen: %v", err)
	}

	got := store.SeenSet(glance.ScopeHousehold)
	if _, ok := got["old"]; ok {
		t.Errorf("old id still present after retention prune; got=%v", got)
	}
	if _, ok := got["fresh"]; !ok {
		t.Errorf("fresh id missing; got=%v", got)
	}
}

func TestNew_PrunesOldEntriesOnLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "glance.json")
	// Hand-craft a file where the entry is far older than the
	// retention window. New must drop it on load and leave the
	// scope empty (then also drop the now-empty scope when its
	// seen_through watermark is zero).
	veryOld := time.Now().Add(-glance.Retention - 24*time.Hour).Unix()
	payload, err := json.Marshal(map[string]map[string]any{
		glance.ScopeHousehold: {
			"seen": map[string]int64{"stale": veryOld},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := glance.New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if got := store.SeenSet(glance.ScopeHousehold); len(got) != 0 {
		t.Errorf("SeenSet after load+prune = %v, want empty", got)
	}
}

func TestMarkAllSeen_SetsWatermarkAndPersists(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "glance.json")
	store, err := glance.New(path)
	if err != nil {
		t.Fatal(err)
	}
	// Relative to now so the reload below never sees an aged-out stamp.
	// Truncated to the second because the watermark round-trips through
	// unix seconds, so a sub-second component would not survive.
	at := time.Now().Add(-time.Hour).Truncate(time.Second)
	if err := store.MarkAllSeen(glance.ScopeHousehold, at); err != nil {
		t.Fatalf("MarkAllSeen: %v", err)
	}
	if got := store.SeenThrough(glance.ScopeHousehold); !got.Equal(at) {
		t.Errorf("SeenThrough = %v, want %v", got, at)
	}

	reloaded, err := glance.New(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got := reloaded.SeenThrough(glance.ScopeHousehold); !got.Equal(at) {
		t.Errorf("reloaded SeenThrough = %v, want %v", got, at)
	}
}

func TestMarkAllSeen_PreservesScopeAcrossPrune_WithEmptySeen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "glance.json")
	store, err := glance.New(path)
	if err != nil {
		t.Fatal(err)
	}
	// Relative to now (and second-truncated) so both the watermark and
	// the other-scope id below stay inside the retention window across
	// the reload.
	at := time.Now().Add(-time.Hour).Truncate(time.Second)
	if err := store.MarkAllSeen(glance.ScopeHousehold, at); err != nil {
		t.Fatalf("MarkAllSeen: %v", err)
	}

	// A subsequent MarkSeen on a different scope must not drop the
	// household scope just because its seen set is empty — the
	// seen_through watermark must survive pruning.
	if err := store.MarkSeen("other-scope", []string{"x"}, at.Add(time.Hour)); err != nil {
		t.Fatalf("MarkSeen other: %v", err)
	}
	if got := store.SeenThrough(glance.ScopeHousehold); !got.Equal(at) {
		t.Errorf("SeenThrough after prune = %v, want %v", got, at)
	}

	reloaded, err := glance.New(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got := reloaded.SeenThrough(glance.ScopeHousehold); !got.Equal(at) {
		t.Errorf("reloaded SeenThrough = %v, want %v", got, at)
	}
}

func TestSeenThrough_AbsentScope_ReturnsZero(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "glance.json")
	store, err := glance.New(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := store.SeenThrough(glance.ScopeHousehold); !got.IsZero() {
		t.Errorf("SeenThrough for unset scope = %v, want zero", got)
	}
}

func TestMarkSeen_CreatesMissingDataDir(t *testing.T) {
	dir := t.TempDir()
	nested := filepath.Join(dir, "nested", "subdir")
	path := filepath.Join(nested, "glance.json")
	store, err := glance.New(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.MarkSeen(glance.ScopeHousehold, []string{"id1"}, time.Now()); err != nil {
		t.Fatalf("MarkSeen on missing parent dir: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected glance.json at %s, got: %v", path, err)
	}
}

func TestMarkSeen_ConcurrentCalls_DoNotCorruptFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "glance.json")
	store, err := glance.New(path)
	if err != nil {
		t.Fatal(err)
	}
	// Well inside the retention window so the reload below keeps all n ids.
	now := time.Now().Add(-time.Hour)

	const n = 50
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_ = store.MarkSeen(glance.ScopeHousehold, []string{
				"id-" + itoa(i),
			}, now)
		}(i)
	}
	wg.Wait()

	reloaded, err := glance.New(path)
	if err != nil {
		t.Fatalf("reload after concurrent: %v", err)
	}
	got := reloaded.SeenSet(glance.ScopeHousehold)
	if len(got) != n {
		t.Errorf("reloaded SeenSet size = %d, want %d", len(got), n)
	}
}

// itoa is a tiny helper that avoids strconv just to keep the test
// imports minimal; the index is always non-negative.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
