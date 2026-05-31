package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
	"time"

	"github.com/skua-app/skua/internal/cameras"
	"github.com/skua-app/skua/internal/capabilities"
	"github.com/skua-app/skua/internal/config"
	"github.com/skua-app/skua/internal/frigate"
	applog "github.com/skua-app/skua/internal/log"
	"github.com/skua-app/skua/internal/sse"
)

func makeChecker(client *frigate.Client, camSpecs []config.CameraSpec, status map[string]bool) *OnlineChecker {
	logger := applog.New("error", "text")
	return &OnlineChecker{
		client:  client,
		cameras: cameras.NewForTest(camSpecs),
		ttl:     15 * time.Second,
		status:  status,
		logger:  logger,
		nowFn:   time.Now,
	}
}

// TestHandleCameras verifies the /api/cameras response shape including streams.
func TestHandleCameras(t *testing.T) {
	fakeFrigate := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer fakeFrigate.Close()

	camSpecs := []config.CameraSpec{
		{ID: "cam1", Name: "Cam 1", StreamMain: "cam1_main", StreamSub: "cam1_sub"},
		{ID: "cam5", Name: "Cam 5", StreamMain: "cam5_main_h264", StreamSub: "cam5_sub"},
	}
	client := frigate.NewClient(fakeFrigate.URL, 5*time.Second)
	checker := makeChecker(client, camSpecs, map[string]bool{"cam1": true, "cam5": true})
	logger := applog.New("error", "text")
	capsStore := capabilities.NewForTest(map[string]capabilities.Capabilities{
		"cam5": {TalkBack: true},
	})
	h := NewHandler(logger, client, nil, checker, cameras.NewForTest(camSpecs), "", nil, "", 0, &http.Client{}, nil, nil, nil, capsStore, nil, RuntimeConfigDeps{})

	req := httptest.NewRequest(http.MethodGet, "/api/cameras", nil)
	w := httptest.NewRecorder()
	h.handleCameras(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	var resp []cameraResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp) != 2 {
		t.Fatalf("want 2 cameras, got %d", len(resp))
	}

	cam1 := resp[0]
	if cam1.ID != "cam1" {
		t.Errorf("want id cam1, got %s", cam1.ID)
	}
	if !cam1.Online {
		t.Error("cam1 should be online")
	}
	if cam1.SnapshotURL != "/api/cameras/cam1/snapshot.jpg" {
		t.Errorf("unexpected snapshot_url: %s", cam1.SnapshotURL)
	}
	if cam1.Streams.Main != "cam1_main" {
		t.Errorf("want streams.main=cam1_main, got %s", cam1.Streams.Main)
	}
	if cam1.Streams.Sub != "cam1_sub" {
		t.Errorf("want streams.sub=cam1_sub, got %s", cam1.Streams.Sub)
	}

	cam5 := resp[1]
	if !cam5.Capabilities.TalkBack {
		t.Error("cam5 should have talk_back")
	}
	if cam5.Streams.Main != "cam5_main_h264" {
		t.Errorf("want streams.main=cam5_main_h264, got %s", cam5.Streams.Main)
	}
}

// TestHandleSnapshot verifies snapshot proxying via the full chi router.
func TestHandleSnapshot(t *testing.T) {
	jpegData := []byte{0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10}
	fakeFrigate := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/cam1/latest.jpg" {
			w.Header().Set("Content-Type", "image/jpeg")
			w.Header().Set("ETag", `"abc123"`)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(jpegData)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer fakeFrigate.Close()

	camSpecs := []config.CameraSpec{{ID: "cam1", Name: "Cam 1", StreamMain: "cam1_main", StreamSub: "cam1_sub"}}
	logger := applog.New("error", "text")
	client := frigate.NewClient(fakeFrigate.URL, 5*time.Second)
	checker := makeChecker(client, camSpecs, map[string]bool{"cam1": true})
	h := NewHandler(logger, client, nil, checker, cameras.NewForTest(camSpecs), "", nil, "", 0, &http.Client{}, nil, nil, nil, capabilities.NewForTest(nil), nil, RuntimeConfigDeps{})

	staticFS := fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("<html></html>")}}
	router := NewRouter(h, sse.NewHub(logger), logger, staticFS)

	req := httptest.NewRequest(http.MethodGet, "/api/cameras/cam1/snapshot.jpg", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "image/jpeg" {
		t.Errorf("want Content-Type image/jpeg, got %s", ct)
	}
	if w.Header().Get("ETag") != `"abc123"` {
		t.Errorf("ETag not passed through: %s", w.Header().Get("ETag"))
	}
	if !bytes.Equal(w.Body.Bytes(), jpegData) {
		t.Error("unexpected snapshot body")
	}
}

