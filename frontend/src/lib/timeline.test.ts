import { describe, expect, it } from 'vitest'
import type { RecordingsSummary } from './api'
import { flattenSummary } from './timeline'

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
