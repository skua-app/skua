package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
)

const (
	// recordingsLookback widens the lower bound on Frigate's /api/{cam}/recordings
	// query. Frigate filters segments on start_time, so a ~10s segment that
	// STARTED just before the requested window but straddles its left edge would
	// be missed without a small lookback. 60s comfortably covers a segment that
	// began just before the window's left edge.
	recordingsLookback = 60 * time.Second
	// recordingsMaxBytes caps the upstream recordings JSON the BFF will read. A
	// windowed span lists at most a few hundred ~10s segments; 8 MiB is a
	// generous ceiling that still bounds a runaway or hostile upstream response.
	recordingsMaxBytes = 8 << 20
	// COVERAGE_GAP_MERGE is the maximum gap (seconds) between two adjacent
	// recorded segments the coalescer will bridge into a single coverage band.
	// Frigate's segments are nominally contiguous but carry sub-second boundary
	// jitter; merging across a small gap keeps continuous recording from
	// fragmenting into a comb of hairline bands, while a real recording gap
	// (motion-only cameras can leave minutes unrecorded) stays a visible gap.
	COVERAGE_GAP_MERGE = 5 * time.Second
)

// frigateRecordingSegment is one entry of Frigate's per-segment recordings
// list (/api/{cam}/recordings). Only the two fields the BFF coalesces are
// decoded; segment_size, motion, objects, duration, and id are ignored.
type frigateRecordingSegment struct {
	StartTime float64 `json:"start_time"`
	EndTime   float64 `json:"end_time"`
}

// coverageSegment is one coalesced coverage range the BFF serves: a span of
// continuous recorded footage. Gaps are the spans BETWEEN entries — the FE
// renders the entries as filled track and leaves the gaps empty.
type coverageSegment struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
}

// handleTimelineRecordings serves GET /api/cameras/{id}/recordings?start=&end=
// — the windowed recording-coverage lane for the recording-timeline scrubber.
// Frigate's /api/{cam}/recordings returns the raw ~10s recording segments for
// [after,before); the BFF coalesces contiguous segments (bridging gaps up to
// COVERAGE_GAP_MERGE) into a small set of coverage ranges the scrubber renders
// as a neutral recorded fill, leaving real gaps as empty track.
//
// The BFF queries Frigate with after=start-recordingsLookback (Frigate filters
// on start_time; the lookback catches a segment that started just before the
// window's left edge and straddles it) and before=end, then decodes the
// segments, sorts them by start (upstream order is not assumed), and walks them
// maintaining one open band: a next segment whose start is within
// COVERAGE_GAP_MERGE of the current band's end extends the band; otherwise the
// band closes and a new one opens.
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
//
// A non-2xx upstream status is passed through with an empty JSON array so the
// FE degrades to no coverage (blank lane) rather than erroring.
func (h *Handler) handleTimelineRecordings(w http.ResponseWriter, r *http.Request) {
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

	ctx, cancel := context.WithTimeout(r.Context(), timelineVODTimeout)
	defer cancel()

	after := strconv.FormatInt(startN-int64(recordingsLookback.Seconds()), 10)
	before := strconv.FormatInt(endN, 10)
	upstream, err := h.frigate.GetRecordings(ctx, id, after, before)
	if err != nil {
		// Mirror handleRecordingsSummary's client-disconnect branch: a cancelled
		// request context (typically the browser walked away from the timeline
		// before the fetch landed) is not an upstream error and there is no one
		// to receive a 502. Check Canceled BEFORE DeadlineExceeded.
		if errors.Is(err, context.Canceled) && r.Context().Err() == context.Canceled {
			h.logger.Debug("timeline recordings cancelled by client", "cam", id)
			return
		}
		status := http.StatusBadGateway
		code := "upstream_error"
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			status = http.StatusGatewayTimeout
			code = "upstream_timeout"
		}
		writeError(w, h.logger, status, code, "recordings upstream error", err)
		return
	}
	defer func() { _ = upstream.Body.Close() }()

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")

	// Non-2xx upstream: pass the status through with an empty array body so the
	// FE degrades to "no coverage" rather than erroring.
	if upstream.StatusCode < 200 || upstream.StatusCode >= 300 {
		w.WriteHeader(upstream.StatusCode)
		_ = json.NewEncoder(w).Encode([]coverageSegment{})
		return
	}

	var segs []frigateRecordingSegment
	if err := json.NewDecoder(io.LimitReader(upstream.Body, recordingsMaxBytes)).Decode(&segs); err != nil {
		writeError(w, h.logger, http.StatusBadGateway, "upstream_error", "decode recordings", err)
		return
	}

	// Upstream order is not guaranteed; sort ascending by start before coalescing.
	sort.Slice(segs, func(i, j int) bool { return segs[i].StartTime < segs[j].StartTime })

	// Coalesce contiguous segments into coverage bands, bridging gaps up to
	// COVERAGE_GAP_MERGE. Walk the sorted segments keeping one open band: a
	// segment whose start is within the merge threshold of the current band's
	// end extends it (end = max so a fully-overlapping segment can't shrink the
	// band); otherwise the band closes and a new one opens. Non-nil so an empty
	// result serialises as [] not null.
	gap := COVERAGE_GAP_MERGE.Seconds()
	out := []coverageSegment{}
	for _, s := range segs {
		if n := len(out); n > 0 && s.StartTime-out[n-1].End <= gap {
			if s.EndTime > out[n-1].End {
				out[n-1].End = s.EndTime
			}
			continue
		}
		out = append(out, coverageSegment{Start: s.StartTime, End: s.EndTime})
	}

	if err := json.NewEncoder(w).Encode(out); err != nil {
		h.logger.Error("encode timeline recordings response", "error", err)
	}
}
