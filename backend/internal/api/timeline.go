package api

import (
	"context"
	"errors"
	"io"
	"net/http"
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
