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
	glanceListTimeout = 5 * time.Second
	// glancePageLimit is the per-Frigate-call page size used by the
	// backward pagination loop. Same value as eventsMaxLimit (Frigate's
	// observed per-request cap).
	glancePageLimit = 200
	// glanceMaxPages caps how many pages the loop will fetch before it
	// surrenders, even if the `hours` window is not yet covered. At
	// glancePageLimit per page this is 1000 source events.
	glanceMaxPages          = 5
	glanceDefaultHours      = 24
	glanceMinHours          = 1
	glanceMaxHours          = 168
	glanceDefaultMaxMoments = 20
	glanceMaxMomentsCeil    = 200
)

// glanceMoment embeds events.Moment and adds a per-moment seen flag
// computed from the household scope's seen-set keyed on the moment's
// representative event id, OR from the seen_through watermark
// (moments whose newest event start sits at or before the watermark
// are also reported as seen).
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
// payload. Walks Frigate backwards (page size glancePageLimit, up to
// glanceMaxPages pages for safety) until the source events cover the
// since = now - hours window, groups the survivors into moments,
// truncates to the newest `max` moments, and annotates each moment
// with seen. seen is true when the moment's representative_event_id
// sits in the household seen-set, OR when its newest event start is
// at or before the household seen_through watermark. unseen_count is
// the count of moments whose seen flag is false. `hours` is the
// optional query param (default glanceDefaultHours, clamped to
// glanceMinHours..glanceMaxHours). `max` is the optional output cap
// (default glanceDefaultMaxMoments, clamped to 1..glanceMaxMomentsCeil)
// — it bounds the number of moments returned, NOT the source events
// fetched.
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

	maxMoments := glanceDefaultMaxMoments
	if s := r.URL.Query().Get("max"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			maxMoments = n
		}
	}
	if maxMoments < 1 {
		maxMoments = 1
	}
	if maxMoments > glanceMaxMomentsCeil {
		maxMoments = glanceMaxMomentsCeil
	}

	ctx, cancel := context.WithTimeout(r.Context(), glanceListTimeout)
	defer cancel()

	since := time.Now().Add(-time.Duration(hours) * time.Hour)

	var allItems []events.Item
	var before time.Time
	for page := 0; page < glanceMaxPages; page++ {
		params := events.ListParams{Limit: glancePageLimit}
		if !before.IsZero() {
			params.Before = before
		}
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
		allItems = append(allItems, resp.Items...)
		// Short page or no cursor ⇒ end of history.
		if resp.NextBefore == nil {
			break
		}
		// Window fully covered: oldest item (last, since Frigate returns
		// newest-first) sits at or below since.
		if len(resp.Items) > 0 {
			oldest := resp.Items[len(resp.Items)-1]
			if t, perr := time.Parse(time.RFC3339, oldest.StartedAt); perr == nil && !t.After(since) {
				break
			}
		}
		next, perr := time.Parse(time.RFC3339, *resp.NextBefore)
		if perr != nil {
			// Unparseable cursor: degrade gracefully rather than burn the
			// safety budget on guaranteed-failing requests.
			break
		}
		before = next
	}

	moments := events.GroupMoments(allItems, since)
	if len(moments) > maxMoments {
		moments = moments[:maxMoments]
	}
	seenSet := h.glance.SeenSet(glance.ScopeHousehold)
	seenThrough := h.glance.SeenThrough(glance.ScopeHousehold)

	out := glanceResponse{Moments: make([]glanceMoment, len(moments))}
	for i, m := range moments {
		_, seen := seenSet[m.RepresentativeEventID]
		if !seen && !seenThrough.IsZero() {
			// Fall back to the watermark: a moment is seen if its
			// newest event started at or before seen_through. Use the
			// first event in m.Events (newest-first) when available;
			// otherwise fall back to the moment's earliest started_at
			// so a malformed cluster still produces a defined answer.
			newest := m.StartedAt
			if len(m.Events) > 0 {
				newest = m.Events[0].StartedAt
			}
			if t, perr := time.Parse(time.RFC3339, newest); perr == nil && !t.After(seenThrough) {
				seen = true
			}
		}
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

// glanceSeenAllRequest is the JSON body for POST /api/glance/seen-all.
// Scope is optional and defaults to ScopeHousehold when absent.
type glanceSeenAllRequest struct {
	Scope string `json:"scope"`
}

// handleGlanceSeenAll serves POST /api/glance/seen-all: advances the
// scope's seen_through watermark to now so subsequent GET /api/glance
// responses render every moment at or before this instant as seen
// (moments are NOT dropped from the list). An empty/absent body is
// allowed and resolves to the household scope.
func (h *Handler) handleGlanceSeenAll(w http.ResponseWriter, r *http.Request) {
	scope := glance.ScopeHousehold
	if r.Body != nil {
		var body glanceSeenAllRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
			writeError(w, h.logger, http.StatusBadRequest, "bad_request", "request body must be valid JSON", err)
			return
		}
		if body.Scope != "" {
			scope = body.Scope
		}
	}
	if err := h.glance.MarkAllSeen(scope, time.Now()); err != nil {
		writeError(w, h.logger, http.StatusInternalServerError, "internal", "could not persist glance seen-all", err)
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
