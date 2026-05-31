package setup

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/skua-app/skua/internal/probe"
	"github.com/skua-app/skua/internal/runtimeconfig"
)

func newStore(t *testing.T) *runtimeconfig.Store {
	t.Helper()
	store, err := runtimeconfig.New(filepath.Join(t.TempDir(), "config.yaml"))
	if err != nil {
		t.Fatalf("runtimeconfig.New: %v", err)
	}
	return store
}

func TestHandler_GETRendersPage(t *testing.T) {
	h := Handler(newStore(t), Options{Mode: ModeInitial}, nil)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}
	if rr.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", rr.Header().Get("Cache-Control"))
	}
	body, _ := io.ReadAll(rr.Body)
	for _, want := range []string{"Welcome to Skua", "Frigate URL", "Save and start", "Test connection"} {
		if !strings.Contains(string(body), want) {
			t.Errorf("body missing %q", want)
		}
	}
}

func TestHandler_DeepRouteFallsBackToPage(t *testing.T) {
	h := Handler(newStore(t), Options{Mode: ModeInitial}, nil)
	req := httptest.NewRequest(http.MethodGet, "/cam/cam1", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("deep route status = %d, want 200 (wizard page)", rr.Code)
	}
}

func TestHandler_UnreachableModeRendersBanner(t *testing.T) {
	h := Handler(newStore(t), Options{
		Mode:       ModeUnreachable,
		ErrMessage: "Cannot reach Frigate at http://frigate:5000",
		Prefill:    runtimeconfig.Values{FrigateURL: "http://frigate:5000"},
	}, nil)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	body, _ := io.ReadAll(rr.Body)
	if !strings.Contains(string(body), "Finish Skua setup") {
		t.Errorf("body missing unreachable headline")
	}
	if !strings.Contains(string(body), "Cannot reach Frigate at http://frigate:5000") {
		t.Errorf("body missing error banner message")
	}
	if !strings.Contains(string(body), `value="http://frigate:5000"`) {
		t.Errorf("body missing prefilled FrigateURL input")
	}
}

func TestHandler_LockedFrigateRendersReadOnly(t *testing.T) {
	h := Handler(newStore(t), Options{
		Mode:    ModeInitial,
		Prefill: runtimeconfig.Values{FrigateURL: "http://env-locked:5000"},
		Locked:  LockedFields{FrigateURL: true},
	}, nil)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	body, _ := io.ReadAll(rr.Body)
	if !strings.Contains(string(body), `id="frigate_url_locked"`) {
		t.Errorf("body missing locked block for FrigateURL")
	}
	if strings.Contains(string(body), `id="frigate_url" name="frigate_url"`) {
		t.Errorf("body should not contain editable input when FrigateURL is locked")
	}
	if !strings.Contains(string(body), "FRIGATE_URL") {
		t.Errorf("body missing env var hint")
	}
}

func TestHandler_HealthzReturns503(t *testing.T) {
	h := Handler(newStore(t), Options{Mode: ModeInitial}, nil)
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("/healthz status = %d, want 503", rr.Code)
	}
}

