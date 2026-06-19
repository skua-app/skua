package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

const (
	// timelineVODTimeout caps the time the BFF will spend on a single
	// playlist or segment fetch from Frigate's recording VOD endpoint.
	// 15s is generous for a household LAN remux-on-demand — Frigate
	// assembles the fMP4 segment from the underlying recordings on
	// first touch, which can be a touch slower than serving a finished
	// preview clip; subsequent fetches hit the upstream cache. Reused
	// for the recordings-summary passthrough, which is a cheap JSON
	// fetch but kept on the same ceiling for simplicity.
	timelineVODTimeout = 15 * time.Second
)

// validVODPart reports whether s is a safe relative VOD playlist or
// segment filename to interpolate into the upstream Frigate VOD URL.
// The recording-timeline route binds the wildcard {*} suffix verbatim;
// without this guard the segment slot is an SSRF / path-traversal
// surface. Allowed characters mirror validUpstreamID — only
// [A-Za-z0-9._-] — and `.` / `..` are rejected explicitly so a
// well-formed but traversal-shaped filename cannot reach upstream.
// In practice the only filenames Frigate emits in the playlist body
// are master.m3u8, index-*.m3u8, init-*.mp4, and seg-*.m4s; everything
// else is invalid and rejected as not_found (mirroring the preview
// handler's reasoning: a malformed part can never resolve to a real
// segment, and surfacing 400 would just leak which check failed).
func validVODPart(s string) bool {
	if s == "" || s == "." || s == ".." {
		return false
	}
	if strings.ContainsAny(s, "/\\") {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z':
		case c >= 'A' && c <= 'Z':
		case c >= '0' && c <= '9':
		case c == '.' || c == '_' || c == '-':
		default:
			return false
		}
	}
	return true
}

// validPreviewClip reports whether s is a safe preview-clip filename to
// interpolate into the static Frigate /clips/previews/{cam}/{file} URL.
// Like validVODPart this is the SSRF / path-traversal guard, but tighter
// for the clip-name grammar: Frigate emits "{firstTs}-{lastTs}.mp4"
// (e.g. 1781748000.109578-1781751600.190408.mp4) — digits, dots, hyphens,
// and the .mp4 suffix only. "", ".", "..", and any name containing '/'
// or '\' are rejected; the stem (name minus ".mp4") must be non-empty and
// contain only [0-9.-]. A malformed name can never resolve to a real clip
// and is surfaced as not_found without leaking which check failed.
func validPreviewClip(s string) bool {
	if s == "" || s == "." || s == ".." {
		return false
	}
	if strings.ContainsAny(s, "/\\") {
		return false
	}
	if !strings.HasSuffix(s, ".mp4") {
		return false
	}
	stem := strings.TrimSuffix(s, ".mp4")
	if stem == "" {
		return false
	}
	for i := 0; i < len(stem); i++ {
		c := stem[i]
		switch {
		case c >= '0' && c <= '9':
		case c == '.' || c == '-':
		default:
			return false
		}
	}
	return true
}

// validPreviewFrame reports whether s is a safe preview-FRAME filename to
// interpolate into Frigate's /api/preview/{file}/thumbnail.webp URL. Like
// validPreviewClip this is the SSRF / path-traversal guard, but for the
// frame-name grammar: Frigate names these "preview_{cam}-{unix.frac}.webp"
// (e.g. preview_cam2-1781852889.819418.webp). "", ".", "..", and any name
// containing '/' or '\' are rejected; the name must start with "preview_"
// and end with ".webp", and every byte must be in [A-Za-z0-9._-]. A
// malformed name can never resolve to a real frame and is surfaced as
// not_found without leaking which check failed.
func validPreviewFrame(s string) bool {
	if s == "" || s == "." || s == ".." {
		return false
	}
	if strings.ContainsAny(s, "/\\") {
		return false
	}
	if !strings.HasPrefix(s, "preview_") || !strings.HasSuffix(s, ".webp") {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z':
		case c >= 'A' && c <= 'Z':
		case c >= '0' && c <= '9':
		case c == '.' || c == '_' || c == '-':
		default:
			return false
		}
	}
	return true
}

