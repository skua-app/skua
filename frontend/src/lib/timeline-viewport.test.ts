import { describe, it, expect } from 'vitest'
import { timeToFraction } from './timeline'
import {
  DEFAULT_VIEW_SPAN,
  MIN_SPAN,
  MAX_SPAN,
  clampViewSpan,
  centredViewStart,
  anchoredViewStart,
  clampViewStart,
  clampPlayhead
} from './timeline-viewport'

// The viewport model's load-bearing property is an INVARIANT, not a value:
// while follow is on, the viewport is centred on the playhead, so
//
//   viewStart   === position - viewSpan/2
//   windowStart === position - viewSpan/2
//   windowEnd   === position + viewSpan/2
//
// and the scrubber therefore draws the playhead dead centre. Every gesture,
// every entry path and every playback tick has to leave that true. These tests
// drive the model through the real gesture math and assert the invariant after
// every single mutation, rather than checking a handful of expected numbers.

// ---------------------------------------------------------------------------
// Harness: mirrors the four viewport writers in
// routes/cam/[id]/timeline/+page.svelte. The reactive state lives in the route;
// all the arithmetic lives in the module under test, so what is reproduced here
// is only the assignment and the follow branching. Keep in step with the route.
// ---------------------------------------------------------------------------
class ViewportModel {
  viewSpan = DEFAULT_VIEW_SPAN
  viewStart = 0
  position = 0
  // follow IS the user's timeline mode: true = 'follow', false = 'fixed'. It is
  // an INPUT to the writers below, never something they write — in the route it
  // is a $derived off the preference. A test sets it the way a user picks the
  // mode, before driving the gestures.
  follow = true
  liveEdge = 0
  playbackFloor = 0

  get windowStart(): number {
    return this.viewStart
  }
  get windowEnd(): number {
    return this.viewStart + this.viewSpan
  }

  // The only writer of viewStart.
  setViewStart(start: number) {
    this.viewStart = clampViewStart(start, this.viewSpan, this.liveEdge)
  }
  centreOnPlayhead() {
    this.setViewStart(centredViewStart(this.position, this.viewSpan))
  }
  // The only writer of position.
  setPosition(t: number) {
    this.position = t
    if (this.follow) this.centreOnPlayhead()
  }
  // A writer of viewSpan: the follow-mode zoom, and the fixed-mode fallback
  // for a gesture that reports no anchor.
  setViewSpan(span: number) {
    this.viewSpan = span
    if (this.follow) this.centreOnPlayhead()
  }
  // The other writer of viewSpan: the fixed-mode zoom, anchored on a position
  // across the drawn window.
  setViewSpanAnchored(span: number, fraction: number) {
    const target = anchoredViewStart(this.viewStart, this.viewSpan, span, fraction)
    this.viewSpan = span
    this.setViewStart(target)
  }

  // --- the route's call sites ---
  // Capture effect: place the playhead (deep-link t, else centred in the last
  // viewSpan up to live), then place the entry window on it explicitly — in
  // fixed mode nothing else would, and in follow mode it is the same pure
  // recompute setPosition just made.
  enter(now: number, tParam: string | null) {
    this.liveEdge = now
    const tSec = tParam !== null ? Number.parseInt(tParam, 10) : Number.NaN
    if (Number.isFinite(tSec)) this.setPosition(tSec > this.liveEdge ? this.liveEdge : tSec)
    else this.setPosition(this.liveEdge - this.viewSpan / 2)
    this.centreOnPlayhead()
  }
  // handleSeek: drag, VHS rush and keyboard nudge all land here.
  seek(t: number) {
    this.setPosition(clampPlayhead(t, this.playbackFloor, this.liveEdge))
  }
  // onZoom: pinch ratio / wheel step, plus WHERE the gesture wants to zoom
  // about (a position across the drawn window). fraction null = the gesture
  // expressed no anchor, which is what pinch passes; it defaults to null so
  // the scenarios written against the pre-anchor model drive it unchanged.
  // The MODE decides: follow ignores the anchor outright.
  zoom(target: number, fraction: number | null = null) {
    const span = clampViewSpan(target)
    if (this.follow || fraction === null) {
      this.setViewSpan(span)
      return
    }
    this.setViewSpanAnchored(span, fraction)
  }
  // Full-res timeupdate and the ensurePlaybackAt coverage snap. Neither clamps
  // in the route: both derive from a chunk whose end is itself capped at
  // liveEdge, so callers here feed values that respect that bound.
  advance(t: number) {
    this.setPosition(t)
  }
}

