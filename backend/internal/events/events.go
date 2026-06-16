// Package events talks to Frigate's /api/events endpoint and translates the
// raw Frigate JSON into the BFF's stable EventItem shape.
//
// Pre-flight notes on Frigate 0.17 event JSON (probed 2026-05-20):
//   - id           — string like "<unix_seconds>-<hash>"
//   - camera       — string
//   - label        — string (raw detector label: person, car, dog, motion, ...)
//   - sub_label    — string|null
//   - start_time   — float64, unix seconds with subseconds
//   - end_time     — null while active, else float64 unix seconds
//   - has_snapshot — bool
//   - has_clip     — bool
//   - top_score    — ALWAYS null at top level in 0.17; real score lives in
//     data.top_score (fallback data.score). We map both.
//
// Frigate /api/events query params (these are what we send upstream):
//   - cameras      — comma-separated camera IDs
//   - labels       — comma-separated labels
//   - before       — unix seconds (INTEGER), not ISO
//   - limit        — integer
//   - has_snapshot — we always pass "1" to skip events without snapshots
package events

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// defaultClipMaxBytes is the fallback per-clip in-memory buffer cap when the
// caller passes a non-positive value to NewClient. Frigate's default 30-second
// clip at 3500kbps is ~13 MiB; 256 MiB covers long high-bitrate HEVC moments
// while still bounding RAM growth under cache pressure. Configurable per
// installation via CLIP_MAX_MIB; see config.Config.ClipMaxBytes.
const defaultClipMaxBytes int64 = 256 << 20

// Kind is the normalised category the BFF exposes (Frigate's raw labels
// vary by detector; the UI groups them into a stable, small set).
type Kind string

const (
	KindPerson  Kind = "person"
	KindVehicle Kind = "vehicle"
	KindAnimal  Kind = "animal"
	KindOther   Kind = "other"
)

// labelKind maps Frigate's raw detector labels to our normalised Kind.
// Anything not present here resolves to KindOther.
var labelKind = map[string]Kind{
	"person":     KindPerson,
	"car":        KindVehicle,
	"truck":      KindVehicle,
	"bus":        KindVehicle,
	"motorcycle": KindVehicle,
	"bicycle":    KindVehicle,
	"dog":        KindAnimal,
	"cat":        KindAnimal,
	"bird":       KindAnimal,
}

// KindFor returns the BFF Kind for a raw Frigate label.
func KindFor(label string) Kind {
	if k, ok := labelKind[label]; ok {
		return k
	}
	return KindOther
}

// Item is the BFF's stable EventItem shape; see docs/api-contract.md.
type Item struct {
	ID              string   `json:"id"`
	CamID           string   `json:"cam_id"`
	StartedAt       string   `json:"started_at"` // RFC3339 UTC
	EndedAt         *string  `json:"ended_at"`   // nil while in progress
	DurationSeconds *int64   `json:"duration_seconds"`
	Label           string   `json:"label"`
	Kind            Kind     `json:"kind"`
	Score           *float64 `json:"score"`
	HasSnapshot     bool     `json:"has_snapshot"`
	HasClip         bool     `json:"has_clip"`
}

// ListResponse is the JSON envelope for GET /api/events.
type ListResponse struct {
	Items      []Item  `json:"items"`
	NextBefore *string `json:"next_before"`
}

// ListParams are the supported BFF query filters.
type ListParams struct {
	Cameras []string
	Labels  []string
	Before  time.Time // zero ⇒ no filter
	Limit   int
}

// FrigateEvent matches the subset of fields we read from Frigate's
// /api/events response items AND from the /ws "events" topic payload.
type FrigateEvent struct {
	ID          string   `json:"id"`
	Camera      string   `json:"camera"`
	Label       string   `json:"label"`
	SubLabel    *string  `json:"sub_label"`
	StartTime   float64  `json:"start_time"`
	EndTime     *float64 `json:"end_time"`
	HasSnapshot bool     `json:"has_snapshot"`
	HasClip     bool     `json:"has_clip"`
	// In Frigate 0.17 the top-level `top_score` is always null; the real
	// score lives in `data.top_score` (fallback `data.score`).
	TopScore *float64 `json:"top_score"`
	Data     struct {
		TopScore *float64 `json:"top_score"`
		Score    *float64 `json:"score"`
	} `json:"data"`
}