// validUnixSeconds reports whether s is a non-empty digit-only string
// suitable for the integer-unix-seconds slots in Frigate's VOD URL
// (/vod/{cam}/start/{start}/end/{end}/...). The validity of the time
// range itself (end > start, within recording retention) is decided
// by Frigate; the BFF only enforces that the slots stay well-formed
// so the upstream URL remains parseable.
func validUnixSeconds(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// handleTimelineVOD serves GET / HEAD
// /api/cameras/{id}/vod/{start}/{end}/* — a thin reverse proxy in
// front of Frigate's recording VOD endpoint
// (/vod/{cam}/start/{start}/end/{end}/{rest}), which emits an HLS
// fMP4 ladder (master playlist → index playlist → init.mp4 + seg-*.m4s).
//
// The proxy is codec-agnostic and intentionally minimal: no buffering,
// no transcode, no hev1→hvc1 retag (Frigate's recording remux already
// emits browser-friendly tags), and no playlist-body rewriting. The
// HLS ladder uses relative URIs throughout, so the path-embedded
// {start}/{end} slots survive relative resolution and clients fetch
// every child playlist / segment through the same /api/cameras/{id}/vod/
// prefix without any URL surgery in the BFF. Range is forwarded
// verbatim, the upstream status code and the byte-range / caching
// headers (Content-Type, Content-Length, Content-Range, Accept-Ranges,
// ETag) are passed through, and HEAD skips the body copy.
//
// Cache-Control by suffix: playlist (.m3u8) responses are "no-store"
// because the playlist mutates while the window includes "now" (an
// active recording grows its segment list every couple of seconds);
// segment payloads (.mp4 init, .m4s seg) are write-once for a given
// time range and get a year of immutable public caching so clients
// and any reverse proxy can hold them. Content-Disposition is not
// touched — Frigate's VOD does not send attachment for these.
//
// Errors:
//   - invalid camera id → 400 invalid_id (caller-facing typo on a
//     known route).
//   - camera not in registry → 404 not_found.
//   - non-numeric start or end → 400 invalid_range (the upstream URL
//     would be malformed; we do not validate end > start here, Frigate
//     decides on the actual range).
//   - malformed rest (traversal, slashes, non-allowed bytes) → 404
//     not_found (a malformed part can never resolve to a real
//     segment; do not leak which check failed).
//   - context deadline → 504 upstream_timeout.
//   - any other upstream / transport failure → 502 upstream_error.
//   - Frigate's VOD endpoint may return 416 / 404 / etc. directly;
//     those statuses are passed through to the client verbatim.
func (h *Handler) handleTimelineVOD(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !validUpstreamID(id) {
		writeError(w, h.logger, http.StatusBadRequest, "invalid_id", "invalid camera id", nil)
		return
	}
	if _, ok := h.cameras.Find(id); !ok {
		writeError(w, h.logger, http.StatusNotFound, "not_found", "camera not found", nil)
		return
	}

	start := chi.URLParam(r, "start")
	end := chi.URLParam(r, "end")
	if !validUnixSeconds(start) || !validUnixSeconds(end) {
		writeError(w, h.logger, http.StatusBadRequest, "invalid_range", "start and end must be integer unix seconds", nil)
		return
	}

	rest := chi.URLParam(r, "*")
	if !validVODPart(rest) {
		writeError(w, h.logger, http.StatusNotFound, "not_found", "segment not found", nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), timelineVODTimeout)
	defer cancel()

	upstream, err := h.frigate.OpenVOD(ctx, r.Method, id, start, end, rest, r.Header.Get("Range"))
	if err != nil {
		// Client-disconnect path: iOS <video> opens HLS segments in
		// parallel and cancels some immediately as it picks a ladder
		// position. The request context fires Canceled, which propagates
		// through our derived ctx into OpenVOD's transport error. There
		// is no client left to receive a 502, so log at Debug and bail
		// without writing a status or body. Check Canceled BEFORE
		// DeadlineExceeded — our own timeout also cancels the derived
		// ctx and would otherwise be misclassified.
		if errors.Is(err, context.Canceled) && r.Context().Err() == context.Canceled {
			h.logger.Debug("timeline vod cancelled by client", "cam", id, "rest", rest)
			return
		}
		status := http.StatusBadGateway
		code := "upstream_error"
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			status = http.StatusGatewayTimeout
			code = "upstream_timeout"
		}
		writeError(w, h.logger, status, code, "vod upstream error", err)
		return
	}
	defer func() { _ = upstream.Body.Close() }()

	for _, hdr := range []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges", "ETag"} {
		if v := upstream.Header.Get(hdr); v != "" {
			w.Header().Set(hdr, v)
		}
	}
	switch {
	case strings.HasSuffix(rest, ".m3u8"):
		w.Header().Set("Cache-Control", "no-store")
	case strings.HasSuffix(rest, ".mp4"), strings.HasSuffix(rest, ".m4s"):
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	}
	w.WriteHeader(upstream.StatusCode)

	if r.Method == http.MethodHead {
		return
	}
	if _, err := io.Copy(w, upstream.Body); err != nil {
		h.logger.Debug("stream timeline vod", "cam", id, "rest", rest, "error", err)
	}
}

