package api

import (
	"encoding/json"
	"fmt"
	"io"
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

// previewBytes is a deterministic body the fake Frigate hands back from
// /api/{camera}/start/.../end/.../preview.mp4. Not actually playable;
// the test only verifies that the BFF proxies the bytes through and
// honours Range — codec contents are irrelevant.
var previewBytes = []byte("PREVIEW-MP4-BODY-0123456789-ABCDEFGHIJ")

// previewPathRE matches Frigate's preview.mp4 path with start/end as
// decimal floats. The fake server uses this to parse the URL the BFF
// constructs (so the test fails loudly if the URL grammar drifts).
var previewPathRE = regexp.MustCompile(`^/api/([^/]+)/start/([0-9.]+)/end/([0-9.]+)/preview\.mp4$`)

// frigatePreviewHandler is a fake Frigate that serves the two endpoints
// the glance-preview path depends on: GET /api/review/{id} returning a
// single review JSON object, and GET / HEAD on the preview.mp4 path
// returning video/mp4 bytes with Frigate's attachment disposition and
// full Range support. Reviews whose id is in `notFoundIDs` reply 404;
// reviews in `serverErrIDs` reply 500. The fake captures the last
// Range header it received so tests can assert passthrough.
type frigatePreviewFake struct {
	reviews      map[string]events.FrigateReviewItem
	notFoundIDs  map[string]struct{}
	serverErrIDs map[string]struct{}
	gotRange     string
}

func (f *frigatePreviewFake) handler(t *testing.T) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// /api/review/{id} (single object).
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

		// /api/{camera}/start/{start}/end/{end}/preview.mp4
		if m := previewPathRE.FindStringSubmatch(r.URL.Path); m != nil {
			// Validate start/end parse as floats; the BFF formats with
			// 6 decimal places, so non-parseable values are a contract
			// regression worth surfacing.
			if _, err := strconv.ParseFloat(m[2], 64); err != nil {
				t.Errorf("upstream start not a float: %q", m[2])
			}
			if _, err := strconv.ParseFloat(m[3], 64); err != nil {
				t.Errorf("upstream end not a float: %q", m[3])
			}
			f.gotRange = r.Header.Get("Range")

			w.Header().Set("Content-Type", "video/mp4")
			w.Header().Set("Accept-Ranges", "bytes")
			// Frigate sends attachment; the BFF must override to inline.
			w.Header().Set("Content-Disposition", "attachment; filename=\"preview.mp4\"")

			if rng := r.Header.Get("Range"); rng != "" {
				// Minimal byte-range handling: support a single
				// "bytes=<start>-<end>" form.
				var rs, re int
				if _, err := fmt.Sscanf(rng, "bytes=%d-%d", &rs, &re); err == nil &&
					rs >= 0 && re < len(previewBytes) && rs <= re {
					slice := previewBytes[rs : re+1]
					w.Header().Set("Content-Range",
						fmt.Sprintf("bytes %d-%d/%d", rs, re, len(previewBytes)))
					w.Header().Set("Content-Length", strconv.Itoa(len(slice)))
					w.WriteHeader(http.StatusPartialContent)
					if r.Method != http.MethodHead {
						_, _ = w.Write(slice)
					}
					return
				}
			}
			w.Header().Set("Content-Length", strconv.Itoa(len(previewBytes)))
			w.WriteHeader(http.StatusOK)
			if r.Method != http.MethodHead {
				_, _ = w.Write(previewBytes)
			}
			return
		}

		http.NotFound(w, r)
	})
}

