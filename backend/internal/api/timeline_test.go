package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/fstest"

	"github.com/skua-app/skua/internal/cameras"
	"github.com/skua-app/skua/internal/config"
	"github.com/skua-app/skua/internal/frigate"
	applog "github.com/skua-app/skua/internal/log"
	"github.com/skua-app/skua/internal/sse"
)

// vodPathRE matches Frigate's recording VOD path. The fake server
// uses this to parse the URL the BFF constructs (so the test fails
// loudly if the path grammar drifts).
var vodPathRE = regexp.MustCompile(`^/vod/([^/]+)/start/([0-9]+)/end/([0-9]+)/([^/]+)$`)

// recordingsSummaryRE matches Frigate's recordings summary path.
var recordingsSummaryRE = regexp.MustCompile(`^/api/([^/]+)/recordings/summary$`)

// frigateTimelineFake captures the requests the BFF sends so tests
// can assert passthrough behaviour (Range, timezone) and so the
// traversal cases can verify that NO upstream request was made.
type frigateTimelineFake struct {
	mu                sync.Mutex
	vodHits           int32
	gotRange          string
	gotTimezone       string
	playlistBody      []byte
	segmentBody       []byte
	segmentRangeReply bool // when true, honour Range on segment fetches with 206
	summaryBody       []byte
	summaryStatus     int
}

func (f *frigateTimelineFake) handler(t *testing.T) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if m := vodPathRE.FindStringSubmatch(r.URL.Path); m != nil {
			atomic.AddInt32(&f.vodHits, 1)
			f.mu.Lock()
			f.gotRange = r.Header.Get("Range")
			f.mu.Unlock()

			rest := m[4]
			if strings.HasSuffix(rest, ".m3u8") {
				w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
				w.Header().Set("Content-Length", strconv.Itoa(len(f.playlistBody)))
				w.WriteHeader(http.StatusOK)
				if r.Method != http.MethodHead {
					_, _ = w.Write(f.playlistBody)
				}
				return
			}

			// segment (.mp4 init or .m4s).
			w.Header().Set("Content-Type", "video/mp4")
			w.Header().Set("Accept-Ranges", "bytes")
			w.Header().Set("ETag", `"abc"`)

			if f.segmentRangeReply {
				if rng := r.Header.Get("Range"); rng != "" {
					var rs, re int
					if _, err := fmt.Sscanf(rng, "bytes=%d-%d", &rs, &re); err == nil &&
						rs >= 0 && re < len(f.segmentBody) && rs <= re {
						slice := f.segmentBody[rs : re+1]
						w.Header().Set("Content-Range",
							fmt.Sprintf("bytes %d-%d/%d", rs, re, len(f.segmentBody)))
						w.Header().Set("Content-Length", strconv.Itoa(len(slice)))
						w.WriteHeader(http.StatusPartialContent)
						if r.Method != http.MethodHead {
							_, _ = w.Write(slice)
						}
						return
					}
				}
			}
			w.Header().Set("Content-Length", strconv.Itoa(len(f.segmentBody)))
			w.WriteHeader(http.StatusOK)
			if r.Method != http.MethodHead {
				_, _ = w.Write(f.segmentBody)
			}
			return
		}

		if recordingsSummaryRE.MatchString(r.URL.Path) {
			f.mu.Lock()
			f.gotTimezone = r.URL.Query().Get("timezone")
			f.mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			status := f.summaryStatus
			if status == 0 {
				status = http.StatusOK
			}
			w.WriteHeader(status)
			_, _ = w.Write(f.summaryBody)
			return
		}

		http.NotFound(w, r)
	})
}

func newTimelineFake() *frigateTimelineFake {
	return &frigateTimelineFake{
		playlistBody: []byte("#EXTM3U\n#EXT-X-VERSION:7\n#EXT-X-STREAM-INF:BANDWIDTH=1\nindex-0.m3u8\n"),
		segmentBody:  []byte("SEG-M4S-BODY-0123456789-ABCDEFGHIJ"),
		summaryBody:  []byte(`[{"day":"2026-06-15","events":3,"hours":[{"hour":"00","events":1}]}]`),
	}
}

// timelineRouter spins up the fake Frigate, wires up an api.Router
// against it, and returns the BFF router + fake so tests can inspect
// captured headers and the upstream-hit counter.
func timelineRouter(t *testing.T, fake *frigateTimelineFake) http.Handler {
	t.Helper()
	frigateSrv := httptest.NewServer(fake.handler(t))
	t.Cleanup(frigateSrv.Close)

	logger := applog.New("error", "text")
	frigateClient := frigate.NewClient(frigateSrv.URL, &http.Client{})
	camSpecs := []config.CameraSpec{
		{ID: "cam1", Name: "Cam 1", StreamMain: "cam1_main", StreamSub: "cam1_sub"},
	}
	h := NewHandler(HandlerDeps{
		Logger:     logger,
		Frigate:    frigateClient,
		Cameras:    cameras.NewForTest(camSpecs),
		HTTPClient: &http.Client{},
	})
	staticFS := fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("ok")}}
	return NewRouter(h, sse.NewHub(logger), logger, staticFS)
}

func TestHandleTimelineVOD_PlaylistPassthrough(t *testing.T) {
	fake := newTimelineFake()
	router := timelineRouter(t, fake)

	req := httptest.NewRequest(http.MethodGet, "/api/cameras/cam1/vod/1779310000/1779310600/master.m3u8", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (body=%s)", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/vnd.apple.mpegurl" {
		t.Errorf("Content-Type = %q, want application/vnd.apple.mpegurl", ct)
	}
	if cc := w.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store for .m3u8", cc)
	}
	got, _ := io.ReadAll(w.Body)
	if string(got) != string(fake.playlistBody) {
		t.Errorf("body = %q, want %q", got, fake.playlistBody)
	}
}

