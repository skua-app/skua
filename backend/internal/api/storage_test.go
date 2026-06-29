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
		if r.URL.Path != "/api/stats" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
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
}

// TestHandleStorage_EmptyMap verifies a missing/empty service.storage yields
// mounts: [] (never null).
func TestHandleStorage_EmptyMap(t *testing.T) {
	fakeFrigate := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if _, err := fmt.Fprint(w, `{"service":{}}`); err != nil {
			t.Errorf("fprint: %v", err)
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
	// Assert the raw JSON carries [] not null.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
		t.Fatalf("decode raw: %v", err)
	}
	if string(raw["mounts"]) != "[]" {
		t.Errorf("want mounts:[] , got %s", string(raw["mounts"]))
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
