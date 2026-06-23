// Frigate /api/review-backed glance "moments". Each review item is a
// per-camera activity segment that Frigate has already grouped from its
// own tracked-object detections; the BFF reshapes it into a Moment for
// the glance "while you were away" UI.
//
// Pre-flight notes on Frigate 0.17 /api/review JSON (probed 2026-06-15):
//   - id          — string like "<unix_seconds>.<frac>-<hash>"
//   - camera      — string
//   - start_time  — float64 unix seconds (with subseconds)
//   - end_time    — null while the segment is still active, else float64
//   - severity    — "alert" | "detection"
//   - thumb_path  — filesystem path on Frigate; NOT an HTTP endpoint, ignored
//   - data:
//     detections       — []string of tracked-object event ids inside the segment
//     objects          — []string raw Frigate labels (person, car, ...)
//     verified_objects — []string (ignored)
//     sub_labels       — []string (ignored)
//     zones            — []string of zone names from Frigate config
//     audio            — []string (ignored)
//     thumb_time       — float64 (ignored; UI uses detection-id thumbs)
//     metadata         — null (ignored)
//   - has_been_reviewed — bool; ignored, Skua keeps its own seen-state
//
// Query params we send: after (unix seconds, int), limit (int), and
// cameras (csv). Frigate also supports labels, zones, severity,
// reviewed, and before — none of which the glance path needs today.
package events

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ErrReviewNotFound is returned by GetReview when the upstream responds
// with 404. The glance preview handler distinguishes this from generic
// upstream failures so it can surface a 404 not_found to the client
// instead of 502 upstream_error.
var ErrReviewNotFound = errors.New("review not found")

// FrigateReviewItem matches the subset of Frigate's /api/review item we read.
type FrigateReviewItem struct {
	ID        string   `json:"id"`
	Camera    string   `json:"camera"`
	StartTime float64  `json:"start_time"`
	EndTime   *float64 `json:"end_time"`
	Severity  string   `json:"severity"`
	Data      struct {
		Detections []string `json:"detections"`
		Objects    []string `json:"objects"`
		Zones      []string `json:"zones"`
	} `json:"data"`
}

// ReviewParams are the supported BFF query filters for ListReview.
// After / Before are inclusive lower / upper bounds on the segment's
// start_time, in line with Frigate's /api/review semantics. Either
// may be zero to omit the bound.
type ReviewParams struct {
	Cameras []string
	After   time.Time // zero ⇒ no filter
	Before  time.Time // zero ⇒ no filter
	Limit   int
}

// Moment is the BFF shape served by GET /api/glance: one Frigate review
// segment, reshaped. ThumbEventID is the tracked-object event id whose
// snapshot the UI should use as the moment thumbnail (the latest
// detection in the segment by unix-seconds prefix).
type Moment struct {
	ID           string   `json:"id"`            // Frigate review id
	CamID        string   `json:"cam_id"`        // Frigate camera id
	StartedAt    string   `json:"started_at"`    // RFC3339 UTC
	EndedAt      *string  `json:"ended_at"`      // nil while the segment is still active
	Severity     string   `json:"severity"`      // "alert" | "detection"
	Kinds        []Kind   `json:"kinds"`         // normalised kinds, stable encounter order, deduped
	Labels       []string `json:"labels"`        // raw Frigate labels, deduped, sorted ascending
	Zones        []string `json:"zones"`         // zone names from Frigate config, as supplied
	DetectionIDs []string `json:"detection_ids"` // tracked-object event ids in the segment
	ThumbEventID string   `json:"thumb_event_id"`
}

// ListReview fetches a single page of review items from Frigate. The
// glance handler is the only caller; it always passes after (the window
// boundary) and limit (the per-call cap) and does not page. Non-200
// upstream responses surface as errors, in the same shape as List.
func (c *Client) ListReview(ctx context.Context, p ReviewParams) ([]FrigateReviewItem, error) {
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

	reqURL := c.baseURL + "/api/review"
	if encoded := q.Encode(); encoded != "" {
		reqURL = reqURL + "?" + encoded
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("frigate review: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("frigate review returned %d", resp.StatusCode)
	}

	var raw []FrigateReviewItem
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode review: %w", err)
	}
	return raw, nil
}

