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
	applog "github.com/skua-app/skua/internal/log"
	"github.com/skua-app/skua/internal/names"
	"github.com/skua-app/skua/internal/sse"
)

func newNamesTestRouter(t *testing.T) (http.Handler, *names.Store, []config.CameraSpec) {
	t.Helper()
	logger := applog.New("error", "text")
	cams := []config.CameraSpec{
		{ID: "cam1", Name: "Cam 1"},
		{ID: "cam2", Name: "Cam 2"},
	}
	store, err := names.New(filepath.Join(t.TempDir(), "camera_names.yaml"), func(id string) bool {
		_, ok := config.FindCamera(cams, id)
		return ok
	})
	if err != nil {
		t.Fatal(err)
	}
	checker := makeChecker(nil, cams, map[string]bool{"cam1": true, "cam2": true})
	h := NewHandler(logger, nil, nil, checker, cameras.NewForTest(cams), "", nil, "", 0, &http.Client{}, nil, nil, store, capabilities.NewForTest(nil), nil)
	staticFS := fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("ok")}}
	return NewRouter(h, sse.NewHub(logger), logger, staticFS), store, cams
}

func TestCameraNamesAPI_EmptyOnFreshStore(t *testing.T) {
	router, _, _ := newNamesTestRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/api/camera-names", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d, body %s", w.Code, w.Body.String())
	}
	var resp cameraNamesResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Names) != 0 {
		t.Errorf("expected empty map, got %v", resp.Names)
	}
}

func TestCameraNamesAPI_PutPersistsAndMergesIntoCameras(t *testing.T) {
	router, _, _ := newNamesTestRouter(t)

	put := func(id, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPut, "/api/camera-names/"+id, bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}

	w := put("cam1", `{"name":"  Yard  "}`)
	if w.Code != http.StatusOK {
		t.Fatalf("PUT cam1 status %d, body %s", w.Code, w.Body.String())
	}
	var resp updateCameraNameResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode put: %v", err)
	}
	if resp.CamID != "cam1" || resp.Name != "Yard" {
		t.Errorf("PUT response: cam_id=%q name=%q (want cam1/Yard)", resp.CamID, resp.Name)
	}

	// List endpoint reflects the override.
	w = httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/camera-names", nil))
	var list cameraNamesResponse
	if err := json.NewDecoder(w.Body).Decode(&list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if list.Names["cam1"] != "Yard" {
		t.Errorf("list cam1 = %q, want %q", list.Names["cam1"], "Yard")
	}

	// /api/cameras merges the override into Name.
	w = httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/cameras", nil))
	var cams []cameraResponse
	if err := json.NewDecoder(w.Body).Decode(&cams); err != nil {
		t.Fatalf("decode cameras: %v", err)
	}
	gotCam1 := ""
	gotCam2 := ""
	for _, c := range cams {
		switch c.ID {
		case "cam1":
			gotCam1 = c.Name
		case "cam2":
			gotCam2 = c.Name
		}
	}
	if gotCam1 != "Yard" {
		t.Errorf("/api/cameras cam1.name = %q, want %q", gotCam1, "Yard")
	}
	if gotCam2 != "Cam 2" {
		t.Errorf("/api/cameras cam2.name = %q, want config default %q", gotCam2, "Cam 2")
	}
}

func TestCameraNamesAPI_EmptyClearsOverride(t *testing.T) {
	router, store, _ := newNamesTestRouter(t)
	if _, err := store.Set("cam1", "Yard"); err != nil {
		t.Fatalf("seed: %v", err)
	}

	req := httptest.NewRequest(http.MethodPut, "/api/camera-names/cam1", bytes.NewBufferString(`{"name":"   "}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d, body %s", w.Code, w.Body.String())
	}
	var resp updateCameraNameResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Name != "Cam 1" {
		t.Errorf("after clear, response name = %q, want config default %q", resp.Name, "Cam 1")
	}
	if _, ok := store.All()["cam1"]; ok {
		t.Errorf("override should be gone, got %v", store.All())
	}
}

func TestCameraNamesAPI_UnknownCameraIs400(t *testing.T) {
	router, _, _ := newNamesTestRouter(t)
	req := httptest.NewRequest(http.MethodPut, "/api/camera-names/cam999", bytes.NewBufferString(`{"name":"X"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status %d, want 400; body %s", w.Code, w.Body.String())
	}
	var body map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	if body["error"] != "camera_not_found" {
		t.Errorf("error code = %q, want camera_not_found", body["error"])
	}
}

func TestCameraNamesAPI_TooLongIs400(t *testing.T) {
	router, _, _ := newNamesTestRouter(t)
	// 31 runes
	body := `{"name":"` +
		"äääääääääääääääääääääääääääääää" +
		`"}`
	req := httptest.NewRequest(http.MethodPut, "/api/camera-names/cam1", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status %d, want 400; body %s", w.Code, w.Body.String())
	}
	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["error"] != "name_too_long" {
		t.Errorf("error code = %q, want name_too_long", resp["error"])
	}
}
