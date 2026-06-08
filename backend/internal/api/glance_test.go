package api

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/skua-app/skua/internal/capabilities"
	"github.com/skua-app/skua/internal/events"
	"github.com/skua-app/skua/internal/glance"
	applog "github.com/skua-app/skua/internal/log"
	"github.com/skua-app/skua/internal/sse"
)

// glanceRouterWith builds an api.Router whose events client talks to the
// supplied upstream and whose glance store is backed by a temp file in t.
// Returns the router, the constructed glance store so tests can pre-seed
// the seen-set, and the on-disk path for tests that need to assert
// presence/absence of the file.
func glanceRouterWith(t *testing.T, upstream http.Handler) (http.Handler, *glance.Store, string) {
	t.Helper()
	frigateSrv := httptest.NewServer(upstream)
	t.Cleanup(frigateSrv.Close)

	logger := applog.New("error", "text")
	eventsClient := events.NewClient(frigateSrv.URL, &http.Client{})
	path := filepath.Join(t.TempDir(), "glance.json")
	glanceStore, err := glance.New(path)
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
	return NewRouter(h, sse.NewHub(logger), logger, staticFS), glanceStore, path
}

func TestHandleGlance_HappyPath_NothingSeen(t *testing.T) {
	// Two same-camera events within the gap → one moment. Both
	// survive because the store has no seen ids.
	t0 := float64(time.Now().Add(-30 * time.Minute).Unix())
	page := []events.FrigateEvent{
		{ID: "a1", Camera: "camA", Label: "person", StartTime: t0, EndTime: ptrF(t0 + 10), HasSnapshot: true},
		{ID: "a2", Camera: "camA", Label: "car", StartTime: t0 + 60, EndTime: ptrF(t0 + 90), HasSnapshot: true, HasClip: true},
	}
	router, _, _ := glanceRouterWith(t, frigateEventsHandler(t, page, nil))

	req := httptest.NewRequest(http.MethodGet, "/api/glance", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (body=%s)", w.Code, w.Body.String())
	}
	if cc := w.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", cc)
	}
	var body glanceResponse
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.UnseenCount != 1 {
		t.Errorf("unseen_count = %d, want 1", body.UnseenCount)
	}
	if len(body.Moments) != 1 {
		t.Fatalf("moments len = %d, want 1", len(body.Moments))
	}
	if body.Moments[0].CamID != "camA" {
		t.Errorf("moments[0].cam_id = %q, want camA", body.Moments[0].CamID)
	}
	if body.Moments[0].Seen {
		t.Errorf("moments[0].seen = true, want false")
	}
	// Each returned moment carries its cluster detections, newest first.
	if len(body.Moments[0].Events) != 2 {
		t.Fatalf("moments[0].events len = %d, want 2", len(body.Moments[0].Events))
	}
	if body.Moments[0].Events[0].ID != "a2" || body.Moments[0].Events[1].ID != "a1" {
		t.Errorf("events order = [%q, %q], want [a2, a1] (newest first)",
			body.Moments[0].Events[0].ID, body.Moments[0].Events[1].ID)
	}
}

func TestHandleGlance_AllMomentsReturned_WithSeenFlag(t *testing.T) {
	// Two separate moments (different cameras). Pre-mark the
	// representative event of one as seen. GET returns both moments,
	// only one carries seen=true, unseen_count=1.
	tOld := float64(time.Now().Add(-2 * time.Hour).Unix())
	tNew := float64(time.Now().Add(-30 * time.Minute).Unix())
	page := []events.FrigateEvent{
		{ID: "old", Camera: "camA", Label: "person", StartTime: tOld, EndTime: ptrF(tOld + 10), HasSnapshot: true},
		{ID: "new", Camera: "camB", Label: "person", StartTime: tNew, EndTime: ptrF(tNew + 10), HasSnapshot: true},
	}
	router, store, _ := glanceRouterWith(t, frigateEventsHandler(t, page, nil))

	// camA has a single event so its rep id is "old".
	if err := store.MarkSeen(glance.ScopeHousehold, []string{"old"}, time.Now()); err != nil {
		t.Fatalf("seed MarkSeen: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/glance", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (body=%s)", w.Code, w.Body.String())
	}
	var body glanceResponse
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Moments) != 2 {
		t.Fatalf("moments len = %d, want 2 (both surfaced)", len(body.Moments))
	}
	if body.UnseenCount != 1 {
		t.Errorf("unseen_count = %d, want 1", body.UnseenCount)
	}
	// Find each moment by rep id and assert the seen flag.
	for _, m := range body.Moments {
		switch m.RepresentativeEventID {
		case "old":
			if !m.Seen {
				t.Errorf("moment %q: seen=false, want true", m.RepresentativeEventID)
			}
		case "new":
			if m.Seen {
				t.Errorf("moment %q: seen=true, want false", m.RepresentativeEventID)
			}
		default:
			t.Errorf("unexpected moment rep id %q", m.RepresentativeEventID)
		}
	}
}

