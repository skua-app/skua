import { describe, expect, it } from 'vitest'
import { formatSize } from './size'

describe('formatSize', () => {
  it('keeps sub-1024 MiB in MiB', () => {
    expect(formatSize(0)).toBe('0.0 MiB')
    expect(formatSize(512)).toBe('512 MiB')
    expect(formatSize(1023)).toBe('1023 MiB')
  })

  it('rolls up to GiB at the 1024 boundary', () => {
    expect(formatSize(1024)).toBe('1.0 GiB')
    expect(formatSize(1536)).toBe('1.5 GiB')
    expect(formatSize(10 * 1024)).toBe('10 GiB')
  })

  it('rolls up to TiB and stops there', () => {
    expect(formatSize(1024 * 1024)).toBe('1.0 TiB')
    expect(formatSize(5.5 * 1024 * 1024)).toBe('5.5 TiB')
    // Beyond TiB it stays in TiB rather than inventing PiB.
    expect(formatSize(2048 * 1024 * 1024)).toBe('2048 TiB')
  })

  it('tightens precision once the value reaches 10', () => {
    expect(formatSize(9.9 * 1024)).toBe('9.9 GiB')
    expect(formatSize(10 * 1024)).toBe('10 GiB')
  })

  it('handles negatives and non-finite input', () => {
    expect(formatSize(-2048)).toBe('-2.0 GiB')
    expect(formatSize(NaN)).toBe('—')
    expect(formatSize(Infinity)).toBe('—')
  })
})
