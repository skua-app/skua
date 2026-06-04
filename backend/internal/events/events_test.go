package events

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// countingClipServer returns a server that records how many GET requests it
// served and a function returning that count. It writes `body` chunked (no
// Content-Length, no Accept-Ranges) — same shape as Frigate 0.17.
func countingClipServer(t *testing.T, body []byte) (*httptest.Server, func() int) {
	t.Helper()
	var hits int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&hits, 1)
		w.Header().Set("Content-Type", "video/mp4")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
	return srv, func() int { return int(atomic.LoadInt64(&hits)) }
}

// newTestClient builds a Client backed by srv with a custom-sized cache, so
// LRU and byte-cap tests don't have to materialise 16-entry / 512 MiB
// scenarios in process memory.
func newTestClient(srvURL string, cache *clipCache) *Client {
	return &Client{
		baseURL:    strings.TrimRight(srvURL, "/"),
		httpClient: &http.Client{},
		clips:      cache,
		inflight:   make(map[string]*clipInflight),
	}
}

func TestKindFor(t *testing.T) {
	cases := map[string]Kind{
		"person":     KindPerson,
		"car":        KindVehicle,
		"truck":      KindVehicle,
		"motorcycle": KindVehicle,
		"bicycle":    KindVehicle,
		"dog":        KindAnimal,
		"cat":        KindAnimal,
		"bird":       KindAnimal,
		"motion":     KindOther,
		"":           KindOther,
		"xyz":        KindOther,
	}
	for label, want := range cases {
		if got := KindFor(label); got != want {
			t.Errorf("KindFor(%q) = %s, want %s", label, got, want)
		}
	}
}

func TestToItem_ScorePrecedence(t *testing.T) {
	score := 0.74
	dataScore := 0.55
	topScore := 0.99
	cases := []struct {
		name string
		in   FrigateEvent
		want float64
	}{
		{
			name: "data.top_score wins",
			in: FrigateEvent{
				StartTime: 1779310000,
				Label:     "person",
				Data: struct {
					TopScore *float64 `json:"top_score"`
					Score    *float64 `json:"score"`
				}{TopScore: &score, Score: &dataScore},
				TopScore: &topScore,
			},
			want: 0.74,
		},
		{
			name: "data.score fallback when data.top_score nil",
			in: FrigateEvent{
				StartTime: 1779310000,
				Label:     "person",
				Data: struct {
					TopScore *float64 `json:"top_score"`
					Score    *float64 `json:"score"`
				}{TopScore: nil, Score: &dataScore},
			},
			want: 0.55,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ToItem(c.in)
			if got.Score == nil || *got.Score != c.want {
				t.Errorf("score = %v, want %v", got.Score, c.want)
			}
		})
	}
}

func TestToItem_Duration(t *testing.T) {
	end := 1779310010.5
	item := ToItem(FrigateEvent{
		StartTime: 1779310000.0,
		EndTime:   &end,
		Label:     "person",
	})
	if item.DurationSeconds == nil {
		t.Fatal("expected duration_seconds set")
	}
	if *item.DurationSeconds != 10 {
		t.Errorf("duration_seconds = %d, want 10", *item.DurationSeconds)
	}
	if item.EndedAt == nil {
		t.Fatal("expected ended_at set")
	}
}

func TestToItem_NilEnd(t *testing.T) {
	item := ToItem(FrigateEvent{StartTime: 1779310000, Label: "person"})
	if item.EndedAt != nil || item.DurationSeconds != nil {
		t.Errorf("expected nil ended_at and duration_seconds for in-progress event")
	}
}

