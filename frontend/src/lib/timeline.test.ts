import { describe, expect, it } from 'vitest'
import type { CoverageSegment, RecordingsSummary } from './api'
import { flattenSummary, footageToWallclock, fractionToTime, timeToFraction } from './timeline'

describe('flattenSummary', () => {
  it('returns an empty array for an empty summary', () => {
    expect(flattenSummary([])).toEqual([])
  })

  it('skips a day with no hours', () => {
    const summary: RecordingsSummary = [{ day: '2026-06-17', events: 0, hours: [] }]
    expect(flattenSummary(summary)).toEqual([])
  })

  it('sorts hours ascending across multiple days regardless of input order', () => {
    // Frigate returns days newest-first and hours newest-first; the
    // input here is deliberately scrambled to prove the sort is real.
    const summary: RecordingsSummary = [
      {
        day: '2026-06-17',
        events: 0,
        hours: [
          { hour: '03', events: 0, motion: 0, objects: 0, duration: 3600 },
          { hour: '01', events: 0, motion: 0, objects: 0, duration: 3600 }
        ]
      },
      {
        day: '2026-06-16',
        events: 0,
        hours: [{ hour: '23', events: 0, motion: 0, objects: 0, duration: 3600 }]
      }
    ]
    const out = flattenSummary(summary)
    expect(out.map((h) => h.hourStart.getTime())).toEqual(
      [...out.map((h) => h.hourStart.getTime())].sort((a, b) => a - b)
    )
    expect(out).toHaveLength(3)
    expect(out[0]!.hourStart).toEqual(new Date('2026-06-16T23:00:00'))
    expect(out[2]!.hourStart).toEqual(new Date('2026-06-17T03:00:00'))
  })

  it('clamps recordedFraction into [0,1]', () => {
    const summary: RecordingsSummary = [
      {
        day: '2026-06-17',
        events: 0,
        hours: [
          { hour: '00', events: 0, motion: 0, objects: 0, duration: 0 },
          { hour: '01', events: 0, motion: 0, objects: 0, duration: 2480 },
          { hour: '02', events: 0, motion: 0, objects: 0, duration: 3600 },
          { hour: '03', events: 0, motion: 0, objects: 0, duration: 9999 }
        ]
      }
    ]
    const out = flattenSummary(summary)
    expect(out[0]!.recordedFraction).toBe(0)
    expect(out[1]!.recordedFraction).toBeCloseTo(0.6889, 4)
    expect(out[2]!.recordedFraction).toBe(1)
    expect(out[3]!.recordedFraction).toBe(1)
  })

  it('carries events and motion through onto the matching hour', () => {
    const summary: RecordingsSummary = [
      {
        day: '2026-06-17',
        events: 12,
        hours: [
          { hour: '14', events: 5, motion: 8, objects: 3, duration: 2480 },
          { hour: '13', events: 7, motion: 9, objects: 4, duration: 3600 }
        ]
      }
    ]
    const out = flattenSummary(summary)
    const h13 = out.find((h) => h.hourStart.getHours() === 13)!
    const h14 = out.find((h) => h.hourStart.getHours() === 14)!
    expect(h13.events).toBe(7)
    expect(h13.motion).toBe(9)
    expect(h14.events).toBe(5)
    expect(h14.motion).toBe(8)
  })
})

describe('timeToFraction', () => {
  it('returns 0.5 at the window midpoint', () => {
    expect(timeToFraction(1500, 1000, 2000)).toBeCloseTo(0.5, 10)
  })

  it('clamps to 0 before the window start', () => {
    expect(timeToFraction(900, 1000, 2000)).toBe(0)
  })

  it('clamps to 1 after the window end', () => {
    expect(timeToFraction(2500, 1000, 2000)).toBe(1)
  })

  it('returns 0 for a degenerate window (end <= start)', () => {
    expect(timeToFraction(1500, 2000, 2000)).toBe(0)
    expect(timeToFraction(1500, 2000, 1000)).toBe(0)
  })
})

describe('fractionToTime', () => {
  it('round-trips with timeToFraction at sample points', () => {
    const start = 1700000000
    const end = start + 6 * 3600
    for (const sample of [start, start + 900, start + 3 * 3600, end - 1, end]) {
      const f = timeToFraction(sample, start, end)
      expect(fractionToTime(f, start, end)).toBeCloseTo(sample, 6)
    }
  })

  it('clamps out-of-range fractions before mapping back', () => {
    expect(fractionToTime(-0.5, 1000, 2000)).toBe(1000)
    expect(fractionToTime(1.7, 1000, 2000)).toBe(2000)
  })
})

describe('footageToWallclock', () => {
  it('maps continuously when coverage is empty', () => {
    expect(footageToWallclock(1000, 0, [])).toBe(1000)
    expect(footageToWallclock(1000, 42, [])).toBe(1042)
  })

  it('maps continuously when a single band spans the whole chunk', () => {
    const coverage: CoverageSegment[] = [{ start: 1000, end: 2000 }]
    expect(footageToWallclock(1000, 0, coverage)).toBe(1000)
    expect(footageToWallclock(1000, 250, coverage)).toBe(1250)
    expect(footageToWallclock(1000, 1000, coverage)).toBe(2000)
  })

  it('jumps an interior gap forward by the gap size', () => {
    // Recorded [1000,1100) then a 200s gap then [1300,1500). Footage splices
    // the two bands: 0..100 -> [1000,1100), 100..300 -> [1300,1500).
    const coverage: CoverageSegment[] = [
      { start: 1000, end: 1100 },
      { start: 1300, end: 1500 }
    ]
    // Before the gap: linear within the first band.
    expect(footageToWallclock(1000, 50, coverage)).toBe(1050)
    expect(footageToWallclock(1000, 100, coverage)).toBe(1100)
    // After the gap: jumped forward by the 200s gap (footage 100 -> band-2 start).
    expect(footageToWallclock(1000, 100.0001, coverage)).toBeCloseTo(1300.0001, 4)
    expect(footageToWallclock(1000, 150, coverage)).toBe(1350)
    expect(footageToWallclock(1000, 300, coverage)).toBe(1500)
  })

  it('is order-independent (sorts coverage before walking)', () => {
    const coverage: CoverageSegment[] = [
      { start: 1300, end: 1500 },
      { start: 1000, end: 1100 }
    ]
    expect(footageToWallclock(1000, 150, coverage)).toBe(1350)
  })

  it('maps footage 0 to the next band start when chunkStart lands in a gap', () => {
    // chunkStart 1150 sits in the [1100,1300) gap; the first usable band is
    // [1300,1500), so footage 0 maps to that band's start.
    const coverage: CoverageSegment[] = [
      { start: 1000, end: 1100 },
      { start: 1300, end: 1500 }
    ]
    expect(footageToWallclock(1150, 0, coverage)).toBe(1300)
    expect(footageToWallclock(1150, 50, coverage)).toBe(1350)
  })

  it('falls back to continuous mapping past all loaded coverage', () => {
    const coverage: CoverageSegment[] = [{ start: 1000, end: 1100 }]
    // Recorded duration is 100s; footage 150 exceeds it -> chunkStart + ct.
    expect(footageToWallclock(1000, 150, coverage)).toBe(1150)
  })
})
