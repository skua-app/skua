package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/skua-app/skua/internal/cameras"
	"github.com/skua-app/skua/internal/capabilities"
	"github.com/skua-app/skua/internal/config"
	"github.com/skua-app/skua/internal/groups"
	applog "github.com/skua-app/skua/internal/log"
	"github.com/skua-app/skua/internal/sse"
)

func newGroupsTestRouter(t *testing.T) (http.Handler, *groups.Store) {
	t.Helper()
	logger := applog.New("error", "text")
	cams := []config.CameraSpec{{ID: "cam1"}, {ID: "cam2"}, {ID: "cam3"}}
	store, err := groups.New(filepath.Join(t.TempDir(), "groups.yaml"), func(id string) bool {
		_, ok := config.FindCamera(cams, id)
		return ok
	})
	if err != nil {
		t.Fatal(err)
	}
	h := NewHandler(logger, nil, nil, nil, cameras.NewForTest(cams), "", nil, "", 0, &http.Client{}, nil, store, nil, capabilities.NewForTest(nil), nil)
	staticFS := fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("ok")}}
	return NewRouter(h, sse.NewHub(logger), logger, staticFS), store
}

func decodeGroup(t *testing.T, body *bytes.Buffer) groupResponse {
	t.Helper()
	var g groupResponse
	if err := json.NewDecoder(body).Decode(&g); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return g
}

func TestGroupsAPI_CreateAndList(t *testing.T) {
	router, _ := newGroupsTestRouter(t)

	body := bytes.NewBufferString(`{"name":"Street"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/groups", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create: status %d, body %s", w.Code, w.Body.String())
	}
	created := decodeGroup(t, w.Body)
	if created.Name != "Street" || created.ID == "" || len(created.CameraIDs) != 0 {
		t.Errorf("unexpected created body: %+v", created)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/groups", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("list: status %d", w.Code)
	}
	var list []groupResponse
	if err := json.NewDecoder(w.Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != created.ID {
		t.Errorf("list: %+v", list)
	}
}

func TestGroupsAPI_DuplicateNameReturns400WithCode(t *testing.T) {
	router, _ := newGroupsTestRouter(t)

	for _, body := range []string{`{"name":"Street"}`, `{"name":"street"}`} {
		req := httptest.NewRequest(http.MethodPost, "/api/groups", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if body == `{"name":"Street"}` {
			if w.Code != http.StatusCreated {
				t.Fatalf("first create: %d", w.Code)
			}
			continue
		}
		if w.Code != http.StatusBadRequest {
			t.Fatalf("dup create: status %d, body %s", w.Code, w.Body.String())
		}
		var resp map[string]string
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatal(err)
		}
		if resp["error"] != "name_duplicate" {
			t.Errorf("error code: got %q, want name_duplicate", resp["error"])
		}
		if resp["message"] == "" {
			t.Error("expected human-readable message")
		}
	}
}

func TestGroupsAPI_UpdateAndDelete(t *testing.T) {
	router, store := newGroupsTestRouter(t)
	g, err := store.Create("Street")
	if err != nil {
		t.Fatal(err)
	}

	patch := `{"camera_ids":["cam1","cam2"]}`
	req := httptest.NewRequest(http.MethodPatch, "/api/groups/"+g.ID, bytes.NewBufferString(patch))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("patch: status %d, body %s", w.Code, w.Body.String())
	}
	updated := decodeGroup(t, w.Body)
	if len(updated.CameraIDs) != 2 {
		t.Errorf("camera_ids: %v", updated.CameraIDs)
	}

	req = httptest.NewRequest(http.MethodDelete, "/api/groups/"+g.ID, nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete: status %d, body %s", w.Code, w.Body.String())
	}
	if _, ok := store.Get(g.ID); ok {
		t.Error("group still present after delete")
	}
}

func TestGroupsAPI_UnknownCameraReturnsCode(t *testing.T) {
	router, store := newGroupsTestRouter(t)
	g, _ := store.Create("Street")
	patch := `{"camera_ids":["cam99"]}`
	req := httptest.NewRequest(http.MethodPatch, "/api/groups/"+g.ID, bytes.NewBufferString(patch))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status %d", w.Code)
	}
	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp["error"] != "camera_not_found" {
		t.Errorf("error code: got %q, want camera_not_found", resp["error"])
	}
}

func TestHandleCameras_IncludesGroups(t *testing.T) {
	_, store := newGroupsTestRouter(t)
	g, _ := store.Create("Street")
	cams := []string{"cam1", "cam2"}
	if _, err := store.Update(g.ID, nil, &cams); err != nil {
		t.Fatal(err)
	}

	// Build a tighter setup: newGroupsTestRouter wires a nil checker, which
	// would panic on /api/cameras. Reuse the same groups store but attach
	// a real OnlineChecker stub.
	logger := applog.New("error", "text")
	camsSpec := []config.CameraSpec{{ID: "cam1"}, {ID: "cam2"}, {ID: "cam3"}}
	checker := makeChecker(nil, camsSpec, map[string]bool{"cam1": true, "cam2": true, "cam3": true})
	h := NewHandler(logger, nil, nil, checker, cameras.NewForTest(camsSpec), "", nil, "", 0, &http.Client{}, nil, store, nil, capabilities.NewForTest(nil), nil)
	staticFS := fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("ok")}}
	router := NewRouter(h, sse.NewHub(logger), logger, staticFS)

	req := httptest.NewRequest(http.MethodGet, "/api/cameras", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d", w.Code)
	}
	var resp []cameraResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	byID := make(map[string]cameraResponse, len(resp))
	for _, c := range resp {
		byID[c.ID] = c
	}
	if got := byID["cam1"].Groups; len(got) != 1 || got[0] != g.ID {
		t.Errorf("cam1.groups: got %v, want [%s]", got, g.ID)
	}
	if got := byID["cam3"].Groups; len(got) != 0 {
		t.Errorf("cam3.groups: got %v, want []", got)
	}
}