// ToItem normalises a raw Frigate event into the BFF Item shape.
func ToItem(e FrigateEvent) Item {
	startedAt := timeFromUnix(e.StartTime)
	item := Item{
		ID:          e.ID,
		CamID:       e.Camera,
		StartedAt:   startedAt.UTC().Format(time.RFC3339),
		Label:       e.Label,
		Kind:        KindFor(e.Label),
		Score:       firstNonNil(e.Data.TopScore, e.Data.Score, e.TopScore),
		HasSnapshot: e.HasSnapshot,
		HasClip:     e.HasClip,
	}
	if e.EndTime != nil {
		ended := timeFromUnix(*e.EndTime).UTC().Format(time.RFC3339)
		item.EndedAt = &ended
		dur := int64(*e.EndTime - e.StartTime)
		if dur < 0 {
			dur = 0
		}
		item.DurationSeconds = &dur
	}
	return item
}

func timeFromUnix(secs float64) time.Time {
	whole := int64(secs)
	frac := int64((secs - float64(whole)) * 1e9)
	return time.Unix(whole, frac)
}

func firstNonNil(values ...*float64) *float64 {
	for _, v := range values {
		if v != nil {
			return v
		}
	}
	return nil
}

// Client talks to Frigate's /api/events endpoints.
type Client struct {
	baseURL      string
	httpClient   *http.Client
	clips        *clipCache
	maxClipBytes int64

	// inflight collapses concurrent cold misses for the same event id into
	// one upstream fetch. Keyed by event id; the value's done channel is
	// closed once the shared fetch completes. See fetchClipShared.
	inflightMu sync.Mutex
	inflight   map[string]*clipInflight
}

// clipInflight is the shared state for one in-progress upstream fetch.
// buf/err are written exactly once, before done is closed; subsequent
// readers may safely read them without holding any lock.
type clipInflight struct {
	done chan struct{}
	buf  []byte
	err  error
}

// NewClient builds a Client. maxClipBytes caps the in-memory buffer size for
// a per-event clip; pass a non-positive value to fall back to
// defaultClipMaxBytes. The clip cache's byte cap is widened to the per-clip
// cap when needed so a single max-size clip still fits.
func NewClient(baseURL string, httpClient *http.Client, maxClipBytes int64) *Client {
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	if maxClipBytes <= 0 {
		maxClipBytes = defaultClipMaxBytes
	}
	cacheBytes := int64(clipCacheMaxBytes)
	if maxClipBytes > cacheBytes {
		cacheBytes = maxClipBytes
	}
	return &Client{
		baseURL:      strings.TrimRight(baseURL, "/"),
		httpClient:   httpClient,
		clips:        newClipCache(clipCacheMaxEntries, cacheBytes),
		maxClipBytes: maxClipBytes,
		inflight:     make(map[string]*clipInflight),
	}
}

// List fetches events from Frigate, normalises them, and computes the
// next_before cursor (start_time of the last returned item, or nil if the
// response was short — meaning we're at the end of history).
func (c *Client) List(ctx context.Context, p ListParams) (ListResponse, error) {
	q := url.Values{}
	q.Set("has_snapshot", "1")
	if len(p.Cameras) > 0 {
		q.Set("cameras", strings.Join(p.Cameras, ","))
	}
	if len(p.Labels) > 0 {
		q.Set("labels", strings.Join(p.Labels, ","))
	}
	if !p.Before.IsZero() {
		q.Set("before", strconv.FormatInt(p.Before.Unix(), 10))
	}
	if p.Limit > 0 {
		q.Set("limit", strconv.Itoa(p.Limit))
	}

	reqURL := c.baseURL + "/api/events?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return ListResponse{}, fmt.Errorf("build request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return ListResponse{}, fmt.Errorf("frigate events: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return ListResponse{}, fmt.Errorf("frigate events returned %d", resp.StatusCode)
	}

	var raw []FrigateEvent
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return ListResponse{}, fmt.Errorf("decode events: %w", err)
	}

	items := make([]Item, len(raw))
	for i, e := range raw {
		items[i] = ToItem(e)
	}

	out := ListResponse{Items: items}
	// Cursor: only emit when the page is full (otherwise we're at the end).
	if p.Limit > 0 && len(items) == p.Limit && len(items) > 0 {
		last := items[len(items)-1].StartedAt
		out.NextBefore = &last
	}
	return out, nil
}

