import { describe, expect, it } from 'vitest'
import { supportsNativeHls } from './hls'

function videoStub(answer: '' | 'maybe' | 'probably'): Pick<HTMLVideoElement, 'canPlayType'> {
  return {
    canPlayType: (type: string) =>
      type === 'application/vnd.apple.mpegurl' ? answer : ('' as CanPlayTypeResult)
  }
}

describe('supportsNativeHls', () => {
  it('returns true when canPlayType reports "maybe"', () => {
    expect(supportsNativeHls(videoStub('maybe'))).toBe(true)
  })

  it('returns true when canPlayType reports "probably"', () => {
    expect(supportsNativeHls(videoStub('probably'))).toBe(true)
  })

  it('returns false when canPlayType reports the empty string', () => {
    expect(supportsNativeHls(videoStub(''))).toBe(false)
  })
})
