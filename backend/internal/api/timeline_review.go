package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/skua-app/skua/internal/events"
)

const (
	// reviewTimelineTimeout caps the upstream /api/review fetch for the
	// scrubber activity lane. A single request; 5s mirrors the events
	// list timeout and is plenty for the LAN-only deployment.
	reviewTimelineTimeout = 5 * time.Second
	// reviewLookback widens the lower bound on Frigate's /api/review query.
	// Frigate filters review items on start_time, so a segment that STARTED
	// just before the requested window but OVERLAPS it would be missed
	// without a small lookback. 30 min is a pragmatic v1 bound — a rare,
	// very-long active segment that started more than 30 min before the
	// window's left edge may still be missed at that edge.
	reviewLookback = 30 * time.Minute
	// reviewTimelineLimit caps how many review items the lane fetch scans
	// per window. The windowed span keeps this short in practice.
	reviewTimelineLimit = 500
)

// reviewSegment is the lean timeline shape served by GET
// /api/cameras/{id}/review: one Frigate review segment reduced to what the
// scrubber's activity lane needs. End is null while the segment is still
// active (Frigate's end_time null), which the FE draws out to the live edge.
type reviewSegment struct {
	ID       string   `json:"id"`
	Severity string   `json:"severity"`
	Start    float64  `json:"start"`
	End      *float64 `json:"end"`
}

// handleTimelineReview serves GET /api/cameras/{id}/review?start=&end= — the
// windowed list of Frigate review segments (grouped alert / detection
// activity) for [start,end), reshaped into lean timeline segments the
// scrubber renders as a thin activity lane along the top of the bar.
//
// The BFF queries Frigate's /api/review with cameras={id}, after=start-
// reviewLookback (Frigate filters on start_time; the lookback catches a
// segment that started just before the window but overlaps it), before=end,
// and limit=reviewTimelineLimit, then passes each item's severity through
// and emits {id, severity, start, end} with end null while the segment is
// still active.
//
// start/end are integer unix seconds (validUnixSeconds, end > start).
//
// Errors mirror the sibling timeline handlers:
//   - invalid camera id                → 400 invalid_id
//   - camera not in registry           → 404 not_found
//   - missing / non-numeric start|end,
//     or end <= start                  → 400 invalid_range
//   - context deadline                 → 504 upstream_timeout
//   - any other upstream failure       → 502 upstream_error
func (h *Handler) handleTimelineReview(w http.ResponseWriter, r *http.Request) {
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

	ctx, cancel := context.WithTimeout(r.Context(), reviewTimelineTimeout)
	defer cancel()

	items, err := h.events.ListReview(ctx, events.ReviewParams{
		Cameras: []string{id},
		After:   time.Unix(startN-int64(reviewLookback.Seconds()), 0),
		Before:  time.Unix(endN, 0),
		Limit:   reviewTimelineLimit,
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

	out := make([]reviewSegment, 0, len(items))
	for _, it := range items {
		out = append(out, reviewSegment{
			ID:       it.ID,
			Severity: it.Severity,
			Start:    it.StartTime,
			End:      it.EndTime,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if err := json.NewEncoder(w).Encode(out); err != nil {
		h.logger.Error("encode timeline review response", "error", err)
	}
}
