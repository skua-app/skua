package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/skua-app/skua/internal/cameras"
	"github.com/skua-app/skua/internal/capabilities"
	"github.com/skua-app/skua/internal/config"
	"github.com/skua-app/skua/internal/go2rtc"
	applog "github.com/skua-app/skua/internal/log"
	"github.com/skua-app/skua/internal/sse"
	"github.com/skua-app/skua/internal/streamoverrides"
)

// recordingGo2RTC is a fake go2rtc /api/webrtc endpoint. It captures each
// inbound request's src query param so tests can assert the merge picked
// the right alias, and returns a stub SDP answer body.
type recordingGo2RTC struct {
	server  *httptest.Server
	mu      sync.Mutex
	srcSeen []string
}

func newRecordingGo2RTC(t *testing.T) *recordingGo2RTC {
	t.Helper()
	r := &recordingGo2RTC{}
	r.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/api/webrtc" || req.Method != http.MethodPost {
			http.NotFound(w, req)
			return
		}
		r.mu.Lock()
		r.srcSeen = append(r.srcSeen, req.URL.Query().Get("src"))
		r.mu.Unlock()
		w.Header().Set("Content-Type", "application/sdp")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("v=0\r\n"))
	}))
	t.Cleanup(r.server.Close)
	return r
}

func (r *recordingGo2RTC) lastSrc(t *testing.T) string {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.srcSeen) == 0 {
		t.Fatal("upstream go2rtc never received a request")
	}
	return r.srcSeen[len(r.srcSeen)-1]
}

func (r *recordingGo2RTC) requests() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.srcSeen)
}

// newStreamingRouter wires a minimal router covering the WHEP path with a
// recording go2rtc upstream and a streamoverrides store seeded from
// initialOverrides.
func newStreamingRouter(
	t *testing.T,
	camSpecs []config.CameraSpec,
	initialOverrides map[string]streamoverrides.Override,
) (http.Handler, *recordingGo2RTC, *streamoverrides.Store) {
	t.Helper()
	logger := applog.New("error", "text")
	upstream := newRecordingGo2RTC(t)
	httpClient := &http.Client{Timeout: 2 * time.Second}
	go2rtcClient := go2rtc.New(upstream.server.URL, httpClient)
	overridesStore := streamoverrides.NewForTest(initialOverrides)
	checker := makeChecker(nil, camSpecs, map[string]bool{})
	h := NewHandler(
		logger,
		nil,
		nil,
		checker,
		cameras.NewForTest(camSpecs),
		upstream.server.URL,
		go2rtcClient,
		"",
		2*time.Second,
		httpClient,
		nil,
		nil,
		nil,
		capabilities.NewForTest(nil),
		overridesStore,
		RuntimeConfigDeps{},
	)
	staticFS := fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("ok")}}
	return NewRouter(h, sse.NewHub(logger), logger, staticFS), upstream, overridesStore
}

func doWhep(t *testing.T, router http.Handler, camID, quality string) *httptest.ResponseRecorder {
	t.Helper()
	url := "/api/webrtc/" + camID + "/whep?quality=" + quality
	req := httptest.NewRequest(http.MethodPost, url, bytes.NewBufferString("v=0\r\n"))
	req.Header.Set("Content-Type", "application/sdp")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func TestWhep_MainNoOverrideUsesRegistry(t *testing.T) {
	router, upstream, _ := newStreamingRouter(t,
		[]config.CameraSpec{
			{ID: "cam1", Name: "Cam 1", StreamMain: "cam1_main_h264", StreamSub: "cam1_sub"},
		},
		nil,
	)
	w := doWhep(t, router, "cam1", "main")
	if w.Code != http.StatusCreated {
		t.Fatalf("status %d, body %s", w.Code, w.Body.String())
	}
	if got := upstream.lastSrc(t); got != "cam1_main_h264" {
		t.Errorf("upstream src = %q, want %q", got, "cam1_main_h264")
	}
}

func TestWhep_MainOverrideUsesOverride(t *testing.T) {
	router, upstream, _ := newStreamingRouter(t,
		[]config.CameraSpec{
			{ID: "cam1", Name: "Cam 1", StreamMain: "cam1_main_h264", StreamSub: "cam1_sub"},
		},
		map[string]streamoverrides.Override{
			"cam1": {Main: "cam1_sub"},
		},
	)
	w := doWhep(t, router, "cam1", "main")
	if w.Code != http.StatusCreated {
		t.Fatalf("status %d, body %s", w.Code, w.Body.String())
	}
	if got := upstream.lastSrc(t); got != "cam1_sub" {
		t.Errorf("upstream src = %q, want %q", got, "cam1_sub")
	}
}

func TestWhep_SubNoOverrideEmptyStreamReturns400(t *testing.T) {
	router, upstream, _ := newStreamingRouter(t,
		[]config.CameraSpec{
			{ID: "cam1", Name: "Cam 1", StreamMain: "cam1_main_h264", StreamSub: ""},
		},
		nil,
	)
	w := doWhep(t, router, "cam1", "sub")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status %d, want 400; body %s", w.Code, w.Body.String())
	}
	if upstream.requests() != 0 {
		t.Errorf("expected no upstream calls, got %d", upstream.requests())
	}
	var body map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["error"] != "no stream configured for quality" {
		t.Errorf("error = %q, want 'no stream configured for quality'", body["error"])
	}
}

func TestWhep_SubOverrideFillsEmptyRegistrySlot(t *testing.T) {
	router, upstream, _ := newStreamingRouter(t,
		[]config.CameraSpec{
			{ID: "cam1", Name: "Cam 1", StreamMain: "cam1_main_h264", StreamSub: ""},
		},
		map[string]streamoverrides.Override{
			"cam1": {Sub: "cam1_alt_sub"},
		},
	)
	w := doWhep(t, router, "cam1", "sub")
	if w.Code != http.StatusCreated {
		t.Fatalf("status %d, body %s", w.Code, w.Body.String())
	}
	if got := upstream.lastSrc(t); got != "cam1_alt_sub" {
		t.Errorf("upstream src = %q, want %q", got, "cam1_alt_sub")
	}
}

func TestWhep_InvalidQualityReturns400(t *testing.T) {
	router, upstream, _ := newStreamingRouter(t,
		[]config.CameraSpec{
			{ID: "cam1", Name: "Cam 1", StreamMain: "cam1_main_h264", StreamSub: "cam1_sub"},
		},
		nil,
	)
	w := doWhep(t, router, "cam1", "thumb")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status %d, want 400; body %s", w.Code, w.Body.String())
	}
	if upstream.requests() != 0 {
		t.Errorf("expected no upstream calls, got %d", upstream.requests())
	}
	var body map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["error"] != "quality must be main or sub" {
		t.Errorf("error = %q, want 'quality must be main or sub'", body["error"])
	}
}

func TestWhep_UnknownCameraReturns404(t *testing.T) {
	router, upstream, _ := newStreamingRouter(t,
		[]config.CameraSpec{
			{ID: "cam1", Name: "Cam 1", StreamMain: "cam1_main_h264", StreamSub: "cam1_sub"},
		},
		nil,
	)
	w := doWhep(t, router, "cam999", "main")
	if w.Code != http.StatusNotFound {
		t.Fatalf("status %d, want 404; body %s", w.Code, w.Body.String())
	}
	if upstream.requests() != 0 {
		t.Errorf("expected no upstream calls, got %d", upstream.requests())
	}
}
