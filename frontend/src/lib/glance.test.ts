import { describe, expect, it } from 'vitest'
import type { Moment } from './api'
import { isMomentLive } from './glance'

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
    events: [
      {
        id: 'evt-1',
        cam_id: 'camA',
        started_at: '2026-06-04T22:00:00Z',
        ended_at: '2026-06-04T22:00:30Z',
        duration_seconds: 30,
        label: 'person',
        kind: 'person',
        score: null,
        has_snapshot: true,
        has_clip: false
      }
    ],
    ...overrides
  }
}

describe('isMomentLive', () => {
  it('returns true when the moment has no ended_at (still live)', () => {
    expect(isMomentLive(makeMoment({ ended_at: null }))).toBe(true)
  })

  it('returns false when the moment has ended', () => {
    expect(isMomentLive(makeMoment({ ended_at: '2026-06-04T22:00:30Z' }))).toBe(false)
  })
})
