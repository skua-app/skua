package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/skua-app/skua/internal/cameras"
	"github.com/skua-app/skua/internal/config"
	"github.com/skua-app/skua/internal/events"
	applog "github.com/skua-app/skua/internal/log"
	"github.com/skua-app/skua/internal/sse"
)

// eventReviewFake is a fake Frigate that serves GET /api/review with
// after / before / limit honoured. It returns reviews from `all` whose
// start_time falls within the [after, before] window, capped to limit
// items, newest-first.
//
// errOnReview, when set, short-circuits review requests with the
// supplied status. Lets a test exercise the 502 mapping without
// depending on Frigate-side semantics.
type eventReviewFake struct {
	all         []events.FrigateReviewItem
	errOnReview int
	gotQuery    string
	mu          sync.Mutex
}

func (f *eventReviewFake) handler(t *testing.T) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/review" {
			http.NotFound(w, r)
			return
		}
		f.mu.Lock()
		f.gotQuery = r.URL.RawQuery
		f.mu.Unlock()
		if f.errOnReview != 0 {
			http.Error(w, "boom", f.errOnReview)
			return
		}
		q := r.URL.Query()
		var after, before int64 = -1 << 62, 1 << 62
		if s := q.Get("after"); s != "" {
			if n, err := strconv.ParseInt(s, 10, 64); err == nil {
				after = n
			}
		}
		if s := q.Get("before"); s != "" {
			if n, err := strconv.ParseInt(s, 10, 64); err == nil {
				before = n
			}
		}
		limit := len(f.all)
		if s := q.Get("limit"); s != "" {
			if n, err := strconv.Atoi(s); err == nil && n > 0 {
				limit = n
			}
		}
		out := make([]events.FrigateReviewItem, 0, len(f.all))
		for _, it := range f.all {
			start := int64(it.StartTime)
			if start < after || start > before {
				continue
			}
			out = append(out, it)
			if len(out) >= limit {
				break
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(out)
	})
}

// timelineReviewRouter wires the timeline review handler against the
// eventReviewFake (it serves GET /api/review with after / before / limit
// honoured) plus a one-camera registry so the handler's registry guard
// passes.
func timelineReviewRouter(t *testing.T, fake *eventReviewFake) http.Handler {
	t.Helper()
	frigateSrv := httptest.NewServer(fake.handler(t))
	t.Cleanup(frigateSrv.Close)

	logger := applog.New("error", "text")
	eventsClient := events.NewClient(frigateSrv.URL, &http.Client{}, 0)
	camSpecs := []config.CameraSpec{
		{ID: "cam1", Name: "Cam 1", StreamMain: "cam1_main", StreamSub: "cam1_sub"},
	}
	h := NewHandler(HandlerDeps{
		Logger:     logger,
		Events:     eventsClient,
		Cameras:    cameras.NewForTest(camSpecs),
		HTTPClient: &http.Client{},
	})
	staticFS := fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("ok")}}
	return NewRouter(h, sse.NewHub(logger), logger, staticFS)
}

func TestHandleTimelineReview_ReshapesSegments(t *testing.T) {
	// One alert with an end and one detection still active (end nil). Both
	// fall inside the requested window once the lookback widens `after`.
	fake := &eventReviewFake{
		all: []events.FrigateReviewItem{
			{
				ID:        "rev-alert",
				Camera:    "cam1",
				StartTime: 1779310100,
				EndTime:   ptrF(1779310160),
				Severity:  "alert",
			},
			{
				ID:        "rev-detect",
				Camera:    "cam1",
				StartTime: 1779310200,
				EndTime:   nil,
				Severity:  "detection",
			},
		},
	}
	router := timelineReviewRouter(t, fake)

	req := httptest.NewRequest(http.MethodGet,
		"/api/cameras/cam1/review?start=1779310000&end=1779310300", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (body=%s)", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	if cc := w.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", cc)
	}

	var got []reviewSegment
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d segments, want 2 (body=%s)", len(got), w.Body.String())
	}

	if got[0].ID != "rev-alert" || got[0].Severity != "alert" {
		t.Errorf("segment[0] = %+v, want id=rev-alert severity=alert", got[0])
	}
	if got[0].Start != 1779310100 {
		t.Errorf("segment[0].Start = %v, want 1779310100", got[0].Start)
	}
	if got[0].End == nil || *got[0].End != 1779310160 {
		t.Errorf("segment[0].End = %v, want 1779310160", got[0].End)
	}

	if got[1].ID != "rev-detect" || got[1].Severity != "detection" {
		t.Errorf("segment[1] = %+v, want id=rev-detect severity=detection", got[1])
	}
	if got[1].End != nil {
		t.Errorf("segment[1].End = %v, want null (active segment)", *got[1].End)
	}
}

func TestHandleTimelineReview_BadRangeReturns400(t *testing.T) {
	fake := &eventReviewFake{}
	router := timelineReviewRouter(t, fake)

	// end <= start → invalid_range, before any upstream call.
	req := httptest.NewRequest(http.MethodGet,
		"/api/cameras/cam1/review?start=1779310300&end=1779310300", nil)
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
