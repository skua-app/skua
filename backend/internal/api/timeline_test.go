package api

import (
	"context"
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
	"time"

	"github.com/skua-app/skua/internal/cameras"
	"github.com/skua-app/skua/internal/config"
	"github.com/skua-app/skua/internal/events"
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

// previewFramesPathRE matches Frigate's preview-frames path
// (/api/preview/{cam}/start/{start}/end/{end}/frames).
var previewFramesPathRE = regexp.MustCompile(`^/api/preview/([^/]+)/start/([0-9]+)/end/([0-9]+)/frames$`)

// previewClipsListPathRE matches Frigate's preview-clips LIST path
// (/api/preview/{cam}/start/{start}/end/{end} — no trailing /frames).
var previewClipsListPathRE = regexp.MustCompile(`^/api/preview/([^/]+)/start/([0-9]+)/end/([0-9]+)$`)

// previewClipFilePathRE matches Frigate's static preview clip file path
// (/clips/previews/{cam}/{file}).
var previewClipFilePathRE = regexp.MustCompile(`^/clips/previews/([^/]+)/([^/]+)$`)

// previewFramePathRE matches Frigate's single preview-frame image path
// (/api/preview/{file}/thumbnail.webp). The camera is NOT a separate
// segment — it is encoded in the frame filename.
var previewFramePathRE = regexp.MustCompile(`^/api/preview/([^/]+)/thumbnail\.webp$`)

// The preview path (/api/{cam}/start/{start}/end/{end}/preview.mp4) is
// matched with previewPathRE, declared in glance_preview_test.go (same
// package) — the timeline preview proxy rides the same upstream URL.

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
	previewHits       int32
	gotPreviewRange   string
	previewBody       []byte
	previewFramesHits int32
	previewFramesBody []byte
	framesStatus      int
	clipsListHits     int32
	clipsListBody     []byte
	clipsListStatus   int
	clipFileHits      int32
	gotClipRange      string
	clipFileBody      []byte
	frameFileHits     int32
	frameFileBody     []byte
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

		if previewFramesPathRE.MatchString(r.URL.Path) {
			atomic.AddInt32(&f.previewFramesHits, 1)
			w.Header().Set("Content-Type", "application/json")
			status := f.framesStatus
			if status == 0 {
				status = http.StatusOK
			}
			w.WriteHeader(status)
			_, _ = w.Write(f.previewFramesBody)
			return
		}

		if previewClipsListPathRE.MatchString(r.URL.Path) {
			atomic.AddInt32(&f.clipsListHits, 1)
			w.Header().Set("Content-Type", "application/json")
			status := f.clipsListStatus
			if status == 0 {
				status = http.StatusOK
			}
			w.WriteHeader(status)
			_, _ = w.Write(f.clipsListBody)
			return
		}

		if previewClipFilePathRE.MatchString(r.URL.Path) {
			atomic.AddInt32(&f.clipFileHits, 1)
			f.mu.Lock()
			f.gotClipRange = r.Header.Get("Range")
			f.mu.Unlock()

			w.Header().Set("Content-Type", "video/mp4")
			w.Header().Set("Accept-Ranges", "bytes")
			// Frigate's static clip sends attachment; the BFF must drop it.
			w.Header().Set("Content-Disposition", "attachment; filename=clip.mp4")

			if rng := r.Header.Get("Range"); rng != "" {
				var rs, re int
				if _, err := fmt.Sscanf(rng, "bytes=%d-%d", &rs, &re); err == nil &&
					rs >= 0 && re < len(f.clipFileBody) && rs <= re {
					slice := f.clipFileBody[rs : re+1]
					w.Header().Set("Content-Range",
						fmt.Sprintf("bytes %d-%d/%d", rs, re, len(f.clipFileBody)))
					w.Header().Set("Content-Length", strconv.Itoa(len(slice)))
					w.WriteHeader(http.StatusPartialContent)
					if r.Method != http.MethodHead {
						_, _ = w.Write(slice)
					}
					return
				}
			}
			w.Header().Set("Content-Length", strconv.Itoa(len(f.clipFileBody)))
			w.WriteHeader(http.StatusOK)
			if r.Method != http.MethodHead {
				_, _ = w.Write(f.clipFileBody)
			}
			return
		}

		if previewFramePathRE.MatchString(r.URL.Path) {
			atomic.AddInt32(&f.frameFileHits, 1)
			w.Header().Set("Content-Type", "image/webp")
			w.Header().Set("Accept-Ranges", "bytes")
			// Frigate marks these immutable; the BFF replaces Cache-Control.
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			// Frigate's static frame may send attachment; the BFF must drop it.
			w.Header().Set("Content-Disposition", "attachment; filename=thumbnail.webp")
			w.Header().Set("Content-Length", strconv.Itoa(len(f.frameFileBody)))
			w.WriteHeader(http.StatusOK)
			if r.Method != http.MethodHead {
				_, _ = w.Write(f.frameFileBody)
			}
			return
		}

		if previewPathRE.MatchString(r.URL.Path) {
			atomic.AddInt32(&f.previewHits, 1)
			f.mu.Lock()
			f.gotPreviewRange = r.Header.Get("Range")
			f.mu.Unlock()

			w.Header().Set("Content-Type", "video/mp4")
			w.Header().Set("Accept-Ranges", "bytes")
			// Frigate's preview.mp4 sends attachment; the BFF must drop it.
			w.Header().Set("Content-Disposition", "attachment; filename=preview.mp4")

			if rng := r.Header.Get("Range"); rng != "" {
				var rs, re int
				if _, err := fmt.Sscanf(rng, "bytes=%d-%d", &rs, &re); err == nil &&
					rs >= 0 && re < len(f.previewBody) && rs <= re {
					slice := f.previewBody[rs : re+1]
					w.Header().Set("Content-Range",
						fmt.Sprintf("bytes %d-%d/%d", rs, re, len(f.previewBody)))
					w.Header().Set("Content-Length", strconv.Itoa(len(slice)))
					w.WriteHeader(http.StatusPartialContent)
					if r.Method != http.MethodHead {
						_, _ = w.Write(slice)
					}
					return
				}
			}
			w.Header().Set("Content-Length", strconv.Itoa(len(f.previewBody)))
			w.WriteHeader(http.StatusOK)
			if r.Method != http.MethodHead {
				_, _ = w.Write(f.previewBody)
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
		playlistBody:  []byte("#EXTM3U\n#EXT-X-VERSION:7\n#EXT-X-STREAM-INF:BANDWIDTH=1\nindex-0.m3u8\n"),
		segmentBody:   []byte("SEG-M4S-BODY-0123456789-ABCDEFGHIJ"),
		summaryBody:   []byte(`[{"day":"2026-06-15","events":3,"hours":[{"hour":"00","events":1}]}]`),
		previewBody:   []byte("PREVIEW-MP4-BODY-0123456789-ABCDEFGHIJ"),
		clipFileBody:  []byte("PREVIEW-CLIP-BODY-0123456789-ABCDEFGHIJ"),
		frameFileBody: []byte("RIFF....WEBP-FRAME-BODY-0123456789"),
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
	eventsClient := events.NewClient(frigateSrv.URL, &http.Client{}, 0)
	camSpecs := []config.CameraSpec{
		{ID: "cam1", Name: "Cam 1", StreamMain: "cam1_main", StreamSub: "cam1_sub"},
	}
	h := NewHandler(HandlerDeps{
		Logger:     logger,
		Frigate:    frigateClient,
		Events:     eventsClient,
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

// assertNotErrorEnvelope fails if body parses as the writeError
// {"error":...,"message":...} envelope. The cancelled-client cases
// must produce no body at all; this check guards against accidentally
// surfacing a 502 / 504 envelope while the recorder still reports
// status 200 (httptest default).
func assertNotErrorEnvelope(t *testing.T, body []byte) {
	t.Helper()
	if len(body) == 0 {
		return
	}
	var env map[string]string
	if err := json.Unmarshal(body, &env); err == nil {
		if _, hasError := env["error"]; hasError {
			t.Errorf("response carried error envelope %s, want no body", body)
		}
	}
}

// TestHandleTimelineVOD_ClientCancelled_NoErrorWritten exercises the
// iOS HLS-cancel pattern: a request whose context is already cancelled
// reaches the handler, OpenVOD's transport call fails immediately with
// context.Canceled, and the handler must bail without writing a 502
// (no client left to receive one). The check order (Canceled BEFORE
// DeadlineExceeded) is what keeps the playback-cancel storm out of
// the ERROR log.
func TestHandleTimelineVOD_ClientCancelled_NoErrorWritten(t *testing.T) {
	fake := newTimelineFake()
	router := timelineRouter(t, fake)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	req := httptest.NewRequest(http.MethodGet, "/api/cameras/cam1/vod/1779310000/1779310600/master.m3u8", nil).WithContext(ctx)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code == http.StatusBadGateway {
		t.Fatalf("client-cancelled VOD got 502 (body=%s), want no error written", w.Body.String())
	}
	if w.Code == http.StatusGatewayTimeout {
		t.Fatalf("client-cancelled VOD got 504 (body=%s), want no error written", w.Body.String())
	}
	assertNotErrorEnvelope(t, w.Body.Bytes())
}

// TestHandleRecordingsSummary_ClientCancelled_NoErrorWritten mirrors
// the VOD case for the summary handler: a cancelled request context
// must skip writeError and leave no envelope behind.
func TestHandleRecordingsSummary_ClientCancelled_NoErrorWritten(t *testing.T) {
	fake := newTimelineFake()
	router := timelineRouter(t, fake)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	req := httptest.NewRequest(http.MethodGet, "/api/cameras/cam1/recordings-summary", nil).WithContext(ctx)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code == http.StatusBadGateway {
		t.Fatalf("client-cancelled summary got 502 (body=%s), want no error written", w.Body.String())
	}
	if w.Code == http.StatusGatewayTimeout {
		t.Fatalf("client-cancelled summary got 504 (body=%s), want no error written", w.Body.String())
	}
	assertNotErrorEnvelope(t, w.Body.Bytes())
}

// TestHandleTimelineVOD_UpstreamFailure_Still502 guards against the
// cancellation branch accidentally swallowing real upstream failures.
// The fake Frigate is shut down before the request fires, so the
// transport returns a connect-refused error (not context.Canceled);
// the handler must still surface 502 upstream_error with the standard
// envelope. The request context is fresh so r.Context().Err() is nil
// and the canceled-branch predicate must NOT trigger.
func TestHandleTimelineVOD_UpstreamFailure_Still502(t *testing.T) {
	logger := applog.New("error", "text")
	frigateSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// never reached — server is closed below before the request runs.
	}))
	frigateSrv.Close()

	frigateClient := frigate.NewClient(frigateSrv.URL, &http.Client{Timeout: 2 * time.Second})
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
	router := NewRouter(h, sse.NewHub(logger), logger, staticFS)

	req := httptest.NewRequest(http.MethodGet, "/api/cameras/cam1/vod/1779310000/1779310600/master.m3u8", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadGateway {
		t.Fatalf("dead upstream: want 502, got %d (body=%s)", w.Code, w.Body.String())
	}
	var body map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["error"] != "upstream_error" {
		t.Errorf("error = %q, want upstream_error", body["error"])
	}
}

func TestHandleTimelinePreview_Passthrough(t *testing.T) {
	fake := newTimelineFake()
	router := timelineRouter(t, fake)

	req := httptest.NewRequest(http.MethodGet, "/api/cameras/cam1/preview.mp4?start=1779310000&end=1779310600", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (body=%s)", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); ct != "video/mp4" {
		t.Errorf("Content-Type = %q, want video/mp4", ct)
	}
	// Frigate sends Content-Disposition: attachment; the BFF drops it so
	// the preview plays inline in a <video src>.
	if cd := w.Header().Get("Content-Disposition"); cd != "" {
		t.Errorf("Content-Disposition = %q, want it stripped (empty)", cd)
	}
	if cc := w.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", cc)
	}
	got, _ := io.ReadAll(w.Body)
	if string(got) != string(fake.previewBody) {
		t.Errorf("body = %q, want %q", got, fake.previewBody)
	}
}

func TestHandleTimelinePreview_RangeForwarded(t *testing.T) {
	fake := newTimelineFake()
	router := timelineRouter(t, fake)

	req := httptest.NewRequest(http.MethodGet, "/api/cameras/cam1/preview.mp4?start=1779310000&end=1779310600", nil)
	req.Header.Set("Range", "bytes=4-9")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusPartialContent {
		t.Fatalf("want 206, got %d (body=%s)", w.Code, w.Body.String())
	}
	fake.mu.Lock()
	gotRange := fake.gotPreviewRange
	fake.mu.Unlock()
	if gotRange != "bytes=4-9" {
		t.Errorf("upstream got Range = %q, want bytes=4-9 (verbatim forward)", gotRange)
	}
	if cr := w.Header().Get("Content-Range"); cr == "" {
		t.Errorf("Content-Range missing, want passthrough")
	} else if want := fmt.Sprintf("bytes 4-9/%d", len(fake.previewBody)); cr != want {
		t.Errorf("Content-Range = %q, want %q", cr, want)
	}
	got, _ := io.ReadAll(w.Body)
	if string(got) != string(fake.previewBody[4:10]) {
		t.Errorf("body = %q, want %q", got, fake.previewBody[4:10])
	}
}

func TestHandleTimelinePreview_InvalidStart_BadRequest(t *testing.T) {
	fake := newTimelineFake()
	router := timelineRouter(t, fake)

	req := httptest.NewRequest(http.MethodGet, "/api/cameras/cam1/preview.mp4?start=abc&end=1779310600", nil)
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
	if atomic.LoadInt32(&fake.previewHits) != 0 {
		t.Errorf("upstream hit on invalid_range: %d", fake.previewHits)
	}
}

func TestHandleTimelinePreview_EndNotAfterStart_BadRequest(t *testing.T) {
	fake := newTimelineFake()
	router := timelineRouter(t, fake)

	req := httptest.NewRequest(http.MethodGet, "/api/cameras/cam1/preview.mp4?start=1779310600&end=1779310600", nil)
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
	if atomic.LoadInt32(&fake.previewHits) != 0 {
		t.Errorf("upstream hit on end <= start: %d", fake.previewHits)
	}
}

func TestHandleTimelinePreview_UnknownCamera_NotFound(t *testing.T) {
	fake := newTimelineFake()
	router := timelineRouter(t, fake)

	req := httptest.NewRequest(http.MethodGet, "/api/cameras/camX/preview.mp4?start=1779310000&end=1779310600", nil)
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
	if atomic.LoadInt32(&fake.previewHits) != 0 {
		t.Errorf("upstream hit on unknown camera: %d", fake.previewHits)
	}
}

// previewBoundsResp mirrors the handler's previewBounds envelope for
// decoding in the frames tests.
type previewBoundsResp struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	Count int     `json:"count"`
}

func TestHandleTimelinePreviewFrames_Bounds(t *testing.T) {
	fake := newTimelineFake()
	fake.previewFramesBody = []byte(`[` +
		`"preview_cam1-1779310000.0.webp",` +
		`"preview_cam1-1779310300.5.webp",` +
		`"preview_cam1-1779310600.0.webp"` +
		`]`)
	router := timelineRouter(t, fake)

	req := httptest.NewRequest(http.MethodGet, "/api/cameras/cam1/preview-frames?start=1779310000&end=1779313600", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (body=%s)", w.Code, w.Body.String())
	}
	if cc := w.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", cc)
	}
	var got previewBoundsResp
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Start != 1779310000.0 {
		t.Errorf("start = %v, want 1779310000", got.Start)
	}
	if got.End != 1779310600.0 {
		t.Errorf("end = %v, want 1779310600", got.End)
	}
	if got.Count != 3 {
		t.Errorf("count = %d, want 3", got.Count)
	}
}