// Gesture math transcribed from lib/components/TimelineScrubber.svelte. The
// scrubber is stateless about the viewport — it reads windowEnd-windowStart as
// the current span — so driving these against the model reproduces what a real
// finger/wheel/key produces.
const TRACK_W = 1000
function panTarget(m: ViewportModel, startPosition: number, startX: number, x: number): number {
  const span = m.windowEnd - m.windowStart
  return Math.round(startPosition - ((x - startX) / TRACK_W) * span)
}
function wheelSpan(m: ViewportModel, deltaY: number): number {
  const span = m.windowEnd - m.windowStart
  return span * (deltaY > 0 ? 1.15 : 1 / 1.15)
}
function pinchSpan(startSpan: number, startDist: number, dist: number): number {
  return (startSpan * startDist) / dist
}
function keyTarget(m: ViewportModel, dir: -1 | 1, shift: boolean): number {
  const step = shift ? 300 : 60
  return Math.round(Math.max(m.windowStart, Math.min(m.windowEnd, m.position + dir * step)))
}

// ---------------------------------------------------------------------------
// Invariant checker
// ---------------------------------------------------------------------------
// viewStart and windowStart are asserted EXACTLY: both are the same expression
// the code evaluates, so they are bitwise identical or the model is wrong.
//
// windowEnd is viewStart + viewSpan, i.e. (p - s/2) + s, which is a
// re-association of p + s/2 and so not guaranteed exact in IEEE754. Measured
// over 2,000,000 random (position, span) pairs at unix-second magnitudes the
// error is exactly 0 every time, but the guarantee does not hold in general, so
// it is asserted to within one ULP at this scale (2^-22 s ≈ 240 ns) rather than
// bitwise. That is ~7 orders of magnitude below one pixel at the tightest zoom.
const ULP = 2 ** -22

type Violation = { at: string; what: string; expected: number; actual: number }

function checkInvariants(m: ViewportModel, at: string, out: Violation[]) {
  // The playhead never outruns live. This is what keeps the live-edge clamp
  // inert, which is in turn what makes the centring invariant below hold.
  if (m.position > m.liveEdge) {
    out.push({ at, what: 'position <= liveEdge', expected: m.liveEdge, actual: m.position })
  }
  // The live-edge guarantee itself, which holds whether or not follow is on.
  const maxStart = m.liveEdge - m.viewSpan / 2
  if (m.viewStart > maxStart + ULP) {
    out.push({
      at,
      what: 'viewStart <= liveEdge - viewSpan/2',
      expected: maxStart,
      actual: m.viewStart
    })
  }
  if (!m.follow) return
  const centre = m.position - m.viewSpan / 2
  if (m.viewStart !== centre) {
    out.push({
      at,
      what: 'viewStart === position - viewSpan/2',
      expected: centre,
      actual: m.viewStart
    })
  }
  if (m.windowStart !== centre) {
    out.push({
      at,
      what: 'windowStart === position - viewSpan/2',
      expected: centre,
      actual: m.windowStart
    })
  }
  const end = m.position + m.viewSpan / 2
  if (Math.abs(m.windowEnd - end) > ULP) {
    out.push({
      at,
      what: 'windowEnd === position + viewSpan/2',
      expected: end,
      actual: m.windowEnd
    })
  }
  // What the invariant is FOR: TimelineScrubber places the playhead with
  // timeToFraction, so centring must survive all the way to the drawn fraction.
  const f = timeToFraction(m.position, m.windowStart, m.windowEnd)
  if (Math.abs(f - 0.5) > 1e-9) {
    out.push({ at, what: 'playhead draws dead centre', expected: 0.5, actual: f })
  }
}

const NOW = 1787000000
// Permissive floor: what playbackFloor falls back to before the recording
// summary lands (liveEdge - 7d).
const PERMISSIVE_FLOOR = NOW - 7 * 24 * 3600

function freshModel(floor = NOW - 30 * 86400, now = NOW, t: string | null = null): ViewportModel {
  const m = new ViewportModel()
  m.playbackFloor = floor
  m.enter(now, t)
  return m
}

// ---------------------------------------------------------------------------

