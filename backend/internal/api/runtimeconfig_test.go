package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"testing/fstest"

	"github.com/skua-app/skua/internal/capabilities"
	applog "github.com/skua-app/skua/internal/log"
	"github.com/skua-app/skua/internal/probe"
	"github.com/skua-app/skua/internal/runtimeconfig"
	"github.com/skua-app/skua/internal/sse"
)

// runtimeConfigTestRig builds an api.Handler + router with the given
// RuntimeConfigDeps and zero values for every other dependency. The
// new endpoints don't touch frigateClient / cameras / etc.
func runtimeConfigTestRig(t *testing.T, deps RuntimeConfigDeps) (http.Handler, *runtimeconfig.Store) {
	t.Helper()
	logger := applog.New("error", "text")
	if deps.Store == nil {
		store, err := runtimeconfig.New(filepath.Join(t.TempDir(), "config.yaml"))
		if err != nil {
			t.Fatalf("runtimeconfig.New: %v", err)
		}
		deps.Store = store
	}
	h := NewHandler(HandlerDeps{
		Logger:       logger,
		HTTPClient:   &http.Client{},
		Capabilities: capabilities.NewForTest(nil),
		Runtime:      deps,
	})
	staticFS := fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("ok")}}
	r := NewRouter(h, sse.NewHub(logger), logger, staticFS)
	return r, deps.Store
}

func TestGetRuntimeConfig_Shape(t *testing.T) {
	r, store := runtimeConfigTestRig(t, RuntimeConfigDeps{
		FrigateURL:        "http://frigate:5000",
		FrigateUIURL:      "http://frigate:5000",
		Go2RTCURL:         "",
		FrigateURLFromEnv: true,
	})
	if err := store.Save(runtimeconfig.Values{
		FrigateURL: "http://overlay:5000",
		Go2RTCURL:  "http://overlay:1984",
	}); err != nil {
		t.Fatalf("seed Save: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/runtime-config", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rr.Code, rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var resp runtimeConfigResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Effective.FrigateURL != "http://frigate:5000" {
		t.Errorf("Effective.FrigateURL = %q, want http://frigate:5000", resp.Effective.FrigateURL)
	}
	if resp.Overlay.FrigateURL != "http://overlay:5000" {
		t.Errorf("Overlay.FrigateURL = %q, want http://overlay:5000", resp.Overlay.FrigateURL)
	}
	if resp.Overlay.Go2RTCURL != "http://overlay:1984" {
		t.Errorf("Overlay.Go2RTCURL = %q, want http://overlay:1984", resp.Overlay.Go2RTCURL)
	}
	if !resp.Locked.FrigateURL {
		t.Errorf("Locked.FrigateURL = false, want true")
	}
	if resp.Locked.Go2RTCURL {
		t.Errorf("Locked.Go2RTCURL = true, want false")
	}
}

func TestPutRuntimeConfig_DropsLockedFieldsServerSide(t *testing.T) {
	r, store := runtimeConfigTestRig(t, RuntimeConfigDeps{
		FrigateURLFromEnv: true,
	})
	// Seed an existing overlay frigate_url. The PUT will try to overwrite
	// it, but the FrigateURL field is env-locked, so the server must keep
	// the existing overlay value.
	if err := store.Save(runtimeconfig.Values{FrigateURL: "http://existing:5000"}); err != nil {
		t.Fatalf("seed Save: %v", err)
	}

	body := `{"frigate_url":"http://attacker:5000","go2rtc_url":"http://new:1984"}`
	req := httptest.NewRequest(http.MethodPut, "/api/runtime-config", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rr.Code, rr.Body.String())
	}
	got := store.Get()
	if got.FrigateURL != "http://existing:5000" {
		t.Errorf("overlay FrigateURL = %q, want preserved %q", got.FrigateURL, "http://existing:5000")
	}
	if got.Go2RTCURL != "http://new:1984" {
		t.Errorf("overlay Go2RTCURL = %q, want %q", got.Go2RTCURL, "http://new:1984")
	}
}

func TestPutRuntimeConfig_ValidationErrors(t *testing.T) {
	cases := []struct {
		name     string
		body     string
		wantCode string
		wantHTTP int
	}{
		{"empty", `{"frigate_url":""}`, "frigate_url_required", http.StatusBadRequest},
		{"frigate_malformed", `{"frigate_url":"not a url"}`, "frigate_url_invalid", http.StatusBadRequest},
		{"go2rtc_malformed", `{"frigate_url":"http://frigate:5000","go2rtc_url":"ftp://no"}`, "go2rtc_url_invalid", http.StatusBadRequest},
		{"ui_malformed", `{"frigate_url":"http://frigate:5000","frigate_ui_url":"::"}`, "frigate_ui_url_invalid", http.StatusBadRequest},
		{"invalid_body", `{not json}`, "invalid_body", http.StatusBadRequest},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			r, _ := runtimeConfigTestRig(t, RuntimeConfigDeps{})
			req := httptest.NewRequest(http.MethodPut, "/api/runtime-config", strings.NewReader(c.body))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)
			if rr.Code != c.wantHTTP {
				t.Fatalf("status = %d, want %d; body = %s", rr.Code, c.wantHTTP, rr.Body.String())
			}
			var body map[string]string
			if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if body["error"] != c.wantCode {
				t.Errorf("error = %q, want %q", body["error"], c.wantCode)
			}
		})
	}
}