// parseWindowSeconds parses one of the start / end query values of the
// preview route. The FE sends integer unix seconds, but Frigate's own
// preview path formats the window with a decimal suffix, so a single
// decimal point is accepted. The value must be non-empty and contain
// only digits with at most one '.', which keeps strconv.ParseFloat from
// admitting exponents, signs, "Inf", or "NaN" — any of those would
// format into a malformed (or remotely surprising) upstream URL. ok is
// false on any rejection.
func parseWindowSeconds(s string) (float64, bool) {
	if s == "" {
		return 0, false
	}
	dots := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= '0' && c <= '9':
		case c == '.':
			dots++
			if dots > 1 {
				return 0, false
			}
		default:
			return 0, false
		}
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

// handleTimelinePreview serves GET / HEAD
// /api/cameras/{id}/preview.mp4?start=&end= — a thin reverse proxy in
// front of Frigate's arbitrary-window preview endpoint
// (/api/{cam}/start/{start}/end/{end}/preview.mp4), the low-res H.264
// timelapse the timeline scrubber uses for fast seeking. It reuses the
// same events.OpenPreview path the glance moment preview rides on, but
// takes the camera + window straight from the request instead of
// resolving a review id.
//
// The proxy mirrors handleTimelineVOD: Range is forwarded verbatim and
// the upstream status code plus the byte-range / caching headers
// (Content-Type, Content-Length, Content-Range, Accept-Ranges, ETag)
// are passed through; HEAD skips the body copy. Two deliberate
// differences from the VOD path:
//   - Content-Disposition is dropped. Frigate's preview.mp4 sends
//     `attachment`; not forwarding it keeps the clip playing inline in a
//     `<video src>` rather than triggering a download.
//   - Cache-Control is "no-store". A window ending at "now" keeps
//     growing as footage accrues, so caching it would serve a stale,
//     short preview; keeping it simple and uncached is correct for the
//     MVP.
//
// Errors:
//   - invalid camera id → 400 invalid_id.
//   - camera not in registry → 404 not_found.
//   - missing / unparseable start or end, or end <= start → 400
//     invalid_range (a preview needs a real span).
//   - client-cancelled request → Debug + return (no error written; the
//     scrubber abandons in-flight previews as the finger moves).
//   - context deadline → 504 upstream_timeout.
//   - any other upstream / transport failure → 502 upstream_error.
//   - Frigate's preview endpoint may return 416 / 503 / etc. directly;
//     those statuses are passed through verbatim.
func (h *Handler) handleTimelinePreview(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !validUpstreamID(id) {
		writeError(w, h.logger, http.StatusBadRequest, "invalid_id", "invalid camera id", nil)
		return
	}
	if _, ok := h.cameras.Find(id); !ok {
		writeError(w, h.logger, http.StatusNotFound, "not_found", "camera not found", nil)
		return
	}

	q := r.URL.Query()
	start, okStart := parseWindowSeconds(q.Get("start"))
	end, okEnd := parseWindowSeconds(q.Get("end"))
	if !okStart || !okEnd || end <= start {
		writeError(w, h.logger, http.StatusBadRequest, "invalid_range", "start and end must be numbers with end > start", nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), timelineVODTimeout)
	defer cancel()

	upstream, err := h.events.OpenPreview(ctx, r.Method, id, start, end, r.Header.Get("Range"))
	if err != nil {
		// Mirror handleTimelineVOD's client-disconnect branch: the
		// scrubber abandons in-flight previews as the finger keeps
		// moving, cancelling the request context. There is no client
		// left to receive a 502, so log at Debug and bail. Check Canceled
		// BEFORE DeadlineExceeded — our own timeout also cancels the
		// derived ctx and would otherwise be misclassified.
		if errors.Is(err, context.Canceled) && r.Context().Err() == context.Canceled {
			h.logger.Debug("timeline preview cancelled by client", "cam", id)
			return
		}
		status := http.StatusBadGateway
		code := "upstream_error"
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			status = http.StatusGatewayTimeout
			code = "upstream_timeout"
		}
		writeError(w, h.logger, status, code, "preview upstream error", err)
		return
	}
	defer func() { _ = upstream.Body.Close() }()

	for _, hdr := range []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges", "ETag"} {
		if v := upstream.Header.Get(hdr); v != "" {
			w.Header().Set(hdr, v)
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(upstream.StatusCode)

	if r.Method == http.MethodHead {
		return
	}
	if _, err := io.Copy(w, upstream.Body); err != nil {
		h.logger.Debug("stream timeline preview", "cam", id, "error", err)
	}
}

