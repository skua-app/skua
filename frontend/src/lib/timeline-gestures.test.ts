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
    expect(press({ pressX: PLAYHEAD_X - PLAYHEAD_GRAB_PX - 1 })).toBe('idle')
    expect(press({ pressX: PLAYHEAD_X + PLAYHEAD_GRAB_PX + 1 })).toBe('idle')
    // Far away, both directions, including the track edges.
    expect(press({ pressX: 0 })).toBe('idle')
    expect(press({ pressX: 1000 })).toBe('idle')
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
    expect(press({ playheadX: 0, pressX: PLAYHEAD_GRAB_PX + 1 })).toBe('idle')
    expect(press({ playheadX: 1000, pressX: 1000 })).toBe('playhead')
    expect(press({ playheadX: 1000, pressX: 1000 - PLAYHEAD_GRAB_PX - 1 })).toBe('idle')
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
    expect(press({ pressX: justOutsideMouse, pointerType: 'mouse' })).toBe('idle')
    expect(press({ pressX: justOutsideMouse, pointerType: 'touch' })).toBe('playhead')
  })

  it('is exactly 44px wide for touch, inclusive, and no wider', () => {
    for (const dir of [-1, 1]) {
      const edge = PLAYHEAD_X + dir * PLAYHEAD_GRAB_TOUCH_PX
      expect(press({ pressX: edge, pointerType: 'touch' })).toBe('playhead')
      expect(press({ pressX: edge + dir, pointerType: 'touch' })).toBe('idle')
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
      expect(press({ pressX: PLAYHEAD_X + 200, pointerType })).toBe('idle')
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
    // A tap on the handle resolves to 'playhead', NOT 'idle'. Only 'idle' can
    // become a click, so the handle cannot reach the click branch at all — and
    // 'playhead' seeks on pointermove, which a tap has none of. The mouse-only
    // click gate is a second, independent guard on top of that.
    expect(press({ pressX: PLAYHEAD_X, pointerType: 'touch' })).toBe('playhead')
    expect(press({ pressX: PLAYHEAD_X, pointerType: 'pen' })).toBe('playhead')
    // And a touch tap on empty track resolves to 'idle', where the pointerType
    // gate on release is what stops it seeking.
    expect(press({ pressX: PLAYHEAD_X + 200, pointerType: 'touch' })).toBe('idle')
  })
})