func TestHandleTimelinePreviewFrames_HyphenatedCameraID(t *testing.T) {
	// The timestamp is the substring after the LAST '-', so a hyphenated
	// camera id baked into the filename ("front-door") parses correctly.
	fake := newTimelineFake()
	fake.previewFramesBody = []byte(`[` +
		`"preview_front-door-1779310000.0.webp",` +
		`"preview_front-door-1779310600.0.webp"` +
		`]`)
	router := timelineRouter(t, fake)

	req := httptest.NewRequest(http.MethodGet, "/api/cameras/cam1/preview-frames?start=1779310000&end=1779313600", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (body=%s)", w.Code, w.Body.String())
	}
	var got previewBoundsResp
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Start != 1779310000.0 || got.End != 1779310600.0 || got.Count != 2 {
		t.Errorf("got %+v, want {1779310000 1779310600 2}", got)
	}
}

func TestHandleTimelinePreviewFrames_EmptyArray(t *testing.T) {
	fake := newTimelineFake()
	fake.previewFramesBody = []byte(`[]`)
	router := timelineRouter(t, fake)

	req := httptest.NewRequest(http.MethodGet, "/api/cameras/cam1/preview-frames?start=1779310000&end=1779313600", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (body=%s)", w.Code, w.Body.String())
	}
	var got previewBoundsResp
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Start != 0 || got.End != 0 || got.Count != 0 {
		t.Errorf("got %+v, want zero bounds with count 0", got)
	}
}

