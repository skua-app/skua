import type { EventItem, GlanceMoment, Moment } from '$lib/api'

// unseenRepresentativeIds returns the representative event id of every
// moment whose seen flag is false. Used by mark-all-seen to compute the
// set of ids to send to POST /api/glance/seen in one batch.
export function unseenRepresentativeIds(moments: GlanceMoment[]): string[] {
  return moments.filter((m) => !m.seen).map((m) => m.representative_event_id)
}

// isMomentLive reports whether a moment cluster is still in progress
// — every clustered event has an open-ended ended_at. Used by the
// peek to decide whether the review modal should also offer an
// "Open live" shortcut into the focus view.
export function isMomentLive(moment: Moment): boolean {
  return moment.ended_at === null
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