// ServeClip serves the per-event MP4 clip for eventID via the buffered
// clip pipeline. Thin wrapper over serveClip that pins the upstream
// URL to Frigate's per-event /api/events/{id}/clip.mp4 and reuses the
// event id as both the cache/single-flight key and the ETag validator.
//
// Why buffer instead of reverse-proxy: iOS Safari's <video> element issues a
// Range request on mount and aborts with a broken-video icon if the server
// does not reply 206 / Content-Length / Accept-Ranges. Frigate 0.17 responds
// with Transfer-Encoding: chunked and ignores Range; the previous reverse-
// proxy forwarded that verbatim, which broke inline playback on iOS. Reading
// the body into memory flattens the chunked stream into a known-length byte
// slice that http.ServeContent can range-serve cleanly.
//
// Why cache: <video> typically issues 10–20 parallel Range subrequests per
// source, and a fresh upstream fetch per range (~35 MiB read each) saturated
// the BFF and caused individual ranges to time out or be cancelled, breaking
// playback. We cache the buffered bytes by key so concurrent and repeat
// ranges share one upstream fetch. Event clips are immutable in Frigate
// (write-once), so no TTL — eviction is LRU under count and byte caps. See
// clipcache.go.
//
// On non-2xx upstream, oversized body, or transport failure, ServeClip
// returns an error and writes nothing to w. Callers translate the error to
// writeError (502 by default, 504 if their context deadline expired). When
// downloadName is non-empty the response is served as an attachment with
// that filename; otherwise inline. Cache hits never error.
func (c *Client) ServeClip(ctx context.Context, eventID string, w http.ResponseWriter, r *http.Request, downloadName string) error {
	upstreamURL := fmt.Sprintf("%s/api/events/%s/clip.mp4", c.baseURL, eventID)
	return c.serveClip(ctx, eventID, upstreamURL, w, r, downloadName)
}

// ServeMomentClip serves Frigate's full-resolution time-range clip for
// the [start, end] window (unix seconds, with subseconds) on the given
// camera through the same buffered + hev1→hvc1 retag pipeline as
// ServeClip. key is the cache / single-flight / ETag validator; the
// glance preview handler passes the Frigate review id so successive
// requests against the same moment hit the same cached bytes.
//
// Unlike per-event clips, a moment clip's source segment may still be
// growing when the BFF first caches it. Because the cache is content-
// addressed by `key` and gets a long immutable Cache-Control, a still-
// active review served before its segment finalises can serve a
// slightly-short cached window until LRU evicts the entry — acceptable
// for the household glance UI.
func (c *Client) ServeMomentClip(ctx context.Context, key, camera string, start, end float64, w http.ResponseWriter, r *http.Request, downloadName string) error {
	upstreamURL := c.baseURL + "/api/" + url.PathEscape(camera) +
		"/start/" + strconv.FormatFloat(start, 'f', 6, 64) +
		"/end/" + strconv.FormatFloat(end, 'f', 6, 64) +
		"/clip.mp4"
	return c.serveClip(ctx, key, upstreamURL, w, r, downloadName)
}

// serveClip is the shared clip-serving pipeline behind ServeClip and
// ServeMomentClip. `key` is the cache / single-flight / ETag validator;
// `upstreamURL` is the Frigate endpoint to fetch on a cold miss. The
// ETag and the immutable Cache-Control are correct only for sources
// that don't change once observed — true for per-event clips and
// acceptable for finalised review windows (see ServeMomentClip).
func (c *Client) serveClip(ctx context.Context, key, upstreamURL string, w http.ResponseWriter, r *http.Request, downloadName string) error {
	buf, modTime, ok := c.clips.Get(key)
	if !ok {
		fetched, err := c.fetchClipShared(ctx, key, upstreamURL)
		if err != nil {
			return err
		}
		buf = fetched
		modTime = time.Time{}
	}

	// Set Content-Type explicitly: http.ServeContent's sniffing relies on the
	// name argument, and we pass "" because the request path has no .mp4
	// segment when this handler is reached via chi.
	w.Header().Set("Content-Type", "video/mp4")
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("ETag", `"`+key+`"`)
	if downloadName != "" {
		w.Header().Set("Content-Disposition", `attachment; filename="`+downloadName+`"`)
	} else {
		w.Header().Set("Content-Disposition", "inline")
	}
	// Clear the server's WriteTimeout for this response: a legitimate slow
	// download of a clip up to the configured per-clip cap can outlive the
	// default HTTPTimeout*2 window, and being killed mid-stream would truncate the
	// file. Same pattern used by the SSE handler. Best-effort: some
	// ResponseWriters don't expose deadlines via the controller, in which
	// case the error is ignored (events.Client has no logger field; adding
	// one just for a degradation log isn't worth the dependency).
	_ = http.NewResponseController(w).SetWriteDeadline(time.Time{})
	// Zero modtime makes http.ServeContent skip If-Modified-Since handling
	// and rely on the ETag we set above.
	http.ServeContent(w, r, "", modTime, bytes.NewReader(buf))
	return nil
}

