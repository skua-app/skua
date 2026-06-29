package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/skua-app/skua/internal/frigate"
	applog "github.com/skua-app/skua/internal/log"
)

func newStorageHandler(t *testing.T, frigateURL string) *Handler {
	t.Helper()
	logger := applog.New("error", "text")
	client := frigate.NewClient(frigateURL, &http.Client{Timeout: 5 * time.Second})
	return NewHandler(HandlerDeps{
		Logger:  logger,
		Frigate: client,
	})
}

// TestHandleStorage_SortAndPassthrough verifies the handler returns mounts
// sorted with /media paths first (path-ascending within each group) and the
// MiB numbers intact.
func TestHandleStorage_SortAndPassthrough(t *testing.T) {
	fakeFrigate := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/stats":
			w.Header().Set("Content-Type", "application/json")
			// Intentionally unsorted; /dev/shm + /tmp/cache should land after /media.
			if _, err := fmt.Fprint(w, `{"service":{"storage":{
				"/tmp/cache":{"total":1024.0,"used":256.0,"free":768.0,"mount_type":"tmpfs"},
				"/media/frigate/recordings":{"total":500000.5,"used":120000.2,"free":379999.3,"mount_type":"ext4"},
				"/dev/shm":{"total":128.0,"used":8.0,"free":120.0,"mount_type":"tmpfs"},
				"/media/frigate/clips":{"total":500000.5,"used":1000.0,"free":499000.5,"mount_type":"ext4"}
			}}}`); err != nil {
				t.Errorf("fprint: %v", err)
			}
		case "/api/recordings/storage":
			w.Header().Set("Content-Type", "application/json")
			// Intentionally unsorted; cam2 heaviest, then cam3, then cam1.
			if _, err := fmt.Fprint(w, `{
				"cam1":{"usage":258140.59,"bandwidth":970.67,"usage_percent":6.77},
				"cam3":{"usage":1053488.36,"bandwidth":2648.65,"usage_percent":27.62},
				"cam2":{"usage":956768.77,"bandwidth":935.27,"usage_percent":25.08}
			}`); err != nil {
				t.Errorf("fprint: %v", err)
			}
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer fakeFrigate.Close()

	h := newStorageHandler(t, fakeFrigate.URL)
	req := httptest.NewRequest(http.MethodGet, "/api/storage", nil)
	w := httptest.NewRecorder()
	h.handleStorage(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (body=%s)", w.Code, w.Body.String())
	}
	var resp storageResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	wantOrder := []string{
		"/media/frigate/clips",
		"/media/frigate/recordings",
		"/dev/shm",
		"/tmp/cache",
	}
	if len(resp.Mounts) != len(wantOrder) {
		t.Fatalf("want %d mounts, got %d", len(wantOrder), len(resp.Mounts))
	}
	for i, want := range wantOrder {
		if resp.Mounts[i].Path != want {
			t.Errorf("mount[%d]: want path %s, got %s", i, want, resp.Mounts[i].Path)
		}
	}

	rec := resp.Mounts[1] // /media/frigate/recordings
	if rec.Type != "ext4" {
		t.Errorf("want type ext4, got %s", rec.Type)
	}
	if rec.TotalMiB != 500000.5 || rec.UsedMiB != 120000.2 || rec.FreeMiB != 379999.3 {
		t.Errorf("MiB numbers not passed through: %+v", rec)
	}

	// Cameras sorted by usage_mib descending: cam3 > cam2 > cam1.
	wantCamOrder := []string{"cam3", "cam2", "cam1"}
	if len(resp.Cameras) != len(wantCamOrder) {
		t.Fatalf("want %d cameras, got %d", len(wantCamOrder), len(resp.Cameras))
	}
	for i, want := range wantCamOrder {
		if resp.Cameras[i].ID != want {
			t.Errorf("camera[%d]: want id %s, got %s", i, want, resp.Cameras[i].ID)
		}
	}

	cam3 := resp.Cameras[0]
	if cam3.UsageMiB != 1053488.36 || cam3.BandwidthMiBPerHr != 2648.65 || cam3.UsagePercent != 27.62 {
		t.Errorf("camera numbers not passed through: %+v", cam3)
	}
}

// TestHandleStorage_EmptyMap verifies a missing/empty service.storage yields
// mounts: [] (never null).
func TestHandleStorage_EmptyMap(t *testing.T) {
	fakeFrigate := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/stats":
			if _, err := fmt.Fprint(w, `{"service":{}}`); err != nil {
				t.Errorf("fprint: %v", err)
			}
		case "/api/recordings/storage":
			if _, err := fmt.Fprint(w, `{}`); err != nil {
				t.Errorf("fprint: %v", err)
			}
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer fakeFrigate.Close()

	h := newStorageHandler(t, fakeFrigate.URL)
	req := httptest.NewRequest(http.MethodGet, "/api/storage", nil)
	w := httptest.NewRecorder()
	h.handleStorage(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	// Assert the raw JSON carries [] not null for both arrays.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
		t.Fatalf("decode raw: %v", err)
	}
	if string(raw["mounts"]) != "[]" {
		t.Errorf("want mounts:[] , got %s", string(raw["mounts"]))
	}
	if string(raw["cameras"]) != "[]" {
		t.Errorf("want cameras:[] , got %s", string(raw["cameras"]))
	}
}

// TestHandleStorage_PerCameraDegrade verifies that when the per-camera
// /api/recordings/storage call fails, the mounts still populate and cameras
// degrades to [] with an overall 200 — the per-camera block must not break
// the mounts view.
func TestHandleStorage_PerCameraDegrade(t *testing.T) {
	fakeFrigate := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/stats":
			w.Header().Set("Content-Type", "application/json")
			if _, err := fmt.Fprint(w, `{"service":{"storage":{
				"/media/frigate/recordings":{"total":500000.5,"used":120000.2,"free":379999.3,"mount_type":"ext4"}
			}}}`); err != nil {
				t.Errorf("fprint: %v", err)
			}
		case "/api/recordings/storage":
			w.WriteHeader(http.StatusInternalServerError)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer fakeFrigate.Close()

	h := newStorageHandler(t, fakeFrigate.URL)
	req := httptest.NewRequest(http.MethodGet, "/api/storage", nil)
	w := httptest.NewRecorder()
	h.handleStorage(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (body=%s)", w.Code, w.Body.String())
	}
	body := w.Body.Bytes()
	var resp storageResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Mounts) != 1 {
		t.Fatalf("want 1 mount despite per-camera failure, got %d", len(resp.Mounts))
	}
	// cameras must be present and empty, not null.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatalf("decode raw: %v", err)
	}
	if string(raw["cameras"]) != "[]" {
		t.Errorf("want cameras:[] on degrade, got %s", string(raw["cameras"]))
	}
}

// TestHandleStorage_Upstream5xx verifies a Frigate 5xx returns the 502
// upstream_error envelope.
func TestHandleStorage_Upstream5xx(t *testing.T) {
	fakeFrigate := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer fakeFrigate.Close()

	h := newStorageHandler(t, fakeFrigate.URL)
	req := httptest.NewRequest(http.MethodGet, "/api/storage", nil)
	w := httptest.NewRecorder()
	h.handleStorage(w, req)

	if w.Code != http.StatusBadGateway {
		t.Fatalf("want 502, got %d", w.Code)
	}
	var body map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	if body["error"] != "upstream_error" {
		t.Errorf("want error upstream_error, got %s", body["error"])
	}
}
