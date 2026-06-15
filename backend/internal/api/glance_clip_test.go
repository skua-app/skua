package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/skua-app/skua/internal/capabilities"
	"github.com/skua-app/skua/internal/events"
	"github.com/skua-app/skua/internal/glance"
	applog "github.com/skua-app/skua/internal/log"
	"github.com/skua-app/skua/internal/sse"
)

// clipBytes is the deterministic body the fake Frigate returns for
// /api/{camera}/start/.../end/.../clip.mp4. The BFF buffers it via the
// clip pipeline and serves it through http.ServeContent, so we get
// real 200 / 206 / Content-Range behaviour without needing a real MP4.
var clipBytes = []byte("MOMENT-CLIP-MP4-0123456789-ABCDEFGHIJKLMNOPQRSTUVWX")

// clipPathRE matches Frigate's time-range clip path; the fake server
// uses it to verify the BFF constructs the URL Frigate expects.
var clipPathRE = regexp.MustCompile(`^/api/([^/]+)/start/([0-9.]+)/end/([0-9.]+)/clip\.mp4$`)

// frigateClipFake is a fake Frigate covering the two endpoints the
// glance clip path depends on: GET /api/review/{id} returning a single
// review JSON object, and GET on the time-range clip path returning
// mp4 bytes the chunked + Range-ignoring way (just like Frigate
// itself; the BFF's buffered pipeline is exactly the workaround). The
// fake honours notFoundIDs / serverErrIDs so individual tests can
// exercise the review error legs.
type frigateClipFake struct {
	reviews      map[string]events.FrigateReviewItem
	notFoundIDs  map[string]struct{}
	serverErrIDs map[string]struct{}
}

func (f *frigateClipFake) handler(t *testing.T) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/review/") {
			id := strings.TrimPrefix(r.URL.Path, "/api/review/")
			if _, ok := f.notFoundIDs[id]; ok {
				http.Error(w, "missing", http.StatusNotFound)
				return
			}
			if _, ok := f.serverErrIDs[id]; ok {
				http.Error(w, "boom", http.StatusInternalServerError)
				return
			}
			item, ok := f.reviews[id]
			if !ok {
				http.Error(w, "missing", http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(item)
			return
		}

		if m := clipPathRE.FindStringSubmatch(r.URL.Path); m != nil {
			if _, err := strconv.ParseFloat(m[2], 64); err != nil {
				t.Errorf("upstream start not a float: %q", m[2])
			}
			if _, err := strconv.ParseFloat(m[3], 64); err != nil {
				t.Errorf("upstream end not a float: %q", m[3])
			}
			// Frigate ignores Range and replies chunked; that is the
			// shape the buffered BFF pipeline absorbs. Do not set
			// Content-Length; do not honour Range here.
			w.Header().Set("Content-Type", "video/mp4")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(clipBytes)
			return
		}

		http.NotFound(w, r)
	})
}

func clipRouter(t *testing.T, fake *frigateClipFake) http.Handler {
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

func TestHandleGlanceClip_HappyPath_BodyServedInline(t *testing.T) {
	fake := &frigateClipFake{
		reviews: map[string]events.FrigateReviewItem{
			"rev-A": {
				ID:        "rev-A",
				Camera:    "camA",
				StartTime: 1779310000,
				EndTime:   ptrF(1779310060),
				Severity:  "alert",
			},
		},
	}
	router := clipRouter(t, fake)

	req := httptest.NewRequest(http.MethodGet, "/api/glance/rev-A/clip.mp4", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (body=%s)", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); ct != "video/mp4" {
		t.Errorf("Content-Type = %q, want video/mp4", ct)
	}
	if cd := w.Header().Get("Content-Disposition"); cd != "inline" {
		t.Errorf("Content-Disposition = %q, want inline", cd)
	}
	if cc := w.Header().Get("Cache-Control"); cc != "public, max-age=31536000, immutable" {
		t.Errorf("Cache-Control = %q", cc)
	}
	if etag := w.Header().Get("ETag"); etag != `"rev-A"` {
		t.Errorf("ETag = %q, want \"rev-A\"", etag)
	}
	// ServeContent flattens the chunked upstream into a Content-Length
	// response — the whole point of the buffered pipeline.
	if cl := w.Header().Get("Content-Length"); cl != strconv.Itoa(len(clipBytes)) {
		t.Errorf("Content-Length = %q, want %d", cl, len(clipBytes))
	}
	if !strings.Contains(w.Body.String(), string(clipBytes)) {
		t.Errorf("body mismatch: got %q, want to contain %q", w.Body.String(), clipBytes)
	}
}

func TestHandleGlanceClip_RangeRequestReturns206(t *testing.T) {
	fake := &frigateClipFake{
		reviews: map[string]events.FrigateReviewItem{
			"rev-A": {
				ID:        "rev-A",
				Camera:    "camA",
				StartTime: 1779310000,
				EndTime:   ptrF(1779310060),
			},
		},
	}
	router := clipRouter(t, fake)

	req := httptest.NewRequest(http.MethodGet, "/api/glance/rev-A/clip.mp4", nil)
	req.Header.Set("Range", "bytes=8-15")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusPartialContent {
		t.Fatalf("want 206, got %d (body=%s)", w.Code, w.Body.String())
	}
	if cd := w.Header().Get("Content-Disposition"); cd != "inline" {
		t.Errorf("Content-Disposition = %q, want inline", cd)
	}
	// http.ServeContent emits Accept-Ranges + Content-Range against the
	// buffered byte slice.
	if ar := w.Header().Get("Accept-Ranges"); ar != "bytes" {
		t.Errorf("Accept-Ranges = %q, want bytes", ar)
	}
	if cr := w.Header().Get("Content-Range"); cr == "" {
		t.Errorf("Content-Range missing")
	} else if want := "bytes 8-15/" + strconv.Itoa(len(clipBytes)); cr != want {
		t.Errorf("Content-Range = %q, want %q", cr, want)
	}
	if got, want := w.Body.String(), string(clipBytes[8:16]); got != want {
		t.Errorf("body = %q, want %q", got, want)
	}
}

func TestHandleGlanceClip_InvalidID_NotFound(t *testing.T) {
	fake := &frigateClipFake{reviews: map[string]events.FrigateReviewItem{}}
	router := clipRouter(t, fake)

	// Percent-encoded space fails validUpstreamID at the boundary.
	req := httptest.NewRequest(http.MethodGet, "/api/glance/has%20space/clip.mp4", nil)
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

func TestHandleGlanceClip_UpstreamReview404_NotFound(t *testing.T) {
	fake := &frigateClipFake{
		reviews:     map[string]events.FrigateReviewItem{},
		notFoundIDs: map[string]struct{}{"rev-missing": {}},
	}
	router := clipRouter(t, fake)

	req := httptest.NewRequest(http.MethodGet, "/api/glance/rev-missing/clip.mp4", nil)
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

func TestHandleGlanceClip_UpstreamReview500_BadGateway(t *testing.T) {
	fake := &frigateClipFake{
		reviews:      map[string]events.FrigateReviewItem{},
		serverErrIDs: map[string]struct{}{"rev-boom": {}},
	}
	router := clipRouter(t, fake)

	req := httptest.NewRequest(http.MethodGet, "/api/glance/rev-boom/clip.mp4", nil)
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
