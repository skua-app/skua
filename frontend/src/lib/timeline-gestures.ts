// Recording-timeline POINTER GESTURE MODEL — pure arithmetic, no state, no
// effects, nothing Svelte. TimelineScrubber owns the pointer bookkeeping (which
// pointers are down, where they started, whether the press has travelled); this
// module owns the two decisions that bookkeeping feeds, so both are testable
// without dispatching synthetic PointerEvents at a mounted component.
//
// The decisions are deliberately separate:
//
//   resolveGesture     what a press MEANS — settled once, at pointerdown
//   exceedsClickSlop   whether a press has become a drag — sticky, per move
//
// All coordinates are CSS pixels measured from the track's left edge.

import type { TimelineMode } from '$lib/api'

// How far a press may travel and still count as a click rather than a drag.
// A THRESHOLD, never a timer: a click that shifts a pixel under an unsteady
// hand must still seek, and a drag that begins slowly must never seek to its
// press point when it ends somewhere else. 5px is the usual platform drag
// threshold and is comfortably below any deliberate drag.
export const CLICK_SLOP_PX = 5

// Half-width of the playhead's grab zone, from the centre of its line. The line
// itself is 2px, so this is "what the playhead occupies plus a modest margin" —
// a 16px-wide target, sized for a mouse. It is NOT a touch target: the 44px
// handle, and the visible affordance that has to come with it, are a separate
// piece of work.
export const PLAYHEAD_GRAB_PX = 8

// What a press on the track means. Settled at pointerdown and then fixed for
// the life of the gesture — see resolveGesture.
//
//   tape      follow mode's relative drag: the film moves under a centred
//             playhead, which seeks. The only gesture follow mode has.
//   playhead  fixed mode: the press landed on the playhead, so the drag moves
//             the playhead along a stationary track, which seeks.
//   pan       fixed mode: shift is held, so the drag moves the viewport and
//             the playhead keeps playing where it is.
//   idle      fixed mode: a press on empty track. It moves nothing. If it
//             never travels past the slop it becomes a click, which seeks to
//             where it landed; if it does travel, it does nothing at all.
//             Panning is an explicit gesture, so a bare drag is inert.
export type Gesture = 'tape' | 'playhead' | 'pan' | 'idle'

export type GesturePress = {
  mode: TimelineMode
  // Modifier state AT POINTERDOWN. Read once, deliberately: see below.
  shiftKey: boolean
  // Where the press landed, and where the playhead is drawn, both in pixels
  // from the track's left edge.
  pressX: number
  playheadX: number
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
// hit test, so shift+drag starting on the playhead pans rather than seeks. An
// explicit modifier beats a positional guess.
export function resolveGesture({ mode, shiftKey, pressX, playheadX }: GesturePress): Gesture {
  // Follow mode has exactly one pointer gesture and no modifier bindings: the
  // tape is already draggable, so shift means nothing and every press is the
  // relative drag it has always been.
  if (mode === 'follow') return 'tape'
  if (shiftKey) return 'pan'
  if (Math.abs(pressX - playheadX) <= PLAYHEAD_GRAB_PX) return 'playhead'
  return 'idle'
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