// previewRouter spins up the fake Frigate, wires up an api.Router
// against it, and returns both the BFF router and the fake so tests
// can inspect captured headers.
func previewRouter(t *testing.T, fake *frigatePreviewFake) http.Handler {
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

func TestHandleGlancePreview_HappyPath_BodyProxiedInline(t *testing.T) {
	fake := &frigatePreviewFake{
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
	router := previewRouter(t, fake)

	req := httptest.NewRequest(http.MethodGet, "/api/glance/rev-A/preview.mp4", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (body=%s)", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); ct != "video/mp4" {
		t.Errorf("Content-Type = %q, want video/mp4", ct)
	}
	if cd := w.Header().Get("Content-Disposition"); cd != "inline" {
		t.Errorf("Content-Disposition = %q, want inline (must override Frigate's attachment)", cd)
	}
	if cc := w.Header().Get("Cache-Control"); cc != "private, max-age=86400" {
		t.Errorf("Cache-Control = %q", cc)
	}
	if ar := w.Header().Get("Accept-Ranges"); ar != "bytes" {
		t.Errorf("Accept-Ranges = %q, want bytes (passthrough)", ar)
	}
	got, _ := io.ReadAll(w.Body)
	if string(got) != string(previewBytes) {
		t.Errorf("body = %q, want %q", got, previewBytes)
	}
}

func TestHandleGlancePreview_RangeForwardedAndPartial(t *testing.T) {
	fake := &frigatePreviewFake{
		reviews: map[string]events.FrigateReviewItem{
			"rev-A": {
				ID:        "rev-A",
				Camera:    "camA",
				StartTime: 1779310000,
				EndTime:   ptrF(1779310060),
			},
		},
	}
	router := previewRouter(t, fake)

	req := httptest.NewRequest(http.MethodGet, "/api/glance/rev-A/preview.mp4", nil)
	req.Header.Set("Range", "bytes=8-15")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusPartialContent {
		t.Fatalf("want 206, got %d (body=%s)", w.Code, w.Body.String())
	}
	if fake.gotRange != "bytes=8-15" {
		t.Errorf("upstream got Range = %q, want bytes=8-15 (verbatim forward)", fake.gotRange)
	}
	if cr := w.Header().Get("Content-Range"); cr == "" {
		t.Errorf("Content-Range missing, want passthrough")
	} else if want := fmt.Sprintf("bytes 8-15/%d", len(previewBytes)); cr != want {
		t.Errorf("Content-Range = %q, want %q", cr, want)
	}
	if cd := w.Header().Get("Content-Disposition"); cd != "inline" {
		t.Errorf("Content-Disposition = %q, want inline", cd)
	}
	got, _ := io.ReadAll(w.Body)
	if string(got) != string(previewBytes[8:16]) {
		t.Errorf("body = %q, want %q", got, previewBytes[8:16])
	}
}

func TestHandleGlancePreview_HEADSkipsBody(t *testing.T) {
	fake := &frigatePreviewFake{
		reviews: map[string]events.FrigateReviewItem{
			"rev-A": {
				ID:        "rev-A",
				Camera:    "camA",
				StartTime: 1779310000,
				EndTime:   ptrF(1779310060),
			},
		},
	}
	router := previewRouter(t, fake)

	req := httptest.NewRequest(http.MethodHead, "/api/glance/rev-A/preview.mp4", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	if w.Body.Len() != 0 {
		t.Errorf("HEAD body length = %d, want 0", w.Body.Len())
	}
	if ct := w.Header().Get("Content-Type"); ct != "video/mp4" {
		t.Errorf("Content-Type = %q, want video/mp4", ct)
	}
	if cd := w.Header().Get("Content-Disposition"); cd != "inline" {
		t.Errorf("Content-Disposition = %q, want inline", cd)
	}
}

func TestHandleGlancePreview_InvalidID_NotFound(t *testing.T) {
	fake := &frigatePreviewFake{reviews: map[string]events.FrigateReviewItem{}}
	router := previewRouter(t, fake)

	// Slash and percent-encoded slash both fail validation; the route
	// regex won't even match a literal `/` in {id}, but `..` and
	// other punctuation reach the handler and get rejected.
	req := httptest.NewRequest(http.MethodGet, "/api/glance/has%20space/preview.mp4", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d (body=%s)", w.Code, w.Body.String())
	}
	var body map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["error"] != "not_found" {
		t.Errorf("error = %q, want not_found", body["error"])
	}
}

func TestHandleGlancePreview_UpstreamReview404_NotFound(t *testing.T) {
	fake := &frigatePreviewFake{
		reviews:     map[string]events.FrigateReviewItem{},
		notFoundIDs: map[string]struct{}{"rev-missing": {}},
	}
	router := previewRouter(t, fake)

	req := httptest.NewRequest(http.MethodGet, "/api/glance/rev-missing/preview.mp4", nil)
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

func TestHandleGlancePreview_UpstreamReview500_BadGateway(t *testing.T) {
	fake := &frigatePreviewFake{
		reviews:      map[string]events.FrigateReviewItem{},
		serverErrIDs: map[string]struct{}{"rev-boom": {}},
	}
	router := previewRouter(t, fake)

	req := httptest.NewRequest(http.MethodGet, "/api/glance/rev-boom/preview.mp4", nil)
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