func TestHandleTimelinePreviewFrames_InvalidStart_BadRequest(t *testing.T) {
	fake := newTimelineFake()
	router := timelineRouter(t, fake)

	req := httptest.NewRequest(http.MethodGet, "/api/cameras/cam1/preview-frames?start=abc&end=1779313600", nil)
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
	if atomic.LoadInt32(&fake.previewFramesHits) != 0 {
		t.Errorf("upstream hit on invalid_range: %d", fake.previewFramesHits)
	}
}

func TestHandleTimelinePreviewFrames_EndNotAfterStart_BadRequest(t *testing.T) {
	fake := newTimelineFake()
	router := timelineRouter(t, fake)

	req := httptest.NewRequest(http.MethodGet, "/api/cameras/cam1/preview-frames?start=1779313600&end=1779313600", nil)
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
	if atomic.LoadInt32(&fake.previewFramesHits) != 0 {
		t.Errorf("upstream hit on end <= start: %d", fake.previewFramesHits)
	}
}

func TestHandleTimelinePreviewFrames_UnknownCamera_NotFound(t *testing.T) {
	fake := newTimelineFake()
	router := timelineRouter(t, fake)

	req := httptest.NewRequest(http.MethodGet, "/api/cameras/camX/preview-frames?start=1779310000&end=1779313600", nil)
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
	if atomic.LoadInt32(&fake.previewFramesHits) != 0 {
		t.Errorf("upstream hit on unknown camera: %d", fake.previewFramesHits)
	}
}

