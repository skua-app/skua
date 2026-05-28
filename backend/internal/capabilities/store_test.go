package capabilities

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestNew_MissingFileReturnsEmptyStore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "capabilities.yaml")
	store, err := New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if got := store.All(); len(got) != 0 {
		t.Errorf("want empty map, got %v", got)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("file should not be created on missing-load, got err=%v", err)
	}
}

func TestNew_MalformedYAMLFailsFast(t *testing.T) {
	path := filepath.Join(t.TempDir(), "capabilities.yaml")
	if err := os.WriteFile(path, []byte("capabilities: : not yaml"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := New(path); err == nil {
		t.Fatal("expected error for malformed YAML, got nil")
	}
}

func TestNew_LoadsExistingEntries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "capabilities.yaml")
	body := "capabilities:\n  cam5:\n    talk_back: true\n    ptz: false\n  cam6:\n    talk_back: true\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	store, err := New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if got := store.Get("cam5"); !got.TalkBack || got.PTZ {
		t.Errorf("cam5: got %+v, want {TalkBack:true PTZ:false}", got)
	}
	if got := store.Get("cam6"); !got.TalkBack || got.PTZ {
		t.Errorf("cam6: got %+v, want {TalkBack:true PTZ:false}", got)
	}
}

func TestGet_UnknownCameraReturnsZero(t *testing.T) {
	store, err := New(filepath.Join(t.TempDir(), "capabilities.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	got := store.Get("cam999")
	if got.TalkBack || got.PTZ {
		t.Errorf("unknown cam returned %+v, want zero", got)
	}
}

func TestForget_AbsentEntryDoesNotWriteFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "capabilities.yaml")
	store, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Forget("cam_never_existed"); err != nil {
		t.Fatalf("Forget: %v", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("Forget on absent cam should not create file; stat err=%v", err)
	}
}

func TestForget_PresentEntryRemovesAndPersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "capabilities.yaml")
	body := "capabilities:\n  cam5:\n    talk_back: true\n  cam6:\n    ptz: true\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	store, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Forget("cam5"); err != nil {
		t.Fatalf("Forget: %v", err)
	}
	if got := store.Get("cam5"); got.TalkBack || got.PTZ {
		t.Errorf("after Forget, cam5 should be zero; got %+v", got)
	}
	if got := store.Get("cam6"); !got.PTZ {
		t.Errorf("cam6 should be untouched; got %+v", got)
	}

	// Reload from disk to confirm persistence.
	reloaded, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := reloaded.All()["cam5"]; ok {
		t.Error("cam5 should be absent from reloaded store")
	}
	if got := reloaded.Get("cam6"); !got.PTZ {
		t.Errorf("cam6 not persisted: got %+v", got)
	}
}