describe('clampViewSpan', () => {
  it('bounds to [MIN_SPAN, MAX_SPAN] and rounds to a whole second', () => {
    expect(clampViewSpan(0)).toBe(MIN_SPAN)
    expect(clampViewSpan(MIN_SPAN - 1)).toBe(MIN_SPAN)
    expect(clampViewSpan(MAX_SPAN + 1)).toBe(MAX_SPAN)
    expect(clampViewSpan(1e12)).toBe(MAX_SPAN)
    expect(clampViewSpan(-1e12)).toBe(MIN_SPAN)
    expect(clampViewSpan(3600.4)).toBe(3600)
    expect(clampViewSpan(3600.5)).toBe(3601)
  })
  it('leaves the default span untouched', () => {
    expect(clampViewSpan(DEFAULT_VIEW_SPAN)).toBe(DEFAULT_VIEW_SPAN)
    expect(DEFAULT_VIEW_SPAN).toBeGreaterThanOrEqual(MIN_SPAN)
    expect(DEFAULT_VIEW_SPAN).toBeLessThanOrEqual(MAX_SPAN)
  })
})

describe('centredViewStart', () => {
  it('puts the playhead at the middle of the span', () => {
    expect(centredViewStart(1000, 600)).toBe(700)
    expect(centredViewStart(NOW, DEFAULT_VIEW_SPAN)).toBe(NOW - 1800)
    // Idempotent: recomputing from the same inputs never drifts, which is what
    // lets a pinch call it on every pointermove.
    let s = centredViewStart(NOW, 3601)
    for (let i = 0; i < 1000; i++) s = centredViewStart(NOW, 3601)
    expect(s).toBe(centredViewStart(NOW, 3601))
  })
})

describe('anchoredViewStart', () => {
  // The load-bearing property: the wall-clock time under `fraction` before the
  // zoom is the wall-clock time under it after. Recovering the anchor from the
  // new window re-associates (t - f*next) + f*next, which IEEE754 does not
  // guarantee to be exact, so it is asserted to within one ULP at unix-second
  // magnitude — the same tolerance, and the same reason, as windowEnd above.
  it('holds the time under the anchor fixed across any span change', () => {
    const start = NOW - 1800
    const span = 3600
    for (const fraction of [0, 0.01, 0.25, 1 / 3, 0.5, 0.75, 0.999, 1]) {
      const anchorTime = start + fraction * span
      for (const next of [MIN_SPAN, 900, 1799, 3600, 3601, 4 * 3600, MAX_SPAN]) {
        const nextStart = anchoredViewStart(start, span, next, fraction)
        expect(Math.abs(nextStart + fraction * next - anchorTime)).toBeLessThanOrEqual(ULP)
      }
    }
  })

  it('is a no-op when the span does not change', () => {
    for (const fraction of [0, 0.25, 0.5, 1]) {
      expect(anchoredViewStart(NOW - 1800, 3600, 3600, fraction)).toBe(NOW - 1800)
    }
  })

  it('anchored at the midpoint is the centred window', () => {
    // Why follow mode never needs this function: at fraction 0.5 the anchor IS
    // the centre, so the anchored window and the centred window are the same
    // window. Follow mode still uses centredViewStart, because that is the
    // expression its invariant is stated in.
    const span = 3600
    const start = centredViewStart(NOW, span)
    for (const next of [MIN_SPAN, 1800, 7200, MAX_SPAN]) {
      expect(anchoredViewStart(start, span, next, 0.5)).toBe(centredViewStart(NOW, next))
    }
  })

  it('anchored at an edge pins that edge', () => {
    // fraction 0: the left edge does not move. fraction 1: the right edge does not.
    const start = NOW - 1800
    expect(anchoredViewStart(start, 3600, 600, 0)).toBe(start)
    expect(anchoredViewStart(start, 3600, 600, 1) + 600).toBe(start + 3600)
  })

  it('zooms toward the anchor, not away from it', () => {
    // A concrete case, checked by hand: a 1h window, cursor a quarter of the way
    // across it, zoomed to 10 minutes. The quarter mark is 15 min into the old
    // window and must be 2.5 min into the new one.
    const start = 1_000_000
    expect(anchoredViewStart(start, 3600, 600, 0.25)).toBe(start + 900 - 150)
  })
})

