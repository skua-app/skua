package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/skua-app/skua/internal/events"
)

const glanceListTimeout = 5 * time.Second

// glanceResponse is the JSON envelope for GET /api/glance: the household
// last_seen (RFC3339 string, or null when never-seen), the count of
// unseen moments, and the moment slice itself.
type glanceResponse struct {
	LastSeen    *string         `json:"last_seen"`
	UnseenCount int             `json:"unseen_count"`
	Moments     []events.Moment `json:"moments"`
}

// glanceAckRequest is the JSON body for POST /api/glance/ack.
type glanceAckRequest struct {
	SeenThrough string `json:"seen_through"`
}

// glanceAckResponse is the JSON envelope for POST /api/glance/ack: the
// resulting household last_seen after the monotonic merge.
type glanceAckResponse struct {
	LastSeen *string `json:"last_seen"`
}

// handleGlance serves GET /api/glance: the "while you were away"
// payload. Pulls a fixed lookback window of recent events (capped at
// eventsMaxLimit), runs the Phase 1 moment grouping with since set to
// the stored last_seen, and returns the surviving moments plus their
// count. There is no client-supplied since or limit on this endpoint;
// the window is fixed.
func (h *Handler) handleGlance(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), glanceListTimeout)
	defer cancel()

	resp, err := h.events.List(ctx, events.ListParams{Limit: eventsMaxLimit})
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

	lastSeen := h.glance.LastSeen()
	moments := events.GroupMoments(resp.Items, lastSeen)

	out := glanceResponse{
		LastSeen:    formatLastSeen(lastSeen),
		UnseenCount: len(moments),
		Moments:     moments,
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if err := json.NewEncoder(w).Encode(out); err != nil {
		h.logger.Error("encode glance response", "error", err)
	}
}

// handleGlanceAck serves POST /api/glance/ack: parses an RFC3339
// seen_through, advances the stored last_seen monotonically via the
// glance store, and returns the resulting value. The /api group's
// cross-site guard already enforces Sec-Fetch-Site on this mutating
// route; no extra check here.
func (h *Handler) handleGlanceAck(w http.ResponseWriter, r *http.Request) {
	var body glanceAckRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, h.logger, http.StatusBadRequest, "bad_request", "request body must be valid JSON", err)
		return
	}
	if body.SeenThrough == "" {
		writeError(w, h.logger, http.StatusBadRequest, "bad_request", "seen_through is required", nil)
		return
	}
	t, err := time.Parse(time.RFC3339, body.SeenThrough)
	if err != nil {
		writeError(w, h.logger, http.StatusBadRequest, "bad_request", "seen_through must be ISO 8601", err)
		return
	}

	current, err := h.glance.Ack(t)
	if err != nil {
		writeError(w, h.logger, http.StatusInternalServerError, "internal", "could not persist glance ack", err)
		return
	}

	out := glanceAckResponse{LastSeen: formatLastSeen(current)}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if err := json.NewEncoder(w).Encode(out); err != nil {
		h.logger.Error("encode glance ack response", "error", err)
	}
}

// formatLastSeen converts a possibly-zero time.Time into the *string
// shape used in the glance API: nil when never-seen, otherwise an
// RFC3339 UTC string.
func formatLastSeen(t time.Time) *string {
	if t.IsZero() {
		return nil
	}
	s := t.UTC().Format(time.RFC3339)
	return &s
}