// TestHandleSnapshotMissing verifies 502 when Frigate returns 404 for a camera.
func TestHandleSnapshotMissing(t *testing.T) {
	fakeFrigate := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer fakeFrigate.Close()

	camSpecs := []config.CameraSpec{{ID: "cam1", Name: "Cam 1", StreamMain: "cam1_main", StreamSub: "cam1_sub"}}
	logger := applog.New("error", "text")
	client := frigate.NewClient(fakeFrigate.URL, 5*time.Second)
	checker := makeChecker(client, camSpecs, map[string]bool{"cam1": false})
	h := NewHandler(logger, client, nil, checker, cameras.NewForTest(camSpecs), "", nil, "", 0, &http.Client{}, nil, nil, nil, capabilities.NewForTest(nil), nil, RuntimeConfigDeps{})

	staticFS := fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("<html></html>")}}
	router := NewRouter(h, sse.NewHub(logger), logger, staticFS)

	req := httptest.NewRequest(http.MethodGet, "/api/cameras/cam1/snapshot.jpg", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadGateway {
		t.Fatalf("want 502, got %d", w.Code)
	}
}

// TestOnlineChecker_StatsOnline verifies camera_fps > 0 → online, fps = 0 + detection enabled → offline.
func TestOnlineChecker_StatsOnline(t *testing.T) {
	cameras := []config.CameraSpec{
		{ID: "cam1", StreamMain: "cam1_main", StreamSub: "cam1_sub"},
		{ID: "cam2", StreamMain: "cam2_main", StreamSub: "cam2_sub"},
	}
	fakeFrigate := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/stats" {
			w.Header().Set("Content-Type", "application/json")
			if _, err := fmt.Fprint(w, `{"cameras":{"cam1":{"camera_fps":5.2,"detection_enabled":true},"cam2":{"camera_fps":0,"detection_enabled":true}}}`); err != nil {
				t.Errorf("fprint: %v", err)
			}
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer fakeFrigate.Close()

	client := frigate.NewClient(fakeFrigate.URL, 5*time.Second)
	checker := makeChecker(client, cameras, make(map[string]bool))
	checker.refreshAll()

	if !checker.IsOnline("cam1") {
		t.Error("cam1 should be online (fps > 0)")
	}
	if checker.IsOnline("cam2") {
		t.Error("cam2 should be offline (fps = 0, detection enabled)")
	}
}

// TestOnlineChecker_StatsError verifies all cameras are marked offline when /api/stats fails.
func TestOnlineChecker_StatsError(t *testing.T) {
	cameras := []config.CameraSpec{
		{ID: "cam1"}, {ID: "cam2"},
	}
	fakeFrigate := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer fakeFrigate.Close()

	client := frigate.NewClient(fakeFrigate.URL, 5*time.Second)
	checker := makeChecker(client, cameras, make(map[string]bool))
	checker.refreshAll()

	if checker.IsOnline("cam1") || checker.IsOnline("cam2") {
		t.Error("all cameras should be offline when stats request fails")
	}
}

// TestOnlineChecker_DetectionDisabledFallback verifies snapshot probe used when detection is off.
func TestOnlineChecker_DetectionDisabledFallback(t *testing.T) {
	cameras := []config.CameraSpec{{ID: "cam1", StreamMain: "cam1_main", StreamSub: "cam1_sub"}}
	fakeFrigate := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/stats":
			w.Header().Set("Content-Type", "application/json")
			if _, err := fmt.Fprint(w, `{"cameras":{"cam1":{"camera_fps":0,"detection_enabled":false}}}`); err != nil {
				t.Errorf("fprint: %v", err)
			}
		case "/api/cam1/latest.jpg":
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer fakeFrigate.Close()

	client := frigate.NewClient(fakeFrigate.URL, 5*time.Second)
	checker := makeChecker(client, cameras, make(map[string]bool))
	checker.refreshAll()

	if !checker.IsOnline("cam1") {
		t.Error("cam1 should be online: detection disabled but snapshot responds 200")
	}
}

// TestOnlineChecker_DetectionDisabledFallbackOffline verifies camera stays offline
// when detection is disabled and snapshot probe also fails.
func TestOnlineChecker_DetectionDisabledFallbackOffline(t *testing.T) {
	cameras := []config.CameraSpec{{ID: "cam1", StreamMain: "cam1_main", StreamSub: "cam1_sub"}}
	fakeFrigate := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/stats":
			w.Header().Set("Content-Type", "application/json")
			if _, err := fmt.Fprint(w, `{"cameras":{"cam1":{"camera_fps":0,"detection_enabled":false}}}`); err != nil {
				t.Errorf("fprint: %v", err)
			}
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer fakeFrigate.Close()

	client := frigate.NewClient(fakeFrigate.URL, 5*time.Second)
	checker := makeChecker(client, cameras, make(map[string]bool))
	checker.refreshAll()

	if checker.IsOnline("cam1") {
		t.Error("cam1 should be offline: detection disabled and snapshot returns 404")
	}
}

// TestOnlineChecker_MissingCamera verifies cameras absent from stats are marked offline.
func TestOnlineChecker_MissingCamera(t *testing.T) {
	cameras := []config.CameraSpec{{ID: "cam99"}}
	fakeFrigate := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/stats" {
			w.Header().Set("Content-Type", "application/json")
			if _, err := fmt.Fprint(w, `{"cameras":{}}`); err != nil {
				t.Errorf("fprint: %v", err)
			}
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer fakeFrigate.Close()

	client := frigate.NewClient(fakeFrigate.URL, 5*time.Second)
	checker := makeChecker(client, cameras, make(map[string]bool))
	checker.refreshAll()

	if checker.IsOnline("cam99") {
		t.Error("cam99 should be offline: not present in stats")
	}
}