func TestList_BuildsParams(t *testing.T) {
	var gotQuery url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("[]"))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, nil)
	_, err := c.List(context.Background(), ListParams{
		Cameras: []string{"cam5", "cam7"},
		Labels:  []string{"person", "car"},
		Before:  time.Unix(1779000000, 0),
		Limit:   25,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if gotQuery.Get("cameras") != "cam5,cam7" {
		t.Errorf("cameras = %q", gotQuery.Get("cameras"))
	}
	if gotQuery.Get("labels") != "person,car" {
		t.Errorf("labels = %q", gotQuery.Get("labels"))
	}
	if gotQuery.Get("before") != "1779000000" {
		t.Errorf("before = %q (must be unix seconds integer)", gotQuery.Get("before"))
	}
	if gotQuery.Get("limit") != "25" {
		t.Errorf("limit = %q", gotQuery.Get("limit"))
	}
	if gotQuery.Get("has_snapshot") != "1" {
		t.Errorf("has_snapshot must always be 1; got %q", gotQuery.Get("has_snapshot"))
	}
}

func TestList_CursorOnFullPage(t *testing.T) {
	page := []FrigateEvent{
		{ID: "1", Camera: "cam7", Label: "person", StartTime: 1779310000, HasSnapshot: true},
		{ID: "2", Camera: "cam7", Label: "person", StartTime: 1779309900, HasSnapshot: true},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(page)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, nil)
	resp, err := c.List(context.Background(), ListParams{Limit: 2})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if resp.NextBefore == nil {
		t.Fatal("expected next_before when page is full")
	}
	if *resp.NextBefore != resp.Items[1].StartedAt {
		t.Errorf("next_before = %q, want %q", *resp.NextBefore, resp.Items[1].StartedAt)
	}
}

// fakeFrigateClipServer returns an httptest.Server that mimics Frigate 0.17's
// /api/events/<id>/clip.mp4 endpoint: it writes the given body with a chunked
// Transfer-Encoding (Content-Length not set, no Accept-Ranges). The point of
// the buffering BFF is exactly to absorb this shape; once the BFF has read
// the body, callers see normal Range behaviour regardless of upstream.
func fakeFrigateClipServer(t *testing.T, gotPath *string, body []byte) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if gotPath != nil {
			*gotPath = r.URL.Path
		}
		w.Header().Set("Content-Type", "video/mp4")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
}

