import { describe, it, expect } from 'vitest'
import {
  CLICK_SLOP_PX,
  PLAYHEAD_GRAB_PX,
  PLAYHEAD_GRAB_TOUCH_PX,
  grabHalfWidth,
  resolveGesture,
  exceedsClickSlop,
  type Gesture
} from './timeline-gestures'

// The gesture matrix, as code. Each case below is one row of it, so a change to
// what a press means fails here rather than only on a device.

const PLAYHEAD_X = 400

function press(over: Partial<Parameters<typeof resolveGesture>[0]> = {}): Gesture {
  return resolveGesture({
    mode: 'fixed',
    shiftKey: false,
    pressX: 100,
    playheadX: PLAYHEAD_X,
    pointerType: 'mouse',
    ...over
  })
}

describe('resolveGesture: follow mode', () => {
  it('is always the tape drag, wherever the press lands', () => {
    for (const pressX of [0, 100, PLAYHEAD_X, PLAYHEAD_X + 1, 1000]) {
      expect(press({ mode: 'follow', pressX })).toBe('tape')
    }
  })

  it('ignores shift — the tape is already draggable, so it binds nothing', () => {
    // The matrix says shift+drag in follow mode is "same as drag". A press on
    // the playhead with shift held is still the one gesture follow mode has.
    expect(press({ mode: 'follow', shiftKey: true })).toBe('tape')
    expect(press({ mode: 'follow', shiftKey: true, pressX: PLAYHEAD_X })).toBe('tape')
  })
})

describe('resolveGesture: fixed mode', () => {
  it('grabs the playhead within the margin, inclusive, on both sides', () => {
    expect(press({ pressX: PLAYHEAD_X })).toBe('playhead')
    expect(press({ pressX: PLAYHEAD_X - PLAYHEAD_GRAB_PX })).toBe('playhead')
    expect(press({ pressX: PLAYHEAD_X + PLAYHEAD_GRAB_PX })).toBe('playhead')
  })

  it('is inert on empty track, just outside the margin', () => {
    expect(press({ pressX: PLAYHEAD_X - PLAYHEAD_GRAB_PX - 1 })).toBe('track')
    expect(press({ pressX: PLAYHEAD_X + PLAYHEAD_GRAB_PX + 1 })).toBe('track')
    // Far away, both directions, including the track edges.
    expect(press({ pressX: 0 })).toBe('track')
    expect(press({ pressX: 1000 })).toBe('track')
  })

  it('pans when shift is held, anywhere on the track', () => {
    expect(press({ shiftKey: true, pressX: 0 })).toBe('pan')
    expect(press({ shiftKey: true, pressX: 1000 })).toBe('pan')
  })

  it('lets shift win over the playhead hit test', () => {
    // Where the two cases overlap, the explicit modifier beats the positional
    // guess: shift+drag starting exactly on the playhead pans.
    expect(press({ shiftKey: true, pressX: PLAYHEAD_X })).toBe('pan')
    expect(press({ shiftKey: false, pressX: PLAYHEAD_X })).toBe('playhead')
  })

  it('works with the playhead parked against either track edge', () => {
    // The playhead is clamped into the window, so it can sit at x=0 or x=width.
    // The grab zone is not centred on anything but the line itself.
    expect(press({ playheadX: 0, pressX: 0 })).toBe('playhead')
    expect(press({ playheadX: 0, pressX: PLAYHEAD_GRAB_PX + 1 })).toBe('track')
    expect(press({ playheadX: 1000, pressX: 1000 })).toBe('playhead')
    expect(press({ playheadX: 1000, pressX: 1000 - PLAYHEAD_GRAB_PX - 1 })).toBe('track')
  })
})

describe('exceedsClickSlop', () => {
  it('is a distance threshold, not a timer', () => {
    // A click that shifts under an unsteady hand is still a click.
    for (let d = 0; d <= CLICK_SLOP_PX; d++) {
      expect(exceedsClickSlop(200, 200 + d)).toBe(false)
      expect(exceedsClickSlop(200, 200 - d)).toBe(false)
    }
    expect(exceedsClickSlop(200, 200 + CLICK_SLOP_PX + 1)).toBe(true)
    expect(exceedsClickSlop(200, 200 - CLICK_SLOP_PX - 1)).toBe(true)
  })

  it('is symmetric and depends on nothing but the two coordinates', () => {
    expect(exceedsClickSlop(0, 50)).toBe(true)
    expect(exceedsClickSlop(50, 0)).toBe(true)
    expect(exceedsClickSlop(-30, -30)).toBe(false)
  })

  it('latched by the caller, makes a slow out-and-back a drag', () => {
    // The predicate itself is memoryless; the caller latches it. Replaying a
    // drag that wanders out past the slop and returns to its origin, the way
    // the scrubber does, must end as a drag and not as a click on that origin.
    const startX = 300
    let moved = false
    for (const x of [301, 303, 306, 340, 320, 302, 300]) {
      moved = moved || exceedsClickSlop(startX, x)
    }
    expect(moved).toBe(true)
    // Whereas a press that never leaves the slop stays a click throughout.
    let jitter = false
    for (const x of [300, 302, 299, 303, 300]) {
      jitter = jitter || exceedsClickSlop(startX, x)
    }
    expect(jitter).toBe(false)
  })
})