// previewClipResp mirrors the handler's previewClip envelope entries for
// decoding in the preview-clips tests.
type previewClipResp struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	Src   string  `json:"src"`
}

func TestHandleTimelinePreviewClips_RewritesSrc(t *testing.T) {
	fake := newTimelineFake()
	fake.clipsListBody = []byte(`[` +
		`{"camera":"cam1","src":"/clips/previews/cam1/1781748000.109578-1781751600.190408.mp4","type":"video/mp4","start":1781748000.109578,"end":1781751600.190408},` +
		`{"camera":"cam1","src":"/clips/previews/cam1/1781751600.190408-1781755200.271238.mp4","type":"video/mp4","start":1781751600.190408,"end":1781755200.271238}` +
		`]`)
	router := timelineRouter(t, fake)

	req := httptest.NewRequest(http.MethodGet, "/api/cameras/cam1/preview-clips?start=1781748000&end=1781755200", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (body=%s)", w.Code, w.Body.String())
	}
	if cc := w.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", cc)
	}
	var got []previewClipResp
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2 (body=%s)", len(got), w.Body.String())
	}
	if got[0].Src != "/api/cameras/cam1/preview-clip/1781748000.109578-1781751600.190408.mp4" {
		t.Errorf("src[0] = %q, want rewritten BFF route", got[0].Src)
	}
	if got[0].Start != 1781748000.109578 || got[0].End != 1781751600.190408 {
		t.Errorf("clip[0] span = {%v,%v}, want carried through", got[0].Start, got[0].End)
	}
	if got[1].Src != "/api/cameras/cam1/preview-clip/1781751600.190408-1781755200.271238.mp4" {
		t.Errorf("src[1] = %q, want rewritten BFF route", got[1].Src)
	}
}

