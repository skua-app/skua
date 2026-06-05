package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/skua-app/skua/internal/events"
	"github.com/skua-app/skua/internal/glance"
)

const glanceListTimeout = 5 * time.Second

// glanceMoment embeds events.Moment and adds a per-moment seen flag
// computed from the household scope's seen-set keyed on the moment's
// representative event id.
type glanceMoment struct {
	events.Moment
	Seen bool `json:"seen"`
}

// glanceResponse is the JSON envelope for GET /api/glance: the count
// of moments whose representative event has not yet been marked seen,
// and the surviving moments themselves (all of them, each carrying
// its seen flag).
type glanceResponse struct {
	UnseenCount int            `json:"unseen_count"`
	Moments     []glanceMoment `json:"moments"`
}

// glanceSeenRequest is the JSON body for POST /api/glance/seen.
// EventIDs is required; Scope is reserved for a future per-user split
// and defaults to ScopeHousehold when absent.
type glanceSeenRequest struct {
	EventIDs *[]string `json:"event_ids"`
	Scope    string    `json:"scope"`
}

// handleGlance serves GET /api/glance: the "while you were away"
// payload. Pulls a fixed lookback window of recent events (capped at
// eventsMaxLimit), groups them into moments with no time filter, and
// annotates each moment with seen = its representative_event_id is
// in the household seen-set. unseen_count is the count of moments
// whose seen flag is false. There is no client-supplied since or
// limit; the window is fixed.
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

	moments := events.GroupMoments(resp.Items, time.Time{})
	seenSet := h.glance.SeenSet(glance.ScopeHousehold)

	out := glanceResponse{Moments: make([]glanceMoment, len(moments))}
	for i, m := range moments {
		_, seen := seenSet[m.RepresentativeEventID]
		out.Moments[i] = glanceMoment{Moment: m, Seen: seen}
		if !seen {
			out.UnseenCount++
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if err := json.NewEncoder(w).Encode(out); err != nil {
		h.logger.Error("encode glance response", "error", err)
	}
}

// handleGlanceSeen serves POST /api/glance/seen: marks the supplied
// event ids as seen in the requested scope (default household). The
// /api group's cross-site guard already enforces Sec-Fetch-Site on
// this mutating route; no extra check here. There is no per-user
// identity in v1 — scope is accepted on the wire for forward-compat
// only.
func (h *Handler) handleGlanceSeen(w http.ResponseWriter, r *http.Request) {
	var body glanceSeenRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, h.logger, http.StatusBadRequest, "bad_request", "request body must be valid JSON", err)
		return
	}
	if body.EventIDs == nil {
		writeError(w, h.logger, http.StatusBadRequest, "bad_request", "event_ids is required", nil)
		return
	}
	scope := body.Scope
	if scope == "" {
		scope = glance.ScopeHousehold
	}
	ids := *body.EventIDs
	if len(ids) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err := h.glance.MarkSeen(scope, ids, time.Now()); err != nil {
		writeError(w, h.logger, http.StatusInternalServerError, "internal", "could not persist glance seen", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
