package glance_test

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/skua-app/skua/internal/glance"
)

func TestNew_MissingFile_ReturnsNeverSeen(t *testing.T) {
	dir := t.TempDir()
	store, err := glance.New(filepath.Join(dir, "glance.json"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !store.LastSeen().IsZero() {
		t.Errorf("LastSeen = %v, want zero (never-seen)", store.LastSeen())
	}
}

func TestNew_CorruptFile_StartsFromNeverSeen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "glance.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := glance.New(path)
	if err != nil {
		t.Fatalf("New must not error on corrupt file, got: %v", err)
	}
	if !store.LastSeen().IsZero() {
		t.Errorf("LastSeen = %v, want zero after corrupt file", store.LastSeen())
	}
}

func TestNew_UnparseableLastSeen_StartsFromNeverSeen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "glance.json")
	if err := os.WriteFile(path, []byte(`{"last_seen":"not-a-date"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := glance.New(path)
	if err != nil {
		t.Fatalf("New must not error on unparseable last_seen, got: %v", err)
	}
	if !store.LastSeen().IsZero() {
		t.Errorf("LastSeen = %v, want zero", store.LastSeen())
	}
}

func TestNew_NullLastSeen_StartsFromNeverSeen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "glance.json")
	if err := os.WriteFile(path, []byte(`{"last_seen":null}`), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := glance.New(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !store.LastSeen().IsZero() {
		t.Errorf("LastSeen = %v, want zero", store.LastSeen())
	}
}

func TestAck_PersistsAndReloadsRoundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "glance.json")
	store, err := glance.New(path)
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 6, 4, 12, 30, 0, 0, time.UTC)
	got, err := store.Ack(want)
	if err != nil {
		t.Fatalf("Ack: %v", err)
	}
	if !got.Equal(want) {
		t.Errorf("Ack returned %v, want %v", got, want)
	}

	reloaded, err := glance.New(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if !reloaded.LastSeen().Equal(want) {
		t.Errorf("reloaded LastSeen = %v, want %v", reloaded.LastSeen(), want)
	}
}

func TestAck_OlderTimestamp_IsNoop(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "glance.json")
	store, err := glance.New(path)
	if err != nil {
		t.Fatal(err)
	}
	current := time.Date(2026, 6, 4, 12, 30, 0, 0, time.UTC)
	if _, err := store.Ack(current); err != nil {
		t.Fatalf("seed Ack: %v", err)
	}
	stat1, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat after seed: %v", err)
	}
	// Sleep enough that any subsequent rename would change mtime.
	time.Sleep(20 * time.Millisecond)

	older := current.Add(-1 * time.Hour)
	got, err := store.Ack(older)
	if err != nil {
		t.Fatalf("older Ack: %v", err)
	}
	if !got.Equal(current) {
		t.Errorf("Ack(older) returned %v, want %v (no advance)", got, current)
	}
	stat2, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat after older: %v", err)
	}
	if !stat1.ModTime().Equal(stat2.ModTime()) {
		t.Errorf("file mtime changed; older Ack should not rewrite")
	}
	if !store.LastSeen().Equal(current) {
		t.Errorf("LastSeen = %v, want %v (no advance)", store.LastSeen(), current)
	}
}

func TestAck_EqualTimestamp_IsNoop(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "glance.json")
	store, err := glance.New(path)
	if err != nil {
		t.Fatal(err)
	}
	current := time.Date(2026, 6, 4, 12, 30, 0, 0, time.UTC)
	if _, err := store.Ack(current); err != nil {
		t.Fatalf("seed Ack: %v", err)
	}
	stat1, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat after seed: %v", err)
	}
	time.Sleep(20 * time.Millisecond)
	if _, err := store.Ack(current); err != nil {
		t.Fatalf("equal Ack: %v", err)
	}
	stat2, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat after equal: %v", err)
	}
	if !stat1.ModTime().Equal(stat2.ModTime()) {
		t.Errorf("file mtime changed; equal Ack should not rewrite")
	}
}

func TestAck_CreatesMissingDataDir(t *testing.T) {
	dir := t.TempDir()
	nested := filepath.Join(dir, "nested", "subdir")
	path := filepath.Join(nested, "glance.json")
	store, err := glance.New(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Ack(time.Date(2026, 6, 4, 12, 30, 0, 0, time.UTC)); err != nil {
		t.Fatalf("Ack on missing parent dir: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected glance.json at %s, got: %v", path, err)
	}
}

func TestAck_ConcurrentCalls_DoNotCorruptFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "glance.json")
	store, err := glance.New(path)
	if err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 6, 4, 12, 0, 0, 0, time.UTC)

	const n = 50
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, _ = store.Ack(base.Add(time.Duration(i) * time.Second))
		}(i)
	}
	wg.Wait()

	reloaded, err := glance.New(path)
	if err != nil {
		t.Fatalf("reload after concurrent: %v", err)
	}
	// Latest must win monotonically; cannot be earlier than base.
	if reloaded.LastSeen().Before(base) {
		t.Errorf("reloaded LastSeen = %v, want >= %v", reloaded.LastSeen(), base)
	}
}