describe('clampViewStart', () => {
  it('is inert while the viewport centre is at or before the live edge', () => {
    expect(clampViewStart(NOW - 1800, 3600, NOW)).toBe(NOW - 1800)
    expect(clampViewStart(NOW - 5000, 3600, NOW)).toBe(NOW - 5000)
    // Centre exactly at live: still inert.
    expect(clampViewStart(NOW - 1800, 3600, NOW)).toBe(NOW - 1800)
  })
  it('holds the viewport CENTRE at the live edge once pushed past it', () => {
    const clamped = clampViewStart(NOW + 500, 3600, NOW)
    expect(clamped).toBe(NOW - 1800)
    expect(clamped + 3600 / 2).toBe(NOW)
  })
  it('leaves the window END free to overhang live by half a span', () => {
    // The overhang is load-bearing: it is what draws the scrubber's right-hand
    // hatch band and parks the LIVE capsule inside the track.
    const start = clampViewStart(NOW - 1800, 3600, NOW)
    expect(start + 3600).toBe(NOW + 1800)
  })
  it('is one-sided — there is no floor-side clamp', () => {
    // A ?t= deep-link older than the permissive floor must still land centred.
    const old = NOW - 400 * 86400
    expect(clampViewStart(old, 3600, NOW)).toBe(old)
  })
})

describe('clampPlayhead', () => {
  it('bounds a seek target to the playable domain', () => {
    expect(clampPlayhead(NOW, PERMISSIVE_FLOOR, NOW)).toBe(NOW)
    expect(clampPlayhead(NOW + 1e6, PERMISSIVE_FLOOR, NOW)).toBe(NOW)
    expect(clampPlayhead(PERMISSIVE_FLOOR - 1e6, PERMISSIVE_FLOOR, NOW)).toBe(PERMISSIVE_FLOOR)
    expect(clampPlayhead(NOW - 60, PERMISSIVE_FLOOR, NOW)).toBe(NOW - 60)
  })
})

describe('viewport invariant: entry', () => {
  it('holds for the default entry, and the window is the last viewSpan up to live', () => {
    const v: Violation[] = []
    const m = freshModel()
    checkInvariants(m, 'default entry', v)
    expect(v).toEqual([])
    expect(m.windowEnd).toBe(NOW)
    expect(m.windowEnd - m.windowStart).toBe(DEFAULT_VIEW_SPAN)
  })

  it('holds for ?t= deep links, including older than the permissive floor', () => {
    const v: Violation[] = []
    for (const age of [0, 60, 1800, 3600, 86400, 6 * 86400, 30 * 86400, 400 * 86400]) {
      const t = NOW - age
      // playbackFloor is still the permissive fallback: the summary has not
      // landed, and the entry position is deliberately not clamped by it.
      const m = freshModel(PERMISSIVE_FLOOR, NOW, String(t))
      checkInvariants(m, `deep-link age=${age}`, v)
      expect(m.position).toBe(t)
      expect(m.windowStart).toBe(t - DEFAULT_VIEW_SPAN / 2)
    }
    expect(v).toEqual([])
  })

  it('holds for a future ?t= (clamped to live) and for malformed ?t=', () => {
    const v: Violation[] = []
    const future = freshModel(PERMISSIVE_FLOOR, NOW, String(NOW + 100000))
    checkInvariants(future, 'future t', v)
    expect(future.position).toBe(NOW)
    for (const raw of ['', 'abc', 'NaN', '-', '+']) {
      const m = freshModel(PERMISSIVE_FLOOR, NOW, raw)
      checkInvariants(m, `malformed ${JSON.stringify(raw)}`, v)
      // Falls back to the default window, never NaN.
      expect(Number.isFinite(m.position)).toBe(true)
      expect(m.windowEnd).toBe(NOW)
    }
    // parseInt is lenient, so a partially-numeric t parses to its leading
    // digits rather than falling back: ?t=12x lands the playhead at unix 12.
    // Pre-existing route behaviour, only reachable by hand-editing the URL (the
    // app generates t itself) — pinned here so a future parse change is a
    // deliberate one. The invariant holds either way, which is the point.
    const lenient = freshModel(PERMISSIVE_FLOOR, NOW, '12x')
    checkInvariants(lenient, 'lenient parse', v)
    expect(lenient.position).toBe(12)
    expect(v).toEqual([])
  })
})

