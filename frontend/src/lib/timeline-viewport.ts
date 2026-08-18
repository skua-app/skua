// Recording-timeline VIEWPORT MODEL — pure arithmetic, no state, no effects,
// nothing Svelte. The route (cam/[id]/timeline/+page.svelte) owns the reactive
// state and the writers; this module owns the numbers those writers compute, so
// the model is testable without mounting a component.
//
// The model has three parts:
//   viewport  viewStart + viewSpan — the only thing that decides what is drawn
//   playhead  position — what is playing
//   follow    whether the viewport tracks the playhead
//
// While follow is on the viewport is kept centred on the playhead, so
// viewStart === centredViewStart(position, viewSpan) and the scrubber draws the
// playhead dead centre. All times are wall-clock unix SECONDS; spans are
// seconds. Nothing here reads the clock or the DOM.

// Default viewport span on entry: the last hour.
export const DEFAULT_VIEW_SPAN = 3600
// Zoom bounds for the viewport span (pinch / wheel). 10 min keeps a useful
// minimum context; 12 h is the widest pan the scrubber still resolves.
export const MIN_SPAN = 10 * 60
export const MAX_SPAN = 12 * 3600

// Zoom: turn a requested absolute span (a pinch distance ratio or a wheel step,
// either of which can be fractional and unbounded) into a legal viewport span —
// clamped to [MIN_SPAN, MAX_SPAN] and rounded to a whole second.
export function clampViewSpan(span: number): number {
  return Math.round(Math.max(MIN_SPAN, Math.min(MAX_SPAN, span)))
}

// The viewport left edge that puts the playhead dead centre. A pure function of
// (position, viewSpan) — never an accumulation — so calling it repeatedly at any
// rate (a pinch fires it on every pointermove) cannot drift.
export function centredViewStart(position: number, viewSpan: number): number {
  return position - viewSpan / 2
}

// Clamp the viewport CENTRE to the live edge. While the viewport was derived
// from the playhead this bound was emergent: the window was a function of the
// playhead and the playhead was clamped to liveEdge. A viewport that is real
// state has no such glue, so the bound is stated here.
//
// Deliberately one-sided, and on the centre rather than on the window end: the
// window legitimately extends up to viewSpan/2 PAST liveEdge, and that overhang
// is what draws the scrubber's right-hand hatch band and parks the LIVE capsule
// inside the track. There is no floor-side clamp because the caller's playback
// floor is a permissive fallback until the recording summary lands, and a ?t=
// deep-link is deliberately not clamped by it — clamping the viewport by it
// would knock an older event's entry window off-centre.
export function clampViewStart(start: number, viewSpan: number, liveEdge: number): number {
  const maxStart = liveEdge - viewSpan / 2
  return start > maxStart ? maxStart : start
}

// Clamp a seek target to the PLAYABLE domain. The playhead is bounded by what
// can actually be played, never by the viewport (which follows it), so a drag
// that runs off either end parks the playhead on the bound rather than seeking
// into nothing.
export function clampPlayhead(t: number, playbackFloor: number, liveEdge: number): number {
  return t < playbackFloor ? playbackFloor : t > liveEdge ? liveEdge : t
}
