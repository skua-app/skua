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
	// audioTimelineTimeout caps the upstream /api/events fetch for the
	// scrubber audio lane. A single request; 5s mirrors reviewTimelineTimeout
	// and is plenty for the LAN-only deployment.
	audioTimelineTimeout = 5 * time.Second
	// audioLookback widens the lower bound on Frigate's /api/events query.
	// Frigate filters events on start_time, so an audio event that STARTED
	// just before the requested window but OVERLAPS it would be missed
	// without a small lookback. Audio events are short (well under a couple
	// of minutes), so a 5-min lower-bound widening reliably catches one that
	// started just before the window's left edge.
	audioLookback = 5 * time.Minute
	// audioTimelineLimit caps how many events the lane fetch scans per window.
	// /api/events has no audio-only upstream filter, so this caps the raw
	// mixed object+audio list before our type filter — in a very busy window
	// object events could crowd out audio, acceptable at household rates and
	// bounded by the time window.
	audioTimelineLimit = 500
)

// audioMarker is the lean timeline shape served by GET
// /api/cameras/{id}/audio-events: one Frigate audio-detection event reduced to
// what the scrubber's audio lane needs. End is null while the event is still
// active (Frigate's end_time null), which the FE draws out to the live edge.
type audioMarker struct {
	ID    string   `json:"id"`
	Label string   `json:"label"`
	Start float64  `json:"start"`
	End   *float64 `json:"end"`
}

// handleTimelineAudioEvents serves GET /api/cameras/{id}/audio-events?start=&end=
// — the windowed list of Frigate audio-detection events for [start,end),
// reshaped into lean timeline markers the scrubber renders as a thin activity
// lane directly under the review lane.
//
// The BFF queries Frigate's /api/events with cameras={id}, after=start-
// audioLookback (Frigate filters on start_time; the lookback catches an event
// that started just before the window but overlaps it), before=end, and
// limit=audioTimelineLimit, keeps only the audio-detection events
// (data.type == "audio"), then emits {id, label, start, end} with end null
// while the event is still active.
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
func (h *Handler) handleTimelineAudioEvents(w http.ResponseWriter, r *http.Request) {
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

	ctx, cancel := context.WithTimeout(r.Context(), audioTimelineTimeout)
	defer cancel()

	items, err := h.events.ListAudioEvents(ctx, events.AudioEventParams{
		Cameras: []string{id},
		After:   time.Unix(startN-int64(audioLookback.Seconds()), 0),
		Before:  time.Unix(endN, 0),
		Limit:   audioTimelineLimit,
	})
	if err != nil {
		status := http.StatusBadGateway
		code := "upstream_error"
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			status = http.StatusGatewayTimeout
			code = "upstream_timeout"
		}
		writeError(w, h.logger, status, code, "audio events upstream error", err)
		return
	}

	out := make([]audioMarker, 0, len(items))
	for _, it := range items {
		out = append(out, audioMarker{
			ID:    it.ID,
			Label: it.Label,
			Start: it.StartTime,
			End:   it.EndTime,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if err := json.NewEncoder(w).Encode(out); err != nil {
		h.logger.Error("encode timeline audio-events response", "error", err)
	}
}