// fetchClipShared single-flights concurrent cold misses for the same
// cache key. The first caller spawns the upstream fetch on a context
// decoupled from any specific request's cancellation, so a probe Range
// request that gives up mid-fetch (iOS issues 10–20 parallel ranges and
// frequently cancels the first few) does not poison the sibling
// waiters. All callers block on the shared done channel; the resulting
// bytes + cache Put happen exactly once before the inflight entry is
// released. Errors are shared across waiters but are not cached — a
// subsequent cold miss retries.
func (c *Client) fetchClipShared(ctx context.Context, key, upstreamURL string) ([]byte, error) {
	c.inflightMu.Lock()
	if existing, ok := c.inflight[key]; ok {
		c.inflightMu.Unlock()
		select {
		case <-existing.done:
			return existing.buf, existing.err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	call := &clipInflight{done: make(chan struct{})}
	c.inflight[key] = call
	c.inflightMu.Unlock()

	go func() {
		// Detach from the triggering caller's cancellation but preserve its
		// deadline if it had one, so the upstream fetch survives a probe
		// cancel yet remains time-bounded.
		fetchCtx := context.WithoutCancel(ctx)
		if d, ok := ctx.Deadline(); ok {
			var cancel context.CancelFunc
			fetchCtx, cancel = context.WithDeadline(fetchCtx, d)
			defer cancel()
		}
		buf, err := c.fetchClip(fetchCtx, key, upstreamURL)
		call.buf = buf
		call.err = err
		if err == nil {
			c.clips.Put(key, buf, time.Time{})
		}
		c.inflightMu.Lock()
		delete(c.inflight, key)
		c.inflightMu.Unlock()
		close(call.done)
	}()

	select {
	case <-call.done:
		return call.buf, call.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// fetchClip pulls the raw MP4 bytes for upstreamURL from Frigate, capped
// at c.maxClipBytes. `key` is the cache / log identifier; the upstream
// URL is supplied by the caller so the same buffer/retag pipeline can
// front either per-event or per-window clip endpoints.
func (c *Client) fetchClip(ctx context.Context, key, upstreamURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, upstreamURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("frigate clip request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("frigate clip returned %d", resp.StatusCode)
	}

	// Read one byte past the cap so we can distinguish "fits" from "overflow".
	buf, err := io.ReadAll(io.LimitReader(resp.Body, c.maxClipBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read clip body: %w", err)
	}
	if int64(len(buf)) > c.maxClipBytes {
		return nil, fmt.Errorf("clip body exceeds %d-byte limit", c.maxClipBytes)
	}

	// Rewrite hev1 sample-entry tags to hvc1 so iOS Safari will decode the
	// HEVC bitstream in <video>. The patch is four bytes per video sample
	// entry; the bitstream itself is untouched and the output length is
	// unchanged. On any parse failure we degrade to the untouched bytes
	// (desktop browsers still play; iOS Safari is broken only for that
	// clip, not the whole endpoint).
	retagged, rerr := remuxHvc1(buf)
	if rerr != nil {
		slog.Warn("hev1→hvc1 retag failed; serving original clip bytes",
			"clip_key", key, "error", rerr)
	} else {
		buf = retagged
	}
	return buf, nil
}

// FetchImage fetches a per-event image (thumbnail or snapshot) from Frigate.
// `name` should be "thumbnail.jpg" or "snapshot.jpg". The caller is
// responsible for closing the returned body. On non-200, body is nil and the
// upstream status is returned in the error.
func (c *Client) FetchImage(ctx context.Context, eventID, name string) (io.ReadCloser, string, error) {
	reqURL := fmt.Sprintf("%s/api/events/%s/%s", c.baseURL, eventID, name)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, "", fmt.Errorf("build request: %w", err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("frigate request: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, "", fmt.Errorf("frigate returned %d", resp.StatusCode)
	}
	return resp.Body, resp.Header.Get("ETag"), nil
}
