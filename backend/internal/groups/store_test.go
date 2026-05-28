package groups_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/skua-app/skua/internal/groups"
)

func knownCamera(id string) bool {
	switch id {
	case "cam1", "cam2", "cam3", "cam4", "cam5", "cam6", "cam7":
		return true
	}
	return false
}

func newTestStore(t *testing.T) (*groups.Store, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "groups.yaml")
	store, err := groups.New(path, knownCamera)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	return store, path
}

func TestNew_MissingFile_ReturnsEmptyStore(t *testing.T) {
	store, _ := newTestStore(t)
	if got := store.List(); len(got) != 0 {
		t.Fatalf("want empty store, got %d groups", len(got))
	}
}

func TestNew_MalformedYAML_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "groups.yaml")
	if err := os.WriteFile(path, []byte("groups: [this is not valid"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := groups.New(path, knownCamera); err == nil {
		t.Fatal("expected error on malformed YAML, got nil")
	}
}

func TestCreate_HappyPath(t *testing.T) {
	store, _ := newTestStore(t)
	g, err := store.Create("Street")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if g.ID == "" {
		t.Error("expected non-empty id")
	}
	if g.Name != "Street" {
		t.Errorf("name: got %q, want %q", g.Name, "Street")
	}
	if len(g.CameraIDs) != 0 {
		t.Errorf("camera_ids: got %v, want []", g.CameraIDs)
	}

	list := store.List()
	if len(list) != 1 || list[0].ID != g.ID {
		t.Errorf("list: %+v", list)
	}
}

func TestCreate_TrimsWhitespaceAndValidatesEmpty(t *testing.T) {
	store, _ := newTestStore(t)
	if _, err := store.Create("   "); !errors.Is(err, groups.ErrNameEmpty) {
		t.Errorf("empty name: got %v, want ErrNameEmpty", err)
	}
	if _, err := store.Create(""); !errors.Is(err, groups.ErrNameEmpty) {
		t.Errorf("empty name: got %v, want ErrNameEmpty", err)
	}
}

func TestCreate_RejectsLongName(t *testing.T) {
	store, _ := newTestStore(t)
	long := strings.Repeat("ä", 31)
	if _, err := store.Create(long); !errors.Is(err, groups.ErrNameTooLong) {
		t.Errorf("got %v, want ErrNameTooLong", err)
	}
}

func TestCreate_RejectsDuplicateNameCaseInsensitive(t *testing.T) {
	store, _ := newTestStore(t)
	if _, err := store.Create("Street"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create("street"); !errors.Is(err, groups.ErrNameDuplicate) {
		t.Errorf("got %v, want ErrNameDuplicate", err)
	}
}

func TestUpdate_NameAndCameras(t *testing.T) {
	store, _ := newTestStore(t)
	g, _ := store.Create("Street")

	newName := "Street — front"
	cams := []string{"cam1", "cam2"}
	updated, err := store.Update(g.ID, &newName, &cams)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Name != newName {
		t.Errorf("name: got %q, want %q", updated.Name, newName)
	}
	if len(updated.CameraIDs) != 2 || updated.CameraIDs[0] != "cam1" || updated.CameraIDs[1] != "cam2" {
		t.Errorf("camera_ids: got %v", updated.CameraIDs)
	}
}

func TestUpdate_UnknownCamera(t *testing.T) {
	store, _ := newTestStore(t)
	g, _ := store.Create("Street")
	cams := []string{"cam99"}
	if _, err := store.Update(g.ID, nil, &cams); !errors.Is(err, groups.ErrCameraNotFound) {
		t.Errorf("got %v, want ErrCameraNotFound", err)
	}
}

func TestUpdate_DuplicateNameRejected(t *testing.T) {
	store, _ := newTestStore(t)
	g1, _ := store.Create("Street")
	_, _ = store.Create("House")

	dup := "House"
	if _, err := store.Update(g1.ID, &dup, nil); !errors.Is(err, groups.ErrNameDuplicate) {
		t.Errorf("got %v, want ErrNameDuplicate", err)
	}
}

func TestUpdate_RenameToSelf_Allowed(t *testing.T) {
	store, _ := newTestStore(t)
	g, _ := store.Create("Street")
	same := "Street"
	if _, err := store.Update(g.ID, &same, nil); err != nil {
		t.Errorf("rename to self should be allowed, got %v", err)
	}
}

func TestUpdate_SingleMembershipReconciliation(t *testing.T) {
	store, _ := newTestStore(t)
	a, _ := store.Create("Street")
	b, _ := store.Create("House")

	camsA := []string{"cam1", "cam2", "cam3"}
	if _, err := store.Update(a.ID, nil, &camsA); err != nil {
		t.Fatal(err)
	}
	camsB := []string{"cam3", "cam4"}
	if _, err := store.Update(b.ID, nil, &camsB); err != nil {
		t.Fatal(err)
	}

	gotA, _ := store.Get(a.ID)
	if got := gotA.CameraIDs; len(got) != 2 || got[0] != "cam1" || got[1] != "cam2" {
		t.Errorf("group A after reconcile: got %v, want [cam1 cam2]", got)
	}
	gotB, _ := store.Get(b.ID)
	if got := gotB.CameraIDs; len(got) != 2 || got[0] != "cam3" || got[1] != "cam4" {
		t.Errorf("group B after reconcile: got %v, want [cam3 cam4]", got)
	}
}

func TestUpdate_DuplicateCameraInSameGroupRejected(t *testing.T) {
	store, _ := newTestStore(t)
	g, _ := store.Create("Street")
	cams := []string{"cam1", "cam1"}
	if _, err := store.Update(g.ID, nil, &cams); !errors.Is(err, groups.ErrDuplicateCamera) {
		t.Errorf("got %v, want ErrDuplicateCamera", err)
	}
}

func TestUpdate_NotFound(t *testing.T) {
	store, _ := newTestStore(t)
	name := "X"
	if _, err := store.Update("missing-id", &name, nil); !errors.Is(err, groups.ErrNotFound) {
		t.Errorf("got %v, want ErrNotFound", err)
	}
}

func TestDelete_RemovesGroup(t *testing.T) {
	store, _ := newTestStore(t)
	a, _ := store.Create("Street")
	if err := store.Delete(a.ID); err != nil {
		t.Fatal(err)
	}
	if _, ok := store.Get(a.ID); ok {
		t.Error("deleted group still present")
	}
	if list := store.List(); len(list) != 0 {
		t.Errorf("list after delete: %v", list)
	}
}

func TestDelete_NotFound(t *testing.T) {
	store, _ := newTestStore(t)
	if err := store.Delete("missing"); !errors.Is(err, groups.ErrNotFound) {
		t.Errorf("got %v, want ErrNotFound", err)
	}
}

func TestGroupFor(t *testing.T) {
	store, _ := newTestStore(t)
	g, _ := store.Create("Street")
	cams := []string{"cam1", "cam2"}
	if _, err := store.Update(g.ID, nil, &cams); err != nil {
		t.Fatal(err)
	}
	if got := store.GroupFor("cam1"); got != g.ID {
		t.Errorf("GroupFor(cam1): got %q, want %q", got, g.ID)
	}
	if got := store.GroupFor("cam7"); got != "" {
		t.Errorf("GroupFor(cam7): got %q, want empty", got)
	}
}

func TestPersistence_ReloadFromDisk(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "groups.yaml")
	s1, err := groups.New(path, knownCamera)
	if err != nil {
		t.Fatal(err)
	}
	g, _ := s1.Create("Street")
	cams := []string{"cam1", "cam2"}
	if _, err := s1.Update(g.ID, nil, &cams); err != nil {
		t.Fatal(err)
	}

	s2, err := groups.New(path, knownCamera)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	got, ok := s2.Get(g.ID)
	if !ok {
		t.Fatal("group missing after reload")
	}
	if got.Name != "Street" {
		t.Errorf("name after reload: %q", got.Name)
	}
	if len(got.CameraIDs) != 2 || got.CameraIDs[0] != "cam1" || got.CameraIDs[1] != "cam2" {
		t.Errorf("cameras after reload: %v", got.CameraIDs)
	}
}

func TestPersistence_FileCreatedInMissingDir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "subdir", "groups.yaml")
	store, err := groups.New(path, knownCamera)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create("X"); err != nil {
		t.Fatalf("create should auto-mkdir: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("expected file created at %s: %v", path, err)
	}
}
