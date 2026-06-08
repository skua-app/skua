package events

import (
	"sort"
	"time"
)

// MomentGap is the maximum gap between two consecutive events' started_at
// values, on the same camera, that still keeps them in the same moment.
// Events from the same camera whose started_at fall more than MomentGap
// apart start a new moment.
const MomentGap = 5 * time.Minute

// Moment is the BFF's per-camera time-cluster of recent events. Phase 1
// of the glance feature: derived on demand from a slice of Items, no
// persistence and not yet surfaced in the UI. See docs/api-contract.md.
type Moment struct {
	CamID                 string   `json:"cam_id"`
	StartedAt             string   `json:"started_at"` // RFC3339 UTC, earliest event start in the cluster
	EndedAt               *string  `json:"ended_at"`   // null when any clustered event is still in progress
	Kinds                 []Kind   `json:"kinds"`      // distinct kinds in stable encounter order
	Labels                []string `json:"labels"`     // distinct raw labels, sorted ascending
	EventCount            int      `json:"event_count"`
	RepresentativeEventID string   `json:"representative_event_id"`
	RepresentativeHasClip bool     `json:"representative_has_clip"`
	// Events lists every detection in the cluster, sorted by started_at
	// descending (newest first). Length always equals EventCount.
	Events []Item `json:"events"`
}

// momentEvent is an Item with its parsed timestamps cached. Internal to
// the grouping pipeline.
type momentEvent struct {
	item    Item
	started time.Time
	ended   time.Time
	hasEnd  bool
}

// builtMoment carries a finished Moment plus its latest event started_at,
// used only for the final descending sort on the outer slice.
type builtMoment struct {
	moment      Moment
	latestStart time.Time
}

// GroupMoments collapses a slice of Items into per-camera time-clusters
// ("moments"). The function is pure: no I/O, no time.Now reads, no
// mutation of the input.
//
// If since is non-zero, any Item whose started_at is not strictly after
// since is dropped before grouping.
//
// Grouping is strictly per camera (never across cameras) and strictly by
// MomentGap on started_at; kind / label changes inside a cluster do not
// split a moment. Within each camera the events are sorted by started_at
// ascending, then walked: when the gap between consecutive started_at
// values exceeds MomentGap, a new moment begins; otherwise the current
// moment extends.
//
// Per moment:
//   - started_at is the earliest started_at in the cluster.
//   - ended_at is the latest ended_at among clustered events, or null when
//     any event in the cluster is still in progress (nil EndedAt).
//   - kinds are de-duplicated in stable encounter order.
//   - labels are de-duplicated and sorted ascending.
//   - representative_event_id is the id of the highest-score event; nil
//     scores rank below any real score; ties break by most recent
//     started_at. representative_has_clip is that event's has_clip.
//
// Moments are returned sorted by their latest event started_at
// descending (most recent moment first).
func GroupMoments(items []Item, since time.Time) []Moment {
	hasSince := !since.IsZero()
	byCam := make(map[string][]momentEvent)
	for _, it := range items {
		started, err := time.Parse(time.RFC3339, it.StartedAt)
		if err != nil {
			continue
		}
		if hasSince && !started.After(since) {
			continue
		}
		ev := momentEvent{item: it, started: started}
		if it.EndedAt != nil {
			if e, perr := time.Parse(time.RFC3339, *it.EndedAt); perr == nil {
				ev.ended = e
				ev.hasEnd = true
			}
		}
		byCam[it.CamID] = append(byCam[it.CamID], ev)
	}

	var clusters []builtMoment
	for camID, evs := range byCam {
		sort.Slice(evs, func(i, j int) bool {
			return evs[i].started.Before(evs[j].started)
		})

		var cluster []momentEvent
		flush := func() {
			if len(cluster) == 0 {
				return
			}
			clusters = append(clusters, buildMoment(camID, cluster))
			cluster = nil
		}

		for i, ev := range evs {
			if i > 0 && ev.started.Sub(evs[i-1].started) > MomentGap {
				flush()
			}
			cluster = append(cluster, ev)
		}
		flush()
	}

	sort.SliceStable(clusters, func(i, j int) bool {
		return clusters[i].latestStart.After(clusters[j].latestStart)
	})

	out := make([]Moment, len(clusters))
	for i, b := range clusters {
		out[i] = b.moment
	}
	return out
}

// buildMoment assembles a Moment from a non-empty cluster of events
// already sorted ascending by started_at.
func buildMoment(camID string, cluster []momentEvent) builtMoment {
	earliest := cluster[0].started
	latest := cluster[len(cluster)-1].started

	var endedAt *string
	anyMissingEnd := false
	var maxEnd time.Time
	for _, ev := range cluster {
		if !ev.hasEnd {
			anyMissingEnd = true
			break
		}
		if ev.ended.After(maxEnd) {
			maxEnd = ev.ended
		}
	}
	if !anyMissingEnd {
		s := maxEnd.UTC().Format(time.RFC3339)
		endedAt = &s
	}

	kindSeen := make(map[Kind]struct{})
	var kinds []Kind
	labelSeen := make(map[string]struct{})
	var labels []string

	repIdx := 0
	for i, ev := range cluster {
		if _, ok := kindSeen[ev.item.Kind]; !ok {
			kindSeen[ev.item.Kind] = struct{}{}
			kinds = append(kinds, ev.item.Kind)
		}
		if _, ok := labelSeen[ev.item.Label]; !ok {
			labelSeen[ev.item.Label] = struct{}{}
			labels = append(labels, ev.item.Label)
		}
		if i == 0 {
			continue
		}
		rep := cluster[repIdx]
		switch {
		case scoreLess(rep.item.Score, ev.item.Score):
			repIdx = i
		case scoresEqual(rep.item.Score, ev.item.Score):
			if ev.started.After(rep.started) {
				repIdx = i
			}
		}
	}
	sort.Strings(labels)

	// Reverse the ascending cluster into a newest-first Item slice. The
	// input slice itself is left untouched.
	reversed := make([]Item, len(cluster))
	for i, ev := range cluster {
		reversed[len(cluster)-1-i] = ev.item
	}

	return builtMoment{
		moment: Moment{
			CamID:                 camID,
			StartedAt:             earliest.UTC().Format(time.RFC3339),
			EndedAt:               endedAt,
			Kinds:                 kinds,
			Labels:                labels,
			EventCount:            len(cluster),
			RepresentativeEventID: cluster[repIdx].item.ID,
			RepresentativeHasClip: cluster[repIdx].item.HasClip,
			Events:                reversed,
		},
		latestStart: latest,
	}
}

// scoreLess reports whether a is "less than" b under the representative
// ordering: nil ranks below any real value; otherwise numeric compare.
func scoreLess(a, b *float64) bool {
	if a == nil && b == nil {
		return false
	}
	if a == nil {
		return true
	}
	if b == nil {
		return false
	}
	return *a < *b
}

func scoresEqual(a, b *float64) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}