func TestHandleGlanceSeen_MissingBody_BadRequest(t *testing.T) {
	router, _, _ := glanceRouterWith(t, http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("upstream must not be called on seen validation failure")
	}))
	req := httptest.NewRequest(http.MethodPost, "/api/glance/seen", strings.NewReader(""))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

func TestHandleGlanceSeen_MissingEventIDs_BadRequest(t *testing.T) {
	router, _, _ := glanceRouterWith(t, http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("upstream must not be called on seen validation failure")
	}))
	// Body is valid JSON but lacks event_ids.
	req := httptest.NewRequest(http.MethodPost, "/api/glance/seen", strings.NewReader(`{"scope":"household"}`))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
	var body map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	if body["error"] != "bad_request" {
		t.Errorf("error = %q, want bad_request", body["error"])
	}
}

func TestHandleGlanceSeen_NonArrayEventIDs_BadRequest(t *testing.T) {
	router, _, _ := glanceRouterWith(t, http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("upstream must not be called on seen validation failure")
	}))
	req := httptest.NewRequest(http.MethodPost, "/api/glance/seen", strings.NewReader(`{"event_ids":"oops"}`))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

func TestHandleGlanceSeen_EmptyArray_NoContentNoWrite(t *testing.T) {
	router, _, path := glanceRouterWith(t, http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("upstream must not be called on empty-id seen")
	}))
	req := httptest.NewRequest(http.MethodPost, "/api/glance/seen", strings.NewReader(`{"event_ids":[]}`))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d", w.Code)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("glance.json exists after empty seen, want missing (err=%v)", err)
	}
}

func TestHandleGlanceSeen_ValidIDs_PersistsAndFollowupGlanceReflects(t *testing.T) {
	tOld := float64(time.Now().Add(-2 * time.Hour).Unix())
	tNew := float64(time.Now().Add(-30 * time.Minute).Unix())
	page := []events.FrigateEvent{
		{ID: "old", Camera: "camA", Label: "person", StartTime: tOld, EndTime: ptrF(tOld + 10), HasSnapshot: true},
		{ID: "new", Camera: "camB", Label: "person", StartTime: tNew, EndTime: ptrF(tNew + 10), HasSnapshot: true},
	}
	router, _, _ := glanceRouterWith(t, frigateEventsHandler(t, page, nil))

	seenReq := httptest.NewRequest(http.MethodPost, "/api/glance/seen",
		strings.NewReader(`{"event_ids":["old"]}`))
	seenW := httptest.NewRecorder()
	router.ServeHTTP(seenW, seenReq)
	if seenW.Code != http.StatusNoContent {
		t.Fatalf("seen want 204, got %d (body=%s)", seenW.Code, seenW.Body.String())
	}

	glReq := httptest.NewRequest(http.MethodGet, "/api/glance", nil)
	glW := httptest.NewRecorder()
	router.ServeHTTP(glW, glReq)
	if glW.Code != http.StatusOK {
		t.Fatalf("glance want 200, got %d (body=%s)", glW.Code, glW.Body.String())
	}
	var glBody glanceResponse
	if err := json.NewDecoder(glW.Body).Decode(&glBody); err != nil {
		t.Fatalf("decode glance: %v", err)
	}
	if len(glBody.Moments) != 2 {
		t.Fatalf("moments len = %d, want 2", len(glBody.Moments))
	}
	if glBody.UnseenCount != 1 {
		t.Errorf("unseen_count = %d, want 1", glBody.UnseenCount)
	}
	for _, m := range glBody.Moments {
		if m.RepresentativeEventID == "old" && !m.Seen {
			t.Errorf("moment %q: seen=false, want true after POST /seen", m.RepresentativeEventID)
		}
		if m.RepresentativeEventID == "new" && m.Seen {
			t.Errorf("moment %q: seen=true, want false", m.RepresentativeEventID)
		}
	}
}

