// Frigate /api/events-backed audio-detection events. Frigate emits an event
// (data.type == "audio") whenever its audio detector recognises a sound class
// (speech, bark, ...). The BFF surfaces these as a discrete activity lane on
// the recording-timeline scrubber, mirroring the review-lane path.
package events

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// AudioEventParams are the supported BFF query filters for ListAudioEvents.
// After / Before are inclusive lower / upper bounds on the event's start_time,
// in line with Frigate's /api/events semantics. Either may be zero to omit the
// bound.
type AudioEventParams struct {
	Cameras []string
	After   time.Time // zero ⇒ no filter
	Before  time.Time // zero ⇒ no filter
	Limit   int
}

// ListAudioEvents fetches a window of events from Frigate's /api/events and
// returns only the audio-detection ones (data.type == "audio"). Unlike List it
// does NOT set has_snapshot — audio events may lack a snapshot and the lane
// needs none. Non-200 upstream responses surface as errors, in the same shape
// as List.
//
// v1 limitation: /api/events has no audio-only upstream filter, so Limit caps
// the raw mixed object+audio list BEFORE our type filter. In a very busy
// window object events could crowd out audio events, acceptable at household
// rates and bounded by the time window — same spirit as reviewTimelineLimit.
func (c *Client) ListAudioEvents(ctx context.Context, p AudioEventParams) ([]FrigateEvent, error) {
	q := url.Values{}
	if len(p.Cameras) > 0 {
		q.Set("cameras", strings.Join(p.Cameras, ","))
	}
	if !p.After.IsZero() {
		q.Set("after", strconv.FormatInt(p.After.Unix(), 10))
	}
	if !p.Before.IsZero() {
		q.Set("before", strconv.FormatInt(p.Before.Unix(), 10))
	}
	if p.Limit > 0 {
		q.Set("limit", strconv.Itoa(p.Limit))
	}

	reqURL := c.baseURL + "/api/events?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("frigate events: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("frigate events returned %d", resp.StatusCode)
	}

	var raw []FrigateEvent
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode events: %w", err)
	}

	out := make([]FrigateEvent, 0, len(raw))
	for _, e := range raw {
		if e.Data.Type == "audio" {
			out = append(out, e)
		}
	}
	return out, nil
}