func TestHandler_APICatchAllReturns503JSON(t *testing.T) {
	h := Handler(newStore(t), Options{Mode: ModeInitial}, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/cameras", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("/api/cameras status = %d, want 503", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var resp apiError
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Error != "setup_required" {
		t.Errorf("error = %q, want setup_required", resp.Error)
	}
}

func TestSave_RejectsEmptyFrigateURL(t *testing.T) {
	h := Handler(newStore(t), Options{Mode: ModeInitial}, make(chan struct{}, 1))
	req := httptest.NewRequest(http.MethodPost, "/api/setup/save",
		strings.NewReader(`{"frigate_url":""}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	var resp apiError
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Error != "frigate_url_required" {
		t.Errorf("error = %q, want frigate_url_required", resp.Error)
	}
}

func TestSave_RejectsMalformedURL(t *testing.T) {
	h := Handler(newStore(t), Options{Mode: ModeInitial}, make(chan struct{}, 1))
	req := httptest.NewRequest(http.MethodPost, "/api/setup/save",
		strings.NewReader(`{"frigate_url":"not a url"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	var resp apiError
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Error != "frigate_url_invalid" {
		t.Errorf("error = %q, want frigate_url_invalid", resp.Error)
	}
}

func TestSave_PersistsAndSignalsRestart(t *testing.T) {
	store := newStore(t)
	restart := make(chan struct{})
	h := Handler(store, Options{Mode: ModeInitial}, restart)

	req := httptest.NewRequest(http.MethodPost, "/api/setup/save",
		strings.NewReader(`{"frigate_url":"http://frigate:5000","go2rtc_url":"http://frigate:1984"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rr.Code, rr.Body.String())
	}
	got := store.Get()
	if got.FrigateURL != "http://frigate:5000" {
		t.Errorf("FrigateURL = %q, want http://frigate:5000", got.FrigateURL)
	}
	if got.Go2RTCURL != "http://frigate:1984" {
		t.Errorf("Go2RTCURL = %q, want http://frigate:1984", got.Go2RTCURL)
	}

	select {
	case <-restart:
		// expected
	default:
		t.Errorf("restart channel was not signalled after successful save")
	}
}

func TestSave_LockedFrigateURLIgnoresPayloadValue(t *testing.T) {
	store := newStore(t)
	restart := make(chan struct{})
	h := Handler(store, Options{
		Mode:    ModeInitial,
		Prefill: runtimeconfig.Values{FrigateURL: "http://env-locked:5000"},
		Locked:  LockedFields{FrigateURL: true},
	}, restart)

	// Try to overwrite the locked URL via the API body. The server must
	// ignore it and persist the prefill value (or rather, since the URL
	// is env-locked, nothing the operator can do should change the URL
	// in the overlay file).
	req := httptest.NewRequest(http.MethodPost, "/api/setup/save",
		strings.NewReader(`{"frigate_url":"http://attacker:5000"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rr.Code, rr.Body.String())
	}
	got := store.Get()
	if got.FrigateURL != "http://env-locked:5000" {
		t.Errorf("FrigateURL = %q, want locked prefill value", got.FrigateURL)
	}
}

func TestTest_ReturnsFrigateError(t *testing.T) {
	h := Handler(newStore(t), Options{Mode: ModeInitial, TestTimeout: 200 * (1)}, nil)
	// Use a port-0 closed address to force a connection refused / dial error.
	req := httptest.NewRequest(http.MethodPost, "/api/setup/test",
		strings.NewReader(`{"frigate_url":"http://127.0.0.1:1","go2rtc_url":""}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	var resp probe.Report
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Frigate.OK {
		t.Errorf("Frigate.OK = true, want false for unreachable target")
	}
	if resp.Frigate.Error == "" {
		t.Errorf("Frigate.Error is empty, want a diagnostic message")
	}
	if !resp.Go2RTC.Skipped {
		t.Errorf("Go2RTC.Skipped = false, want true for empty URL")
	}
}

func TestTest_ReachesUpstreamHappyPath(t *testing.T) {
	frigateUp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/stats" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"cpu_usages":{}}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer frigateUp.Close()
	go2rtcUp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/streams" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"cam1":{}}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer go2rtcUp.Close()

	h := Handler(newStore(t), Options{Mode: ModeInitial}, nil)
	body := `{"frigate_url":"` + frigateUp.URL + `","go2rtc_url":"` + go2rtcUp.URL + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/setup/test", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	var resp probe.Report
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp.Frigate.OK {
		t.Errorf("Frigate.OK = false, want true; error = %q", resp.Frigate.Error)
	}
	if !resp.Go2RTC.OK {
		t.Errorf("Go2RTC.OK = false, want true; error = %q", resp.Go2RTC.Error)
	}
}

// URL validation lives in internal/probe and is covered by probe_test.go
// after the E7.1 extraction.