func TestHandleTimelineVOD_SegmentRangeForwarded(t *testing.T) {
	fake := newTimelineFake()
	fake.segmentRangeReply = true
	router := timelineRouter(t, fake)

	req := httptest.NewRequest(http.MethodGet, "/api/cameras/cam1/vod/1779310000/1779310600/seg-12.m4s", nil)
	req.Header.Set("Range", "bytes=4-9")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusPartialContent {
		t.Fatalf("want 206, got %d (body=%s)", w.Code, w.Body.String())
	}
	fake.mu.Lock()
	gotRange := fake.gotRange
	fake.mu.Unlock()
	if gotRange != "bytes=4-9" {
		t.Errorf("upstream got Range = %q, want bytes=4-9 (verbatim forward)", gotRange)
	}
	if cr := w.Header().Get("Content-Range"); cr == "" {
		t.Errorf("Content-Range missing, want passthrough")
	} else if want := fmt.Sprintf("bytes 4-9/%d", len(fake.segmentBody)); cr != want {
		t.Errorf("Content-Range = %q, want %q", cr, want)
	}
	if ar := w.Header().Get("Accept-Ranges"); ar != "bytes" {
		t.Errorf("Accept-Ranges = %q, want bytes (passthrough)", ar)
	}
	if cc := w.Header().Get("Cache-Control"); cc != "public, max-age=31536000, immutable" {
		t.Errorf("Cache-Control = %q, want immutable for .m4s", cc)
	}
	got, _ := io.ReadAll(w.Body)
	if string(got) != string(fake.segmentBody[4:10]) {
		t.Errorf("body = %q, want %q", got, fake.segmentBody[4:10])
	}
}

func TestHandleTimelineVOD_HEADSkipsBody(t *testing.T) {
	fake := newTimelineFake()
	router := timelineRouter(t, fake)

	req := httptest.NewRequest(http.MethodHead, "/api/cameras/cam1/vod/1779310000/1779310600/init-0.mp4", nil)
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
	if cc := w.Header().Get("Cache-Control"); cc != "public, max-age=31536000, immutable" {
		t.Errorf("Cache-Control = %q, want immutable for .mp4", cc)
	}
}

func TestHandleTimelineVOD_InvalidStart_BadRequest(t *testing.T) {
	fake := newTimelineFake()
	router := timelineRouter(t, fake)

	req := httptest.NewRequest(http.MethodGet, "/api/cameras/cam1/vod/abc/1779310600/master.m3u8", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d (body=%s)", w.Code, w.Body.String())
	}
	var body map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["error"] != "invalid_range" {
		t.Errorf("error = %q, want invalid_range", body["error"])
	}
	if atomic.LoadInt32(&fake.vodHits) != 0 {
		t.Errorf("upstream hit on invalid_range: %d", fake.vodHits)
	}
}

func TestHandleTimelineVOD_UnknownCamera_NotFound(t *testing.T) {
	fake := newTimelineFake()
	router := timelineRouter(t, fake)

	req := httptest.NewRequest(http.MethodGet, "/api/cameras/camX/vod/1779310000/1779310600/master.m3u8", nil)
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
	if atomic.LoadInt32(&fake.vodHits) != 0 {
		t.Errorf("upstream hit on unknown camera: %d", fake.vodHits)
	}
}

func TestHandleTimelineVOD_TraversalRejected_NoUpstream(t *testing.T) {
	// validVODPart is the SSRF / path-traversal guard for the wildcard
	// {*} slot. Every case below must surface 404 and MUST NOT touch
	// the upstream Frigate fake — a request that reaches Frigate with
	// a traversal-shaped path defeats the guard's purpose.
	cases := []struct {
		name string
		rest string
	}{
		{"parentTraversal", "../config"},
		{"dotdot", ".."},
		{"slashInside", "foo/bar"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := newTimelineFake()
			router := timelineRouter(t, fake)

			req := httptest.NewRequest(http.MethodGet,
				"/api/cameras/cam1/vod/1779310000/1779310600/"+tc.rest, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != http.StatusNotFound {
				t.Fatalf("rest=%q: want 404, got %d (body=%s)", tc.rest, w.Code, w.Body.String())
			}
			if atomic.LoadInt32(&fake.vodHits) != 0 {
				t.Errorf("rest=%q: upstream was hit (%d), want zero", tc.rest, fake.vodHits)
			}
		})
	}
}

func TestHandleRecordingsSummary_PassthroughWithTimezone(t *testing.T) {
	fake := newTimelineFake()
	router := timelineRouter(t, fake)

	req := httptest.NewRequest(http.MethodGet, "/api/cameras/cam1/recordings-summary?timezone=Europe/Berlin", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (body=%s)", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	fake.mu.Lock()
	gotTZ := fake.gotTimezone
	fake.mu.Unlock()
	if gotTZ != "Europe/Berlin" {
		t.Errorf("upstream timezone = %q, want Europe/Berlin", gotTZ)
	}
	got, _ := io.ReadAll(w.Body)
	if string(got) != string(fake.summaryBody) {
		t.Errorf("body = %q, want verbatim %q", got, fake.summaryBody)
	}
}

func TestHandleRecordingsSummary_NoTimezone_NotForwarded(t *testing.T) {
	fake := newTimelineFake()
	router := timelineRouter(t, fake)

	req := httptest.NewRequest(http.MethodGet, "/api/cameras/cam1/recordings-summary", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	fake.mu.Lock()
	gotTZ := fake.gotTimezone
	fake.mu.Unlock()
	if gotTZ != "" {
		t.Errorf("upstream timezone = %q, want empty (omitted)", gotTZ)
	}
}
