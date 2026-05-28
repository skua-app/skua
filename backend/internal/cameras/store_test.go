package cameras

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/skua-app/skua/internal/frigate"
)

// frigateConfigJSON renders the cameras subtree of /api/config from a flat
// map[id]mainAlias. enabled is true for every entry; the unit tests don't
// exercise disabled-skipping (that's covered by inspection of derivation in
// the api package tests).
func frigateConfigJSON(t *testing.T, cams map[string]string) string {
	t.Helper()
	body := `{"cameras":{`
	first := true
	for id, main := range cams {
		if !first {
			body += ","
		}
		first = false
		body += fmt.Sprintf(`%q:{"name":%q,"enabled":true,"live":{"streams":{"Main":%q,"Sub":%q}}}`,
			id, id, main, id+"_sub")
	}
	body += `}}`
	return body
}

// newFakeFrigate spins up an httptest server returning the supplied cams
// payload for /api/config, and 500 for anything else.
func newFakeFrigate(t *testing.T, cams map[string]string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/config" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(frigateConfigJSON(t, cams)))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// readPersisted decodes the cameras.yaml on disk back into a sorted ID list.
func readPersisted(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read persisted: %v", err)
	}
	var fs fileSchema
	if err := yaml.Unmarshal(data, &fs); err != nil {
		t.Fatalf("parse persisted: %v", err)
	}
	out := make([]string, 0, len(fs.Cameras))
	for _, c := range fs.Cameras {
		out = append(out, c.ID)
	}
	return out
}

func TestRefresh_AddedCameraAppearsInDiffAndHook(t *testing.T) {
	srv := newFakeFrigate(t, map[string]string{
		"cam1": "cam1_main_h264",
		"cam2": "cam2_main_h264",
	})
	client := frigate.NewClient(srv.URL, 5*time.Second)

	path := filepath.Join(t.TempDir(), "cameras.yaml")
	store := NewForTestWithFrigate(path, client, []CameraSpec{
		{ID: "cam1", Name: "cam1", StreamMain: "cam1_main_h264"},
	})

	var addedSpecs []CameraSpec
	var removedIDs []string
	store.SetOnAdded(func(specs []CameraSpec) { addedSpecs = specs })
	store.SetOnRemoved(func(ids []string) { removedIDs = ids })

	diff, err := store.Refresh(context.Background())
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if !reflect.DeepEqual(diff.Added, []string{"cam2"}) {
		t.Errorf("diff.Added = %v, want [cam2]", diff.Added)
	}
	if len(diff.Removed) != 0 {
		t.Errorf("diff.Removed = %v, want empty", diff.Removed)
	}
	if len(addedSpecs) != 1 || addedSpecs[0].ID != "cam2" {
		t.Errorf("OnAdded called with %+v, want [{cam2 ...}]", addedSpecs)
	}
	if len(removedIDs) != 0 {
		t.Errorf("OnRemoved should not fire, got %v", removedIDs)
	}
	if got := readPersisted(t, path); !reflect.DeepEqual(got, []string{"cam1", "cam2"}) {
		t.Errorf("persisted ids = %v, want [cam1 cam2]", got)
	}
}

func TestRefresh_RemovedCameraAppearsInDiffAndHook(t *testing.T) {
	srv := newFakeFrigate(t, map[string]string{
		"cam1": "cam1_main_h264",
	})
	client := frigate.NewClient(srv.URL, 5*time.Second)

	path := filepath.Join(t.TempDir(), "cameras.yaml")
	store := NewForTestWithFrigate(path, client, []CameraSpec{
		{ID: "cam1", Name: "cam1", StreamMain: "cam1_main_h264"},
		{ID: "cam7", Name: "cam7", StreamMain: "cam7_main_h264"},
	})

	var addedSpecs []CameraSpec
	var removedIDs []string
	store.SetOnAdded(func(specs []CameraSpec) { addedSpecs = specs })
	store.SetOnRemoved(func(ids []string) { removedIDs = ids })

	diff, err := store.Refresh(context.Background())
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if len(diff.Added) != 0 {
		t.Errorf("diff.Added = %v, want empty", diff.Added)
	}
	if !reflect.DeepEqual(diff.Removed, []string{"cam7"}) {
		t.Errorf("diff.Removed = %v, want [cam7]", diff.Removed)
	}
	if len(addedSpecs) != 0 {
		t.Errorf("OnAdded should not fire, got %v", addedSpecs)
	}
	if !reflect.DeepEqual(removedIDs, []string{"cam7"}) {
		t.Errorf("OnRemoved called with %v, want [cam7]", removedIDs)
	}
	if got := readPersisted(t, path); !reflect.DeepEqual(got, []string{"cam1"}) {
		t.Errorf("persisted ids = %v, want [cam1]", got)
	}
}

func TestRefresh_FrigateErrorLeavesStateUntouched(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	client := frigate.NewClient(srv.URL, 5*time.Second)

	path := filepath.Join(t.TempDir(), "cameras.yaml")
	initial := []CameraSpec{
		{ID: "cam1", Name: "cam1", StreamMain: "cam1_main_h264"},
		{ID: "cam2", Name: "cam2", StreamMain: "cam2_main_h264"},
	}
	store := NewForTestWithFrigate(path, client, initial)
	addedCalled := false
	removedCalled := false
	store.SetOnAdded(func(_ []CameraSpec) { addedCalled = true })
	store.SetOnRemoved(func(_ []string) { removedCalled = true })

	if _, err := store.Refresh(context.Background()); err == nil {
		t.Fatal("expected error on Frigate 500, got nil")
	}

	if got := store.Snapshot(); !reflect.DeepEqual(got, initial) {
		t.Errorf("snapshot mutated after error: got %v, want %v", got, initial)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("cameras.yaml should NOT be written on Frigate error; stat err=%v", err)
	}
	if addedCalled || removedCalled {
		t.Errorf("hooks should not fire on error; added=%v removed=%v", addedCalled, removedCalled)
	}
}

func TestRefresh_EmptyDiffSkipsHooks(t *testing.T) {
	srv := newFakeFrigate(t, map[string]string{
		"cam1": "cam1_main_h264",
	})
	client := frigate.NewClient(srv.URL, 5*time.Second)

	path := filepath.Join(t.TempDir(), "cameras.yaml")
	store := NewForTestWithFrigate(path, client, []CameraSpec{
		{ID: "cam1", Name: "cam1", StreamMain: "cam1_main_h264"},
	})
	addedCalled := false
	removedCalled := false
	store.SetOnAdded(func(_ []CameraSpec) { addedCalled = true })
	store.SetOnRemoved(func(_ []string) { removedCalled = true })

	diff, err := store.Refresh(context.Background())
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if diff.Added == nil || diff.Removed == nil {
		t.Errorf("diff slices should be empty, not nil; got %+v", diff)
	}
	if len(diff.Added) != 0 || len(diff.Removed) != 0 {
		t.Errorf("diff non-empty: %+v", diff)
	}
	if addedCalled || removedCalled {
		t.Errorf("hooks should not fire on empty diff; added=%v removed=%v", addedCalled, removedCalled)
	}
}
