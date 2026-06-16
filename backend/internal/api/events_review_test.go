package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/skua-app/skua/internal/capabilities"
	"github.com/skua-app/skua/internal/events"
	"github.com/skua-app/skua/internal/glance"
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

func eventReviewRouter(t *testing.T, fake *eventReviewFake) http.Handler {
	t.Helper()
	frigateSrv := httptest.NewServer(fake.handler(t))
	t.Cleanup(frigateSrv.Close)

	logger := applog.New("error", "text")
	eventsClient := events.NewClient(frigateSrv.URL, &http.Client{}, 0)
	glanceStore, err := glance.New(filepath.Join(t.TempDir(), "glance.json"))
	if err != nil {
		t.Fatalf("glance.New: %v", err)
	}
	h := NewHandler(HandlerDeps{
		Logger:       logger,
		Events:       eventsClient,
		FrigateUIURL: "http://frigate.example/ui",
		HTTPClient:   &http.Client{},
		Capabilities: capabilities.NewForTest(nil),
		Glance:       glanceStore,
	})
	staticFS := fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("ok")}}
	return NewRouter(h, sse.NewHub(logger), logger, staticFS)
}

func TestHandleEventReview_FoundReturnsID(t *testing.T) {
	// Two reviews around the event timestamp. The target detection id
	// lives in rev-B; rev-A must NOT match.
	eventID := "1779310050.0-target"
	fake := &eventReviewFake{
		all: []events.FrigateReviewItem{
			{
				ID:        "rev-A",
				Camera:    "camA",
				StartTime: 1779310000,
				EndTime:   ptrF(1779310010),
				Severity:  "alert",
				Data: struct {
					Detections []string `json:"detections"`
					Objects    []string `json:"objects"`
					Zones      []string `json:"zones"`
				}{Detections: []string{"1779310005.0-other"}},
			},
			{
				ID:        "rev-B",
				Camera:    "camB",
				StartTime: 1779310040,
				EndTime:   ptrF(1779310080),
				Severity:  "detection",
				Data: struct {
					Detections []string `json:"detections"`
					Objects    []string `json:"objects"`
					Zones      []string `json:"zones"`
				}{Detections: []string{"1779310045.0-decoy", eventID}},
			},
		},
	}
	router := eventReviewRouter(t, fake)

	req := httptest.NewRequest(http.MethodGet, "/api/events/"+eventID+"/review", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (body=%s)", w.Code, w.Body.String())
	}
	var body eventReviewResponse
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.ReviewID != "rev-B" {
		t.Errorf("review_id = %q, want rev-B", body.ReviewID)
	}
	// Both `after` and `before` must reach Frigate (recall window).
	if !strings.Contains(fake.gotQuery, "after=") {
		t.Errorf("upstream missing after: %q", fake.gotQuery)
	}
	if !strings.Contains(fake.gotQuery, "before=") {
		t.Errorf("upstream missing before: %q", fake.gotQuery)
	}
}

func TestHandleEventReview_NoMatchReturns404(t *testing.T) {
	eventID := "1779310050.0-target"
	fake := &eventReviewFake{
		all: []events.FrigateReviewItem{
			{
				ID:        "rev-A",
				Camera:    "camA",
				StartTime: 1779310000,
				EndTime:   ptrF(1779310010),
				Severity:  "alert",
				Data: struct {
					Detections []string `json:"detections"`
					Objects    []string `json:"objects"`
					Zones      []string `json:"zones"`
				}{Detections: []string{"1779310005.0-other"}},
			},
		},
	}
	router := eventReviewRouter(t, fake)

	req := httptest.NewRequest(http.MethodGet, "/api/events/"+eventID+"/review", nil)
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

func TestHandleEventReview_InvalidIDReturns404(t *testing.T) {
	fake := &eventReviewFake{}
	router := eventReviewRouter(t, fake)

	// Percent-encoded space fails validUpstreamID at the boundary.
	req := httptest.NewRequest(http.MethodGet, "/api/events/has%20space/review", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", w.Code)
	}
}

func TestHandleEventReview_UnparseablePrefixReturns404(t *testing.T) {
	// Well-formed under validUpstreamID (no special chars) but the
	// prefix before '-' is not a float, so EventTimestampPrefix
	// returns 0 → 404 before any upstream call.
	fake := &eventReviewFake{}
	router := eventReviewRouter(t, fake)

	req := httptest.NewRequest(http.MethodGet, "/api/events/notaprefix-only/review", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d (body=%s)", w.Code, w.Body.String())
	}
	// No upstream request should have been made.
	if fake.gotQuery != "" {
		t.Errorf("upstream called with %q, want no call", fake.gotQuery)
	}
}

func TestHandleEventReview_UpstreamErrorReturns502(t *testing.T) {
	fake := &eventReviewFake{errOnReview: http.StatusInternalServerError}
	router := eventReviewRouter(t, fake)

	req := httptest.NewRequest(http.MethodGet, "/api/events/1779310050.0-target/review", nil)
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