func TestHandleTimelinePreviewClips_DropsInvalidEntry(t *testing.T) {
	fake := newTimelineFake()
	fake.clipsListBody = []byte(`[` +
		`{"camera":"cam1","src":"/clips/previews/cam1/1781748000.0-1781751600.0.mp4","type":"video/mp4","start":1781748000,"end":1781751600},` +
		`{"camera":"cam1","src":"/clips/previews/cam1/notes.txt","type":"text/plain","start":1781751600,"end":1781755200}` +
		`]`)
	router := timelineRouter(t, fake)

	req := httptest.NewRequest(http.MethodGet, "/api/cameras/cam1/preview-clips?start=1781748000&end=1781755200", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (body=%s)", w.Code, w.Body.String())
	}
	var got []previewClipResp
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1 (the .txt entry must be dropped)", len(got))
	}
	if got[0].Src != "/api/cameras/cam1/preview-clip/1781748000.0-1781751600.0.mp4" {
		t.Errorf("src = %q, want the valid rewritten entry", got[0].Src)
	}
}

func TestHandleTimelinePreviewClips_InvalidStart_BadRequest(t *testing.T) {
	fake := newTimelineFake()
	router := timelineRouter(t, fake)

	req := httptest.NewRequest(http.MethodGet, "/api/cameras/cam1/preview-clips?start=abc&end=1781755200", nil)
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
	if atomic.LoadInt32(&fake.clipsListHits) != 0 {
		t.Errorf("upstream hit on invalid_range: %d", fake.clipsListHits)
	}
}