describe('entry in fixed mode', () => {
  // The one thing entry cannot inherit from follow mode. While follow was
  // implicitly on for every entry, the window was placed as a side effect of
  // setPosition centring; in fixed mode setPosition does not centre, so the
  // capture effect places it explicitly or the viewport stays on whatever the
  // previous camera left behind.
  function fixedEntry(now: number, t: string | null = null): ViewportModel {
    const m = new ViewportModel()
    m.follow = false
    m.playbackFloor = now - 30 * 86400
    // A stale window from a previous camera, deliberately nowhere near `now`.
    m.viewStart = now - 400 * 86400
    m.enter(now, t)
    return m
  }

  it('centres the entry window on the playhead even though nothing tracks it', () => {
    const m = fixedEntry(NOW)
    expect(m.position).toBe(NOW - DEFAULT_VIEW_SPAN / 2)
    expect(m.viewStart).toBe(centredViewStart(m.position, m.viewSpan))
    expect(m.windowEnd).toBe(NOW)
  })

  it('centres a deep-link entry window too, and then stops tracking', () => {
    const t = NOW - 6 * 3600
    const m = fixedEntry(NOW, String(t))
    expect(m.position).toBe(t)
    expect(m.viewStart).toBe(t - DEFAULT_VIEW_SPAN / 2)
    // From there the track is stationary: playback moves the playhead and the
    // window does not follow it, which is the whole of fixed mode.
    const parked = m.viewStart
    for (let i = 0; i < 4000; i++) m.advance(m.position + 1)
    expect(m.viewStart).toBe(parked)
    expect(m.position).toBe(t + 4000)
    // And the playhead has legitimately left the window. Nothing re-engages.
    expect(m.position).toBeGreaterThan(m.windowEnd)
  })
})

describe('viewport invariant: gestures', () => {
  it('holds through a relative pan, both directions, at several zoom levels', () => {
    const v: Violation[] = []
    const m = freshModel()
    for (const span of [MIN_SPAN, DEFAULT_VIEW_SPAN, 6 * 3600]) {
      m.zoom(span)
      checkInvariants(m, `pan zoom=${span}`, v)
      for (const dir of [-1, 1]) {
        const startX = 500
        const startPosition = m.position
        for (let px = 1; px <= 300; px++) {
          m.seek(panTarget(m, startPosition, startX, startX + dir * px))
          checkInvariants(m, `pan span=${span} dir=${dir} px=${px}`, v)
        }
      }
    }
    expect(v).toEqual([])
  })

  it('holds through wheel zoom driven to both clamps', () => {
    const v: Violation[] = []
    const m = freshModel()
    for (let i = 0; i < 40; i++) {
      m.zoom(wheelSpan(m, 1))
      checkInvariants(m, `wheel out ${i}`, v)
    }
    expect(m.viewSpan).toBe(MAX_SPAN)
    for (let i = 0; i < 80; i++) {
      m.zoom(wheelSpan(m, -1))
      checkInvariants(m, `wheel in ${i}`, v)
    }
    expect(m.viewSpan).toBe(MIN_SPAN)
    expect(v).toEqual([])
  })

  it('holds through a pinch ratio and its pan handoff', () => {
    const v: Violation[] = []
    const m = freshModel()
    const startDist = 200
    const startSpan = m.windowEnd - m.windowStart
    for (let d = 40; d <= 800; d += 7) {
      m.zoom(pinchSpan(startSpan, startDist, d))
      checkInvariants(m, `pinch dist=${d}`, v)
    }
    // Second finger lifts: the scrubber re-arms a pan from the remaining
    // pointer, so a pinch flows into a pan without a jump.
    const startPosition = m.position
    for (let px = 1; px <= 200; px++) {
      m.seek(panTarget(m, startPosition, 300, 300 + px))
      checkInvariants(m, `handoff px=${px}`, v)
    }
    expect(v).toEqual([])
  })

  it('holds through keyboard nudges, plain and shifted, at several zoom levels', () => {
    const v: Violation[] = []
    const m = freshModel()
    for (const span of [MIN_SPAN, DEFAULT_VIEW_SPAN, MAX_SPAN]) {
      m.zoom(span)
      for (const shift of [false, true]) {
        for (const dir of [-1, 1] as const) {
          for (let i = 0; i < 40; i++) {
            m.seek(keyTarget(m, dir, shift))
            checkInvariants(m, `key span=${span} shift=${shift} dir=${dir} i=${i}`, v)
          }
        }
      }
    }
    expect(v).toEqual([])
  })
})