// ReviewItemToMoment is a pure mapper from a Frigate review item to a
// BFF Moment. No I/O, no time.Now reads. Kinds are derived from
// data.objects via KindFor in stable encounter order; labels are the
// distinct raw labels sorted ascending. zones and detection_ids are
// copied as supplied. ThumbEventID is the detection id whose
// unix-seconds prefix (the portion before the first '-') is the
// largest — the most recent tracked-object event in the segment. An
// empty detections list yields an empty ThumbEventID. end_time null or
// <= 0 leaves EndedAt nil.
func ReviewItemToMoment(r FrigateReviewItem) Moment {
	startedAt := timeFromUnix(r.StartTime).UTC().Format(time.RFC3339)

	var endedAt *string
	if r.EndTime != nil && *r.EndTime > 0 {
		s := timeFromUnix(*r.EndTime).UTC().Format(time.RFC3339)
		endedAt = &s
	}

	// Initialise as empty (non-nil) slices so json.Marshal emits [] not null
	// when the source review data is empty. A null here wedges the glance UI:
	// GlancePeek dereferences these arrays at render time. The dedup / sort /
	// encounter-order behaviour below is unchanged — only nil → empty.
	kinds := []Kind{}
	if len(r.Data.Objects) > 0 {
		seen := make(map[Kind]struct{})
		for _, label := range r.Data.Objects {
			k := KindFor(label)
			if _, ok := seen[k]; ok {
				continue
			}
			seen[k] = struct{}{}
			kinds = append(kinds, k)
		}
	}

	labels := []string{}
	if len(r.Data.Objects) > 0 {
		seen := make(map[string]struct{})
		for _, label := range r.Data.Objects {
			if _, ok := seen[label]; ok {
				continue
			}
			seen[label] = struct{}{}
			labels = append(labels, label)
		}
		sort.Strings(labels)
	}

	zones := []string{}
	if len(r.Data.Zones) > 0 {
		zones = append(zones, r.Data.Zones...)
	}

	detections := []string{}
	if len(r.Data.Detections) > 0 {
		detections = append(detections, r.Data.Detections...)
	}

	thumbID := ""
	if len(r.Data.Detections) > 0 {
		thumbID = r.Data.Detections[0]
		best := EventTimestampPrefix(thumbID)
		for _, id := range r.Data.Detections[1:] {
			ts := EventTimestampPrefix(id)
			if ts > best {
				best = ts
				thumbID = id
			}
		}
	}

	return Moment{
		ID:           r.ID,
		CamID:        r.Camera,
		StartedAt:    startedAt,
		EndedAt:      endedAt,
		Severity:     r.Severity,
		Kinds:        kinds,
		Labels:       labels,
		Zones:        zones,
		DetectionIDs: detections,
		ThumbEventID: thumbID,
	}
}

// GetReview fetches a single review segment by id from Frigate's
// /api/review/{id} endpoint. Frigate returns a JSON object (not an
// array) for the single-id form. A 404 upstream surfaces as
// ErrReviewNotFound so the caller can distinguish "not in Frigate"
// from a generic upstream failure.
func (c *Client) GetReview(ctx context.Context, id string) (FrigateReviewItem, error) {
	reqURL := c.baseURL + "/api/review/" + url.PathEscape(id)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return FrigateReviewItem{}, fmt.Errorf("build request: %w", err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return FrigateReviewItem{}, fmt.Errorf("frigate review: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		return FrigateReviewItem{}, ErrReviewNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return FrigateReviewItem{}, fmt.Errorf("frigate review returned %d", resp.StatusCode)
	}
	var item FrigateReviewItem
	if err := json.NewDecoder(resp.Body).Decode(&item); err != nil {
		return FrigateReviewItem{}, fmt.Errorf("decode review: %w", err)
	}
	return item, nil
}

// OpenPreview issues a Frigate preview.mp4 request for the supplied
// camera and [start, end] window (unix seconds, formatted with 6
// decimal places to match Frigate's URL grammar). method is the
// caller's HTTP method (typically GET or HEAD); rangeHeader, when
// non-empty, is forwarded as the upstream Range header so byte-range
// playback flows end-to-end. The returned *http.Response is the raw
// upstream response — the caller owns Body and is responsible for
// closing it. Any status code is returned to the caller; non-2xx is
// not treated as an error here.
func (c *Client) OpenPreview(ctx context.Context, method, camera string, start, end float64, rangeHeader string) (*http.Response, error) {
	reqURL := c.baseURL + "/api/" + url.PathEscape(camera) +
		"/start/" + strconv.FormatFloat(start, 'f', 6, 64) +
		"/end/" + strconv.FormatFloat(end, 'f', 6, 64) +
		"/preview.mp4"
	req, err := http.NewRequestWithContext(ctx, method, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	if rangeHeader != "" {
		req.Header.Set("Range", rangeHeader)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("frigate preview: %w", err)
	}
	return resp, nil
}

// EventTimestampPrefix parses the unix-seconds float prefix of a
// Frigate event id ("<unix>.<frac>-<hash>") — the same encoding
// detection ids inside review segments and tracked-object event ids
// share. Returns 0 on any parse failure so a malformed id sorts
// below well-formed ones. Exported for the events→review resolver
// (it needs the event start to pick a recall window for /api/review).
func EventTimestampPrefix(id string) float64 {
	dash := strings.Index(id, "-")
	if dash <= 0 {
		return 0
	}
	v, err := strconv.ParseFloat(id[:dash], 64)
	if err != nil {
		return 0
	}
	return v
}
