import { describe, it, expect } from 'vitest'
import {
  TICK_STEPS,
  TICK_MIN_PX,
  MICRO_DIVISIONS,
  MIN_MICRO_PX,
  HOUR,
  LABEL_HALF_W,
  chooseTickStep,
  tickPositions,
  microTickFractions,
  hourLineFractions,
  clampLabelPx,
  bandGeometry,
  pointerDistance,
  pointerMidX
} from './timeline-track'

// The track layout, as code. Every property asserted here is one the rendered
// timeline depends on and none of which a screenshot would catch drifting: a
// step ladder that picks the wrong rung crowds the labels, marks that stop
// landing on absolute multiples make them jitter as the window pans, and a
// micro tick on a major boundary draws a hairline through a labelled one.

// A width that makes the label budget exactly ten, so the ladder's rungs can be
// pinned at a known count rather than at whatever a real track happens to be.
const TEN_LABELS_W = 10 * TICK_MIN_PX

describe('chooseTickStep', () => {
  it('takes the smallest rung whose label count fits the budget', () => {
    // Ten labels of budget: one minute steps serve a ten-minute span exactly,
    // and one second more of span needs an eleventh label — which does not fit,
    // so the ladder moves up a rung. Asserted from both sides of that edge,
    // because the failure that matters is the boundary moving, not the choice.
    expect(chooseTickStep(600, TEN_LABELS_W)).toBe(60)
    expect(chooseTickStep(601, TEN_LABELS_W)).toBe(120)
    // The same edge one rung higher: 120s steps serve 1200s, not 1201s.
    expect(chooseTickStep(1200, TEN_LABELS_W)).toBe(120)
    expect(chooseTickStep(1201, TEN_LABELS_W)).toBe(300)
  })

  it('never returns a rung that is not on the ladder', () => {
    for (const span of [1, 60, 600, 3599, 3600, 20000, 86400]) {
      expect(TICK_STEPS).toContain(chooseTickStep(span, TEN_LABELS_W))
    }
  })

  it('falls back to a ten-label budget on a zero-width track', () => {
    // Before the ResizeObserver fires the width is 0. Dividing by the label
    // count that implies would give an infinite budget and the finest rung on
    // every span; the fallback has to behave like a real, modest track instead.
    for (const span of [600, 601, 1200, 1201, 7200]) {
      expect(chooseTickStep(span, 0)).toBe(chooseTickStep(span, TEN_LABELS_W))
    }
  })

  it('keeps a budget of at least one label on a track too narrow for one', () => {
    // floor(width / TICK_MIN_PX) is 0 below one label's worth of room. A budget
    // of zero would exhaust the ladder on every span; one keeps the answer the
    // widest rung the span actually needs.
    expect(chooseTickStep(600, TICK_MIN_PX - 1)).toBe(600)
    expect(chooseTickStep(60, TICK_MIN_PX - 1)).toBe(60)
  })

  it('falls back to the largest rung for a span the ladder cannot serve', () => {
    const largest = TICK_STEPS[TICK_STEPS.length - 1]!
    // A day-and-a-half window on a one-label budget: even 12h steps need more
    // labels than there is room for, so there is no rung that fits and the
    // coarsest one is the least bad answer.
    expect(chooseTickStep(largest * 100, TICK_MIN_PX)).toBe(largest)
  })
})

describe('the micro subdivision of the ladder', () => {
  it('divides every rung exactly, so the coincidence test stays integer', () => {
    // microTickFractions skips a mark that is also a major boundary with
    // `t % majorStep === 0`. That is exact arithmetic only while every step on
    // the ladder divides by the division count without a remainder — otherwise
    // microStep is fractional, the walk accumulates float error, and the modulo
    // silently stops matching. Asserted over the WHOLE ladder: a rung added
    // later that breaks this fails here rather than on a device.
    for (const step of TICK_STEPS) {
      expect(step % MICRO_DIVISIONS).toBe(0)
    }
  })
})