describe('viewport invariant: playback', () => {
  it('holds through a timeupdate advance across a coverage gap, and the snap', () => {
    const v: Violation[] = []
    // liveEdge in the past so playback has room to run forward.
    const m = freshModel(NOW - 30 * 86400, NOW - 4000)
    let t = m.position
    for (let i = 0; i < 1200; i++) {
      // A realistic non-integer timeupdate cadence; footageToWallclock returns
      // floats and jumps forward by the gap size when playback crosses one.
      t += 0.2503
      if (i === 500) t += 137.4
      m.advance(t)
      checkInvariants(m, `tick ${i}`, v)
    }
    // ensurePlaybackAt's coverage snap moves the playhead forward onto footage.
    m.advance(Math.floor(t) + 43)
    checkInvariants(m, 'coverage snap', v)
    expect(v).toEqual([])
  })

  it('holds while parked at the live edge, at several zoom levels', () => {
    const v: Violation[] = []
    const m = freshModel()
    for (const span of [MIN_SPAN, 1800, DEFAULT_VIEW_SPAN, 4 * 3600, MAX_SPAN]) {
      m.zoom(span)
      m.seek(NOW + 100000) // over-seek: clamped to live
      checkInvariants(m, `park span=${span}`, v)
      expect(m.position).toBe(NOW)
      // Parked at live, the window still overhangs live by half a span — the
      // hatch band and the LIVE capsule depend on it.
      expect(m.windowEnd - NOW).toBeCloseTo(span / 2, 6)
      // Keep dragging into the future: the playhead stays parked, the viewport
      // stays put, and the invariant holds.
      const startPosition = m.position
      for (let px = 1; px <= 40; px++) {
        m.seek(panTarget(m, startPosition, 500, 500 - px))
        checkInvariants(m, `park push span=${span} px=${px}`, v)
      }
      expect(m.position).toBe(NOW)
    }
    expect(v).toEqual([])
  })
})

describe('viewport invariant: randomised session', () => {
  // Sizing. The walk's job is to explore INTERLEAVINGS of the seven mutation
  // kinds that the fixed scenarios above each exercise in isolation — it is
  // what found the only reachable divergence during the refactor. With seven
  // branches, 3,000 steps covers every ordered pair (49) and triple (343) many
  // times over and hits each branch ~400 times, so the marginal value of more
  // steps in the same walk falls away fast. Six independent seeds beat one
  // 18,000-step walk at the same cost: a single walk drifts into one region of
  // the state space, and each seed is reproducible on its own when it fails.
  // The coverage assertions at the end are what actually defend this number —
  // if a change ever made 3,000 steps too few to reach the interesting states,
  // they fail loudly instead of the walk quietly going vacuous.
  const SEEDS = [1, 7, 12345, 99991, 20260817, 2 ** 31 - 9]
  const STEPS = 3000

  it('holds across interleaved pans, zooms, ticks, snaps, nudges and re-arms', () => {
    const v: Violation[] = []
    let panned = 0
    let zoomed = 0
    let ticked = 0
    let snapped = 0
    let nudged = 0
    let rearmed = 0
    let atLive = 0
    let atFloor = 0
    let minSpan = Infinity
    let maxSpan = 0

    for (const s of SEEDS) {
      let seed = s
      const rnd = () => (seed = (seed * 1103515245 + 12345) & 0x7fffffff) / 0x7fffffff
      // A deliberately SHALLOW playable domain: one hour, as on a camera an
      // hour into its first recording. A pan step moves at most ~0.45 x span
      // and the walk drifts forward (playback and the coverage snap both
      // advance), so a deep floor is simply never reached and the floor clamp
      // goes untested — at one hour every seed parks on BOTH bounds. It also
      // puts the whole playable domain inside a single viewport at most zoom
      // levels, which is the awkward case where the window is wider than
      // everything there is to play.
      const m = freshModel(NOW - 3600)
      let startPosition = m.position
      let startX = 500
      for (let i = 0; i < STEPS; i++) {
        const r = rnd()
        if (r < 0.4) {
          m.seek(panTarget(m, startPosition, startX, startX + (rnd() - 0.5) * 900))
          panned++
        } else if (r < 0.55) {
          m.zoom(wheelSpan(m, rnd() > 0.5 ? 1 : -1))
          zoomed++
        } else if (r < 0.65) {
          m.zoom(pinchSpan(m.windowEnd - m.windowStart, 200, 40 + rnd() * 700))
          zoomed++
        } else if (r < 0.8) {
          // timeupdate: forward-biased, and never past live (the route's chunk
          // end is itself capped at liveEdge).
          m.advance(Math.min(m.position + (rnd() - 0.3) * 5, m.liveEdge))
          ticked++
        } else if (r < 0.87) {
          m.seek(keyTarget(m, rnd() > 0.5 ? 1 : -1, rnd() > 0.5))
          nudged++
        } else if (r < 0.94) {
          m.advance(Math.min(m.position + Math.floor(rnd() * 600), m.liveEdge))
          snapped++
        } else {
          // pointerdown: re-arm a relative pan from the current position.
          startPosition = m.position
          startX = 200 + rnd() * 600
          rearmed++
        }
        checkInvariants(m, `seed=${s} step=${i}`, v)
        if (m.position === m.liveEdge) atLive++
        if (m.position === m.playbackFloor) atFloor++
        if (m.viewSpan < minSpan) minSpan = m.viewSpan
        if (m.viewSpan > maxSpan) maxSpan = m.viewSpan
      }
    }

    expect(v).toEqual([])

    // Coverage: the walk must actually have reached the states that matter.
    // These are what justify the step count — not the number itself.
    expect(panned).toBeGreaterThan(100)
    expect(zoomed).toBeGreaterThan(100)
    expect(ticked).toBeGreaterThan(100)
    expect(snapped).toBeGreaterThan(100)
    expect(nudged).toBeGreaterThan(100)
    expect(rearmed).toBeGreaterThan(100)
    // Both playable bounds parked on, and both zoom clamps reached. Observed
    // ~2400 live parks and ~440 floor parks across the six seeds, so these
    // thresholds carry an order of magnitude of slack.
    expect(atLive).toBeGreaterThan(200)
    expect(atFloor).toBeGreaterThan(50)
    expect(minSpan).toBe(MIN_SPAN)
    expect(maxSpan).toBe(MAX_SPAN)
  })
})

