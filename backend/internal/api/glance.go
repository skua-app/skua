package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/skua-app/skua/internal/events"
	"github.com/skua-app/skua/internal/glance"
)

const (
	glanceListTimeout   = 5 * time.Second
	glanceLookbackLimit = 20
	glanceDefaultHours  = 24
	glanceMinHours      = 1
	glanceMaxHours      = 168
)

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
// payload. Pulls a fixed lookback of glanceLookbackLimit recent events,
// filters them by since = max(now - hours, cleared_at) where hours is
// the optional query param (default glanceDefaultHours, clamped to
// glanceMinHours..glanceMaxHours), groups the survivors into moments,
// and annotates each moment with seen = its representative_event_id
// is in the household seen-set. unseen_count is the count of moments
// whose seen flag is false.
func (h *Handler) handleGlance(w http.ResponseWriter, r *http.Request) {
	hours := glanceDefaultHours
	if s := r.URL.Query().Get("hours"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			hours = n
		}
	}
	if hours < glanceMinHours {
		hours = glanceMinHours
	}
	if hours > glanceMaxHours {
		hours = glanceMaxHours
	}

	ctx, cancel := context.WithTimeout(r.Context(), glanceListTimeout)
	defer cancel()

	resp, err := h.events.List(ctx, events.ListParams{Limit: glanceLookbackLimit})
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

	since := time.Now().Add(-time.Duration(hours) * time.Hour)
	if cleared := h.glance.ClearedAt(glance.ScopeHousehold); cleared.After(since) {
		since = cleared
	}

	moments := events.GroupMoments(resp.Items, since)
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

// glanceClearRequest is the JSON body for POST /api/glance/clear.
// Scope is optional and defaults to ScopeHousehold when absent.
type glanceClearRequest struct {
	Scope string `json:"scope"`
}

// handleGlanceClear serves POST /api/glance/clear: sets the scope's
// cleared_at watermark to now so subsequent GET /api/glance responses
// drop moments at or before this instant. An empty/absent body is
// allowed and resolves to the household scope.
func (h *Handler) handleGlanceClear(w http.ResponseWriter, r *http.Request) {
	scope := glance.ScopeHousehold
	if r.Body != nil {
		var body glanceClearRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
			writeError(w, h.logger, http.StatusBadRequest, "bad_request", "request body must be valid JSON", err)
			return
		}
		if body.Scope != "" {
			scope = body.Scope
		}
	}
	if err := h.glance.Clear(scope, time.Now()); err != nil {
		writeError(w, h.logger, http.StatusInternalServerError, "internal", "could not persist glance clear", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
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
