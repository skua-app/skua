import type { Moment } from '$lib/api'

// isMomentLive reports whether a moment cluster is still in progress
// — every clustered event has an open-ended ended_at. Used by the
// peek to decide whether the review modal should also offer an
// "Open live" shortcut into the focus view.
export function isMomentLive(moment: Moment): boolean {
  return moment.ended_at === null
}