// previewFramesMaxBytes caps the upstream preview-frames JSON the BFF
// will read. A busy clock-hour bucket lists a few hundred KB of
// filenames; 4 MiB is a generous ceiling that still bounds a runaway or
// hostile upstream response.
const previewFramesMaxBytes = 4 << 20

// previewBounds is the reduced shape served by handleTimelinePreviewFrames:
// the real covered wall-clock span of a Frigate preview bucket plus the
// frame count. Start / End are unix seconds (float, Frigate timestamps
// carry subseconds) and are 0 when Count is 0 or the first/last filename
// failed to parse — the FE checks Count > 0 && End > Start before trusting
// them. The full frames array is intentionally NOT surfaced.
type previewBounds struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	Count int     `json:"count"`
}

// parsePreviewFrameTs extracts the unix-second timestamp from a Frigate
// preview-frame filename like "preview_<cam>-<unixSeconds.frac>.webp".
// The timestamp is the substring after the LAST '-' and before the
// trailing ".webp" — splitting on the last '-' so hyphenated camera ids
// (e.g. "front-door") parse correctly. ok is false on any malformed name.
func parsePreviewFrameTs(name string) (float64, bool) {
	dash := strings.LastIndex(name, "-")
	if dash < 0 || dash+1 >= len(name) {
		return 0, false
	}
	ts := strings.TrimSuffix(name[dash+1:], ".webp")
	v, err := strconv.ParseFloat(ts, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

// handleTimelinePreviewFrames serves GET
// /api/cameras/{id}/preview-frames?start=&end= — the real covered
// wall-clock span of Frigate's preview bucket for [start,end), derived
// from its preview-frames list
// (/api/preview/{cam}/start/{start}/end/{end}/frames). The scrubber uses
// it to map a scrub position within an hour's preview accurately instead
// of assuming the preview spans the full clock hour.
//
// Unlike the preview.mp4 / VOD start-end slots, these are integer unix
// seconds only (the FE sends clock-hour-aligned bucket bounds), validated
// with validUnixSeconds. The BFF reads the upstream JSON array of frame
// filenames (capped at previewFramesMaxBytes), parses the first and last
// names into timestamps, and returns only {start, end, count}. The full
// array never reaches the client.
//
// Errors:
//   - invalid camera id → 400 invalid_id.
//   - camera not in registry → 404 not_found.
//   - missing / non-numeric start or end, or end <= start → 400
//     invalid_range.
//   - client-cancelled request → Debug + return (no error written).
//   - context deadline → 504 upstream_timeout.
//   - any other upstream / transport failure, or undecodable upstream
//     body → 502 upstream_error.
//   - a non-2xx upstream status is passed through with a zero-count body
//     so the FE degrades to the clock-hour assumption.
func (h *Handler) handleTimelinePreviewFrames(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !validUpstreamID(id) {
		writeError(w, h.logger, http.StatusBadRequest, "invalid_id", "invalid camera id", nil)
		return
	}
	if _, ok := h.cameras.Find(id); !ok {
		writeError(w, h.logger, http.StatusNotFound, "not_found", "camera not found", nil)
		return
	}

	q := r.URL.Query()
	start := q.Get("start")
	end := q.Get("end")
	startN, errStart := strconv.ParseInt(start, 10, 64)
	endN, errEnd := strconv.ParseInt(end, 10, 64)
	if !validUnixSeconds(start) || !validUnixSeconds(end) || errStart != nil || errEnd != nil || endN <= startN {
		writeError(w, h.logger, http.StatusBadRequest, "invalid_range", "start and end must be integer unix seconds with end > start", nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), timelineVODTimeout)
	defer cancel()

	upstream, err := h.frigate.GetPreviewFrames(ctx, id, start, end)
	if err != nil {
		// Mirror handleTimelinePreview's client-disconnect branch: a
		// cancelled request context is not an upstream error and there is
		// no one to receive a 502. Check Canceled BEFORE DeadlineExceeded.
		if errors.Is(err, context.Canceled) && r.Context().Err() == context.Canceled {
			h.logger.Debug("timeline preview frames cancelled by client", "cam", id)
			return
		}
		status := http.StatusBadGateway
		code := "upstream_error"
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			status = http.StatusGatewayTimeout
			code = "upstream_timeout"
		}
		writeError(w, h.logger, status, code, "preview frames upstream error", err)
		return
	}
	defer func() { _ = upstream.Body.Close() }()

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")

	// Non-2xx upstream: pass the status through with a zero-count body so
	// the FE degrades to assuming the preview spans the full clock hour.
	if upstream.StatusCode < 200 || upstream.StatusCode >= 300 {
		w.WriteHeader(upstream.StatusCode)
		_ = json.NewEncoder(w).Encode(previewBounds{})
		return
	}

	var frames []string
	if err := json.NewDecoder(io.LimitReader(upstream.Body, previewFramesMaxBytes)).Decode(&frames); err != nil {
		writeError(w, h.logger, http.StatusBadGateway, "upstream_error", "decode preview frames", err)
		return
	}

	out := previewBounds{Count: len(frames)}
	if len(frames) > 0 {
		// Only trust the span when BOTH ends parse — a single malformed
		// filename leaves zero bounds (with the real count) so the FE
		// falls back to the clock-hour assumption.
		if first, okFirst := parsePreviewFrameTs(frames[0]); okFirst {
			if last, okLast := parsePreviewFrameTs(frames[len(frames)-1]); okLast {
				out.Start = first
				out.End = last
			}
		}
	}
	_ = json.NewEncoder(w).Encode(out)
}

// previewClipsMaxBytes caps the upstream preview-clips JSON the BFF will
// read. A clock-hour window lists at most a handful of clip entries; 1 MiB
// is a generous ceiling that still bounds a runaway or hostile upstream
// response.
const previewClipsMaxBytes = 1 << 20

// frigatePreviewClip is one entry of Frigate's preview-clips list
// (/api/preview/{cam}/start/{start}/end/{end}). Only the fields the BFF
// reduces are decoded.
type frigatePreviewClip struct {
	Camera string  `json:"camera"`
	Src    string  `json:"src"`
	Type   string  `json:"type"`
	Start  float64 `json:"start"`
	End    float64 `json:"end"`
}

// previewClip is one entry of the reduced list the BFF serves: the clip's
// covered span plus a Src REWRITTEN to the BFF /preview-clip route so the
// client never reaches Frigate's /clips/previews path directly.
type previewClip struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	Src   string  `json:"src"`
}

