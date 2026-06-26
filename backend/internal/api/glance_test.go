package api

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
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

func ptrF(v float64) *float64 { return &v }

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
	eventsClient := events.NewClient(frigateSrv.URL, &http.Client{}, 0)
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

// frigateReviewHandler serves GET /api/review with after/limit/cameras
// honoured the way Frigate does: review items are returned newest-first,
// `after` drops items whose start_time is not strictly greater than
// after-unix, `limit` truncates, `cameras` is a csv allowlist. Any
// other path (notably /api/events) returns 404 so an accidental
// regression to the old code path surfaces as an upstream_error.
//
// gotPath captures the upstream request path (without query) so the
// test can assert that the glance handler hit /api/review and never
// touched /api/events. gotQuery captures the URL query for the last
// /api/review request.
func frigateReviewHandler(t *testing.T, items []events.FrigateReviewItem, gotPath *string, gotQuery *url.Values, requestCount *int) http.Handler {
	t.Helper()
	sorted := make([]events.FrigateReviewItem, len(items))
	copy(sorted, items)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].StartTime > sorted[j].StartTime })
	var mu sync.Mutex
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		if gotPath != nil {
			*gotPath = r.URL.Path
		}
		if requestCount != nil {
			*requestCount++
		}
		mu.Unlock()

		if r.URL.Path != "/api/review" {
			http.NotFound(w, r)
			return
		}
		q := r.URL.Query()
		if gotQuery != nil {
			mu.Lock()
			*gotQuery = q
			mu.Unlock()
		}
		after := math.Inf(-1)
		if s := q.Get("after"); s != "" {
			if n, err := strconv.ParseInt(s, 10, 64); err == nil {
				after = float64(n)
			}
		}
		limit := len(sorted)
		if s := q.Get("limit"); s != "" {
			if n, err := strconv.Atoi(s); err == nil && n > 0 {
				limit = n
			}
		}
		var allowed map[string]struct{}
		if s := q.Get("cameras"); s != "" {
			allowed = make(map[string]struct{})
			for _, c := range strings.Split(s, ",") {
				allowed[c] = struct{}{}
			}
		}
		out := make([]events.FrigateReviewItem, 0, limit)
		for _, it := range sorted {
			if it.StartTime <= after {
				continue
			}
			if allowed != nil {
				if _, ok := allowed[it.Camera]; !ok {
					continue
				}
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

// segment builds a FrigateReviewItem for tests with the supplied fields,
// a deterministic id derived from start_time, and a single detection +
// object so common-case mappings produce non-empty Kinds/Labels and a
// non-empty ThumbEventID without ceremony per test.
func segment(id, cam string, start float64, end *float64, severity, label string) events.FrigateReviewItem {
	r := events.FrigateReviewItem{
		ID:        id,
		Camera:    cam,
		StartTime: start,
		EndTime:   end,
		Severity:  severity,
	}
	r.Data.Objects = []string{label}
	r.Data.Detections = []string{fmt.Sprintf("%.1f-det", start)}
	return r
}

func TestHandleGlance_HappyPath_NothingSeen(t *testing.T) {
	now := time.Now()
	t0 := float64(now.Add(-30 * time.Minute).Unix())
	items := []events.FrigateReviewItem{segment("rev-A", "camA", t0, ptrF(t0+60), "alert", "person")}
	// Add a second object + detection so we can assert dedup + thumb pick.
	items[0].Data.Objects = []string{"person", "person", "car"}
	items[0].Data.Detections = []string{
		fmt.Sprintf("%.1f-old", t0+5),
		fmt.Sprintf("%.1f-new", t0+45),
	}
	items[0].Data.Zones = []string{"ZoneA"}

	var gotPath string
	router, _, _ := glanceRouterWith(t, frigateReviewHandler(t, items, &gotPath, nil, nil))

	req := httptest.NewRequest(http.MethodGet, "/api/glance", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (body=%s)", w.Code, w.Body.String())
	}
	if cc := w.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", cc)
	}
	if gotPath != "/api/review" {
		t.Errorf("upstream path = %q, want /api/review (must NOT call /api/events)", gotPath)
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
	m := body.Moments[0]
	if m.ID != "rev-A" {
		t.Errorf("id = %q, want rev-A", m.ID)
	}
	if m.CamID != "camA" {
		t.Errorf("cam_id = %q, want camA", m.CamID)
	}
	if m.Severity != "alert" {
		t.Errorf("severity = %q, want alert", m.Severity)
	}
	if m.Seen {
		t.Errorf("seen = true, want false")
	}
	// Labels: deduped + sorted ascending.
	wantLabels := []string{"car", "person"}
	if got := m.Labels; len(got) != len(wantLabels) || got[0] != wantLabels[0] || got[1] != wantLabels[1] {
		t.Errorf("labels = %v, want %v", got, wantLabels)
	}
	// Zones: as supplied.
	if len(m.Zones) != 1 || m.Zones[0] != "ZoneA" {
		t.Errorf("zones = %v, want [ZoneA]", m.Zones)
	}
	// ThumbEventID is the latest detection by unix-seconds prefix.
	wantThumb := fmt.Sprintf("%.1f-new", t0+45)
	if m.ThumbEventID != wantThumb {
		t.Errorf("thumb_event_id = %q, want %q", m.ThumbEventID, wantThumb)
	}
}

func TestHandleGlance_SeenFlagByID(t *testing.T) {
	// Two segments on different cameras. Pre-mark one moment id as
	// seen. GET returns both moments, only that one carries seen=true,
	// unseen_count=1.
	now := time.Now()
	tOld := float64(now.Add(-2 * time.Hour).Unix())
	tNew := float64(now.Add(-30 * time.Minute).Unix())
	items := []events.FrigateReviewItem{
		segment("rev-old", "camA", tOld, ptrF(tOld+10), "alert", "person"),
		segment("rev-new", "camB", tNew, ptrF(tNew+10), "detection", "person"),
	}
	router, store, _ := glanceRouterWith(t, frigateReviewHandler(t, items, nil, nil, nil))

	if err := store.MarkSeen(glance.ScopeHousehold, []string{"rev-old"}, time.Now()); err != nil {
		t.Fatalf("seed MarkSeen: %v", err)
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
	if len(body.Moments) != 2 {
		t.Fatalf("moments len = %d, want 2", len(body.Moments))
	}
	if body.UnseenCount != 1 {
		t.Errorf("unseen_count = %d, want 1", body.UnseenCount)
	}
	for _, m := range body.Moments {
		switch m.ID {
		case "rev-old":
			if !m.Seen {
				t.Errorf("moment %q: seen=false, want true", m.ID)
			}
		case "rev-new":
			if m.Seen {
				t.Errorf("moment %q: seen=true, want false", m.ID)
			}
		default:
			t.Errorf("unexpected moment id %q", m.ID)
		}
	}
}

func TestHandleGlance_SeenThroughWatermark(t *testing.T) {
	// Two moments within window. MarkAllSeen at a point between them;
	// both moments survive, but the older one is reported with
	// seen=true (it is NOT dropped from the list).
	now := time.Now()
	tOld := float64(now.Add(-2 * time.Hour).Unix())
	tNew := float64(now.Add(-10 * time.Minute).Unix())
	items := []events.FrigateReviewItem{
		segment("rev-old", "camA", tOld, ptrF(tOld+10), "alert", "person"),
		segment("rev-new", "camB", tNew, ptrF(tNew+10), "detection", "person"),
	}
	router, store, _ := glanceRouterWith(t, frigateReviewHandler(t, items, nil, nil, nil))

	if err := store.MarkAllSeen(glance.ScopeHousehold, time.Now().Add(-1*time.Hour)); err != nil {
		t.Fatalf("MarkAllSeen: %v", err)
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
	if len(body.Moments) != 2 {
		t.Fatalf("moments len = %d, want 2 (watermark only flips seen)", len(body.Moments))
	}
	if body.UnseenCount != 1 {
		t.Errorf("unseen_count = %d, want 1", body.UnseenCount)
	}
	for _, m := range body.Moments {
		switch m.ID {
		case "rev-old":
			if !m.Seen {
				t.Errorf("moment %q: seen=false, want true (started_at at or before watermark)", m.ID)
			}
		case "rev-new":
			if m.Seen {
				t.Errorf("moment %q: seen=true, want false (after watermark)", m.ID)
			}
		default:
			t.Errorf("unexpected moment id %q", m.ID)
		}
	}
}

func TestHandleGlance_HoursFiltersOlderSegments(t *testing.T) {
	// One segment 2h ago, one 30m ago. With ?hours=1, only the recent
	// one survives because the BFF passes after = now-1h to Frigate
	// and the fake upstream filters strictly on after.
	now := time.Now()
	tOld := float64(now.Add(-2 * time.Hour).Unix())
	tNew := float64(now.Add(-30 * time.Minute).Unix())
	items := []events.FrigateReviewItem{
		segment("rev-old", "camA", tOld, ptrF(tOld+10), "alert", "person"),
		segment("rev-new", "camB", tNew, ptrF(tNew+10), "detection", "person"),
	}
	var gotQuery url.Values
	router, _, _ := glanceRouterWith(t, frigateReviewHandler(t, items, nil, &gotQuery, nil))

	req := httptest.NewRequest(http.MethodGet, "/api/glance?hours=1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	if gotQuery.Get("after") == "" {
		t.Errorf("upstream `after` not set: %v", gotQuery)
	}
	var body glanceResponse
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Moments) != 1 {
		t.Fatalf("moments len = %d, want 1 (only recent within hours=1)", len(body.Moments))
	}
	if body.Moments[0].ID != "rev-new" {
		t.Errorf("id = %q, want rev-new", body.Moments[0].ID)
	}
}

func TestHandleGlance_MaxTruncatesAndForwardsLimit(t *testing.T) {
	// 25 segments at distinct times. With ?max=10 the BFF over-fetches
	// limit = 10*glanceEmptyOverfetchFactor from Frigate (so the empty
	// filter can still fill 10 real moments) AND truncates the surviving
	// moments to 10. Newest-first order is preserved.
	now := time.Now()
	var items []events.FrigateReviewItem
	for i := 0; i < 25; i++ {
		started := float64(now.Add(-time.Duration(i*30) * time.Second).Unix())
		items = append(items, segment(
			fmt.Sprintf("rev-%02d", i),
			fmt.Sprintf("cam%02d", i),
			started, ptrF(started+5), "detection", "person",
		))
	}
	var gotQuery url.Values
	router, _, _ := glanceRouterWith(t, frigateReviewHandler(t, items, nil, &gotQuery, nil))

	req := httptest.NewRequest(http.MethodGet, "/api/glance?max=10", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (body=%s)", w.Code, w.Body.String())
	}
	wantLimit := strconv.Itoa(10 * glanceEmptyOverfetchFactor)
	if gotQuery.Get("limit") != wantLimit {
		t.Errorf("upstream limit = %q, want %s (over-fetch)", gotQuery.Get("limit"), wantLimit)
	}
	var body glanceResponse
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Moments) != 10 {
		t.Fatalf("moments len = %d, want 10", len(body.Moments))
	}
	if body.Moments[0].ID != "rev-00" {
		t.Errorf("moments[0].id = %q, want rev-00 (newest)", body.Moments[0].ID)
	}
	if body.Moments[9].ID != "rev-09" {
		t.Errorf("moments[9].id = %q, want rev-09 (10th-newest)", body.Moments[9].ID)
	}
}

func TestHandleGlance_MaxClampedToCeiling(t *testing.T) {
	now := time.Now()
	t0 := float64(now.Add(-30 * time.Minute).Unix())
	items := []events.FrigateReviewItem{segment("rev-A", "camA", t0, ptrF(t0+10), "alert", "person")}
	var gotQuery url.Values
	router, _, _ := glanceRouterWith(t, frigateReviewHandler(t, items, nil, &gotQuery, nil))

	req := httptest.NewRequest(http.MethodGet, "/api/glance?max=99999", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	if gotQuery.Get("limit") != strconv.Itoa(glanceMaxMomentsCeil) {
		t.Errorf("upstream limit = %q, want %d (ceiling clamp)",
			gotQuery.Get("limit"), glanceMaxMomentsCeil)
	}
}

// emptySegment builds a review item with no tracked-object detections —
// the motion-only "0 events" case the glance handler now drops. Objects /
// zones are left empty too so it carries no kinds; the empty-moment
// predicate keys solely on len(DetectionIDs).
func emptySegment(id, cam string, start float64, end *float64, severity string) events.FrigateReviewItem {
	return events.FrigateReviewItem{
		ID:        id,
		Camera:    cam,
		StartTime: start,
		EndTime:   end,
		Severity:  severity,
	}
}

func TestHandleGlance_OmitsEmptyMoments(t *testing.T) {
	// One real moment with detections, one empty motion-only segment. The
	// empty one is dropped from the response AND from unseen_count.
	now := time.Now()
	tReal := float64(now.Add(-30 * time.Minute).Unix())
	tEmpty := float64(now.Add(-20 * time.Minute).Unix())
	items := []events.FrigateReviewItem{
		segment("rev-real", "camA", tReal, ptrF(tReal+10), "alert", "person"),
		emptySegment("rev-empty", "camB", tEmpty, ptrF(tEmpty+10), "detection"),
	}
	router, _, _ := glanceRouterWith(t, frigateReviewHandler(t, items, nil, nil, nil))

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
	if len(body.Moments) != 1 {
		t.Fatalf("moments len = %d, want 1 (empty motion-only moment dropped)", len(body.Moments))
	}
	if body.Moments[0].ID != "rev-real" {
		t.Errorf("moments[0].id = %q, want rev-real", body.Moments[0].ID)
	}
	if body.UnseenCount != 1 {
		t.Errorf("unseen_count = %d, want 1 (empty moment not counted)", body.UnseenCount)
	}
}

func TestHandleGlance_FillsToMaxRealMomentsWithEmptiesInterleaved(t *testing.T) {
	// Empties interleaved with real moments. With ?max=5 the result still
	// fills 5 real moments (the over-fetch + filter), stays newest-first,
	// and unseen_count equals the unseen moments actually returned.
	now := time.Now()
	var items []events.FrigateReviewItem
	for i := 0; i < 20; i++ {
		started := float64(now.Add(-time.Duration(i*30) * time.Second).Unix())
		id := fmt.Sprintf("rev-%02d", i)
		cam := fmt.Sprintf("cam%02d", i)
		if i%2 == 0 {
			// Even indices are real moments (newest is i=0).
			items = append(items, segment(id, cam, started, ptrF(started+5), "detection", "person"))
		} else {
			items = append(items, emptySegment(id, cam, started, ptrF(started+5), "detection"))
		}
	}
	router, _, _ := glanceRouterWith(t, frigateReviewHandler(t, items, nil, nil, nil))

	req := httptest.NewRequest(http.MethodGet, "/api/glance?max=5", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (body=%s)", w.Code, w.Body.String())
	}
	var body glanceResponse
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Moments) != 5 {
		t.Fatalf("moments len = %d, want 5 real moments", len(body.Moments))
	}
	// Newest-first, real ids only: rev-00, rev-02, rev-04, rev-06, rev-08.
	wantIDs := []string{"rev-00", "rev-02", "rev-04", "rev-06", "rev-08"}
	for i, want := range wantIDs {
		if body.Moments[i].ID != want {
			t.Errorf("moments[%d].id = %q, want %q (newest-first, empties skipped)", i, body.Moments[i].ID, want)
		}
		if len(body.Moments[i].DetectionIDs) == 0 {
			t.Errorf("moments[%d] has no detections; empty moment leaked through", i)
		}
	}
	// All five are unseen (nothing pre-seeded), so unseen_count == 5.
	if body.UnseenCount != 5 {
		t.Errorf("unseen_count = %d, want 5 (count of unseen returned)", body.UnseenCount)
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

func TestHandleGlance_CallsReviewOnlyOnce(t *testing.T) {
	// Sanity: the new path is a single upstream call, not the old
	// backwards-paged /api/events loop.
	now := time.Now()
	t0 := float64(now.Add(-30 * time.Minute).Unix())
	items := []events.FrigateReviewItem{segment("rev-A", "camA", t0, ptrF(t0+10), "alert", "person")}
	var count int
	router, _, _ := glanceRouterWith(t, frigateReviewHandler(t, items, nil, nil, &count))

	req := httptest.NewRequest(http.MethodGet, "/api/glance", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	if count != 1 {
		t.Errorf("upstream request count = %d, want 1", count)
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
	now := time.Now()
	tOld := float64(now.Add(-2 * time.Hour).Unix())
	tNew := float64(now.Add(-30 * time.Minute).Unix())
	items := []events.FrigateReviewItem{
		segment("rev-old", "camA", tOld, ptrF(tOld+10), "alert", "person"),
		segment("rev-new", "camB", tNew, ptrF(tNew+10), "detection", "person"),
	}
	router, _, _ := glanceRouterWith(t, frigateReviewHandler(t, items, nil, nil, nil))

	seenReq := httptest.NewRequest(http.MethodPost, "/api/glance/seen",
		strings.NewReader(`{"event_ids":["rev-old"]}`))
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
		if m.ID == "rev-old" && !m.Seen {
			t.Errorf("moment %q: seen=false, want true after POST /seen", m.ID)
		}
		if m.ID == "rev-new" && m.Seen {
			t.Errorf("moment %q: seen=true, want false", m.ID)
		}
	}
}

func TestHandleGlanceSeenAll_EmptyBody_HouseholdScope(t *testing.T) {
	router, store, _ := glanceRouterWith(t, http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("upstream must not be called on seen-all")
	}))
	req := httptest.NewRequest(http.MethodPost, "/api/glance/seen-all", strings.NewReader(""))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d (body=%s)", w.Code, w.Body.String())
	}
	if got := store.SeenThrough(glance.ScopeHousehold); got.IsZero() {
		t.Errorf("SeenThrough is zero after seen-all; want non-zero")
	}
}

func TestHandleGlanceSeenAll_WithScopeBody(t *testing.T) {
	router, store, _ := glanceRouterWith(t, http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("upstream must not be called on seen-all")
	}))
	req := httptest.NewRequest(http.MethodPost, "/api/glance/seen-all",
		strings.NewReader(`{"scope":"household"}`))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d", w.Code)
	}
	if got := store.SeenThrough(glance.ScopeHousehold); got.IsZero() {
		t.Errorf("SeenThrough is zero after seen-all; want non-zero")
	}
}
