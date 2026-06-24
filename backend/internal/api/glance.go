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
	glanceListTimeout       = 5 * time.Second
	glanceDefaultHours      = 24
	glanceMinHours          = 1
	glanceMaxHours          = 168
	glanceDefaultMaxMoments = 20
	glanceMaxMomentsCeil    = 200
	// glanceEmptyOverfetchFactor bounds the upstream review page so that,
	// after empty (no-detection) moments are filtered out, the filter can
	// still surface up to maxMoments real moments. maxMoments is the OUTPUT
	// cap; without over-fetching, a window full of empties would starve the
	// list. The fetched page is min(maxMoments * factor, glanceMaxMomentsCeil)
	// so the upstream request stays bounded.
	glanceEmptyOverfetchFactor = 4
)

// glanceMoment embeds events.Moment and adds a per-moment seen flag
// computed from the household scope's seen-set keyed on the moment id
// (the Frigate review segment id), OR from the seen_through watermark
// (moments whose started_at sits at or before the watermark are also
// reported as seen).
type glanceMoment struct {
	events.Moment
	Seen bool `json:"seen"`
}

// glanceResponse is the JSON envelope for GET /api/glance: the count
// of moments not yet marked seen, and the surviving moments themselves
// (all of them, each carrying its seen flag).
type glanceResponse struct {
	UnseenCount int            `json:"unseen_count"`
	Moments     []glanceMoment `json:"moments"`
}

// glanceSeenRequest is the JSON body for POST /api/glance/seen.
// EventIDs carries the ids the client considers seen; in v1 these are
// the review-sourced moment ids. The field is intentionally not
// renamed this phase so the existing frontend continues to send them
// under the same key. Scope is reserved for a future per-user split
// and defaults to ScopeHousehold when absent.
type glanceSeenRequest struct {
	EventIDs *[]string `json:"event_ids"`
	Scope    string    `json:"scope"`
}

// handleGlance serves GET /api/glance: the "while you were away"
// payload. Fetches Frigate review segments via /api/review with
// after = now - hours and limit = max, reshapes each into a Moment,
// and annotates each moment with seen. seen is true when the
// moment's id sits in the household seen-set, OR when its
// started_at is at or before the household seen_through watermark.
// unseen_count is the count of moments whose seen flag is false.
// `hours` is the optional query param (default glanceDefaultHours,
// clamped to glanceMinHours..glanceMaxHours). `max` is the optional
// output cap (default glanceDefaultMaxMoments, clamped to
// 1..glanceMaxMomentsCeil) and is forwarded to Frigate as the upstream
// limit; Frigate /api/review already returns segments newest-first.
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

	after := time.Now().Add(-time.Duration(hours) * time.Hour)

	// Over-fetch upstream so the empty-moment filter below still has enough
	// real moments to fill maxMoments. The page is bounded by the ceiling.
	upstreamLimit := min(maxMoments*glanceEmptyOverfetchFactor, glanceMaxMomentsCeil)

	reviewItems, err := h.events.ListReview(ctx, events.ReviewParams{
		After: after,
		Limit: upstreamLimit,
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

	// Defensive truncate in case upstream ignores limit; keep Frigate's
	// newest-first order.
	if len(reviewItems) > upstreamLimit {
		reviewItems = reviewItems[:upstreamLimit]
	}

	seenSet := h.glance.SeenSet(glance.ScopeHousehold)
	seenThrough := h.glance.SeenThrough(glance.ScopeHousehold)

	// Build moments, skipping empty motion-only segments (no tracked-object
	// detections) so the "while you were away" list never shows a moment with
	// nothing to look at. Preserve newest-first order, then truncate the
	// surviving moments to maxMoments and compute unseen_count over that final
	// list so the badge count matches what the client renders.
	out := glanceResponse{Moments: make([]glanceMoment, 0, maxMoments)}
	for _, it := range reviewItems {
		m := events.ReviewItemToMoment(it)
		if len(m.DetectionIDs) == 0 {
			continue
		}
		_, seen := seenSet[m.ID]
		if !seen && !seenThrough.IsZero() {
			if t, perr := time.Parse(time.RFC3339, m.StartedAt); perr == nil && !t.After(seenThrough) {
				seen = true
			}
		}
		out.Moments = append(out.Moments, glanceMoment{Moment: m, Seen: seen})
		if len(out.Moments) >= maxMoments {
			break
		}
	}
	for _, m := range out.Moments {
		if !m.Seen {
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
// ids as seen in the requested scope (default household). The seen
// store is id-agnostic; in v1 these ids are the review-sourced moment
// ids. The /api group's cross-site guard already enforces
// Sec-Fetch-Site on this mutating route; no extra check here. There is
// no per-user identity in v1 — scope is accepted on the wire for
// forward-compat only.
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
