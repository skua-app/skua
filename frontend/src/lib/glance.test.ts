import { describe, expect, it } from 'vitest'
import type { Moment } from './api'
import { isMomentLive } from './glance'

function makeMoment(overrides: Partial<Moment> = {}): Moment {
  return {
    id: 'rev-1',
    cam_id: 'camA',
    started_at: '2026-06-04T22:00:00Z',
    ended_at: '2026-06-04T22:00:30Z',
    severity: 'detection',
    kinds: ['person'],
    labels: ['person'],
    zones: [],
    detection_ids: ['1779310005.0-aaa'],
    thumb_event_id: '1779310005.0-aaa',
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
