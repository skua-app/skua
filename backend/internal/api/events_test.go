package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/skua-app/skua/internal/capabilities"
	"github.com/skua-app/skua/internal/events"
	applog "github.com/skua-app/skua/internal/log"
	"github.com/skua-app/skua/internal/sse"
)

func eventsRouterWith(t *testing.T, upstream http.Handler) (http.Handler, *httptest.Server) {
	t.Helper()
	frigateSrv := httptest.NewServer(upstream)
	t.Cleanup(frigateSrv.Close)

	logger := applog.New("error", "text")
	eventsClient := events.NewClient(frigateSrv.URL, &http.Client{})
	h := NewHandler(logger, nil, eventsClient, nil, nil, "", nil, "http://frigate.example/ui", 0, &http.Client{}, nil, nil, nil, capabilities.NewForTest(nil), nil, RuntimeConfigDeps{})
	staticFS := fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("ok")}}
	return NewRouter(h, sse.NewHub(logger), logger, staticFS), frigateSrv
}

func TestHandleConfig(t *testing.T) {
	router, _ := eventsRouterWith(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))

	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	var cfg map[string]string
	if err := json.NewDecoder(w.Body).Decode(&cfg); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if cfg["frigate_ui_url"] != "http://frigate.example/ui" {
		t.Errorf("frigate_ui_url = %q, want http://frigate.example/ui", cfg["frigate_ui_url"])
	}
}

func TestHandleEvents_QueryTranslation(t *testing.T) {
	var gotQuery url.Values
	router, _ := eventsRouterWith(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("[]"))
	}))

	// before is ISO 8601 from the client; gets converted to unix-seconds for Frigate.
	req := httptest.NewRequest(http.MethodGet,
		"/api/events?camera=cam5&camera=cam7&label=person,car&before=2026-05-19T22:00:00Z&limit=10", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (body=%s)", w.Code, w.Body.String())
	}
	if gotQuery.Get("cameras") != "cam5,cam7" {
		t.Errorf("cameras = %q", gotQuery.Get("cameras"))
	}
	if gotQuery.Get("labels") != "person,car" {
		t.Errorf("labels = %q", gotQuery.Get("labels"))
	}
	if gotQuery.Get("before") != fmt.Sprintf("%d", 1779228000) {
		// 2026-05-19T22:00:00Z = 1779228000 unix seconds
		t.Errorf("before = %q, want unix seconds for 2026-05-19T22:00:00Z", gotQuery.Get("before"))
	}
	if gotQuery.Get("limit") != "10" {
		t.Errorf("limit = %q", gotQuery.Get("limit"))
	}
	if gotQuery.Get("has_snapshot") != "1" {
		t.Errorf("has_snapshot must always be 1; got %q", gotQuery.Get("has_snapshot"))
	}
}

func TestHandleEvents_BadBefore(t *testing.T) {
	router, _ := eventsRouterWith(t, http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("upstream must not be called on validation failure")
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/events?before=not-iso", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

func TestValidUpstreamID(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"1779310931.839109-kv3b4h", true},
		{"cam5", true},
		{"front_door", true},
		{"A_b.1-2", true},
		{"", false},
		{"../etc", false},
		{"a/b", false},
		{"a?b", false},
		{"a#b", false},
		{"a b", false},
		{"a%2Fb", false}, // percent-encoded "/" — the "%" itself is rejected
		{"cam5;rm", false},
	}
	for _, c := range cases {
		if got := validUpstreamID(c.in); got != c.want {
			t.Errorf("validUpstreamID(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestHandleEventClip_RejectsHostileID(t *testing.T) {
	router, _ := eventsRouterWith(t, http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		t.Fatalf("upstream must not be called for hostile id, got path %q", r.URL.Path)
	}))
	// chi captures everything between two slashes into {id}, so "%2F" stays
	// percent-encoded in the path segment and validUpstreamID rejects "%".
	req := httptest.NewRequest(http.MethodGet, "/api/events/abc%2F..%2Fadmin/clip.mp4", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d (body=%s)", w.Code, w.Body.String())
	}
}

func TestHandleEventThumbnail_RejectsHostileID(t *testing.T) {
	router, _ := eventsRouterWith(t, http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		t.Fatalf("upstream must not be called for hostile id, got path %q", r.URL.Path)
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/events/abc%20def/thumbnail.jpg", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

func TestHandleEventThumbnail_ImmutableCache(t *testing.T) {
	jpegBytes := []byte{0xff, 0xd8, 0xff, 0xe0}
	router, _ := eventsRouterWith(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/thumbnail.jpg") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "image/jpeg")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(jpegBytes)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/events/1779310931.839109-kv3b4h/thumbnail.jpg", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	if cc := w.Header().Get("Cache-Control"); cc != "public, max-age=31536000, immutable" {
		t.Errorf("Cache-Control = %q", cc)
	}
	if w.Header().Get("ETag") == "" {
		t.Error("ETag must be set (passed through or derived from id)")
	}
	body, _ := io.ReadAll(w.Body)
	if string(body) != string(jpegBytes) {
		t.Errorf("body mismatch")
	}
}