func TestPutRuntimeConfig_DataNotWritable(t *testing.T) {
	// Seed a read-only parent directory so atomicWrite's CreateTemp fails
	// with fs.ErrPermission, exercising the data_not_writable mapping.
	if os.Geteuid() == 0 {
		t.Skip("test requires non-root euid to enforce 0o500 directory perms")
	}
	dir := t.TempDir()
	roDir := filepath.Join(dir, "ro")
	if err := os.MkdirAll(roDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.Chmod(roDir, 0o500); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(roDir, 0o755) })

	store, err := runtimeconfig.New(filepath.Join(roDir, "config.yaml"))
	if err != nil {
		t.Fatalf("runtimeconfig.New: %v", err)
	}
	r, _ := runtimeConfigTestRig(t, RuntimeConfigDeps{Store: store})

	body := `{"frigate_url":"http://frigate:5000"}`
	req := httptest.NewRequest(http.MethodPut, "/api/runtime-config", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body = %s", rr.Code, rr.Body.String())
	}
	var resp map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["error"] != "data_not_writable" {
		t.Errorf("error = %q, want data_not_writable", resp["error"])
	}
	if !strings.Contains(resp["message"], "65532") {
		t.Errorf("message = %q, want uid hint", resp["message"])
	}
}

func TestTestRuntimeConfig_ReturnsReport(t *testing.T) {
	r, _ := runtimeConfigTestRig(t, RuntimeConfigDeps{})
	// Closed port forces a connection-refused diagnostic.
	body := `{"frigate_url":"http://127.0.0.1:1","go2rtc_url":""}`
	req := httptest.NewRequest(http.MethodPost, "/api/runtime-config/test", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rr.Code, rr.Body.String())
	}
	var report probe.Report
	if err := json.NewDecoder(rr.Body).Decode(&report); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if report.Frigate.OK {
		t.Errorf("Frigate.OK = true, want false")
	}
	if report.Frigate.Error == "" {
		t.Errorf("Frigate.Error empty, want diagnostic")
	}
	if !report.Go2RTC.Skipped {
		t.Errorf("Go2RTC.Skipped = false, want true for empty URL")
	}
}

func TestRestartRuntimeConfig_FiresOnceReturns202(t *testing.T) {
	var calls atomic.Int32
	r, _ := runtimeConfigTestRig(t, RuntimeConfigDeps{
		RequestRestart: func() { calls.Add(1) },
	})

	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/runtime-config/restart", nil)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		if rr.Code != http.StatusAccepted {
			t.Fatalf("call %d: status = %d, want 202; body = %s", i, rr.Code, rr.Body.String())
		}
		_, _ = io.ReadAll(rr.Body)
	}
	// Each request calls the closer once. main.go's sync.Once wrapping
	// makes the actual channel close idempotent — this test only verifies
	// the handler hits the func and returns 202.
	if calls.Load() != 3 {
		t.Errorf("RequestRestart calls = %d, want 3 (handler must invoke each time; sync.Once lives in main)", calls.Load())
	}
}

func TestRestartRuntimeConfig_500WhenNotWired(t *testing.T) {
	r, _ := runtimeConfigTestRig(t, RuntimeConfigDeps{}) // no RequestRestart
	req := httptest.NewRequest(http.MethodPost, "/api/runtime-config/restart", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rr.Code)
	}
}
