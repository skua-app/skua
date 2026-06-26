package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/skua-app/skua/internal/cameras"
	"github.com/skua-app/skua/internal/config"
	"github.com/skua-app/skua/internal/frigate"
	applog "github.com/skua-app/skua/internal/log"
	"github.com/skua-app/skua/internal/sse"
)

// recordingsPathRE matches Frigate's per-segment recordings path
// (/api/{cam}/recordings).
var recordingsPathRE = regexp.MustCompile(`^/api/([^/]+)/recordings$`)

// recordingsFake is a fake Frigate that serves GET /api/{cam}/recordings,
// returning the raw ~10s segments in `segs`. status, when non-zero, overrides
// the 200 response so a test can exercise the non-2xx pass-through.
type recordingsFake struct {
	segs     []frigateRecordingSegment
	status   int
	gotQuery string
	mu       sync.Mutex
}

func (f *recordingsFake) handler(t *testing.T) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if recordingsPathRE.FindStringSubmatch(r.URL.Path) == nil {
			http.NotFound(w, r)
			return
		}
		f.mu.Lock()
		f.gotQuery = r.URL.RawQuery
		f.mu.Unlock()
		if f.status != 0 {
			http.Error(w, "boom", f.status)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(f.segs)
	})
}

func timelineRecordingsRouter(t *testing.T, fake *recordingsFake) http.Handler {
	t.Helper()
	frigateSrv := httptest.NewServer(fake.handler(t))
	t.Cleanup(frigateSrv.Close)

	logger := applog.New("error", "text")
	frigateClient := frigate.NewClient(frigateSrv.URL, &http.Client{})
	camSpecs := []config.CameraSpec{
		{ID: "cam1", Name: "Cam 1", StreamMain: "cam1_main", StreamSub: "cam1_sub"},
	}
	h := NewHandler(HandlerDeps{
		Logger:     logger,
		Frigate:    frigateClient,
		Cameras:    cameras.NewForTest(camSpecs),
		HTTPClient: &http.Client{},
	})
	staticFS := fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("ok")}}
	return NewRouter(h, sse.NewHub(logger), logger, staticFS)
}

func getCoverage(t *testing.T, router http.Handler, url string) (int, []coverageSegment) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, url, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	var got []coverageSegment
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v (body=%s)", err, w.Body.String())
	}
	return w.Code, got
}

func TestHandleTimelineRecordings_ContiguousCollapse(t *testing.T) {
	// Three back-to-back 10s segments collapse into one coverage band.
	fake := &recordingsFake{segs: []frigateRecordingSegment{
		{StartTime: 1779310000, EndTime: 1779310010},
		{StartTime: 1779310010, EndTime: 1779310020},
		{StartTime: 1779310020, EndTime: 1779310030},
	}}
	router := timelineRecordingsRouter(t, fake)

	code, got := getCoverage(t, router,
		"/api/cameras/cam1/recordings?start=1779310000&end=1779310100")
	if code != http.StatusOK {
		t.Fatalf("want 200, got %d", code)
	}
	if len(got) != 1 {
		t.Fatalf("got %d bands, want 1 (%+v)", len(got), got)
	}
	if got[0].Start != 1779310000 || got[0].End != 1779310030 {
		t.Errorf("band = %+v, want {1779310000, 1779310030}", got[0])
	}
}

func TestHandleTimelineRecordings_LargeGapSplits(t *testing.T) {
	// A gap larger than COVERAGE_GAP_MERGE (5s) between two runs splits the
	// coverage into two bands. Out-of-order input proves the sort runs first.
	fake := &recordingsFake{segs: []frigateRecordingSegment{
		{StartTime: 1779310300, EndTime: 1779310310}, // second run, listed first
		{StartTime: 1779310000, EndTime: 1779310010},
		{StartTime: 1779310010, EndTime: 1779310020},
		{StartTime: 1779310310, EndTime: 1779310320},
	}}
	router := timelineRecordingsRouter(t, fake)

	code, got := getCoverage(t, router,
		"/api/cameras/cam1/recordings?start=1779310000&end=1779310400")
	if code != http.StatusOK {
		t.Fatalf("want 200, got %d", code)
	}
	if len(got) != 2 {
		t.Fatalf("got %d bands, want 2 (%+v)", len(got), got)
	}
	if got[0].Start != 1779310000 || got[0].End != 1779310020 {
		t.Errorf("band[0] = %+v, want {1779310000, 1779310020}", got[0])
	}
	if got[1].Start != 1779310300 || got[1].End != 1779310320 {
		t.Errorf("band[1] = %+v, want {1779310300, 1779310320}", got[1])
	}
}

func TestHandleTimelineRecordings_SubThresholdJitterMerges(t *testing.T) {
	// A sub-threshold (<=5s) boundary gap is bridged into one band rather than
	// splitting — recording jitter must not fragment continuous coverage.
	fake := &recordingsFake{segs: []frigateRecordingSegment{
		{StartTime: 1779310000, EndTime: 1779310010},
		{StartTime: 1779310013, EndTime: 1779310023}, // 3s gap, under threshold
	}}
	router := timelineRecordingsRouter(t, fake)

	code, got := getCoverage(t, router,
		"/api/cameras/cam1/recordings?start=1779310000&end=1779310100")
	if code != http.StatusOK {
		t.Fatalf("want 200, got %d", code)
	}
	if len(got) != 1 {
		t.Fatalf("got %d bands, want 1 (%+v)", len(got), got)
	}
	if got[0].Start != 1779310000 || got[0].End != 1779310023 {
		t.Errorf("band = %+v, want {1779310000, 1779310023}", got[0])
	}
}

func TestHandleTimelineRecordings_EmptyUpstreamYieldsEmptyArray(t *testing.T) {
	fake := &recordingsFake{segs: []frigateRecordingSegment{}}
	router := timelineRecordingsRouter(t, fake)

	req := httptest.NewRequest(http.MethodGet,
		"/api/cameras/cam1/recordings?start=1779310000&end=1779310100", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	// Must serialise as [] not null.
	if body := w.Body.String(); body != "[]\n" {
		t.Errorf("body = %q, want %q", body, "[]\n")
	}
}

func TestHandleTimelineRecordings_NonOKUpstreamPassedThroughEmpty(t *testing.T) {
	fake := &recordingsFake{status: http.StatusServiceUnavailable}
	router := timelineRecordingsRouter(t, fake)

	req := httptest.NewRequest(http.MethodGet,
		"/api/cameras/cam1/recordings?start=1779310000&end=1779310100", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503 passed through, got %d", w.Code)
	}
	if body := w.Body.String(); body != "[]\n" {
		t.Errorf("body = %q, want %q", body, "[]\n")
	}
}

func TestHandleTimelineRecordings_BadRangeReturns400(t *testing.T) {
	fake := &recordingsFake{}
	router := timelineRecordingsRouter(t, fake)

	// end <= start → invalid_range, before any upstream call.
	req := httptest.NewRequest(http.MethodGet,
		"/api/cameras/cam1/recordings?start=1779310300&end=1779310300", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
	var body map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["error"] != "invalid_range" {
		t.Errorf("error = %q, want invalid_range", body["error"])
	}
	if fake.gotQuery != "" {
		t.Errorf("upstream called with %q, want no call", fake.gotQuery)
	}
}