func TestServeClip_InlineWithRange(t *testing.T) {
	var gotPath string
	body := []byte("fake-mp4-bytes-with-some-length-for-range")
	srv := fakeFrigateClipServer(t, &gotPath, body)
	defer srv.Close()

	c := NewClient(srv.URL, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/events/evt-1/clip.mp4", nil)
	rec := httptest.NewRecorder()
	if err := c.ServeClip(context.Background(), "evt-1", rec, req, ""); err != nil {
		t.Fatalf("ServeClip: %v", err)
	}

	if gotPath != "/api/events/evt-1/clip.mp4" {
		t.Errorf("upstream path = %q, want /api/events/evt-1/clip.mp4", gotPath)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "video/mp4" {
		t.Errorf("Content-Type = %q, want video/mp4", got)
	}
	if got := rec.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Errorf("Cache-Control = %q", got)
	}
	if got := rec.Header().Get("Content-Disposition"); got != "inline" {
		t.Errorf("Content-Disposition = %q, want inline", got)
	}
	if got := rec.Header().Get("Accept-Ranges"); got != "bytes" {
		t.Errorf("Accept-Ranges = %q, want bytes (http.ServeContent adds it)", got)
	}
	if got := rec.Header().Get("ETag"); got != `"evt-1"` {
		t.Errorf("ETag = %q, want \"evt-1\"", got)
	}
	if got := rec.Header().Get("Content-Length"); got != "41" {
		t.Errorf("Content-Length = %q, want 41 (len(body))", got)
	}
	if !bytes.Equal(rec.Body.Bytes(), body) {
		t.Errorf("body mismatch: got %q, want %q", rec.Body.Bytes(), body)
	}
}

func TestServeClip_DownloadDisposition(t *testing.T) {
	srv := fakeFrigateClipServer(t, nil, []byte("body"))
	defer srv.Close()

	c := NewClient(srv.URL, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/events/evt-2/clip.mp4?download=1", nil)
	rec := httptest.NewRecorder()
	if err := c.ServeClip(context.Background(), "evt-2", rec, req, "frigate-evt-2.mp4"); err != nil {
		t.Fatalf("ServeClip: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	got := rec.Header().Get("Content-Disposition")
	want := `attachment; filename="frigate-evt-2.mp4"`
	if got != want {
		t.Errorf("Content-Disposition = %q, want %q", got, want)
	}
}

func TestServeClip_RangeRequest(t *testing.T) {
	body := []byte("0123456789abcdef")
	srv := fakeFrigateClipServer(t, nil, body)
	defer srv.Close()

	c := NewClient(srv.URL, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/events/evt-r/clip.mp4", nil)
	req.Header.Set("Range", "bytes=0-1")
	rec := httptest.NewRecorder()
	if err := c.ServeClip(context.Background(), "evt-r", rec, req, ""); err != nil {
		t.Fatalf("ServeClip: %v", err)
	}

	if rec.Code != http.StatusPartialContent {
		t.Errorf("status = %d, want 206 Partial Content", rec.Code)
	}
	if got := rec.Header().Get("Content-Range"); got != "bytes 0-1/16" {
		t.Errorf("Content-Range = %q, want bytes 0-1/16", got)
	}
	if got := rec.Header().Get("Content-Length"); got != "2" {
		t.Errorf("Content-Length = %q, want 2", got)
	}
	if rec.Body.String() != "01" {
		t.Errorf("body = %q, want %q", rec.Body.String(), "01")
	}
}

func TestServeClip_UpstreamNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/events/missing/clip.mp4", nil)
	rec := httptest.NewRecorder()
	err := c.ServeClip(context.Background(), "missing", rec, req, "")
	if err == nil {
		t.Fatal("expected error for upstream 404")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("error should mention upstream status: %v", err)
	}
	if rec.Body.Len() != 0 {
		t.Errorf("body should be empty on error, got %d bytes", rec.Body.Len())
	}
}

func TestServeClip_BodyOverLimit(t *testing.T) {
	// Stream slightly over the cap so we don't allocate a real 64 MiB buffer
	// in the test process. The fake handler writes in 1 MiB chunks until the
	// client either disconnects or the body cap is exceeded.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "video/mp4")
		w.WriteHeader(http.StatusOK)
		chunk := make([]byte, 1<<20)
		for i := 0; i < int(clipMaxBytes>>20)+2; i++ {
			if _, err := w.Write(chunk); err != nil {
				return
			}
		}
	}))
	defer srv.Close()

	c := NewClient(srv.URL, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/events/big/clip.mp4", nil)
	rec := httptest.NewRecorder()
	err := c.ServeClip(context.Background(), "big", rec, req, "")
	if err == nil {
		t.Fatal("expected error when upstream body exceeds limit")
	}
	if !strings.Contains(err.Error(), "limit") {
		t.Errorf("error should mention size limit: %v", err)
	}
	if rec.Body.Len() != 0 {
		t.Errorf("body should be empty on error, got %d bytes", rec.Body.Len())
	}
}

func TestServeClip_CacheHitSkipsUpstream(t *testing.T) {
	body := []byte("buffered-clip-bytes-for-cache")
	srv, hits := countingClipServer(t, body)
	defer srv.Close()

	c := NewClient(srv.URL, nil)
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/events/cache-1/clip.mp4", nil)
		rec := httptest.NewRecorder()
		if err := c.ServeClip(context.Background(), "cache-1", rec, req, ""); err != nil {
			t.Fatalf("iter %d ServeClip: %v", i, err)
		}
		if rec.Code != http.StatusOK {
			t.Fatalf("iter %d status = %d, want 200", i, rec.Code)
		}
		if !bytes.Equal(rec.Body.Bytes(), body) {
			t.Fatalf("iter %d body mismatch", i)
		}
	}
	if got := hits(); got != 1 {
		t.Errorf("upstream hits = %d, want 1 (subsequent calls must come from cache)", got)
	}
}

func TestServeClip_CacheMissOnDifferentID(t *testing.T) {
	body := []byte("buffered-clip-bytes")
	srv, hits := countingClipServer(t, body)
	defer srv.Close()

	c := NewClient(srv.URL, nil)
	for _, id := range []string{"a", "b", "c"} {
		req := httptest.NewRequest(http.MethodGet, "/api/events/"+id+"/clip.mp4", nil)
		rec := httptest.NewRecorder()
		if err := c.ServeClip(context.Background(), id, rec, req, ""); err != nil {
			t.Fatalf("ServeClip %s: %v", id, err)
		}
	}
	if got := hits(); got != 3 {
		t.Errorf("upstream hits = %d, want 3 (each new id triggers upstream)", got)
	}
}

func TestServeClip_LRUEvictionByCount(t *testing.T) {
	body := []byte("x") // 1 byte, byte cap won't fire
	srv, hits := countingClipServer(t, body)
	defer srv.Close()

	// Cap entries at 2 so the third Put evicts "a".
	c := newTestClient(srv.URL, newClipCache(2, 1<<20))

	for _, id := range []string{"a", "b", "c"} {
		req := httptest.NewRequest(http.MethodGet, "/api/events/"+id+"/clip.mp4", nil)
		rec := httptest.NewRecorder()
		if err := c.ServeClip(context.Background(), id, rec, req, ""); err != nil {
			t.Fatalf("ServeClip %s: %v", id, err)
		}
	}
	if got := hits(); got != 3 {
		t.Fatalf("initial hits = %d, want 3", got)
	}
	if got := c.clips.len(); got != 2 {
		t.Fatalf("cache len = %d, want 2 after eviction", got)
	}

	// "a" should have been evicted as LRU → re-request hits upstream again.
	req := httptest.NewRequest(http.MethodGet, "/api/events/a/clip.mp4", nil)
	rec := httptest.NewRecorder()
	if err := c.ServeClip(context.Background(), "a", rec, req, ""); err != nil {
		t.Fatalf("ServeClip a: %v", err)
	}
	if got := hits(); got != 4 {
		t.Errorf("re-fetch hits = %d, want 4 (a was evicted)", got)
	}

	// "c" should still be cached → no extra upstream call.
	req = httptest.NewRequest(http.MethodGet, "/api/events/c/clip.mp4", nil)
	rec = httptest.NewRecorder()
	if err := c.ServeClip(context.Background(), "c", rec, req, ""); err != nil {
		t.Fatalf("ServeClip c: %v", err)
	}
	if got := hits(); got != 4 {
		t.Errorf("c cache hit hits = %d, want 4 (c still cached)", got)
	}
}

func TestServeClip_EvictionByBytes(t *testing.T) {
	body := make([]byte, 64)
	for i := range body {
		body[i] = 'a'
	}
	srv, hits := countingClipServer(t, body)
	defer srv.Close()

	// Byte cap fits exactly one 64-byte entry. Inserting a second triggers
	// eviction of the first.
	c := newTestClient(srv.URL, newClipCache(16, 64))

	for _, id := range []string{"one", "two"} {
		req := httptest.NewRequest(http.MethodGet, "/api/events/"+id+"/clip.mp4", nil)
		rec := httptest.NewRecorder()
		if err := c.ServeClip(context.Background(), id, rec, req, ""); err != nil {
			t.Fatalf("ServeClip %s: %v", id, err)
		}
	}
	if got := c.clips.len(); got != 1 {
		t.Fatalf("cache len = %d, want 1 (byte cap evicts older entry)", got)
	}

	// "one" was evicted by byte cap → re-request hits upstream.
	req := httptest.NewRequest(http.MethodGet, "/api/events/one/clip.mp4", nil)
	rec := httptest.NewRecorder()
	if err := c.ServeClip(context.Background(), "one", rec, req, ""); err != nil {
		t.Fatalf("ServeClip one: %v", err)
	}
	if got := hits(); got != 3 {
		t.Errorf("hits = %d, want 3 (one evicted, refetched)", got)
	}
}

func TestServeClip_OversizeEntryNotCached(t *testing.T) {
	// 100-byte body, cache cap 32 → single entry exceeds cap, must be served
	// but not cached.
	body := make([]byte, 100)
	srv, hits := countingClipServer(t, body)
	defer srv.Close()

	c := newTestClient(srv.URL, newClipCache(16, 32))

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/events/big/clip.mp4", nil)
		rec := httptest.NewRecorder()
		if err := c.ServeClip(context.Background(), "big", rec, req, ""); err != nil {
			t.Fatalf("iter %d ServeClip: %v", i, err)
		}
		if rec.Code != http.StatusOK {
			t.Fatalf("iter %d status = %d", i, rec.Code)
		}
	}
	if got := c.clips.len(); got != 0 {
		t.Errorf("cache len = %d, want 0 (entry exceeded cache cap)", got)
	}
	if got := hits(); got != 2 {
		t.Errorf("hits = %d, want 2 (no caching → each call fetches upstream)", got)
	}
}