describe('viewport invariant: follow is what ties the two together', () => {
  it('stops re-centring when follow is off, and the live clamp still holds', () => {
    // Nothing in the app clears follow yet. This pins the seam the split
    // exists for: with follow off the playhead moves and the viewport does not,
    // which is precisely what lets the timeline show one stretch of time while
    // playing another. The live-edge clamp is deliberately independent of
    // follow, so it keeps applying.
    const m = freshModel()
    const frozen = m.viewStart
    m.follow = false
    m.seek(NOW - 3600)
    expect(m.position).toBe(NOW - 3600)
    expect(m.viewStart).toBe(frozen)
    m.zoom(MIN_SPAN)
    expect(m.viewStart).toBe(frozen)
    // Clamp still enforced when the viewport is written directly.
    m.setViewStart(NOW + 10000)
    expect(m.viewStart).toBe(NOW - m.viewSpan / 2)
  })
})

// The wall-clock time drawn at `fraction` across the current window — what a
// fixed-mode zoom promises to keep under the cursor.
function timeUnder(m: ViewportModel, fraction: number): number {
  return m.windowStart + fraction * (m.windowEnd - m.windowStart)
}

// A model in fixed mode from the entry onward, as a user who has the
// preference set to 'fixed' gets it.
function fixedModel(floor = NOW - 30 * 86400, now = NOW, t: string | null = null): ViewportModel {
  const m = new ViewportModel()
  m.follow = false
  m.playbackFloor = floor
  m.enter(now, t)
  return m
}

// Is the playhead outside the drawn window? Read directly here rather than
// through a shared predicate: nothing in the model branches on it any more.
function playheadOutside(m: ViewportModel): boolean {
  return m.position < m.windowStart || m.position > m.windowEnd
}

