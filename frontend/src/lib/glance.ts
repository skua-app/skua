import type { EventItem, GlanceMoment, Moment } from '$lib/api'

// unseenRepresentativeIds returns the representative event id of every
// moment whose seen flag is false. Used by mark-all-seen to compute the
// set of ids to send to POST /api/glance/seen in one batch.
export function unseenRepresentativeIds(moments: GlanceMoment[]): string[] {
  return moments.filter((m) => !m.seen).map((m) => m.representative_event_id)
}

// momentRouteTarget decides how a moment row should open: 'focus' for
// a still-live cluster (no ended_at on any clustered event), 'clip'
// otherwise. The peek uses this to route taps either to the focus view
// or into EventModal.
export function momentRouteTarget(moment: Moment): 'focus' | 'clip' {
  return moment.ended_at === null ? 'focus' : 'clip'
}

// momentToEventItem synthesises an EventItem from a moment so the
// existing EventModal can display it without changes. started_at,
// label and kind are cluster-level approximations — the modal uses
// them only for display (heading text and the meta row), not for
// playback, so the loss of fidelity is acceptable. has_clip rides on
// the representative event's flag; has_snapshot is forced true
// because the BFF only surfaces events with a snapshot.
export function momentToEventItem(moment: Moment): EventItem {
  return {
    id: moment.representative_event_id,
    cam_id: moment.cam_id,
    started_at: moment.started_at,
    ended_at: moment.ended_at,
    duration_seconds: null,
    label: moment.labels[0] ?? '',
    kind: moment.kinds[0] ?? 'other',
    score: null,
    has_snapshot: true,
    has_clip: moment.representative_has_clip
  }
}
