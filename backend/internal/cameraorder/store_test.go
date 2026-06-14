package cameraorder

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"gopkg.in/yaml.v3"
)

func alwaysExist(string) bool { return true }

func only(ids ...string) CameraExistsFunc {
	set := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		set[id] = struct{}{}
	}
	return func(id string) bool {
		_, ok := set[id]
		return ok
	}
}

func tempStorePath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "camera_order.yaml")
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

func TestNew_LoadsAndFiltersExistingFile(t *testing.T) {
	path := tempStorePath(t)
	body := []byte("order:\n  - cam1\n  - cam2\n  - cam2\n  - cam_ghost\n  - cam3\n")
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	s, err := New(path, only("cam1", "cam2", "cam3"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	got := s.All()
	want := []string{"cam1", "cam2", "cam3"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("order = %v, want %v (dedup + filter ghost)", got, want)
	}
}

func TestNew_MalformedYAMLFailsFast(t *testing.T) {
	path := tempStorePath(t)
	if err := os.WriteFile(path, []byte("order: [this is not a list"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := New(path, alwaysExist); err == nil {
		t.Fatal("expected error on malformed YAML")
	}
}

func TestSet_PersistsAndPicksUpOnReload(t *testing.T) {
	path := tempStorePath(t)
	exists := only("cam1", "cam2", "cam3")
	s, err := New(path, exists)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	got, err := s.Set([]string{"cam3", "cam1", "cam2"})
	if err != nil {
		t.Fatalf("Set: %v", err)
	}
	want := []string{"cam3", "cam1", "cam2"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Set returned %v, want %v", got, want)
	}

	s2, err := New(path, exists)
	if err != nil {
		t.Fatalf("reload New: %v", err)
	}
	if !reflect.DeepEqual(s2.All(), want) {
		t.Errorf("after reload order = %v, want %v", s2.All(), want)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	var fs fileSchema
	if err := yaml.Unmarshal(raw, &fs); err != nil {
		t.Fatalf("parse on-disk YAML: %v", err)
	}
	if !reflect.DeepEqual(fs.Order, want) {
		t.Errorf("on-disk order = %v, want %v", fs.Order, want)
	}
}

func TestSet_StripsUnknownAndDuplicates(t *testing.T) {
	s, err := New(tempStorePath(t), only("cam1", "cam2"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	got, err := s.Set([]string{"cam2", "cam_ghost", "cam1", "cam1", ""})
	if err != nil {
		t.Fatalf("Set: %v", err)
	}
	want := []string{"cam2", "cam1"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Set returned %v, want %v", got, want)
	}
}

func TestApply_PutsSavedFirstThenAppendsNewCams(t *testing.T) {
	s, err := New(tempStorePath(t), only("cam1", "cam2", "cam3"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := s.Set([]string{"cam3", "cam1"}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	// "cam4" is unknown to the saved order — must be appended at the end.
	// "cam2" was registered but never ordered — must be appended too.
	// "cam99" never had an entry — appended in input order.
	got := s.Apply([]string{"cam1", "cam2", "cam3", "cam4", "cam99"})
	want := []string{"cam3", "cam1", "cam2", "cam4", "cam99"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Apply = %v, want %v", got, want)
	}
}

func TestApply_DropsSavedIdsAbsentFromInput(t *testing.T) {
	s, err := New(tempStorePath(t), only("cam1", "cam2", "cam3"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := s.Set([]string{"cam3", "cam1", "cam2"}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got := s.Apply([]string{"cam1", "cam3"})
	want := []string{"cam3", "cam1"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Apply = %v, want %v", got, want)
	}
}

func TestApply_NilStorePassesThrough(t *testing.T) {
	var s *Store
	got := s.Apply([]string{"cam1", "cam2"})
	want := []string{"cam1", "cam2"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("nil store should pass through, got %v", got)
	}
}

func TestForget_RemovesEntryAndPersists(t *testing.T) {
	path := tempStorePath(t)
	s, err := New(path, only("cam1", "cam2", "cam3"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := s.Set([]string{"cam3", "cam1", "cam2"}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := s.Forget("cam1"); err != nil {
		t.Fatalf("Forget: %v", err)
	}
	want := []string{"cam3", "cam2"}
	if !reflect.DeepEqual(s.All(), want) {
		t.Errorf("after Forget order = %v, want %v", s.All(), want)
	}
	// Reload to confirm persistence.
	s2, err := New(path, alwaysExist)
	if err != nil {
		t.Fatalf("reload New: %v", err)
	}
	if !reflect.DeepEqual(s2.All(), want) {
		t.Errorf("after reload order = %v, want %v", s2.All(), want)
	}
}

func TestForget_AbsentIsNoOp(t *testing.T) {
	path := tempStorePath(t)
	s, err := New(path, only("cam1", "cam2"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := s.Set([]string{"cam1", "cam2"}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	stat1, _ := os.Stat(path)
	if err := s.Forget("cam_ghost"); err != nil {
		t.Fatalf("Forget: %v", err)
	}
	stat2, _ := os.Stat(path)
	if stat1 != nil && stat2 != nil && !stat1.ModTime().Equal(stat2.ModTime()) {
		t.Errorf("Forget on absent id should not rewrite the file")
	}
}