describe('tickPositions', () => {
  it('starts at the first absolute multiple at or after the window start', () => {
    const ticks = tickPositions(1000, 4600, 600)
    expect(ticks[0]?.tSec).toBe(1200)
    for (const t of ticks) expect(t.tSec % 600).toBe(0)
  })

  it('includes a multiple landing exactly on either edge', () => {
    const ticks = tickPositions(1200, 4200, 600).map((t) => t.tSec)
    expect(ticks[0]).toBe(1200)
    expect(ticks[ticks.length - 1]).toBe(4200)
  })

  it('never runs past the window end', () => {
    const ticks = tickPositions(1000, 4600, 600)
    expect(ticks[ticks.length - 1]?.tSec).toBe(4200)
    for (const t of ticks) expect(t.tSec).toBeLessThanOrEqual(4600)
  })

  it('reports each tick as a fraction across the drawn window', () => {
    const ticks = tickPositions(0, 1000, 500)
    expect(ticks.map((t) => t.fraction)).toEqual([0, 0.5, 1])
  })

  it('keeps surviving ticks on the same absolute multiples as the window pans', () => {
    // The anti-jitter property, stated in the source: marks are absolute, not
    // index-decimated. Pan by an amount that is NOT a multiple of the step —
    // the case an index-based scheme gets wrong — and the ticks the two windows
    // share must be the same absolute seconds, not merely the same count.
    const before = tickPositions(1000, 4600, 600).map((t) => t.tSec)
    const after = tickPositions(1137, 4737, 600).map((t) => t.tSec)
    expect(before).toEqual([1200, 1800, 2400, 3000, 3600, 4200])
    expect(after).toEqual([1200, 1800, 2400, 3000, 3600, 4200])
  })

  it('yields nothing for an empty or inverted window', () => {
    expect(tickPositions(1000, 1000, 600)).toEqual([])
    expect(tickPositions(4600, 1000, 600)).toEqual([])
  })
})

describe('microTickFractions', () => {
  // A window and width where the micro layer is comfortably above the pixel
  // guard: 60s majors quartered to 15s, at 14px apart.
  const START = 0
  const END = 600
  const MAJOR = 60
  const ROOMY_W = TEN_LABELS_W

  it('subdivides each major interval into MICRO_DIVISIONS parts', () => {
    const micro = microTickFractions(START, END, MAJOR, ROOMY_W)
    // Every multiple of 15s in the window, less the eleven that are also
    // multiples of 60s.
    expect(micro).toHaveLength(41 - 11)
  })

  it('never places a mark on a major boundary', () => {
    // A micro tick on a major would draw a hairline underneath a labelled tick.
    const span = END - START
    for (const f of microTickFractions(START, END, MAJOR, ROOMY_W)) {
      // Back to the second the fraction came from. Rounded because the round
      // trip through a fraction is float division; the marks themselves are
      // whole seconds, which is what makes the modulo in the source exact.
      expect(Math.round(START + f * span) % MAJOR).not.toBe(0)
    }
  })

  it('derives from the major step it is GIVEN, never from one it picks', () => {
    // Both ladders come from a single chooseTickStep call in the component, so
    // this function must not choose again — with a step the ladder would not
    // have picked for this span, the marks still follow the step passed in.
    const span = END - START
    const micro = microTickFractions(START, END, 300, ROOMY_W)
    for (const f of micro) {
      const t = Math.round(START + f * span)
      expect(t % (300 / MICRO_DIVISIONS)).toBe(0)
      expect(t % 300).not.toBe(0)
    }
  })

  it('drops the whole layer once the marks would crowd', () => {
    // The guard is on pixel spacing, not on count: at 15s marks across a 600s
    // window, MIN_MICRO_PX of spacing needs 360px of track.
    const needed = (MIN_MICRO_PX * (END - START)) / (MAJOR / MICRO_DIVISIONS)
    expect(microTickFractions(START, END, MAJOR, needed)).not.toEqual([])
    expect(microTickFractions(START, END, MAJOR, needed - 1)).toEqual([])
    // A track that has not been measured yet has no room by this rule either.
    expect(microTickFractions(START, END, MAJOR, 0)).toEqual([])
  })

  it('yields nothing for an empty or inverted window', () => {
    expect(microTickFractions(1000, 1000, MAJOR, ROOMY_W)).toEqual([])
    expect(microTickFractions(4600, 1000, MAJOR, ROOMY_W)).toEqual([])
  })
})