func TestHandleGlance_UpstreamErrorMapped(t *testing.T) {
	router, _, _ := glanceRouterWith(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/glance", nil)
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

func TestHandleGlance_HoursParamFiltersOlderEvents(t *testing.T) {
	// One event 2h ago, one 30m ago. With ?hours=1, only the recent
	// one survives. With ?hours=24, both survive.
	tOld := float64(time.Now().Add(-2 * time.Hour).Unix())
	tNew := float64(time.Now().Add(-30 * time.Minute).Unix())
	page := []events.FrigateEvent{
		{ID: "old", Camera: "camA", Label: "person", StartTime: tOld, EndTime: ptrF(tOld + 10), HasSnapshot: true},
		{ID: "new", Camera: "camB", Label: "person", StartTime: tNew, EndTime: ptrF(tNew + 10), HasSnapshot: true},
	}
	router, _, _ := glanceRouterWith(t, frigateEventsHandler(t, page, nil))

	req := httptest.NewRequest(http.MethodGet, "/api/glance?hours=1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	var body glanceResponse
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Moments) != 1 {
		t.Fatalf("moments len = %d, want 1 (only recent within hours=1)", len(body.Moments))
	}
	if body.Moments[0].RepresentativeEventID != "new" {
		t.Errorf("rep id = %q, want new", body.Moments[0].RepresentativeEventID)
	}
}

func TestHandleGlance_ClearWatermarkHidesOlderMoments(t *testing.T) {
	// Two events within window. Clear at a point between them; only
	// the moment strictly after cleared_at survives.
	tOld := float64(time.Now().Add(-2 * time.Hour).Unix())
	tNew := float64(time.Now().Add(-10 * time.Minute).Unix())
	page := []events.FrigateEvent{
		{ID: "old", Camera: "camA", Label: "person", StartTime: tOld, EndTime: ptrF(tOld + 10), HasSnapshot: true},
		{ID: "new", Camera: "camB", Label: "person", StartTime: tNew, EndTime: ptrF(tNew + 10), HasSnapshot: true},
	}
	router, store, _ := glanceRouterWith(t, frigateEventsHandler(t, page, nil))

	// Clear at 1h ago — old event (2h ago) is below the watermark,
	// new event (10m ago) is above it.
	if err := store.Clear(glance.ScopeHousehold, time.Now().Add(-1*time.Hour)); err != nil {
		t.Fatalf("Clear: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/glance", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	var body glanceResponse
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Moments) != 1 {
		t.Fatalf("moments len = %d, want 1 (only above watermark)", len(body.Moments))
	}
	if body.Moments[0].RepresentativeEventID != "new" {
		t.Errorf("rep id = %q, want new", body.Moments[0].RepresentativeEventID)
	}
}

func TestHandleGlanceClear_EmptyBody_HouseholdScope(t *testing.T) {
	router, store, _ := glanceRouterWith(t, http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("upstream must not be called on clear")
	}))
	req := httptest.NewRequest(http.MethodPost, "/api/glance/clear", strings.NewReader(""))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d (body=%s)", w.Code, w.Body.String())
	}
	if got := store.ClearedAt(glance.ScopeHousehold); got.IsZero() {
		t.Errorf("ClearedAt is zero after clear; want non-zero")
	}
}

func TestHandleGlanceClear_WithScopeBody(t *testing.T) {
	router, store, _ := glanceRouterWith(t, http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("upstream must not be called on clear")
	}))
	req := httptest.NewRequest(http.MethodPost, "/api/glance/clear",
		strings.NewReader(`{"scope":"household"}`))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d", w.Code)
	}
	if got := store.ClearedAt(glance.ScopeHousehold); got.IsZero() {
		t.Errorf("ClearedAt is zero after clear; want non-zero")
	}
}

// frigatePagedEventsHandler serves /api/events with paging semantics
// matching Frigate: it honours `limit` and `before` (unix-seconds) and
// returns events sorted by start_time descending. Concurrent-safe
// because httptest may dispatch requests on its own goroutine.
func frigatePagedEventsHandler(t *testing.T, all []events.FrigateEvent, requestCount *int) http.Handler {
	t.Helper()
	sorted := make([]events.FrigateEvent, len(all))
	copy(sorted, all)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].StartTime > sorted[j].StartTime })
	var mu sync.Mutex
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/events" {
			http.NotFound(w, r)
			return
		}
		if requestCount != nil {
			mu.Lock()
			*requestCount++
			mu.Unlock()
		}
		q := r.URL.Query()
		limit := 200
		if s := q.Get("limit"); s != "" {
			if n, err := strconv.Atoi(s); err == nil && n > 0 {
				limit = n
			}
		}
		before := math.Inf(1)
		if s := q.Get("before"); s != "" {
			if n, err := strconv.ParseInt(s, 10, 64); err == nil {
				before = float64(n)
			}
		}
		out := make([]events.FrigateEvent, 0, limit)
		for _, e := range sorted {
			if e.StartTime < before {
				out = append(out, e)
				if len(out) >= limit {
					break
				}
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(out)
	})
}

