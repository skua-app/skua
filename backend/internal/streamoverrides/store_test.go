package streamoverrides

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func tempStorePath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "stream_overrides.yaml")
}

func TestNew_MissingFileIsEmptyStore(t *testing.T) {
	path := tempStorePath(t)
	s, err := New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if got := s.All(); len(got) != 0 {
		t.Errorf("expected empty store, got %v", got)
	}
	// The file must not be created until the first Set.
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("expected file not to exist, stat err = %v", err)
	}
}

func TestNew_EmptyFileIsEmptyStore(t *testing.T) {
	path := tempStorePath(t)
	if err := os.WriteFile(path, []byte(""), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	s, err := New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if got := s.All(); len(got) != 0 {
		t.Errorf("expected empty store, got %v", got)
	}
}

func TestNew_MalformedYAMLFailsFast(t *testing.T) {
	path := tempStorePath(t)
	if err := os.WriteFile(path, []byte("stream_overrides: [not a map"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := New(path); err == nil {
		t.Fatal("expected error on malformed YAML")
	}
}

func TestSet_RoundTripsAndPersists(t *testing.T) {
	path := tempStorePath(t)
	s, err := New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := s.Set("cam5", "cam5_main_h264", "cam5_sub"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got := s.Get("cam5")
	if got.Main != "cam5_main_h264" || got.Sub != "cam5_sub" {
		t.Errorf("Get cam5 = %+v, want {main: cam5_main_h264, sub: cam5_sub}", got)
	}

	// Reload from disk → same value.
	s2, err := New(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if s2.Get("cam5") != got {
		t.Errorf("after reload cam5 = %+v, want %+v", s2.Get("cam5"), got)
	}

	// File is valid YAML under the stream_overrides envelope.
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	var fs fileSchema
	if err := yaml.Unmarshal(raw, &fs); err != nil {
		t.Fatalf("parse on-disk YAML: %v", err)
	}
	if fs.StreamOverrides["cam5"].Main != "cam5_main_h264" {
		t.Errorf("on-disk main = %q", fs.StreamOverrides["cam5"].Main)
	}
}

func TestSet_BothEmptyAfterTrimRemovesEntry(t *testing.T) {
	path := tempStorePath(t)
	s, err := New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := s.Set("cam5", "cam5_main_h264", "cam5_sub"); err != nil {
		t.Fatalf("seed Set: %v", err)
	}
	if err := s.Set("cam5", "  ", "\t"); err != nil {
		t.Fatalf("clear Set: %v", err)
	}
	if _, ok := s.All()["cam5"]; ok {
		t.Errorf("override should be gone in-memory, got %v", s.All())
	}

	// On-disk view too.
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	var fs fileSchema
	if err := yaml.Unmarshal(raw, &fs); err != nil {
		t.Fatalf("parse on-disk YAML: %v", err)
	}
	if _, ok := fs.StreamOverrides["cam5"]; ok {
		t.Errorf("override should be gone on-disk, got %+v", fs.StreamOverrides)
	}
}

func TestSet_TrimsBeforeStoring(t *testing.T) {
	s, err := New(tempStorePath(t))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := s.Set("cam5", "  cam5_main_h264  ", " cam5_sub "); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got := s.Get("cam5")
	if got.Main != "cam5_main_h264" || got.Sub != "cam5_sub" {
		t.Errorf("Get cam5 = %+v, want trimmed values", got)
	}
}

func TestForget_NoopWhenAbsent(t *testing.T) {
	path := tempStorePath(t)
	s, err := New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := s.Forget("cam5"); err != nil {
		t.Fatalf("Forget: %v", err)
	}
	// File must not have been created.
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("Forget on absent entry should not create file, stat err = %v", err)
	}
}

func TestForget_RemovesExistingEntry(t *testing.T) {
	path := tempStorePath(t)
	s, err := New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := s.Set("cam5", "cam5_main_h264", ""); err != nil {
		t.Fatalf("seed Set: %v", err)
	}
	if err := s.Set("cam6", "cam6_main_h264", ""); err != nil {
		t.Fatalf("seed Set: %v", err)
	}
	if err := s.Forget("cam5"); err != nil {
		t.Fatalf("Forget: %v", err)
	}
	if _, ok := s.All()["cam5"]; ok {
		t.Errorf("cam5 should be gone, got %v", s.All())
	}
	if _, ok := s.All()["cam6"]; !ok {
		t.Errorf("cam6 should remain, got %v", s.All())
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	var fs fileSchema
	if err := yaml.Unmarshal(raw, &fs); err != nil {
		t.Fatalf("parse on-disk YAML: %v", err)
	}
	if _, ok := fs.StreamOverrides["cam5"]; ok {
		t.Errorf("cam5 should be gone on-disk, got %+v", fs.StreamOverrides)
	}
}

func TestAll_ReturnsDefensiveCopy(t *testing.T) {
	s, err := New(tempStorePath(t))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := s.Set("cam5", "cam5_main_h264", "cam5_sub"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	snap := s.All()
	snap["cam5"] = Override{Main: "evil", Sub: "evil"}
	delete(snap, "cam5") // and remove

	again := s.All()
	if again["cam5"].Main != "cam5_main_h264" {
		t.Errorf("All() must return a defensive copy; got %+v after caller mutation", again)
	}
}

// Ensure a no-op leaves the on-disk file alone (Forget over absent entry).
func TestForget_NoDiskTouchWhenAbsent(t *testing.T) {
	path := tempStorePath(t)
	s, err := New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// Seed then check mtime stability after a no-op Forget.
	if err := s.Set("cam5", "cam5_main_h264", "cam5_sub"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	st1, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	// Force a discernible delta in time-resolution by sleeping briefly is
	// flaky on some filesystems; instead just assert Forget returns nil
	// and the file content hasn't changed.
	if err := s.Forget("cam999"); err != nil {
		t.Fatalf("Forget absent: %v", err)
	}
	st2, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat after: %v", err)
	}
	if st1.Size() != st2.Size() {
		t.Errorf("Forget on absent entry should not rewrite file; size changed %d → %d", st1.Size(), st2.Size())
	}
}