// handleTimelinePreviewClips serves GET
// /api/cameras/{id}/preview-clips?start=&end= — the list of real preview
// CLIPS Frigate has for [start,end), the same source its own History
// timeline scrubs against (distinct from the degenerate single
// preview.mp4 concat). The BFF reads Frigate's clips list
// (/api/preview/{cam}/start/{start}/end/{end}), and for each entry derives
// the clip filename from src's path base, drops any name that fails the
// traversal whitelist, and re-emits {start, end, src} with src REWRITTEN
// to "/api/cameras/{id}/preview-clip/{file}" so the client only ever
// reaches Frigate's static clip path through the BFF proxy below.
//
// start/end are integer unix seconds (validUnixSeconds, end > start).
//
// Errors:
//   - invalid camera id → 400 invalid_id.
//   - camera not in registry → 404 not_found.
//   - missing / non-numeric start or end, or end <= start → 400
//     invalid_range.
//   - client-cancelled request → Debug + return (no error written).
//   - context deadline → 504 upstream_timeout.
//   - any other upstream / transport failure, or undecodable upstream
//     body → 502 upstream_error.
//   - a non-2xx upstream status is passed through with an empty JSON array
//     so the FE degrades to "no preview" rather than erroring.
func (h *Handler) handleTimelinePreviewClips(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !validUpstreamID(id) {
		writeError(w, h.logger, http.StatusBadRequest, "invalid_id", "invalid camera id", nil)
		return
	}
	if _, ok := h.cameras.Find(id); !ok {
		writeError(w, h.logger, http.StatusNotFound, "not_found", "camera not found", nil)
		return
	}

	q := r.URL.Query()
	start := q.Get("start")
	end := q.Get("end")
	startN, errStart := strconv.ParseInt(start, 10, 64)
	endN, errEnd := strconv.ParseInt(end, 10, 64)
	if !validUnixSeconds(start) || !validUnixSeconds(end) || errStart != nil || errEnd != nil || endN <= startN {
		writeError(w, h.logger, http.StatusBadRequest, "invalid_range", "start and end must be integer unix seconds with end > start", nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), timelineVODTimeout)
	defer cancel()

	upstream, err := h.frigate.ListPreviewClips(ctx, id, start, end)
	if err != nil {
		// Mirror handleTimelinePreview's client-disconnect branch: a
		// cancelled request context is not an upstream error and there is
		// no one to receive a 502. Check Canceled BEFORE DeadlineExceeded.
		if errors.Is(err, context.Canceled) && r.Context().Err() == context.Canceled {
			h.logger.Debug("timeline preview clips cancelled by client", "cam", id)
			return
		}
		status := http.StatusBadGateway
		code := "upstream_error"
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			status = http.StatusGatewayTimeout
			code = "upstream_timeout"
		}
		writeError(w, h.logger, status, code, "preview clips upstream error", err)
		return
	}
	defer func() { _ = upstream.Body.Close() }()

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")

	// Non-2xx upstream: pass the status through with an empty array body so
	// the FE degrades to "no preview" rather than erroring.
	if upstream.StatusCode < 200 || upstream.StatusCode >= 300 {
		w.WriteHeader(upstream.StatusCode)
		_ = json.NewEncoder(w).Encode([]previewClip{})
		return
	}

	var clips []frigatePreviewClip
	if err := json.NewDecoder(io.LimitReader(upstream.Body, previewClipsMaxBytes)).Decode(&clips); err != nil {
		writeError(w, h.logger, http.StatusBadGateway, "upstream_error", "decode preview clips", err)
		return
	}

	// Rewrite every src to the BFF /preview-clip route — the client must
	// never reach Frigate's /clips/previews path directly. Entries whose
	// filename fails the traversal whitelist are dropped defensively.
	out := []previewClip{}
	for _, c := range clips {
		file := c.Src
		if i := strings.LastIndex(file, "/"); i >= 0 {
			file = file[i+1:]
		}
		if !validPreviewClip(file) {
			continue
		}
		out = append(out, previewClip{
			Start: c.Start,
			End:   c.End,
			Src:   "/api/cameras/" + id + "/preview-clip/" + file,
		})
	}
	_ = json.NewEncoder(w).Encode(out)
}

