package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"testing"
	"testing/fstest"

	"github.com/skua-app/skua/internal/cameraorder"
	"github.com/skua-app/skua/internal/cameras"
	"github.com/skua-app/skua/internal/capabilities"
	"github.com/skua-app/skua/internal/config"
	applog "github.com/skua-app/skua/internal/log"
	"github.com/skua-app/skua/internal/sse"
)

func newCameraOrderTestRouter(t *testing.T) (http.Handler, *cameraorder.Store, []config.CameraSpec) {
	t.Helper()
	logger := applog.New("error", "text")
	cams := []config.CameraSpec{
		{ID: "cam1", Name: "Cam 1"},
		{ID: "cam2", Name: "Cam 2"},
		{ID: "cam3", Name: "Cam 3"},
	}
	store, err := cameraorder.New(filepath.Join(t.TempDir(), "camera_order.yaml"), func(id string) bool {
		_, ok := config.FindCamera(cams, id)
		return ok
	})
	if err != nil {
		t.Fatal(err)
	}
	checker := makeChecker(nil, cams, map[string]bool{"cam1": true, "cam2": true, "cam3": true})
	h := NewHandler(HandlerDeps{
		Logger:       logger,
		Checker:      checker,
		Cameras:      cameras.NewForTest(cams),
		HTTPClient:   &http.Client{},
		CameraOrder:  store,
		Capabilities: capabilities.NewForTest(nil),
	})
	staticFS := fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("ok")}}
	return NewRouter(h, sse.NewHub(logger), logger, staticFS), store, cams
}

func TestCameraOrderAPI_EmptyOnFreshStore(t *testing.T) {
	router, _, _ := newCameraOrderTestRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/api/camera-order", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d, body %s", w.Code, w.Body.String())
	}
	var resp cameraOrderResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Order) != 0 {
		t.Errorf("expected empty order, got %v", resp.Order)
	}
}

func TestCameraOrderAPI_PutPersistsAndDropsUnknown(t *testing.T) {
	router, store, _ := newCameraOrderTestRouter(t)

	req := httptest.NewRequest(http.MethodPut, "/api/camera-order",
		bytes.NewBufferString(`{"order":["cam3","cam_ghost","cam1","cam1","cam2"]}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("PUT status %d, body %s", w.Code, w.Body.String())
	}
	var resp cameraOrderResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	want := []string{"cam3", "cam1", "cam2"}
	if !reflect.DeepEqual(resp.Order, want) {
		t.Errorf("PUT response order = %v, want %v", resp.Order, want)
	}
	if !reflect.DeepEqual(store.All(), want) {
		t.Errorf("store order = %v, want %v", store.All(), want)
	}
}

func TestCameras_AppliesOrderAndAppendsNewCams(t *testing.T) {
	router, _, _ := newCameraOrderTestRouter(t)

	// Save an order that omits cam2; cam2 must appear AFTER cam3,cam1 (the
	// saved order) in registry order so newly discovered cams don't vanish.
	put := httptest.NewRequest(http.MethodPut, "/api/camera-order",
		bytes.NewBufferString(`{"order":["cam3","cam1"]}`))
	put.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(httptest.NewRecorder(), put)

	req := httptest.NewRequest(http.MethodGet, "/api/cameras", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/cameras status %d, body %s", w.Code, w.Body.String())
	}
	var resp []cameraResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	gotIDs := make([]string, 0, len(resp))
	for _, c := range resp {
		gotIDs = append(gotIDs, c.ID)
	}
	wantIDs := []string{"cam3", "cam1", "cam2"}
	if !reflect.DeepEqual(gotIDs, wantIDs) {
		t.Errorf("cameras order = %v, want %v", gotIDs, wantIDs)
	}
}

func TestCameraOrderAPI_InvalidBodyRejected(t *testing.T) {
	router, _, _ := newCameraOrderTestRouter(t)
	req := httptest.NewRequest(http.MethodPut, "/api/camera-order", bytes.NewBufferString("not json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status %d, body %s", w.Code, w.Body.String())
	}
}
