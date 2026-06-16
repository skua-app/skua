package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/skua-app/skua/internal/events"
)

const (
	// eventReviewTimeout caps the upstream /api/review fetch. The
	// resolver is a single request; 5s mirrors the events list timeout
	// and is plenty for the LAN-only deployment.
	eventReviewTimeout = 5 * time.Second
	// eventReviewWindow is the half-width of the recall window around
	// the event's timestamp. Frigate review segments tend to be on
	// the order of seconds to a minute or two; ±10 minutes is wide
	// enough to catch the containing segment even when the event id's
	// embedded timestamp is the detection START and the segment runs
	// a bit before / well after. Precision still comes from the exact
	// detection-id match — the window is recall-only, not precision.
	eventReviewWindow = 10 * time.Minute
	// eventReviewLimit caps how many review items we scan per request.
	// Frigate returns segments newest-first; the window keeps this
	// list short in practice, but a busy install with many cameras
	// can still produce a long list around the event time.
	eventReviewLimit = 100
)

// eventReviewResponse is the JSON body for GET /api/events/{id}/review.
type eventReviewResponse struct {
	ReviewID string `json:"review_id"`
}

// handleEventReview serves GET /api/events/{id}/review: given a Frigate
// tracked-object event id, look up the review segment that contains it
// and return its review id so the frontend can deep-link to Frigate's
// review timeline.
//
// Approach: derive the event's unix-seconds timestamp from the id's
// "<unix>.<frac>-<hash>" prefix, query Frigate's /api/review for the
// ±eventReviewWindow window around that timestamp, then linearly scan
// the returned segments' data.detections for the exact id. Detection
// ids are globally unique, so the camera does not need to be passed
// and a cross-camera scan is safe.
//
// Errors:
//   - invalid id format / unparseable prefix → 404 not_found (a
//     malformed id can never resolve to a real review).
//   - upstream context deadline → 504 upstream_timeout.
//   - any other upstream failure → 502 upstream_error.
//   - the event id is well-formed but no review in the window
//     contains it → 404 not_found.
func (h *Handler) handleEventReview(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !validUpstreamID(id) {
		writeError(w, h.logger, http.StatusNotFound, "not_found", "review not found", nil)
		return
	}

	ts := events.EventTimestampPrefix(id)
	if ts <= 0 {
		writeError(w, h.logger, http.StatusNotFound, "not_found", "review not found", nil)
		return
	}
	// Unix-seconds float → time.Time using the same precision the
	// review.go mapper uses for start_time / end_time.
	center := time.Unix(int64(ts), 0)

	ctx, cancel := context.WithTimeout(r.Context(), eventReviewTimeout)
	defer cancel()

	items, err := h.events.ListReview(ctx, events.ReviewParams{
		After:  center.Add(-eventReviewWindow),
		Before: center.Add(eventReviewWindow),
		Limit:  eventReviewLimit,
	})
	if err != nil {
		status := http.StatusBadGateway
		code := "upstream_error"
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			status = http.StatusGatewayTimeout
			code = "upstream_timeout"
		}
		writeError(w, h.logger, status, code, "review upstream error", err)
		return
	}

	for _, it := range items {
		for _, det := range it.Data.Detections {
			if det == id {
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Cache-Control", "no-store")
				if err := json.NewEncoder(w).Encode(eventReviewResponse{ReviewID: it.ID}); err != nil {
					h.logger.Error("encode event review response", "error", err)
				}
				return
			}
		}
	}

	writeError(w, h.logger, http.StatusNotFound, "not_found", "review not found", nil)
}