describe('grabHalfWidth', () => {
  it('gives a finger a 44px target and a cursor a 16px one', () => {
    expect(PLAYHEAD_GRAB_TOUCH_PX * 2).toBe(44)
    expect(PLAYHEAD_GRAB_PX * 2).toBe(16)
    expect(grabHalfWidth('mouse')).toBe(PLAYHEAD_GRAB_PX)
    expect(grabHalfWidth('touch')).toBe(PLAYHEAD_GRAB_TOUCH_PX)
  })

  it('treats a pen, and anything unrecognised, as coarse', () => {
    // A pen is aimed with a hand, not with a cursor, and an unknown pointerType
    // is likelier to be coarse than fine — so the generous target is the safe
    // default for both.
    expect(grabHalfWidth('pen')).toBe(PLAYHEAD_GRAB_TOUCH_PX)
    expect(grabHalfWidth('')).toBe(PLAYHEAD_GRAB_TOUCH_PX)
    expect(grabHalfWidth('trackpad-of-the-future')).toBe(PLAYHEAD_GRAB_TOUCH_PX)
  })
})

describe('the playhead handle', () => {
  it('is catchable by a finger well outside the cursor target', () => {
    // The whole point of the handle: a press that a mouse would call empty
    // track is a playhead grab for a finger.
    const justOutsideMouse = PLAYHEAD_X + PLAYHEAD_GRAB_PX + 1
    expect(press({ pressX: justOutsideMouse, pointerType: 'mouse' })).toBe('track')
    expect(press({ pressX: justOutsideMouse, pointerType: 'touch' })).toBe('playhead')
  })

  it('is exactly 44px wide for touch, inclusive, and no wider', () => {
    for (const dir of [-1, 1]) {
      const edge = PLAYHEAD_X + dir * PLAYHEAD_GRAB_TOUCH_PX
      expect(press({ pressX: edge, pointerType: 'touch' })).toBe('playhead')
      expect(press({ pressX: edge + dir, pointerType: 'touch' })).toBe('track')
    }
  })

  it('is one rule with two numbers, not a separate touch path', () => {
    // Same resolution order, same outcomes, only the width differs — including
    // shift still winning over the hit test, which is a desktop-only modifier
    // but must not behave differently just because a pen reported the press.
    for (const pointerType of ['mouse', 'touch', 'pen']) {
      expect(press({ pressX: PLAYHEAD_X, pointerType })).toBe('playhead')
      expect(press({ pressX: PLAYHEAD_X, pointerType, shiftKey: true })).toBe('pan')
      expect(press({ pressX: PLAYHEAD_X, pointerType, mode: 'follow' })).toBe('tape')
      // Far from the playhead is empty track for every pointer.
      expect(press({ pressX: PLAYHEAD_X + 200, pointerType })).toBe('track')
    }
  })

  it('does not give follow mode a second way to scrub', () => {
    // In follow mode the handle exists but resolves to the one gesture that
    // mode has, at every pointer type and at every distance from the line. It
    // cannot become a competing scrub path because there is no branch for it.
    for (const pointerType of ['mouse', 'touch', 'pen']) {
      for (const pressX of [PLAYHEAD_X, PLAYHEAD_X + 4, PLAYHEAD_X + 30, 0, 1000]) {
        expect(press({ mode: 'follow', pressX, pointerType })).toBe('tape')
      }
    }
  })

  it('does not create a tap-to-seek path on touch', () => {
    // A tap on the handle resolves to 'playhead', NOT 'track'. Only 'track' can
    // become a click, so the handle cannot reach the click branch at all — and
    // 'playhead' seeks on pointermove, which a tap has none of. The mouse-only
    // click gate is a second, independent guard on top of that.
    expect(press({ pressX: PLAYHEAD_X, pointerType: 'touch' })).toBe('playhead')
    expect(press({ pressX: PLAYHEAD_X, pointerType: 'pen' })).toBe('playhead')
    // And a touch tap on the track resolves to 'track', where the pointerType
    // gate on release is what stops it seeking — and the movement threshold,
    // never crossed by a tap, is what stops it panning.
    expect(press({ pressX: PLAYHEAD_X + 200, pointerType: 'touch' })).toBe('track')
  })
})