// handleTimelinePreviewClip serves GET / HEAD
// /api/cameras/{id}/preview-clip/{file} — a thin reverse proxy in front of
// Frigate's static preview clip files (/clips/previews/{cam}/{file}). The
// {file} slot is the SSRF / path-traversal surface, gated by
// validPreviewClip; a malformed name is surfaced as 404 not_found without
// leaking which check failed (mirrors the VOD segment reasoning).
//
// Range is forwarded verbatim and the upstream status code plus the
// byte-range headers (Content-Type, Content-Length, Content-Range,
// Accept-Ranges, ETag) pass through; HEAD skips the body. Content-Disposition
// is dropped so the clip plays inline in a <video src> rather than
// triggering a download. Cache-Control is "no-store" — the current hour's
// clip is still being written.
//
// Errors:
//   - invalid camera id → 400 invalid_id.
//   - camera not in registry → 404 not_found.
//   - malformed {file} (traversal, slash, non-allowed bytes, wrong
//     suffix) → 404 not_found.
//   - client-cancelled request → Debug + return (no error written).
//   - context deadline → 504 upstream_timeout.
//   - any other upstream / transport failure → 502 upstream_error.
//   - Frigate's static endpoint may return 416 / 404 / etc. directly;
//     those statuses are passed through verbatim.
func (h *Handler) handleTimelinePreviewClip(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !validUpstreamID(id) {
		writeError(w, h.logger, http.StatusBadRequest, "invalid_id", "invalid camera id", nil)
		return
	}
	if _, ok := h.cameras.Find(id); !ok {
		writeError(w, h.logger, http.StatusNotFound, "not_found", "camera not found", nil)
		return
	}

	file := chi.URLParam(r, "file")
	if !validPreviewClip(file) {
		writeError(w, h.logger, http.StatusNotFound, "not_found", "clip not found", nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), timelineVODTimeout)
	defer cancel()

	upstream, err := h.frigate.OpenPreviewClip(ctx, r.Method, id, file, r.Header.Get("Range"))
	if err != nil {
		// Mirror handleTimelineVOD's client-disconnect branch: the scrubber
		// abandons in-flight clips as the finger keeps moving, cancelling
		// the request context. Check Canceled BEFORE DeadlineExceeded.
		if errors.Is(err, context.Canceled) && r.Context().Err() == context.Canceled {
			h.logger.Debug("timeline preview clip cancelled by client", "cam", id, "file", file)
			return
		}
		status := http.StatusBadGateway
		code := "upstream_error"
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			status = http.StatusGatewayTimeout
			code = "upstream_timeout"
		}
		writeError(w, h.logger, status, code, "preview clip upstream error", err)
		return
	}
	defer func() { _ = upstream.Body.Close() }()

	for _, hdr := range []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges", "ETag"} {
		if v := upstream.Header.Get(hdr); v != "" {
			w.Header().Set(hdr, v)
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(upstream.StatusCode)

	if r.Method == http.MethodHead {
		return
	}
	if _, err := io.Copy(w, upstream.Body); err != nil {
		h.logger.Debug("stream timeline preview clip", "cam", id, "file", file, "error", err)
	}
}

