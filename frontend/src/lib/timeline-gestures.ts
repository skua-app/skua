// Recording-timeline POINTER GESTURE MODEL — pure arithmetic, no state, no
// effects, nothing Svelte. TimelineScrubber owns the pointer bookkeeping (which
// pointers are down, where they started, whether the press has travelled); this
// module owns the decisions that bookkeeping feeds, so each is testable without
// dispatching synthetic events at a mounted component.
//
// The decisions are deliberately separate:
//
//   resolveGesture     what a press MEANS — settled once, at pointerdown
//   exceedsClickSlop   whether a press has become a drag — sticky, per move
//   resolveWheel       what a wheel event MEANS — pan, zoom, or nothing at all
//
// All coordinates are CSS pixels measured from the track's left edge.

import type { TimelineMode } from '$lib/api'

// How far a press may travel and still count as a click rather than a drag.
// A THRESHOLD, never a timer: a click that shifts a pixel under an unsteady
// hand must still seek, and a drag that begins slowly must never seek to its
// press point when it ends somewhere else. 5px is the usual platform drag
// threshold and is comfortably below any deliberate drag.
export const CLICK_SLOP_PX = 5

// Half-width of the playhead's grab zone, from the centre of its line, for a
// MOUSE. The line itself is 2px, so this is "what the playhead occupies plus a
// modest margin" — a 16px-wide target, which is as much as a cursor needs and
// as little as keeps the rest of the track clickable.
export const PLAYHEAD_GRAB_PX = 8

// The same thing for a finger: 44px wide in total, per Apple's touch-target
// guidance. It is deliberately far wider than the visible grip, because the
// grip has to stay small enough not to cover the coverage, review and audio
// bands it sits over, while still being catchable without looking.
//
// One code path, two numbers. The mouse case is not a separate branch — it is
// this rule with the other constant.
export const PLAYHEAD_GRAB_TOUCH_PX = 22

// Which grab half-width applies to a given pointer. Anything that is not a
// mouse gets the touch target: a pen is aimed with a hand rather than with a
// cursor, and an unknown pointerType is likelier to be coarse than fine, so the
// generous value is the safe default in both cases.
export function grabHalfWidth(pointerType: string): number {
  return pointerType === 'mouse' ? PLAYHEAD_GRAB_PX : PLAYHEAD_GRAB_TOUCH_PX
}

// What a press on the track means. Settled at pointerdown and then fixed for
// the life of the gesture — see resolveGesture.
//
//   tape      follow mode's relative drag: the film moves under a centred
//             playhead, which seeks. The only gesture follow mode has.
//   playhead  fixed mode: the press landed on the playhead, so the drag moves
//             the playhead along a stationary track, which seeks.
//   pan       fixed mode: shift is held, so the drag moves the viewport and
//             the playhead keeps playing where it is.
//   track     fixed mode: a press on the track away from the playhead. It is
//             UNDECIDED at this point — the movement threshold decides. Under
//             the slop it is a click, which seeks to where it landed; past the
//             slop it becomes a pan. The two are mutually exclusive on one
//             latched flag, so neither can steal the other.
//   inert     does nothing at all, ever. Not a press meaning — resolveGesture
//             never returns it. The component moves into it when a gesture has
//             been superseded, such as the finger left over after a two-finger
//             gesture, which is somewhere new and must not resume anything.
export type Gesture = 'tape' | 'playhead' | 'pan' | 'track' | 'inert'

export type GesturePress = {
  mode: TimelineMode
  // Modifier state AT POINTERDOWN. Read once, deliberately: see below.
  shiftKey: boolean
  // Where the press landed, and where the playhead is drawn, both in pixels
  // from the track's left edge.
  pressX: number
  playheadX: number
  // PointerEvent.pointerType. It only selects how wide the playhead's grab zone
  // is — a finger needs 44px, a cursor does not. It is NOT what keeps a touch
  // tap inert; that gate lives at the other end of the gesture, on release.
  pointerType: string
}

// Decide what a press means, ONCE, at pointerdown.
//
// The whole point is that the answer is then fixed for the life of the gesture.
// Pressing shift halfway through a drag must not turn a seek into a pan under
// the user's hand, and releasing it must not turn a pan into a seek — the
// gesture would change meaning mid-flight, which is a subtler version of the
// bug that made `follow` implicit state in the first place. So the caller reads
// shiftKey here and never again for that gesture.
//
// Order is load-bearing where the cases overlap: shift wins over the playhead
// hit test, so shift+drag starting on the playhead pans rather than seeks. That
// is the one thing shift is still FOR, now that a bare track drag pans too — it
// resolves the case where the press lands on the handle but the user wants the
// track. Everywhere else the two are the same gesture, and the modifier is a
// redundant alias kept because it costs nothing.
export function resolveGesture({
  mode,
  shiftKey,
  pressX,
  playheadX,
  pointerType
}: GesturePress): Gesture {
  // Follow mode has exactly one pointer gesture and no modifier bindings: the
  // tape is already draggable, so shift means nothing and every press is the
  // relative drag it has always been.
  if (mode === 'follow') return 'tape'
  if (shiftKey) return 'pan'
  if (Math.abs(pressX - playheadX) <= grabHalfWidth(pointerType)) return 'playhead'
  return 'track'
}

