package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
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
// Returns the router and the constructed glance store so tests can
// pre-seed last_seen or inspect it after Ack.
func glanceRouterWith(t *testing.T, upstream http.Handler) (http.Handler, *glance.Store) {
	t.Helper()
	frigateSrv := httptest.NewServer(upstream)
	t.Cleanup(frigateSrv.Close)

	logger := applog.New("error", "text")
	eventsClient := events.NewClient(frigateSrv.URL, &http.Client{})
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
	return NewRouter(h, sse.NewHub(logger), logger, staticFS), glanceStore
}

func TestHandleGlance_HappyPath_NeverSeen(t *testing.T) {
	// Two same-camera events within the gap → one moment. Both survive
	// because the store has never been acked (last_seen zero).
	page := []events.FrigateEvent{
		{ID: "a1", Camera: "camA", Label: "person", StartTime: 1779310000, EndTime: ptrF(1779310010), HasSnapshot: true},
		{ID: "a2", Camera: "camA", Label: "car", StartTime: 1779310060, EndTime: ptrF(1779310090), HasSnapshot: true, HasClip: true},
	}
	router, _ := glanceRouterWith(t, frigateEventsHandler(t, page, nil))

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
	if body.LastSeen != nil {
		t.Errorf("last_seen = %v, want nil (never-seen)", *body.LastSeen)
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
}

func TestHandleGlance_LastSeenFiltersOutOlderEvents(t *testing.T) {
	// Old event: 2026-05-20T20:46:40Z; new event: 2026-05-20T23:33:20Z.
	page := []events.FrigateEvent{
		{ID: "old", Camera: "camA", Label: "person", StartTime: 1779310000, EndTime: ptrF(1779310010), HasSnapshot: true},
		{ID: "new", Camera: "camA", Label: "person", StartTime: 1779320000, EndTime: ptrF(1779320010), HasSnapshot: true},
	}
	router, store := glanceRouterWith(t, frigateEventsHandler(t, page, nil))

	// Ack to a time between them so only "new" should remain unseen.
	seen, err := store.Ack(mustParse(t, "2026-05-20T22:00:00Z"))
	if err != nil {
		t.Fatalf("seed Ack: %v", err)
	}
	if seen.IsZero() {
		t.Fatal("seed Ack returned zero")
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
	if body.LastSeen == nil || *body.LastSeen == "" {
		t.Fatal("last_seen should be set after Ack")
	}
	if body.UnseenCount != 1 {
		t.Errorf("unseen_count = %d, want 1 (only 'new' survives)", body.UnseenCount)
	}
	if len(body.Moments) != 1 || body.Moments[0].RepresentativeEventID != "new" {
		t.Errorf("unseen moment rep = %+v, want id=new", body.Moments)
	}
}

func TestHandleGlanceAck_MissingBody_BadRequest(t *testing.T) {
	router, _ := glanceRouterWith(t, http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("upstream must not be called on ack validation failure")
	}))
	req := httptest.NewRequest(http.MethodPost, "/api/glance/ack", strings.NewReader(""))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

func TestHandleGlanceAck_EmptySeenThrough_BadRequest(t *testing.T) {
	router, _ := glanceRouterWith(t, http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("upstream must not be called on ack validation failure")
	}))
	req := httptest.NewRequest(http.MethodPost, "/api/glance/ack", strings.NewReader(`{"seen_through":""}`))
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

func TestHandleGlanceAck_BadSeenThrough_BadRequest(t *testing.T) {
	router, _ := glanceRouterWith(t, http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("upstream must not be called on ack validation failure")
	}))
	req := httptest.NewRequest(http.MethodPost, "/api/glance/ack", strings.NewReader(`{"seen_through":"not-iso"}`))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

func TestHandleGlanceAck_ValidAdvancesAndFollowupGlanceReflects(t *testing.T) {
	page := []events.FrigateEvent{
		{ID: "old", Camera: "camA", Label: "person", StartTime: 1779310000, EndTime: ptrF(1779310010), HasSnapshot: true},
		{ID: "new", Camera: "camA", Label: "person", StartTime: 1779320000, EndTime: ptrF(1779320010), HasSnapshot: true},
	}
	router, _ := glanceRouterWith(t, frigateEventsHandler(t, page, nil))

	ackReq := httptest.NewRequest(http.MethodPost, "/api/glance/ack",
		strings.NewReader(`{"seen_through":"2026-05-20T22:00:00Z"}`))
	ackW := httptest.NewRecorder()
	router.ServeHTTP(ackW, ackReq)

	if ackW.Code != http.StatusOK {
		t.Fatalf("ack want 200, got %d (body=%s)", ackW.Code, ackW.Body.String())
	}
	if cc := ackW.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("ack Cache-Control = %q, want no-store", cc)
	}
	var ackBody glanceAckResponse
	if err := json.NewDecoder(ackW.Body).Decode(&ackBody); err != nil {
		t.Fatalf("decode ack: %v", err)
	}
	if ackBody.LastSeen == nil || *ackBody.LastSeen != "2026-05-20T22:00:00Z" {
		t.Errorf("ack last_seen = %v, want 2026-05-20T22:00:00Z", ackBody.LastSeen)
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
	if glBody.UnseenCount != 1 || len(glBody.Moments) != 1 {
		t.Fatalf("unseen_count=%d, moments=%d, want 1/1", glBody.UnseenCount, len(glBody.Moments))
	}
	if glBody.Moments[0].RepresentativeEventID != "new" {
		t.Errorf("unseen rep = %q, want new", glBody.Moments[0].RepresentativeEventID)
	}
}

func TestHandleGlanceAck_OlderTimestampIsNoop(t *testing.T) {
	router, store := glanceRouterWith(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("[]"))
	}))

	// Seed last_seen via the store directly.
	seed := mustParse(t, "2026-05-20T22:00:00Z")
	if _, err := store.Ack(seed); err != nil {
		t.Fatalf("seed: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/glance/ack",
		strings.NewReader(`{"seen_through":"2026-05-20T20:00:00Z"}`))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	var body glanceAckResponse
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.LastSeen == nil || *body.LastSeen != "2026-05-20T22:00:00Z" {
		t.Errorf("last_seen = %v, want 2026-05-20T22:00:00Z (older Ack must be a no-op)", body.LastSeen)
	}
	if !store.LastSeen().Equal(seed) {
		t.Errorf("store LastSeen = %v, want %v", store.LastSeen(), seed)
	}
}

func TestHandleGlance_UpstreamErrorMapped(t *testing.T) {
	router, _ := glanceRouterWith(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
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

func mustParse(t *testing.T, s string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return parsed
}