func TestHandleTimelinePreviewClips_EndNotAfterStart_BadRequest(t *testing.T) {
	fake := newTimelineFake()
	router := timelineRouter(t, fake)

	req := httptest.NewRequest(http.MethodGet, "/api/cameras/cam1/preview-clips?start=1781755200&end=1781755200", nil)
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
	if atomic.LoadInt32(&fake.clipsListHits) != 0 {
		t.Errorf("upstream hit on end <= start: %d", fake.clipsListHits)
	}
}

func TestHandleTimelinePreviewClips_UnknownCamera_NotFound(t *testing.T) {
	fake := newTimelineFake()
	router := timelineRouter(t, fake)

	req := httptest.NewRequest(http.MethodGet, "/api/cameras/camX/preview-clips?start=1781748000&end=1781755200", nil)
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
	if atomic.LoadInt32(&fake.clipsListHits) != 0 {
		t.Errorf("upstream hit on unknown camera: %d", fake.clipsListHits)
	}
}

func TestHandleTimelinePreviewClip_Passthrough(t *testing.T) {
	fake := newTimelineFake()
	router := timelineRouter(t, fake)

	req := httptest.NewRequest(http.MethodGet, "/api/cameras/cam1/preview-clip/1781748000.0-1781751600.0.mp4", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (body=%s)", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); ct != "video/mp4" {
		t.Errorf("Content-Type = %q, want video/mp4", ct)
	}
	// Frigate sends Content-Disposition: attachment; the BFF drops it.
	if cd := w.Header().Get("Content-Disposition"); cd != "" {
		t.Errorf("Content-Disposition = %q, want it stripped (empty)", cd)
	}
	if cc := w.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", cc)
	}
	got, _ := io.ReadAll(w.Body)
	if string(got) != string(fake.clipFileBody) {
		t.Errorf("body = %q, want %q", got, fake.clipFileBody)
	}
}

func TestHandleTimelinePreviewClip_RangeForwarded(t *testing.T) {
	fake := newTimelineFake()
	router := timelineRouter(t, fake)

	req := httptest.NewRequest(http.MethodGet, "/api/cameras/cam1/preview-clip/1781748000.0-1781751600.0.mp4", nil)
	req.Header.Set("Range", "bytes=4-9")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusPartialContent {
		t.Fatalf("want 206, got %d (body=%s)", w.Code, w.Body.String())
	}
	fake.mu.Lock()
	gotRange := fake.gotClipRange
	fake.mu.Unlock()
	if gotRange != "bytes=4-9" {
		t.Errorf("upstream got Range = %q, want bytes=4-9 (verbatim forward)", gotRange)
	}
	if cr := w.Header().Get("Content-Range"); cr == "" {
		t.Errorf("Content-Range missing, want passthrough")
	} else if want := fmt.Sprintf("bytes 4-9/%d", len(fake.clipFileBody)); cr != want {
		t.Errorf("Content-Range = %q, want %q", cr, want)
	}
	got, _ := io.ReadAll(w.Body)
	if string(got) != string(fake.clipFileBody[4:10]) {
		t.Errorf("body = %q, want %q", got, fake.clipFileBody[4:10])
	}
}