func TestHandleGlance_MaxQueryTruncatesMoments(t *testing.T) {
	// 25 unique cameras at distinct times, each producing exactly one
	// moment. With ?max=10 we expect only the 10 most-recent moments.
	now := time.Now()
	var page []events.FrigateEvent
	for i := 0; i < 25; i++ {
		// Newest cameras have the highest index. Stagger 30s apart.
		started := float64(now.Add(-time.Duration(i*30) * time.Second).Unix())
		page = append(page, events.FrigateEvent{
			ID:          fmt.Sprintf("e%02d", i),
			Camera:      fmt.Sprintf("cam%02d", i),
			Label:       "person",
			StartTime:   started,
			EndTime:     ptrF(started + 5),
			HasSnapshot: true,
		})
	}
	router, _, _ := glanceRouterWith(t, frigatePagedEventsHandler(t, page, nil))

	req := httptest.NewRequest(http.MethodGet, "/api/glance?max=10", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (body=%s)", w.Code, w.Body.String())
	}
	var body glanceResponse
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Moments) != 10 {
		t.Fatalf("moments len = %d, want 10 after max truncation", len(body.Moments))
	}
	// Newest first: cam00 (i=0) is most recent.
	if body.Moments[0].CamID != "cam00" {
		t.Errorf("moments[0].cam_id = %q, want cam00 (newest)", body.Moments[0].CamID)
	}
	if body.Moments[9].CamID != "cam09" {
		t.Errorf("moments[9].cam_id = %q, want cam09 (10th-newest)", body.Moments[9].CamID)
	}
}

func TestHandleGlance_MaxClampedToCeiling(t *testing.T) {
	// One moment in store, request max=99999 → silently clamped, no
	// error.
	t0 := float64(time.Now().Add(-30 * time.Minute).Unix())
	page := []events.FrigateEvent{
		{ID: "a1", Camera: "camA", Label: "person", StartTime: t0, EndTime: ptrF(t0 + 10), HasSnapshot: true},
	}
	router, _, _ := glanceRouterWith(t, frigatePagedEventsHandler(t, page, nil))

	req := httptest.NewRequest(http.MethodGet, "/api/glance?max=99999", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	var body glanceResponse
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Moments) != 1 {
		t.Fatalf("moments len = %d, want 1", len(body.Moments))
	}
}

func TestHandleGlance_StopsWhenWindowCovered(t *testing.T) {
	// Build a full first-page (200) plus extras outside the window.
	// Page-1's oldest must sit at or below `since` so the loop breaks
	// after exactly one upstream call.
	now := time.Now()
	// 200 in-page events spaced 30s, plus 50 events farther back.
	// At 30s spacing, the 200th event is 200*30 = 6000s = 100min ago.
	// With hours=1 (since = 60min ago), page-1's oldest is < since →
	// "window covered" → stop after 1 fetch.
	var page []events.FrigateEvent
	for i := 0; i < 250; i++ {
		started := float64(now.Add(-time.Duration(30+i*30) * time.Second).Unix())
		page = append(page, events.FrigateEvent{
			ID:          fmt.Sprintf("e%03d", i),
			Camera:      "camA",
			Label:       "person",
			StartTime:   started,
			EndTime:     ptrF(started + 5),
			HasSnapshot: true,
		})
	}
	var count int
	router, _, _ := glanceRouterWith(t, frigatePagedEventsHandler(t, page, &count))

	req := httptest.NewRequest(http.MethodGet, "/api/glance?hours=1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (body=%s)", w.Code, w.Body.String())
	}
	if count != 1 {
		t.Errorf("upstream request count = %d, want 1 (window covered after first page)", count)
	}
}

func TestHandleGlance_PagesUntilHistoryExhausted(t *testing.T) {
	// 250 events all within the window so page-1's oldest is still
	// inside the window. The loop must fetch page 2 (and break on
	// NextBefore=nil there).
	now := time.Now()
	var page []events.FrigateEvent
	// Spread across 5 minutes total (300s) so all are within hours=1
	// and the page-1 oldest still sits inside the window.
	for i := 0; i < 250; i++ {
		started := float64(now.Add(-time.Duration(i+1) * time.Second).Unix())
		page = append(page, events.FrigateEvent{
			ID:          fmt.Sprintf("e%03d", i),
			Camera:      "camA",
			Label:       "person",
			StartTime:   started,
			EndTime:     ptrF(started + 1),
			HasSnapshot: true,
		})
	}
	var count int
	router, _, _ := glanceRouterWith(t, frigatePagedEventsHandler(t, page, &count))

	req := httptest.NewRequest(http.MethodGet, "/api/glance?hours=1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (body=%s)", w.Code, w.Body.String())
	}
	if count != 2 {
		t.Errorf("upstream request count = %d, want 2 (page 1 full + page 2 short)", count)
	}
}
