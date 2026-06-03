import { describe, expect, it } from 'vitest'
import { formatDuration, relativeTime } from './time'

const NOW = new Date('2026-06-03T12:00:00Z')
const rtf = new Intl.RelativeTimeFormat([], { numeric: 'auto' })
const dtf = new Intl.DateTimeFormat([], { dateStyle: 'medium' })

function isoOffset(seconds: number): string {
  return new Date(NOW.getTime() + seconds * 1000).toISOString()
}

describe('relativeTime', () => {
  it("returns 'now' inside the 45-second threshold", () => {
    expect(relativeTime(isoOffset(0), NOW)).toBe('now')
    expect(relativeTime(isoOffset(-30), NOW)).toBe('now')
    expect(relativeTime(isoOffset(-44), NOW)).toBe('now')
  })

  it('formats minutes between 45s and 1h', () => {
    expect(relativeTime(isoOffset(-60), NOW)).toBe(rtf.format(-1, 'minute'))
    expect(relativeTime(isoOffset(-300), NOW)).toBe(rtf.format(-5, 'minute'))
    expect(relativeTime(isoOffset(-50 * 60), NOW)).toBe(rtf.format(-50, 'minute'))
  })

  it('formats hours between 1h and 24h', () => {
    expect(relativeTime(isoOffset(-3600), NOW)).toBe(rtf.format(-1, 'hour'))
    expect(relativeTime(isoOffset(-3 * 3600), NOW)).toBe(rtf.format(-3, 'hour'))
    expect(relativeTime(isoOffset(-20 * 3600), NOW)).toBe(rtf.format(-20, 'hour'))
  })

  it('formats days between 1d and 7d', () => {
    expect(relativeTime(isoOffset(-86400), NOW)).toBe(rtf.format(-1, 'day'))
    expect(relativeTime(isoOffset(-3 * 86400), NOW)).toBe(rtf.format(-3, 'day'))
  })

  it('formats weeks between 7d and 31d', () => {
    expect(relativeTime(isoOffset(-10 * 86400), NOW)).toBe(rtf.format(-1, 'week'))
    expect(relativeTime(isoOffset(-21 * 86400), NOW)).toBe(rtf.format(-3, 'week'))
  })

  it('falls back to an absolute date past ~31 days', () => {
    const farPastMs = NOW.getTime() - 60 * 86400 * 1000
    const iso = new Date(farPastMs).toISOString()
    expect(relativeTime(iso, NOW)).toBe(dtf.format(farPastMs))
  })
})

describe('formatDuration', () => {
  it("returns '—' for null", () => {
    expect(formatDuration(null)).toBe('—')
  })

  it('formats sub-minute seconds', () => {
    expect(formatDuration(0)).toBe('0s')
    expect(formatDuration(12)).toBe('12s')
    expect(formatDuration(59)).toBe('59s')
  })

  it('formats minutes with and without trailing seconds', () => {
    expect(formatDuration(60)).toBe('1min')
    expect(formatDuration(90)).toBe('1min 30s')
    expect(formatDuration(3540)).toBe('59min')
    expect(formatDuration(3599)).toBe('59min 59s')
  })

  it('formats hours with and without trailing minutes', () => {
    expect(formatDuration(3600)).toBe('1h')
    expect(formatDuration(3900)).toBe('1h 5min')
    expect(formatDuration(7200)).toBe('2h')
    expect(formatDuration(7260)).toBe('2h 1min')
  })
})