describe('hourLineFractions', () => {
  it('draws interior hour boundaries only', () => {
    // 01:00 to 03:00: the only interior whole hour is 02:00, dead centre.
    expect(hourLineFractions(HOUR, 3 * HOUR)).toEqual([0.5])
  })

  it('excludes a boundary sitting exactly on either window edge', () => {
    // A line pinned at 0 or 1 is not a divider, it is the track edge — and a
    // window landing on whole hours is common, not a corner case. Both hours
    // here are edges, so nothing draws.
    expect(hourLineFractions(HOUR, 2 * HOUR)).toEqual([])
    // Move either edge a single second off its boundary and the hour it was
    // sitting on becomes interior, so it starts drawing.
    expect(hourLineFractions(HOUR, 2 * HOUR + 1)).toHaveLength(1)
    expect(hourLineFractions(HOUR - 1, 2 * HOUR)).toHaveLength(1)
  })

  it('walks every whole hour across a wide window', () => {
    expect(hourLineFractions(HOUR - 1, 5 * HOUR + 1)).toHaveLength(5)
  })

  it('yields nothing for an empty or inverted window', () => {
    expect(hourLineFractions(HOUR, HOUR)).toEqual([])
    expect(hourLineFractions(3 * HOUR, HOUR)).toEqual([])
  })
})

describe('clampLabelPx', () => {
  const W = 200

  it('pushes the first and last labels inward off the track edges', () => {
    expect(clampLabelPx(0, W)).toBe(LABEL_HALF_W)
    expect(clampLabelPx(1, W)).toBe(W - LABEL_HALF_W)
  })

  it('leaves an interior label exactly where its mark is', () => {
    expect(clampLabelPx(0.5, W)).toBe(100)
    expect(clampLabelPx(0.25, W)).toBe(50)
  })

  it('does nothing on a track narrower than twice the half width', () => {
    // Below that there is no interior left to clamp into: the two bounds cross
    // and clamping would park every label on the wrong side of the other.
    const narrow = 2 * LABEL_HALF_W - 1
    expect(clampLabelPx(0, narrow)).toBe(0)
    expect(clampLabelPx(1, narrow)).toBe(narrow)
    expect(clampLabelPx(0.5, narrow)).toBe(narrow / 2)
    // An unmeasured track is the same case.
    expect(clampLabelPx(0.5, 0)).toBe(0)
  })
})

describe('bandGeometry', () => {
  const START = 0
  const END = 100

  it('reports the width an ordinary band should have', () => {
    expect(bandGeometry(25, 75, START, END)).toEqual({ x0: 0.25, width: 0.5 })
  })

  it('clamps a band straddling either edge', () => {
    expect(bandGeometry(-50, 50, START, END)).toEqual({ x0: 0, width: 0.5 })
    expect(bandGeometry(50, 150, START, END)).toEqual({ x0: 0.5, width: 0.5 })
    expect(bandGeometry(-50, 150, START, END)).toEqual({ x0: 0, width: 1 })
  })

  it('drops a band fully outside the window, on either side', () => {
    expect(bandGeometry(200, 300, START, END)).toBeNull()
    expect(bandGeometry(-300, -200, START, END)).toBeNull()
    // Touching an edge is still outside: it has no width to draw.
    expect(bandGeometry(100, 200, START, END)).toBeNull()
    expect(bandGeometry(-100, 0, START, END)).toBeNull()
  })

  it('drops a zero-width or inverted band', () => {
    expect(bandGeometry(50, 50, START, END)).toBeNull()
    expect(bandGeometry(75, 25, START, END)).toBeNull()
  })

  it('drops everything on a degenerate window', () => {
    // timeToFraction answers 0 for a window with no extent, so every band
    // collapses rather than dividing by zero.
    expect(bandGeometry(25, 75, 100, 100)).toBeNull()
  })
})

describe('two-pointer geometry', () => {
  it('has no separation with fewer than two pointers', () => {
    expect(pointerDistance([])).toBe(0)
    expect(pointerDistance([300])).toBe(0)
  })

  it('measures the separation regardless of which finger is which', () => {
    expect(pointerDistance([100, 300])).toBe(200)
    expect(pointerDistance([300, 100])).toBe(200)
  })

  it('falls back to the one pointer it has for the midpoint', () => {
    // A pinch that has lost a finger still has a position to anchor on; with no
    // pointers at all there is nothing to report and 0 is as good as anything.
    expect(pointerMidX([])).toBe(0)
    expect(pointerMidX([300])).toBe(300)
  })

  it('is the midpoint between two pointers, in either order', () => {
    expect(pointerMidX([100, 300])).toBe(200)
    expect(pointerMidX([300, 100])).toBe(200)
  })
})
