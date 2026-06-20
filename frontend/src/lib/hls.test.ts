import { describe, expect, it } from 'vitest'
import { canDecodeRecording, supportsNativeHls } from './hls'

function videoStub(answer: '' | 'maybe' | 'probably'): Pick<HTMLVideoElement, 'canPlayType'> {
  return {
    canPlayType: (type: string) =>
      type === 'application/vnd.apple.mpegurl' ? answer : ('' as CanPlayTypeResult)
  }
}

// Native-path stub: reports native HLS support (so canDecodeRecording takes
// the canPlayType branch) and answers the mp4-codecs probe with `mp4Answer`.
function nativeCodecStub(
  mp4Answer: '' | 'maybe' | 'probably'
): Pick<HTMLVideoElement, 'canPlayType'> {
  return {
    canPlayType: (type: string) =>
      type === 'application/vnd.apple.mpegurl' ? 'maybe' : (mp4Answer as CanPlayTypeResult)
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

describe('canDecodeRecording (native path)', () => {
  it('returns true when the native engine can play the codec', () => {
    expect(canDecodeRecording('avc1.4d0032,mp4a.40.2', nativeCodecStub('probably'))).toBe(true)
  })

  it('returns false when the native engine cannot play the codec', () => {
    expect(canDecodeRecording('hev1.1.6.L150.00,mp4a.40.2', nativeCodecStub(''))).toBe(false)
  })
})
