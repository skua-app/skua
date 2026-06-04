package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/skua-app/skua/internal/events"
)

// frigateEventsHandler returns an upstream that, for any /api/events
// request, echoes the supplied events list as JSON. Other paths 404.
func frigateEventsHandler(t *testing.T, page []events.FrigateEvent, gotQuery *url.Values) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/events" {
			http.NotFound(w, r)
			return
		}
		if gotQuery != nil {
			*gotQuery = r.URL.Query()
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(page)
	})
}

func TestHandleMoments_BadSince(t *testing.T) {
	router, _ := eventsRouterWith(t, http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("upstream must not be called on validation failure")
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/moments?since=not-iso", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d (body=%s)", w.Code, w.Body.String())
	}
	var body map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	if body["error"] != "bad_request" {
		t.Errorf("error = %q, want bad_request", body["error"])
	}
}

func TestHandleMoments_BadLimit(t *testing.T) {
	router, _ := eventsRouterWith(t, http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("upstream must not be called on validation failure")
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/moments?limit=-3", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

func TestHandleMoments_HappyPath(t *testing.T) {
	page := []events.FrigateEvent{
		// camA: two events 60s apart → one moment.
		{ID: "a1", Camera: "camA", Label: "person", StartTime: 1779310000, EndTime: ptrF(1779310010), HasSnapshot: true},
		{ID: "a2", Camera: "camA", Label: "car", StartTime: 1779310060, EndTime: ptrF(1779310090), HasSnapshot: true, HasClip: true},
		// camB: one event → one moment.
		{ID: "b1", Camera: "camB", Label: "dog", StartTime: 1779310200, EndTime: ptrF(1779310205), HasSnapshot: true},
	}
	var gotQuery url.Values
	router, _ := eventsRouterWith(t, frigateEventsHandler(t, page, &gotQuery))

	req := httptest.NewRequest(http.MethodGet, "/api/moments", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (body=%s)", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q", ct)
	}
	if cc := w.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", cc)
	}

	var body struct {
		Items []events.Moment `json:"items"`
	}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Items) != 2 {
		t.Fatalf("items len = %d, want 2", len(body.Items))
	}
	// camB is most recent → comes first.
	if body.Items[0].CamID != "camB" {
		t.Errorf("items[0].cam_id = %q, want camB (newest first)", body.Items[0].CamID)
	}
	if body.Items[1].CamID != "camA" {
		t.Errorf("items[1].cam_id = %q, want camA", body.Items[1].CamID)
	}
	if body.Items[1].EventCount != 2 {
		t.Errorf("camA event_count = %d, want 2", body.Items[1].EventCount)
	}
	// Default limit propagated upstream.
	if gotQuery.Get("limit") != "50" {
		t.Errorf("upstream limit = %q, want 50 (eventsDefaultLimit)", gotQuery.Get("limit"))
	}
}

func TestHandleMoments_LimitClampedToMax(t *testing.T) {
	var gotQuery url.Values
	router, _ := eventsRouterWith(t, frigateEventsHandler(t, nil, &gotQuery))

	req := httptest.NewRequest(http.MethodGet, "/api/moments?limit=999", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	if gotQuery.Get("limit") != "200" {
		t.Errorf("upstream limit = %q, want 200 (eventsMaxLimit clamp)", gotQuery.Get("limit"))
	}
}

func TestHandleMoments_SinceForwardedAsFilter(t *testing.T) {
	page := []events.FrigateEvent{
		{ID: "old", Camera: "camA", Label: "person", StartTime: 1779310000, EndTime: ptrF(1779310010), HasSnapshot: true},
		{ID: "new", Camera: "camA", Label: "person", StartTime: 1779320000, EndTime: ptrF(1779320010), HasSnapshot: true},
	}
	router, _ := eventsRouterWith(t, frigateEventsHandler(t, page, nil))

	// Pick a `since` between the two events.
	// 1779310000 → 2026-05-20T20:46:40Z, 1779320000 → 2026-05-20T23:33:20Z.
	req := httptest.NewRequest(http.MethodGet, "/api/moments?since=2026-05-20T22:00:00Z", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (body=%s)", w.Code, w.Body.String())
	}
	var body struct {
		Items []events.Moment `json:"items"`
	}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Items) != 1 {
		t.Fatalf("items len = %d, want 1 after since filter", len(body.Items))
	}
	if body.Items[0].RepresentativeEventID != "new" {
		t.Errorf("rep = %q, want 'new'", body.Items[0].RepresentativeEventID)
	}
}

func TestHandleMoments_UpstreamErrorMapped(t *testing.T) {
	router, _ := eventsRouterWith(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/moments", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadGateway {
		t.Fatalf("want 502, got %d", w.Code)
	}
	var body map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	if body["error"] != "upstream_error" {
		t.Errorf("error = %q, want upstream_error", body["error"])
	}
}

func ptrF(v float64) *float64 { return &v }