// ---------------------------------------------------------------------------
// The click / pan boundary on a fixed-mode track press.
// ---------------------------------------------------------------------------
// A 'track' press is undecided: under the slop it is a click that seeks, past
// the slop it is a pan. Transcribed from TimelineScrubber's pointer handlers,
// which latch one flag and branch both outcomes off it — so the two cannot
// steal each other by construction rather than by ordering.
type PressOutcome = { panned: number; clicked: boolean }

function replayTrackPress(xs: number[], opts: { isMouse?: boolean } = {}): PressOutcome {
  const isMouse = opts.isMouse ?? true
  const startX = xs[0]!
  let movedBeyondSlop = false
  let panLastX = startX
  let panned = 0
  for (const x of xs.slice(1)) {
    if (!movedBeyondSlop && exceedsClickSlop(startX, x)) {
      movedBeyondSlop = true
      // Re-baseline at the crossing point (the scrubber does this for 'track').
      panLastX = x
    }
    if (movedBeyondSlop) {
      panned += panLastX - x
      panLastX = x
    }
  }
  return { panned, clicked: !movedBeyondSlop && isMouse }
}

describe('click versus pan on a fixed-mode track press', () => {
  it('clicks and does not pan when the press never leaves the slop', () => {
    const r = replayTrackPress([300, 301, 303, 299, 302, 300])
    expect(r.clicked).toBe(true)
    expect(r.panned).toBe(0)
  })

  it('pans and does not click once the press leaves the slop', () => {
    const r = replayTrackPress([300, 305, 320, 400])
    expect(r.clicked).toBe(false)
    expect(r.panned).not.toBe(0)
  })

  it('is exact at the boundary, on both sides and in both directions', () => {
    // At the slop: still a click, still no pan. One pixel past it: a pan, and
    // the click is off.
    for (const dir of [-1, 1]) {
      const at = replayTrackPress([300, 300 + dir * CLICK_SLOP_PX])
      expect(at.clicked).toBe(true)
      expect(at.panned).toBe(0)

      const past = replayTrackPress([300, 300 + dir * (CLICK_SLOP_PX + 1)])
      expect(past.clicked).toBe(false)
      // The crossing frame itself contributes nothing — it only re-baselines.
      expect(past.panned).toBe(0)
    }
  })

  it('does not replay the slop distance as a jolt on the first panning frame', () => {
    // Crossing at +6 then travelling to +40 must pan by the 34px travelled
    // SINCE the crossing, not by the whole 40 from the press point. The
    // absorbed distance is the threshold's cost and is under the width of the
    // playhead line.
    const r = replayTrackPress([300, 306, 340])
    expect(r.panned).toBe(-34)
  })

  it('cannot go back to being a click once it has panned', () => {
    // The latch is one-way. A drag that wanders out and returns exactly to its
    // press point has panned, and must not also seek there on release — which
    // would be both gestures firing for one press.
    const r = replayTrackPress([300, 340, 320, 300])
    expect(r.clicked).toBe(false)
    // It panned by the travel since the CROSSING (340 back to 300), not since
    // the press point. Returning to where the press began neither undoes the
    // pan nor re-arms the click.
    expect(r.panned).toBe(40)
  })

  it('pans on touch but never clicks there', () => {
    // A finger drag on the track pans, which is the primary way to pan on a
    // phone. A finger TAP still does nothing at all: no pan (never crossed the
    // slop) and no click (not a mouse).
    const drag = replayTrackPress([300, 305, 380, 420], { isMouse: false })
    expect(drag.panned).not.toBe(0)
    expect(drag.clicked).toBe(false)

    const tap = replayTrackPress([300, 301, 300], { isMouse: false })
    expect(tap.panned).toBe(0)
    expect(tap.clicked).toBe(false)
  })

  it('accumulates a long pan from the crossing point, frame by frame', () => {
    // Many small frames and one big one must land on the same total, since each
    // contributes only its own step.
    const many = replayTrackPress([300, 306, ...Array.from({ length: 100 }, (_, i) => 306 + i * 2)])
    const few = replayTrackPress([300, 306, 504])
    expect(many.panned).toBe(few.panned)
    expect(many.panned).toBe(-198)
  })
})