func TestServeClip_HEADReusesCachedBytes(t *testing.T) {
	body := []byte("head-method-payload-must-be-cached-too")
	srv, hits := countingClipServer(t, body)
	defer srv.Close()

	c := NewClient(srv.URL, nil)

	headReq := httptest.NewRequest(http.MethodHead, "/api/events/h1/clip.mp4", nil)
	headRec := httptest.NewRecorder()
	if err := c.ServeClip(context.Background(), "h1", headRec, headReq, ""); err != nil {
		t.Fatalf("HEAD ServeClip: %v", err)
	}
	if headRec.Code != http.StatusOK {
		t.Fatalf("HEAD status = %d, want 200", headRec.Code)
	}
	if headRec.Body.Len() != 0 {
		t.Errorf("HEAD body length = %d, want 0", headRec.Body.Len())
	}
	wantLen := strconv.Itoa(len(body))
	if got := headRec.Header().Get("Content-Length"); got != wantLen {
		t.Errorf("HEAD Content-Length = %q, want %q", got, wantLen)
	}
	if got := hits(); got != 1 {
		t.Fatalf("hits after HEAD = %d, want 1", got)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/events/h1/clip.mp4", nil)
	getRec := httptest.NewRecorder()
	if err := c.ServeClip(context.Background(), "h1", getRec, getReq, ""); err != nil {
		t.Fatalf("GET ServeClip: %v", err)
	}
	if !bytes.Equal(getRec.Body.Bytes(), body) {
		t.Errorf("GET body mismatch after HEAD primed cache")
	}
	if got := hits(); got != 1 {
		t.Errorf("hits after follow-up GET = %d, want 1 (cache reuse across methods)", got)
	}
}

// TestServeClip_SingleFlightCollapsesColdMiss fires N concurrent ServeClip
// calls for the same fresh event id; the upstream handler blocks on a
// release channel so every goroutine reaches the cold-miss path before any
// fetch can complete. The single-flight layer must collapse the stampede
// into exactly one upstream request whose bytes are then shared with every
// waiter.
func TestServeClip_SingleFlightCollapsesColdMiss(t *testing.T) {
	const n = 20
	var hits int64
	release := make(chan struct{})
	arrived := make(chan struct{}, 1)
	body := []byte("buffered-clip-bytes-for-singleflight-test")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt64(&hits, 1)
		select {
		case arrived <- struct{}{}:
		default:
		}
		<-release
		w.Header().Set("Content-Type", "video/mp4")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, nil)

	type result struct {
		body []byte
		err  error
	}
	results := make(chan result, n)
	var startWG sync.WaitGroup
	startWG.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			startWG.Done()
			req := httptest.NewRequest(http.MethodGet, "/api/events/sf-1/clip.mp4", nil)
			rec := httptest.NewRecorder()
			err := c.ServeClip(context.Background(), "sf-1", rec, req, "")
			results <- result{body: rec.Body.Bytes(), err: err}
		}()
	}
	startWG.Wait()

	select {
	case <-arrived:
	case <-time.After(2 * time.Second):
		t.Fatal("upstream never received the cold-miss fetch")
	}
	close(release)

	for i := 0; i < n; i++ {
		select {
		case r := <-results:
			if r.err != nil {
				t.Fatalf("call %d: %v", i, r.err)
			}
			if !bytes.Equal(r.body, body) {
				t.Errorf("call %d: body mismatch (len=%d want=%d)", i, len(r.body), len(body))
			}
		case <-time.After(2 * time.Second):
			t.Fatal("waiter never returned")
		}
	}
	if got := atomic.LoadInt64(&hits); got != 1 {
		t.Errorf("upstream hits = %d, want 1 (single-flight should collapse cold misses)", got)
	}
}

