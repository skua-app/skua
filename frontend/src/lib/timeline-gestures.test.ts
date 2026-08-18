import { describe, it, expect } from 'vitest'
import {
  CLICK_SLOP_PX,
  PLAYHEAD_GRAB_PX,
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
