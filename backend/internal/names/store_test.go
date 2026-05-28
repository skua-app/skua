package names

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// alwaysExist is the camera-existence stub used by tests that don't care
// about validation; callers that test camera_not_found use their own stub.
func alwaysExist(string) bool { return true }

func tempStorePath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "camera_names.yaml")
}

func TestNew_MissingFileIsEmptyStore(t *testing.T) {
	s, err := New(tempStorePath(t), alwaysExist)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if got := s.All(); len(got) != 0 {
		t.Errorf("expected empty store, got %v", got)
	}
}

func TestNew_LoadsAndTrimsExistingFile(t *testing.T) {
	path := tempStorePath(t)
	body := []byte("names:\n  cam1: \"  Yard  \"\n  cam2: \"\"\n  cam3: Driveway\n")
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	s, err := New(path, alwaysExist)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	all := s.All()
	if all["cam1"] != "Yard" {
		t.Errorf("cam1 = %q, want %q (trimmed)", all["cam1"], "Yard")
	}
	if _, ok := all["cam2"]; ok {
		t.Errorf("empty override should be dropped on load, got %q", all["cam2"])
	}
	if all["cam3"] != "Driveway" {
		t.Errorf("cam3 = %q, want %q", all["cam3"], "Driveway")
	}
}

func TestNew_MalformedYAMLFailsFast(t *testing.T) {
	path := tempStorePath(t)
	if err := os.WriteFile(path, []byte("names: [this is not a map"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := New(path, alwaysExist); err == nil {
		t.Fatal("expected error on malformed YAML")
	}
}

func TestSet_PersistsAndPicksUpOnReload(t *testing.T) {
	path := tempStorePath(t)
	s, err := New(path, alwaysExist)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	got, err := s.Set("cam1", "  Yard  ")
	if err != nil {
		t.Fatalf("Set: %v", err)
	}
	if got != "Yard" {
		t.Errorf("Set returned %q, want %q (trimmed)", got, "Yard")
	}

	// Reload from disk → same value.
	s2, err := New(path, alwaysExist)
	if err != nil {
		t.Fatalf("reload New: %v", err)
	}
	if s2.Get("cam1") != "Yard" {
		t.Errorf("after reload cam1 = %q, want %q", s2.Get("cam1"), "Yard")
	}

	// File is valid YAML and contains the override key.
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	var fs fileSchema
	if err := yaml.Unmarshal(raw, &fs); err != nil {
		t.Fatalf("parse on-disk YAML: %v", err)
	}
	if fs.Names["cam1"] != "Yard" {
		t.Errorf("on-disk cam1 = %q", fs.Names["cam1"])
	}
}

func TestSet_EmptyDeletesOverride(t *testing.T) {
	s, err := New(tempStorePath(t), alwaysExist)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := s.Set("cam1", "Yard"); err != nil {
		t.Fatalf("seed Set: %v", err)
	}
	got, err := s.Set("cam1", "   ")
	if err != nil {
		t.Fatalf("clear Set: %v", err)
	}
	if got != "" {
		t.Errorf("clearing should return empty string, got %q", got)
	}
	if _, ok := s.All()["cam1"]; ok {
		t.Errorf("override should be gone, got %v", s.All())
	}
}

func TestSet_RejectsUnknownCamera(t *testing.T) {
	only := func(id string) bool { return id == "cam1" }
	s, err := New(tempStorePath(t), only)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := s.Set("cam999", "Anything"); !errors.Is(err, ErrCameraNotFound) {
		t.Errorf("expected ErrCameraNotFound, got %v", err)
	}
}

func TestSet_RejectsTooLongName(t *testing.T) {
	s, err := New(tempStorePath(t), alwaysExist)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	long := strings.Repeat("ä", nameMaxLen+1) // count by runes, not bytes
	if _, err := s.Set("cam1", long); !errors.Is(err, ErrNameTooLong) {
		t.Errorf("expected ErrNameTooLong, got %v", err)
	}
}

func TestResolve_PrefersOverride(t *testing.T) {
	s, err := New(tempStorePath(t), alwaysExist)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if got := s.Resolve("cam1", "Cam 1"); got != "Cam 1" {
		t.Errorf("no override → fallback expected, got %q", got)
	}
	if _, err := s.Set("cam1", "Yard"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if got := s.Resolve("cam1", "Cam 1"); got != "Yard" {
		t.Errorf("override expected, got %q", got)
	}
}

func TestResolve_NilStoreFallsBack(t *testing.T) {
	var s *Store
	if got := s.Resolve("cam1", "Cam 1"); got != "Cam 1" {
		t.Errorf("nil store should fall back, got %q", got)
	}
}