// TestServeClip_SingleFlightSurvivesFirstCallerCancel asserts that
// cancelling the request that triggered the shared fetch does not poison
// sibling waiters: the second caller, joined while the upstream was still
// blocked, must still receive the bytes once the upstream unblocks. iOS
// frequently cancels probe Range requests within a few hundred ms; if the
// first one tore down the shared fetch, every other Range would also fail.
func TestServeClip_SingleFlightSurvivesFirstCallerCancel(t *testing.T) {
	var hits int64
	release := make(chan struct{})
	arrived := make(chan struct{}, 1)
	body := []byte("survived-cancel-payload")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt64(&hits, 1)
		select {
		case arrived <- struct{}{}:
		default:
		}
		<-release
		w.Header().Set("Content-Type", "video/mp4")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, nil)

	type result struct {
		body []byte
		err  error
	}

	ctx1, cancel1 := context.WithCancel(context.Background())
	first := make(chan result, 1)
	go func() {
		req := httptest.NewRequest(http.MethodGet, "/api/events/sf-cancel/clip.mp4", nil)
		rec := httptest.NewRecorder()
		err := c.ServeClip(ctx1, "sf-cancel", rec, req, "")
		first <- result{body: rec.Body.Bytes(), err: err}
	}()

	// Wait until the upstream handler has actually been invoked; the
	// inflight entry is in place by the time the goroutine got far enough
	// to issue the HTTP request.
	select {
	case <-arrived:
	case <-time.After(2 * time.Second):
		t.Fatal("first caller never reached upstream")
	}

	second := make(chan result, 1)
	go func() {
		req := httptest.NewRequest(http.MethodGet, "/api/events/sf-cancel/clip.mp4", nil)
		rec := httptest.NewRecorder()
		err := c.ServeClip(context.Background(), "sf-cancel", rec, req, "")
		second <- result{body: rec.Body.Bytes(), err: err}
	}()

	cancel1()
	select {
	case r := <-first:
		if !errors.Is(r.err, context.Canceled) {
			t.Errorf("first caller err = %v, want context.Canceled", r.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("first caller did not return after cancel")
	}

	close(release)
	select {
	case r := <-second:
		if r.err != nil {
			t.Fatalf("second caller err = %v", r.err)
		}
		if !bytes.Equal(r.body, body) {
			t.Errorf("second caller body mismatch (len=%d want=%d)", len(r.body), len(body))
		}
	case <-time.After(2 * time.Second):
		t.Fatal("second caller never received bytes")
	}
	if got := atomic.LoadInt64(&hits); got != 1 {
		t.Errorf("upstream hits = %d, want 1 (shared fetch must survive first-caller cancel)", got)
	}
}

// TestClipCache_ConcurrentPutGetRace fires many concurrent writers and
// readers at a clipCache configured with small caps so evictLocked runs
// continuously under contention. The race detector then sees the map +
// container/list mutations under c.mu interleaved with Get's MoveToFront
// (also under c.mu). The test passes if -race finds nothing, no goroutine
// panics on a corrupted list element, and the entry count stays at or
// below the configured cap after quiesce.
func TestClipCache_ConcurrentPutGetRace(t *testing.T) {
	const (
		maxEntries = 4
		maxBytes   = 4096
		writers    = 6
		readers    = 6
	)
	c := newClipCache(maxEntries, maxBytes)

	deadline := time.Now().Add(200 * time.Millisecond)
	ids := []string{"a", "b", "c", "d", "e", "f", "g", "h"}
	payload := bytes.Repeat([]byte("x"), 512)

	var wg sync.WaitGroup

	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func(seed int) {
			defer wg.Done()
			for i := 0; time.Now().Before(deadline); i++ {
				id := ids[(seed+i)%len(ids)]
				c.Put(id, payload, time.Time{})
			}
		}(w)
	}

	for r := 0; r < readers; r++ {
		wg.Add(1)
		go func(seed int) {
			defer wg.Done()
			for i := 0; time.Now().Before(deadline); i++ {
				_, _, _ = c.Get(ids[(seed+i)%len(ids)])
			}
		}(r)
	}

	wg.Wait()

	if got := c.len(); got > maxEntries {
		t.Errorf("len() = %d, want <= %d after concurrent churn", got, maxEntries)
	}
}

func TestList_NoCursorOnShortPage(t *testing.T) {
	page := []FrigateEvent{{ID: "1", Camera: "cam7", Label: "person", StartTime: 1779310000}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(page)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, nil)
	resp, err := c.List(context.Background(), ListParams{Limit: 5})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if resp.NextBefore != nil {
		t.Errorf("expected nil next_before on short page, got %v", *resp.NextBefore)
	}
}