// Has this press travelled far enough to stop being a click?
//
// The caller latches the result: once true it stays true for the rest of the
// gesture, even if the pointer comes back to where it started. That is what
// makes a slow drag out-and-back land as a drag rather than as a click on its
// origin — the distinction is total travel having exceeded the slop, not where
// the pointer happens to finish.
export function exceedsClickSlop(startX: number, x: number): boolean {
  return Math.abs(x - startX) > CLICK_SLOP_PX
}

// ---------------------------------------------------------------------------
// The wheel.
// ---------------------------------------------------------------------------

// One notch of a wheel reported in LINES rather than pixels (deltaMode 1),
// taken as the usual ~1 line of text. Pages (deltaMode 2) scale by the track
// width instead, which the caller passes in.
export const WHEEL_LINE_PX = 16

// Everything resolveWheel reads off a WheelEvent. Named rather than taking the
// event itself so the model can be exercised with plain objects.
export type WheelInput = {
  deltaX: number
  deltaY: number
  deltaMode: number
  shiftKey: boolean
}

// What a wheel event means.
//
//   pan    move the viewport by `pixels` of track, positive = later. The SAME
//          unit shift+wheel has always spoken in, because it is the same
//          request through the same writer.
//   zoom   change the span; `out` is the direction (away from the user = out).
//   none   do nothing, AND do not preventDefault — the page keeps its own
//          scroll rather than having the gesture swallowed to do nothing.
export type WheelIntent =
  | { kind: 'pan'; pixels: number }
  | { kind: 'zoom'; out: boolean }
  | { kind: 'none' }

// Normalise a delta to CSS pixels. deltaMode can report lines or pages rather
// than pixels, and a browser is free to pick either.
function toPixels(delta: number, deltaMode: number, trackWidth: number): number {
  if (deltaMode === 1) return delta * WHEEL_LINE_PX
  if (deltaMode === 2) return delta * trackWidth
  return delta
}

// What does this wheel event mean, in this mode?
//
// Three cases, in this order, and the order is the whole of the axis problem:
//
// 1. SHIFT WINS OUTRIGHT. Browsers disagree about which axis a shift+wheel
//    lands on — several relocate it to deltaX — so with shift held, deltaX
//    says nothing about direction and only says which field the browser chose.
//    Taking whichever axis moved (preferring deltaX, since that is the one a
//    relocating browser uses) is therefore correct HERE and nowhere else, and
//    resolving shift first is what confines that heuristic to the one case it
//    is true for. Fixed mode only, exactly as before: in follow mode shift has
//    no meaning, so the event is left entirely alone.
//
// 2. UNMODIFIED, HORIZONTAL-DOMINANT — a pan. Without shift, deltaX means what
//    it says, so the intent is read off the DOMINANT axis: |deltaX| > |deltaY|.
//    A Mac trackpad two-finger swipe sideways arrives with deltaX set and
//    deltaY 0 (or near it) and pans; a tilt wheel reports the same shape and
//    pans too; a mouse wheel has deltaX exactly 0 and never reaches this
//    branch. Fixed mode only — panning is a fixed-mode gesture, and follow
//    mode returns `none` rather than swallowing the event.
//
//    Fingers are never purely horizontal, so the boundary matters. It is at
//    45 degrees deliberately: deliberate gestures cluster near 0 (a sideways
//    swipe) and near 90 (a scroll), and 45 is the angle they visit least, so
//    the one place the answer flips is the one place nobody aims. A ratio
//    threshold would move that boundary INTO a region real gestures occupy —
//    a slightly-off vertical scroll — for no gain. A swipe that genuinely sits
//    on the diagonal gets some of each, frame by frame, in proportion to how
//    it is leaning; that is the same answer the two-finger gesture already
//    gives, where separation and midpoint are read independently off one
//    gesture rather than discriminated between.
//
// 3. EVERYTHING ELSE — a zoom, in both modes, about whatever the caller
//    decides to anchor on. The direction is deltaY's sign, so an event with NO
//    vertical component has no direction to give and means nothing: a wheel
//    event with both deltas zero returns `none` rather than being read as
//    "not greater than zero" and silently zooming in.
export function resolveWheel(e: WheelInput, mode: TimelineMode, trackWidth: number): WheelIntent {
  if (e.shiftKey) {
    if (mode !== 'fixed') return { kind: 'none' }
    const raw = e.deltaX !== 0 ? e.deltaX : e.deltaY
    return { kind: 'pan', pixels: toPixels(raw, e.deltaMode, trackWidth) }
  }
  if (Math.abs(e.deltaX) > Math.abs(e.deltaY)) {
    if (mode !== 'fixed') return { kind: 'none' }
    return { kind: 'pan', pixels: toPixels(e.deltaX, e.deltaMode, trackWidth) }
  }
  if (e.deltaY === 0) return { kind: 'none' }
  return { kind: 'zoom', out: e.deltaY > 0 }
}
