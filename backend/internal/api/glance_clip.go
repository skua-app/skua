package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/skua-app/skua/internal/events"
)

// handleGlanceClip serves GET /api/glance/{id}/clip.mp4: resolves the
// Frigate review id to its [start, end] window (shared
// momentClipWindow rule with the preview handler) and serves Frigate's
// full-resolution /api/{camera}/start/{start}/end/{end}/clip.mp4
// through the same buffered + hev1→hvc1 retag pipeline as the
// per-event clip endpoint. The pipeline buffers the whole clip into
// memory so http.ServeContent can satisfy Range requests inline on
// iOS Safari; cache and single-flight are keyed on the review id so
// repeat opens of the same moment share one upstream fetch.
//
// This is the real-time, audio-bearing counterpart to the
// scrub-quality /api/glance/{id}/preview.mp4 endpoint. No HEAD route:
// the buffered pipeline does not have a useful HEAD path, and the
// glance UI only ever issues GETs.
//
// With ?download=1 the response is served as an attachment with the
// filename "frigate-{id}.mp4"; without the flag it stays inline. The
// download and the inline player share the same buffered bytes (cache
// + single-flight are keyed on the review id) — only the
// Content-Disposition differs per request.
//
// Errors:
//   - invalid id format       → 404 not_found (mirrors the preview path)
//   - upstream review 404     → 404 not_found
//   - review or clip deadline → 504 upstream_timeout
//   - any other upstream/transport failure → 502 upstream_error
//
// Reuses eventsClipTimeout (30s) — the time-range clip is reassembled
// by Frigate on demand and is roughly as expensive as a per-event
// clip; the 10s preview timeout is too tight for this path.
func (h *Handler) handleGlanceClip(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !validUpstreamID(id) {
		writeError(w, h.logger, http.StatusNotFound, "not_found", "moment not found", nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), eventsClipTimeout)
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

	var downloadName string
	if r.URL.Query().Get("download") == "1" {
		downloadName = "frigate-" + id + ".mp4"
	}

	if err := h.events.ServeMomentClip(ctx, id, review.Camera, start, end, w, r, downloadName); err != nil {
		status := http.StatusBadGateway
		code := "upstream_error"
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			status = http.StatusGatewayTimeout
			code = "upstream_timeout"
		}
		writeError(w, h.logger, status, code, "moment clip upstream error", err)
		return
	}
}
