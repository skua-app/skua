package api

import (
	"context"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/skua-app/skua/internal/events"
)

const (
	// glancePreviewTimeout caps the time the BFF will spend on the
	// review-id lookup plus the full preview proxy. Frigate's preview
	// segments are short low-res scrub-quality clips, so 10s is
	// generous for a household LAN deployment.
	glancePreviewTimeout = 10 * time.Second
	// glanceMomentPad adds slack around the review segment's
	// [start_time, end_time] window when asking Frigate for the
	// preview.mp4 or full-res clip.mp4. A few seconds of head and tail
	// lets the user scrub just outside the detected activity without
	// hitting empty renderable frames.
	glanceMomentPad = 4.0
)

// momentClipWindow computes the [start, end] unix-seconds window the
// glance clip/preview handlers ask Frigate to render around a review
// segment. start is the segment start minus glanceMomentPad, clamped
// to >= 0. end is the segment end (if present and > 0) or now (for an
// active segment with null/zero end_time), then plus glanceMomentPad.
func momentClipWindow(review events.FrigateReviewItem) (start, end float64) {
	start = review.StartTime - glanceMomentPad
	if start < 0 {
		start = 0
	}
	if review.EndTime != nil && *review.EndTime > 0 {
		end = *review.EndTime
	} else {
		end = float64(time.Now().Unix())
	}
	end += glanceMomentPad
	return
}

// handleGlancePreview serves GET / HEAD /api/glance/{id}/preview.mp4.
// Resolves the Frigate review id to its [start_time, end_time] window
// (padded glancePreviewPad seconds on each side; start clamped to >= 0;
// an active segment with null end_time uses now as the upper bound),
// then reverse-proxies Frigate's
//
//	/api/{camera}/start/{start}/end/{end}/preview.mp4
//
// inline. The proxy is intentionally thin: the BFF forwards Range
// verbatim, copies through the upstream status code, Content-Type,
// Content-Length, Content-Range, Accept-Ranges, and ETag, and streams
// the body without buffering or remuxing. Preview MP4s from Frigate
// are already iOS-friendly and Range-native, so this path is distinct
// from the event-clip pipeline (which buffers + retags hev1→hvc1 for
// inline playback). HEAD requests skip the body copy.
//
// Errors:
//   - invalid id format → 404 not_found (a malformed path segment can
//     never resolve to a real review; surfacing 400 here would just
//     leak BFF validation details for a path the user can't construct
//     by accident).
//   - upstream review 404 → 404 not_found.
//   - context deadline → 504 upstream_timeout.
//   - any other upstream / transport failure → 502 upstream_error.
//   - Frigate's preview endpoint may return 416 / 503 / etc; those
//     statuses are passed through to the client verbatim.
//
// Override: Frigate's preview.mp4 sends Content-Disposition: attachment.
// The BFF rewrites this to "inline" so the client `<video>` element
// plays it in place instead of triggering a download. The browser-
// facing Cache-Control is "private, max-age=86400" — preview segments
// are stable once the review finalises, but they may be re-emitted
// while a segment is still active, so we keep the cache short and
// private (per-device) rather than CDN-friendly public+immutable.
func (h *Handler) handleGlancePreview(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !validUpstreamID(id) {
		writeError(w, h.logger, http.StatusNotFound, "not_found", "moment not found", nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), glancePreviewTimeout)
	defer cancel()

	review, err := h.events.GetReview(ctx, id)
	if err != nil {
		if errors.Is(err, events.ErrReviewNotFound) {
			writeError(w, h.logger, http.StatusNotFound, "not_found", "moment not found", err)
			return
		}
		status := http.StatusBadGateway
		code := "upstream_error"
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			status = http.StatusGatewayTimeout
			code = "upstream_timeout"
		}
		writeError(w, h.logger, status, code, "review upstream error", err)
		return
	}

	start, end := momentClipWindow(review)

	upstream, err := h.events.OpenPreview(ctx, r.Method, review.Camera, start, end, r.Header.Get("Range"))
	if err != nil {
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
	// Frigate sends Content-Disposition: attachment. Override so the
	// browser `<video>` element plays the response inline.
	w.Header().Set("Content-Disposition", "inline")
	w.Header().Set("Cache-Control", "private, max-age=86400")
	w.WriteHeader(upstream.StatusCode)

	if r.Method == http.MethodHead {
		return
	}
	if _, err := io.Copy(w, upstream.Body); err != nil {
		h.logger.Debug("stream glance preview", "id", id, "error", err)
	}
}