// handleTimelinePreviewFrame serves GET / HEAD
// /api/cameras/{id}/preview-frame/{file} — a thin reverse proxy in front of
// Frigate's single static preview-frame image
// (/api/preview/{file}/thumbnail.webp). The scrubber uses it to seek the
// open (in-progress) current hour by webp frame while that hour's preview
// mp4 clip has not been assembled yet.
//
// The {id} is used only for auth / consistency with the other
// camera-scoped routes; the upstream call keys off {file} alone — the
// frame filename ("preview_{cam}-{unix.frac}.webp") already embeds the
// camera, so Frigate takes no separate camera path segment. The {file}
// slot is the SSRF / path-traversal surface, gated by validPreviewFrame;
// a malformed name is surfaced as 404 not_found without leaking which
// check failed (mirrors the preview-clip reasoning).
//
// Range is forwarded verbatim (defensive — frames are small) and the
// upstream status code plus the byte-range headers (Content-Type,
// Content-Length, Content-Range, Accept-Ranges, ETag) pass through; HEAD
// skips the body. Content-Disposition is dropped so the image renders
// inline. Cache-Control is "public, max-age=31536000, immutable": Frigate
// marks these immutable and a given frame file is content-addressed by its
// timestamp and never changes — unlike the no-store clip / list routes.
//
// Errors:
//   - invalid camera id → 400 invalid_id.
//   - camera not in registry → 404 not_found.
//   - malformed {file} (traversal, slash, non-allowed bytes, wrong
//     prefix/suffix) → 404 not_found.
//   - client-cancelled request → Debug + return (no error written).
//   - context deadline → 504 upstream_timeout.
//   - any other upstream / transport failure → 502 upstream_error.
//   - Frigate's static endpoint may return 416 / 404 / etc. directly;
//     those statuses are passed through verbatim.
func (h *Handler) handleTimelinePreviewFrame(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !validUpstreamID(id) {
		writeError(w, h.logger, http.StatusBadRequest, "invalid_id", "invalid camera id", nil)
		return
	}
	if _, ok := h.cameras.Find(id); !ok {
		writeError(w, h.logger, http.StatusNotFound, "not_found", "camera not found", nil)
		return
	}

	file := chi.URLParam(r, "file")
	if !validPreviewFrame(file) {
		writeError(w, h.logger, http.StatusNotFound, "not_found", "frame not found", nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), timelineVODTimeout)
	defer cancel()

	upstream, err := h.frigate.OpenPreviewFrame(ctx, r.Method, file, r.Header.Get("Range"))
	if err != nil {
		// Mirror handleTimelinePreviewClip's client-disconnect branch: the
		// scrubber abandons in-flight frames as the finger keeps moving,
		// cancelling the request context. Check Canceled BEFORE DeadlineExceeded.
		if errors.Is(err, context.Canceled) && r.Context().Err() == context.Canceled {
			h.logger.Debug("timeline preview frame cancelled by client", "cam", id, "file", file)
			return
		}
		status := http.StatusBadGateway
		code := "upstream_error"
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			status = http.StatusGatewayTimeout
			code = "upstream_timeout"
		}
		writeError(w, h.logger, status, code, "preview frame upstream error", err)
		return
	}
	defer func() { _ = upstream.Body.Close() }()

	for _, hdr := range []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges", "ETag"} {
		if v := upstream.Header.Get(hdr); v != "" {
			w.Header().Set(hdr, v)
		}
	}
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.WriteHeader(upstream.StatusCode)

	if r.Method == http.MethodHead {
		return
	}
	if _, err := io.Copy(w, upstream.Body); err != nil {
		h.logger.Debug("stream timeline preview frame", "cam", id, "file", file, "error", err)
	}
}

