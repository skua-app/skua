import { describe, expect, it } from 'vitest'
import type { Moment } from './api'
import { momentRouteTarget, momentToEventItem, pickSeenThrough } from './glance'

function makeMoment(overrides: Partial<Moment> = {}): Moment {
  return {
    cam_id: 'camA',
    started_at: '2026-06-04T22:00:00Z',
    ended_at: '2026-06-04T22:00:30Z',
    kinds: ['person'],
    labels: ['person'],
    event_count: 1,
    representative_event_id: 'evt-1',
    representative_has_clip: false,
    ...overrides
  }
}

describe('pickSeenThrough', () => {
  it('returns null on an empty list', () => {
    expect(pickSeenThrough([])).toBeNull()
  })

  it("returns the first moment's started_at (newest-first ordering)", () => {
    const moments: Moment[] = [
      makeMoment({ started_at: '2026-06-04T22:30:00Z' }),
      makeMoment({ started_at: '2026-06-04T22:00:00Z' })
    ]
    expect(pickSeenThrough(moments)).toBe('2026-06-04T22:30:00Z')
  })
})

describe('momentRouteTarget', () => {
  it("returns 'focus' when the moment is still live (null ended_at)", () => {
    expect(momentRouteTarget(makeMoment({ ended_at: null }))).toBe('focus')
  })

  it("returns 'clip' when the moment has ended", () => {
    expect(momentRouteTarget(makeMoment({ ended_at: '2026-06-04T22:00:30Z' }))).toBe('clip')
  })
})

describe('momentToEventItem', () => {
  it('maps the core fields and carries representative_has_clip into has_clip', () => {
    const m = makeMoment({
      representative_event_id: 'rep-7',
      cam_id: 'camB',
      started_at: '2026-06-04T22:10:00Z',
      ended_at: '2026-06-04T22:10:30Z',
      kinds: ['vehicle', 'person'],
      labels: ['car', 'person'],
      event_count: 3,
      representative_has_clip: true
    })
    const ev = momentToEventItem(m)
    expect(ev).toEqual({
      id: 'rep-7',
      cam_id: 'camB',
      started_at: '2026-06-04T22:10:00Z',
      ended_at: '2026-06-04T22:10:30Z',
      duration_seconds: null,
      label: 'car',
      kind: 'vehicle',
      score: null,
      has_snapshot: true,
      has_clip: true
    })
  })

  it('falls back to empty label and kind "other" when arrays are empty', () => {
    const ev = momentToEventItem(makeMoment({ kinds: [], labels: [] }))
    expect(ev.label).toBe('')
    expect(ev.kind).toBe('other')
  })

  it('propagates null ended_at (still-live moments)', () => {
    const ev = momentToEventItem(makeMoment({ ended_at: null }))
    expect(ev.ended_at).toBeNull()
  })
})
