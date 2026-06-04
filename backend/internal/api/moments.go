package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/skua-app/skua/internal/events"
)

const momentsListTimeout = 5 * time.Second

// momentsResponse is the JSON envelope for GET /api/moments. Phase 1 has
// no pagination cursor: the source-event window is bounded by limit (see
// the handler comment), and clients re-fetch from the top.
type momentsResponse struct {
	Items []events.Moment `json:"items"`
}

// handleMoments serves GET /api/moments: a server-side grouping of recent
// Frigate events into per-camera time-clusters ("moments"). Phase 1 of
// the glance feature — read-only, no persistence, no seen-state, not yet
// surfaced in the UI.
//
// Query params:
//   - since: optional RFC3339 timestamp; events with started_at not
//     strictly after since are excluded before grouping.
//   - limit: optional positive integer (default eventsDefaultLimit,
//     clamped to eventsMaxLimit). This is the lookback window — the
//     number of source events fetched from Frigate, NOT the number of
//     moments returned. There is no pagination cursor in Phase 1.
func (h *Handler) handleMoments(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	limit := eventsDefaultLimit
	if s := q.Get("limit"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n <= 0 {
			writeError(w, h.logger, http.StatusBadRequest, "bad_request", "limit must be a positive integer", nil)
			return
		}
		if n > eventsMaxLimit {
			n = eventsMaxLimit
		}
		limit = n
	}

	var since time.Time
	if s := q.Get("since"); s != "" {
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			writeError(w, h.logger, http.StatusBadRequest, "bad_request", "since must be ISO 8601", err)
			return
		}
		since = t
	}

	params := events.ListParams{Limit: limit}

	ctx, cancel := context.WithTimeout(r.Context(), momentsListTimeout)
	defer cancel()

	resp, err := h.events.List(ctx, params)
	if err != nil {
		status := http.StatusBadGateway
		code := "upstream_error"
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			status = http.StatusGatewayTimeout
			code = "upstream_timeout"
		}
		writeError(w, h.logger, status, code, "events upstream error", err)
		return
	}

	out := momentsResponse{Items: events.GroupMoments(resp.Items, since)}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if err := json.NewEncoder(w).Encode(out); err != nil {
		h.logger.Error("encode moments response", "error", err)
	}
}