describe('the mode decides what a zoom holds in place', () => {
  it('follow mode ignores the reported anchor entirely', () => {
    // The acceptance property for this whole change, stated as a comparison:
    // two identical follow-mode models, one driven with an anchor on every
    // notch and one driven with none, must stay bitwise identical. Follow mode
    // is byte-for-byte what it was before the anchor existed.
    const v: Violation[] = []
    const anchored = freshModel()
    const plain = freshModel()
    // Fractions chosen to be nowhere near the centre, so an anchor that leaked
    // through would show immediately.
    const fractions = [0, 0.05, 0.17, 0.42, 0.83, 1]
    for (let i = 0; i < 30; i++) {
      const target = wheelSpan(anchored, i % 3 === 0 ? 1 : -1)
      anchored.zoom(target, fractions[i % fractions.length])
      plain.zoom(target)
      expect(anchored.viewStart).toBe(plain.viewStart)
      expect(anchored.viewSpan).toBe(plain.viewSpan)
      expect(anchored.follow).toBe(true)
      checkInvariants(anchored, `follow ignores anchor ${i}`, v)
    }
    expect(v).toEqual([])
  })

  it('fixed mode holds the time under the cursor across a run of notches', () => {
    const m = fixedModel()
    const F = 0.2
    const anchor = timeUnder(m, F)
    for (let i = 0; i < 12; i++) {
      m.zoom(wheelSpan(m, -1), F)
      // Every notch, not just the last: the anchor is recomputed from the
      // viewport each time, so an accumulating error would show up here.
      expect(Math.abs(timeUnder(m, F) - anchor)).toBeLessThanOrEqual(ULP)
    }
    expect(m.viewSpan).toBeLessThan(DEFAULT_VIEW_SPAN)
    // The playhead has not moved, and the window is no longer centred on it —
    // which is the point of a stationary track.
    expect(m.position).toBe(NOW - DEFAULT_VIEW_SPAN / 2)
    expect(m.viewStart).not.toBe(centredViewStart(m.position, m.viewSpan))
  })

  it('fixed mode leaves the window put for a gesture that reports no anchor', () => {
    // Pinch, which this change deliberately does not touch. The span changes
    // and the left edge stays where it is; nothing chases the playhead.
    const m = fixedModel()
    const parked = m.viewStart
    const startSpan = m.viewSpan
    for (let d = 40; d <= 800; d += 7) {
      m.zoom(pinchSpan(startSpan, 200, d))
      expect(m.viewStart).toBe(parked)
    }
    expect(m.follow).toBe(false)
  })

  it('fixed mode holds the anchor when the zoom runs into the span clamp', () => {
    const m = fixedModel()
    const F = 0.8
    // Zoom in far past MIN_SPAN. clampViewSpan bounds the span BEFORE the
    // anchor arithmetic sees it, so the anchor is computed against the span
    // actually applied and stays exact rather than drifting by the clamped-off
    // remainder.
    for (let i = 0; i < 60; i++) m.zoom(wheelSpan(m, -1), F)
    expect(m.viewSpan).toBe(MIN_SPAN)
    const anchor = timeUnder(m, F)
    const parked = m.viewStart
    // Further notches request a span below MIN_SPAN: the applied span does not
    // change, so the window does not move at all.
    for (let i = 0; i < 10; i++) m.zoom(wheelSpan(m, -1), F)
    expect(m.viewSpan).toBe(MIN_SPAN)
    expect(m.viewStart).toBe(parked)
    expect(timeUnder(m, F)).toBe(anchor)
    // And the same at the far end.
    for (let i = 0; i < 80; i++) m.zoom(wheelSpan(m, 1), F)
    expect(m.viewSpan).toBe(MAX_SPAN)
    const wideAnchor = timeUnder(m, F)
    for (let i = 0; i < 10; i++) m.zoom(wheelSpan(m, 1), F)
    expect(m.viewSpan).toBe(MAX_SPAN)
    expect(timeUnder(m, F)).toBe(wideAnchor)
  })

  it('lets the live-edge clamp win over the anchor', () => {
    const v: Violation[] = []
    // Parked at live in follow mode, so the window overhangs live by half a
    // span, then switched to fixed — the state a mode change at the live edge
    // lands in. Zooming IN with the cursor out in the overhang pulls the
    // viewport centre PAST live, which is the one bound clampViewStart
    // enforces regardless of mode.
    const m = freshModel()
    m.seek(NOW + 100000)
    expect(m.position).toBe(NOW)
    m.follow = false
    const anchor = timeUnder(m, 1)
    const before = m.viewStart
    m.zoom(wheelSpan(m, -1), 1)
    checkInvariants(m, 'live clamp', v)
    // The bound wins: the centre is held at live and the anchor is NOT kept.
    expect(m.viewStart).toBe(NOW - m.viewSpan / 2)
    expect(timeUnder(m, 1)).toBeLessThan(anchor)
    // The unclamped answer is what got overridden, and by exactly the overshoot.
    const wanted = anchoredViewStart(before, DEFAULT_VIEW_SPAN, m.viewSpan, 1)
    expect(wanted).toBeGreaterThan(m.viewStart)
    expect(v).toEqual([])
  })

  it('keeps a fixed-mode zoom through playback — nothing drags the window back', () => {
    // The reason follow had to stop being gesture-driven state: with the mode
    // deciding, an anchored window simply survives every timeupdate. No
    // hysteresis, no re-engagement, no snap back to centre.
    const m = fixedModel()
    m.zoom(MIN_SPAN, 0) // pin the left edge: the centred playhead falls off the right
    const anchored = m.viewStart
    expect(playheadOutside(m)).toBe(true)
    for (let i = 0; i < 500; i++) m.advance(m.position + 0.25)
    expect(m.viewStart).toBe(anchored)
    expect(m.follow).toBe(false)
  })
})