func TestHandleTimelinePreviewClip_InvalidFilename_NotFound(t *testing.T) {
	// Single-segment names that reach the handler and are rejected by the
	// validPreviewClip whitelist (envelope), plus traversal/slash shapes
	// that never route to the handler at all (router 404, no envelope). All
	// must be 404 with NO upstream hit.
	cases := []struct {
		name     string
		file     string
		envelope bool
	}{
		{"wrongSuffix", "x.txt", true},
		{"lettersInStem", "ab.mp4", true},
		{"emptyStem", ".mp4", true},
		{"webpSuffix", "12-34.webp", true},
		{"parentTraversal", "../x", false},
		{"slashInside", "a/b.mp4", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := newTimelineFake()
			router := timelineRouter(t, fake)

			req := httptest.NewRequest(http.MethodGet, "/api/cameras/cam1/preview-clip/"+tc.file, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != http.StatusNotFound {
				t.Fatalf("file=%q: want 404, got %d (body=%s)", tc.file, w.Code, w.Body.String())
			}
			if atomic.LoadInt32(&fake.clipFileHits) != 0 {
				t.Errorf("file=%q: upstream was hit (%d), want zero", tc.file, fake.clipFileHits)
			}
			if tc.envelope {
				var body map[string]string
				if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
					t.Fatalf("file=%q: decode: %v", tc.file, err)
				}
				if body["error"] != "not_found" {
					t.Errorf("file=%q: error = %q, want not_found", tc.file, body["error"])
				}
			}
		})
	}
}

func TestHandleTimelinePreviewFrame_Passthrough(t *testing.T) {
	fake := newTimelineFake()
	router := timelineRouter(t, fake)

	req := httptest.NewRequest(http.MethodGet, "/api/cameras/cam1/preview-frame/preview_cam1-1781852889.819418.webp", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (body=%s)", w.Code, w.Body.String())
	}
	if atomic.LoadInt32(&fake.frameFileHits) != 1 {
		t.Errorf("upstream hits = %d, want 1", fake.frameFileHits)
	}
	if ct := w.Header().Get("Content-Type"); ct != "image/webp" {
		t.Errorf("Content-Type = %q, want image/webp", ct)
	}
	// Frigate sends Content-Disposition: attachment; the BFF drops it.
	if cd := w.Header().Get("Content-Disposition"); cd != "" {
		t.Errorf("Content-Disposition = %q, want it stripped (empty)", cd)
	}
	// A single frame is content-addressed by timestamp — immutable long-cache,
	// NOT no-store like the clip / list routes.
	if cc := w.Header().Get("Cache-Control"); cc != "public, max-age=31536000, immutable" {
		t.Errorf("Cache-Control = %q, want immutable long-cache", cc)
	}
	got, _ := io.ReadAll(w.Body)
	if string(got) != string(fake.frameFileBody) {
		t.Errorf("body = %q, want %q", got, fake.frameFileBody)
	}
}

func TestHandleTimelinePreviewFrame_InvalidFilename_NotFound(t *testing.T) {
	// Single-segment names that reach the handler and are rejected by the
	// validPreviewFrame whitelist (envelope), plus traversal/slash shapes
	// that never route to the handler at all (router 404, no envelope). All
	// must be 404 with NO upstream hit.
	cases := []struct {
		name     string
		file     string
		envelope bool
	}{
		{"wrongSuffix", "preview_x.txt", true},
		{"missingPrefix", "nope.webp", true},
		{"parentTraversal", "../x", false},
		{"slashInside", "a/b.webp", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := newTimelineFake()
			router := timelineRouter(t, fake)

			req := httptest.NewRequest(http.MethodGet, "/api/cameras/cam1/preview-frame/"+tc.file, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != http.StatusNotFound {
				t.Fatalf("file=%q: want 404, got %d (body=%s)", tc.file, w.Code, w.Body.String())
			}
			if atomic.LoadInt32(&fake.frameFileHits) != 0 {
				t.Errorf("file=%q: upstream was hit (%d), want zero", tc.file, fake.frameFileHits)
			}
			if tc.envelope {
				var body map[string]string
				if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
					t.Fatalf("file=%q: decode: %v", tc.file, err)
				}
				if body["error"] != "not_found" {
					t.Errorf("file=%q: error = %q, want not_found", tc.file, body["error"])
				}
			}
		})
	}
}

func TestHandleTimelinePreviewFrame_UnknownCamera_NotFound(t *testing.T) {
	fake := newTimelineFake()
	router := timelineRouter(t, fake)

	req := httptest.NewRequest(http.MethodGet, "/api/cameras/camX/preview-frame/preview_camX-1781852889.819418.webp", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d (body=%s)", w.Code, w.Body.String())
	}
	if atomic.LoadInt32(&fake.frameFileHits) != 0 {
		t.Errorf("upstream hit on unknown camera: %d", fake.frameFileHits)
	}
}
