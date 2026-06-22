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

// audioEventsFake is a fake Frigate that serves GET /api/events with after /
// before / limit honoured. It returns events from `all` whose start_time falls
// within the [after, before] window, capped to limit. Both object and audio
// events are returned raw — the BFF's ListAudioEvents does the type filtering.
//
// errOnEvents, when set, short-circuits requests with the supplied status so a
// test can exercise the 502 mapping.
type audioEventsFake struct {
	all         []events.FrigateEvent
	errOnEvents int
	gotQuery    string
	mu          sync.Mutex
}

func (f *audioEventsFake) handler(t *testing.T) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/events" {
			http.NotFound(w, r)
			return
		}
		f.mu.Lock()
		f.gotQuery = r.URL.RawQuery
		f.mu.Unlock()
		if f.errOnEvents != 0 {
			http.Error(w, "boom", f.errOnEvents)
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
		out := make([]events.FrigateEvent, 0, len(f.all))
		for _, e := range f.all {
			start := int64(e.StartTime)
			if start < after || start > before {
				continue
			}
			out = append(out, e)
			if len(out) >= limit {
				break
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(out)
	})
}

func timelineAudioRouter(t *testing.T, fake *audioEventsFake) http.Handler {
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

// audioFrigateEvent builds a raw Frigate event with data.type set.
func audioFrigateEvent(id, label string, start float64, end *float64, typ string) events.FrigateEvent {
	e := events.FrigateEvent{ID: id, Camera: "cam1", Label: label, StartTime: start, EndTime: end}
	e.Data.Type = typ
	return e
}

func TestHandleTimelineAudioEvents_KeepsOnlyAudio(t *testing.T) {
	// A mix of object and audio events, all inside the window once the
	// lookback widens `after`. Only the audio ones come back, one of them
	// still active (end_time null → end:null).
	fake := &audioEventsFake{
		all: []events.FrigateEvent{
			audioFrigateEvent("obj-1", "person", 1779310100, ptrF(1779310160), "object"),
			audioFrigateEvent("aud-speech", "speech", 1779310120, ptrF(1779310140), "audio"),
			audioFrigateEvent("aud-active", "bark", 1779310200, nil, "audio"),
		},
	}
	router := timelineAudioRouter(t, fake)

	req := httptest.NewRequest(http.MethodGet,
		"/api/cameras/cam1/audio-events?start=1779310000&end=1779310300", nil)
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

	var got []audioMarker
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d markers, want 2 audio (body=%s)", len(got), w.Body.String())
	}

	if got[0].ID != "aud-speech" || got[0].Label != "speech" {
		t.Errorf("marker[0] = %+v, want id=aud-speech label=speech", got[0])
	}
	if got[0].Start != 1779310120 {
		t.Errorf("marker[0].Start = %v, want 1779310120", got[0].Start)
	}
	if got[0].End == nil || *got[0].End != 1779310140 {
		t.Errorf("marker[0].End = %v, want 1779310140", got[0].End)
	}

	if got[1].ID != "aud-active" {
		t.Errorf("marker[1].ID = %q, want aud-active", got[1].ID)
	}
	if got[1].End != nil {
		t.Errorf("marker[1].End = %v, want null (active event)", *got[1].End)
	}
}

func TestHandleTimelineAudioEvents_InvalidIDReturns400(t *testing.T) {
	fake := &audioEventsFake{}
	router := timelineAudioRouter(t, fake)

	req := httptest.NewRequest(http.MethodGet,
		"/api/cameras/has%20space/audio-events?start=1779310000&end=1779310300", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
	var body map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["error"] != "invalid_id" {
		t.Errorf("error = %q, want invalid_id", body["error"])
	}
}

func TestHandleTimelineAudioEvents_CameraNotFoundReturns404(t *testing.T) {
	fake := &audioEventsFake{}
	router := timelineAudioRouter(t, fake)

	req := httptest.NewRequest(http.MethodGet,
		"/api/cameras/cam9/audio-events?start=1779310000&end=1779310300", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", w.Code)
	}
	var body map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["error"] != "not_found" {
		t.Errorf("error = %q, want not_found", body["error"])
	}
}

func TestHandleTimelineAudioEvents_BadRangeReturns400(t *testing.T) {
	fake := &audioEventsFake{}
	router := timelineAudioRouter(t, fake)

	// end <= start → invalid_range, before any upstream call.
	req := httptest.NewRequest(http.MethodGet,
		"/api/cameras/cam1/audio-events?start=1779310300&end=1779310300", nil)
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

func TestHandleTimelineAudioEvents_UpstreamErrorReturns502(t *testing.T) {
	fake := &audioEventsFake{errOnEvents: http.StatusInternalServerError}
	router := timelineAudioRouter(t, fake)

	req := httptest.NewRequest(http.MethodGet,
		"/api/cameras/cam1/audio-events?start=1779310000&end=1779310300", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadGateway {
		t.Fatalf("want 502, got %d", w.Code)
	}
	var body map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["error"] != "upstream_error" {
		t.Errorf("error = %q, want upstream_error", body["error"])
	}
}
