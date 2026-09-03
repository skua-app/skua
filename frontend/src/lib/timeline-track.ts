// Recording-timeline TRACK LAYOUT — pure arithmetic, no state, no effects,
// nothing Svelte. TimelineScrubber owns the DOM, the reactive props and the
// pointer bookkeeping; this module owns the numbers it draws with, so the
// layout is testable without mounting a component.
//
// The parts, in the order the track stacks them:
//
//   chooseTickStep      which round interval the labelled ticks land on
//   tickPositions       where those ticks are, in seconds and in fractions
//   microTickFractions  the finer unlabelled subdivision between them
//   hourLineFractions   whole-hour dividers, on their own fixed grid
//   clampLabelPx        keeping the first / last label off the track edges
//   bandGeometry        one lane band mapped onto the viewport
//   pointerDistance     two-pointer geometry for the pinch gesture
//   pointerMidX
//
// All times are wall-clock unix SECONDS. Positions come back as fractions in
// [0,1] across the drawn window, except clampLabelPx which speaks CSS pixels
// because a label's half-width is a pixel quantity and cannot be anything else.
// Nothing here reads the clock, the DOM, or a locale: the labels themselves are
// formatted by the component, which is where a locale-dependent string belongs.

import { timeToFraction } from './timeline'

// The label STEP adapts to the zoom span AND the available width so labels stay
// useful at every level: a ladder of round intervals (1m … 12h), choosing the
// smallest entry whose count across the viewport (span / step) fits the pixel
// budget (maxLabels = trackWidth / TICK_MIN_PX).
export const TICK_STEPS = [60, 120, 300, 600, 900, 1800, 3600, 7200, 10800, 21600, 43200]
export const TICK_MIN_PX = 56

export function chooseTickStep(span: number, width: number): number {
  // Before the ResizeObserver fires width is 0; fall back to ~10 labels' worth
  // of budget so the step is sane rather than dividing by a zero label count.
  const maxLabels = width > 0 ? Math.max(1, Math.floor(width / TICK_MIN_PX)) : 10
  for (const step of TICK_STEPS) {
    if (span / step <= maxLabels) return step
  }
  return TICK_STEPS[TICK_STEPS.length - 1]!
}

// Where one labelled tick sits: the absolute second it marks, and that second
// as a position across the drawn window. The LABEL is not here — see the module
// note above.
export type TickPosition = { tSec: number; fraction: number }

// Labelled tick positions across the window, at the given step.
//
// Ticks land on ABSOLUTE multiples of that step — NO index-based decimation.
// Decimating by array index would shift which absolute minutes survive as the
// window pans, making the labels jitter between e.g. :41/:43 and :40/:42;
// absolute marks slide smoothly instead.
export function tickPositions(
  windowStart: number,
  windowEnd: number,
  step: number
): TickPosition[] {
  if (windowEnd <= windowStart) return []
  const out: TickPosition[] = []
  // First step boundary at or after windowStart.
  const first = Math.ceil(windowStart / step) * step
  for (let t = first; t <= windowEnd; t += step) {
    out.push({ tSec: t, fraction: timeToFraction(t, windowStart, windowEnd) })
  }
  return out
}

// Micro-ticks: finer unlabelled marks subdividing each labelled interval.
// MICRO_DIVISIONS=4 quarters each major interval — a first-pass subdivision
// count, tunable. microStep stays integer for every TICK_STEPS entry, so the
// "is this also a major?" modulo test below is exact.
export const MICRO_DIVISIONS = 4
// Hide micro-ticks once their pixel spacing drops below this, so wide zooms
// stay clean instead of crowding. ~9px is a first pass — tune on device.
export const MIN_MICRO_PX = 9

// Micro-tick positions across the window, derived from the SAME major step the
// labelled ticks used — passed in rather than chosen again here, so the two
// ladders can never diverge as the window pans.
export function microTickFractions(
  windowStart: number,
  windowEnd: number,
  majorStep: number,
  trackWidth: number
): number[] {
  if (windowEnd <= windowStart) return []
  const span = windowEnd - windowStart
  const microStep = majorStep / MICRO_DIVISIONS
  // Pixel-spacing guard: drop the whole micro layer when it would crowd.
  if ((microStep / span) * trackWidth < MIN_MICRO_PX) return []
  const out: number[] = []
  const first = Math.ceil(windowStart / microStep) * microStep
  for (let t = first; t <= windowEnd; t += microStep) {
    // Skip marks that are also major boundaries — those get the labelled tick.
    if (t % majorStep === 0) continue
    out.push(timeToFraction(t, windowStart, windowEnd))
  }
  return out
}

// Hour-boundary dividers, computed from TIME on a fixed 3600s grid (NOT from
// any band geometry, whose straddling-edge x0/x1 are clamped to [0,1] and so
// are not real boundaries). Walks whole-hour marks across the window like
// ticks. Keep only STRICTLY interior lines so a boundary landing exactly on a
// window edge does not draw a spurious divider pinned at 0 or 1.
export const HOUR = 3600

export function hourLineFractions(windowStart: number, windowEnd: number): number[] {
  if (windowEnd <= windowStart) return []
  const out: number[] = []
  const first = Math.ceil(windowStart / HOUR) * HOUR
  for (let t = first; t <= windowEnd; t += HOUR) {
    const fraction = timeToFraction(t, windowStart, windowEnd)
    if (fraction > 0 && fraction < 1) out.push(fraction)
  }
  return out
}

// Approximate half-width of a labelled tick in px. The first/last labels are
// clamped so their centre stays this far inside the track, keeping them from
// clipping under .track's overflow:hidden. ~22px is a tune-on-device guess.
export const LABEL_HALF_W = 22

// Clamp a tick's label centre (in px across the track) to stay fully visible.
// The tick MARK itself stays at the true fraction; only the label clamps.
export function clampLabelPx(fraction: number, trackWidth: number): number {
  const x = fraction * trackWidth
  const hi = trackWidth - LABEL_HALF_W
  if (hi < LABEL_HALF_W) return x // track too narrow to clamp meaningfully
  return Math.min(Math.max(x, LABEL_HALF_W), hi)
}

// One lane band mapped onto the viewport: where it starts and how wide it is,
// both as fractions of the drawn window.
export type BandGeometry = { x0: number; width: number }

// Map a time range onto the viewport. timeToFraction clamps to [0,1], so a
// range fully outside the window collapses to zero width and comes back as
// null (x1 <= x0) rather than as a degenerate band the caller has to test for.
//
// Geometry only. What a band IS — its id, its severity, whether an active
// segment's null end resolves to the live edge — stays with each lane, because
// those are three different things that happen to be drawn as rectangles.
export function bandGeometry(
  start: number,
  end: number,
  windowStart: number,
  windowEnd: number
): BandGeometry | null {
  const x0 = timeToFraction(start, windowStart, windowEnd)
  const x1 = timeToFraction(end, windowStart, windowEnd)
  if (x1 <= x0) return null
  return { x0, width: x1 - x0 }
}

// Distance between the two active pointers (used by the pinch gesture). The
// caller keeps the pointerId -> clientX map and passes its values in; fewer
// than two pointers is not a pinch and has no separation.
export function pointerDistance(xs: number[]): number {
  if (xs.length < 2) return 0
  return Math.abs(xs[0]! - xs[1]!)
}

// Midpoint between the two active pointers, in client x. The pan half of the
// two-finger gesture is entirely this value moving.
export function pointerMidX(xs: number[]): number {
  if (xs.length < 2) return xs[0] ?? 0
  return (xs[0]! + xs[1]!) / 2
}
