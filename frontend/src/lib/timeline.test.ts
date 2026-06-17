import { describe, expect, it } from 'vitest'
import type { RecordingsSummary } from './api'
import {
  clockHourBucket,
  flattenSummary,
  fractionToTime,
  hourCells,
  timeToFraction,
  type TimelineHour
} from './timeline'

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

describe('hourCells', () => {
  function hourAt(tSec: number, fraction: number, events = 0): TimelineHour {
    const d = new Date(0)
    d.setTime(tSec * 1000)
    return { hourStart: d, recordedFraction: fraction, events, motion: 0 }
  }

  it('maps an hour fully inside the window to the expected x0/x1', () => {
    const start = 0
    const end = 6 * 3600
    const cells = hourCells([hourAt(2 * 3600, 0.5, 4)], start, end)
    expect(cells).toHaveLength(1)
    expect(cells[0]!.x0).toBeCloseTo(2 / 6, 10)
    expect(cells[0]!.x1).toBeCloseTo(3 / 6, 10)
    expect(cells[0]!.fraction).toBe(0.5)
    expect(cells[0]!.events).toBe(4)
  })

  it('clamps hours that straddle the left or right edge', () => {
    const start = 1800
    const end = 1800 + 3 * 3600
    const cells = hourCells([hourAt(0, 1.0), hourAt(2 * 3600 + 1800, 0.7)], start, end)
    expect(cells).toHaveLength(2)
    expect(cells[0]!.x0).toBe(0)
    expect(cells[0]!.x1).toBeGreaterThan(0)
    expect(cells[0]!.x1).toBeLessThan(1)
    expect(cells[1]!.x1).toBe(1)
    expect(cells[1]!.x0).toBeGreaterThan(0)
    expect(cells[1]!.x0).toBeLessThan(1)
  })

  it('drops hours fully outside the window', () => {
    const start = 10 * 3600
    const end = 12 * 3600
    expect(hourCells([hourAt(0, 1)], start, end)).toEqual([])
    expect(hourCells([hourAt(20 * 3600, 1)], start, end)).toEqual([])
  })

  it('returns cells in ascending x0 order regardless of input', () => {
    const start = 0
    const end = 6 * 3600
    const cells = hourCells(
      [hourAt(4 * 3600, 0.2), hourAt(1 * 3600, 0.4), hourAt(3 * 3600, 0.9)],
      start,
      end
    )
    expect(cells.map((c) => c.x0)).toEqual([...cells.map((c) => c.x0)].sort((a, b) => a - b))
  })
})

describe('clockHourBucket', () => {
  it('returns the local clock-hour containing a mid-hour timestamp', () => {
    // 14:37:12.500 local → bucket 14:00–15:00 local.
    const t = Math.floor(new Date(2026, 5, 18, 14, 37, 12, 500).getTime() / 1000)
    const { start, end } = clockHourBucket(t)
    expect(end - start).toBe(3600)
    expect(start).toBeLessThanOrEqual(t)
    expect(t).toBeLessThan(end)
    const expectedStart = Math.floor(new Date(2026, 5, 18, 14, 0, 0, 0).getTime() / 1000)
    expect(start).toBe(expectedStart)
  })

  it('returns the same hour when the timestamp is exactly on the boundary', () => {
    const t = Math.floor(new Date(2026, 5, 18, 9, 0, 0, 0).getTime() / 1000)
    const { start, end } = clockHourBucket(t)
    expect(start).toBe(t)
    expect(end).toBe(t + 3600)
  })

  it('aligns start to the local hour boundary for an arbitrary timestamp', () => {
    const t = 1779312123
    const { start, end } = clockHourBucket(t)
    const back = new Date(start * 1000)
    expect(back.getMinutes()).toBe(0)
    expect(back.getSeconds()).toBe(0)
    expect(back.getMilliseconds()).toBe(0)
    expect(end - start).toBe(3600)
    expect(start).toBeLessThanOrEqual(t)
    expect(t).toBeLessThan(end)
  })
})