// handleRecordingsSummary serves GET
// /api/cameras/{id}/recordings-summary[?timezone=] as a verbatim
// pass-through of Frigate's /api/{cam}/recordings/summary. The
// response shape is owned upstream and not yet typed by the BFF —
// callers consume it as opaque JSON; the BFF only validates the
// camera id, forwards the optional timezone, and copies bytes
// through. Cache-Control is left to upstream / client defaults: the
// summary is cheap to recompute and changes whenever a new recording
// finishes, so storing it would just create staleness.
func (h *Handler) handleRecordingsSummary(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !validUpstreamID(id) {
		writeError(w, h.logger, http.StatusBadRequest, "invalid_id", "invalid camera id", nil)
		return
	}
	if _, ok := h.cameras.Find(id); !ok {
		writeError(w, h.logger, http.StatusNotFound, "not_found", "camera not found", nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), timelineVODTimeout)
	defer cancel()

	upstream, err := h.frigate.GetRecordingsSummary(ctx, id, r.URL.Query().Get("timezone"))
	if err != nil {
		// Mirror handleTimelineVOD's client-disconnect branch: a
		// cancelled request context (typically the browser walked away
		// from the timeline before the summary fetch landed) is not an
		// upstream error and there is no one to receive a 502.
		if errors.Is(err, context.Canceled) && r.Context().Err() == context.Canceled {
			h.logger.Debug("recordings summary cancelled by client", "cam", id)
			return
		}
		status := http.StatusBadGateway
		code := "upstream_error"
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			status = http.StatusGatewayTimeout
			code = "upstream_timeout"
		}
		writeError(w, h.logger, status, code, "recordings summary upstream error", err)
		return
	}
	defer func() { _ = upstream.Body.Close() }()

	ct := upstream.Header.Get("Content-Type")
	if ct == "" {
		ct = "application/json"
	}
	w.Header().Set("Content-Type", ct)
	w.WriteHeader(upstream.StatusCode)
	if _, err := io.Copy(w, upstream.Body); err != nil {
		h.logger.Debug("stream recordings summary", "cam", id, "error", err)
	}
}
