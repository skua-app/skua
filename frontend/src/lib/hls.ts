// supportsNativeHls reports whether the given video element can play
// Apple HLS natively. Safari (macOS, iOS, iPadOS) returns 'maybe' or
// 'probably' for application/vnd.apple.mpegurl; every other engine
// returns the empty string. Used to gate the dynamic hls.js import:
// native HLS path pays zero bytes for the library.
//
// Takes the element so unit tests can pass a stub whose canPlayType
// returns a controlled value — no environment reads.
export function supportsNativeHls(video: Pick<HTMLVideoElement, 'canPlayType'>): boolean {
  return video.canPlayType('application/vnd.apple.mpegurl') !== ''
}

// canDecodeRecording reports whether this device can decode an fMP4 recording
// carrying the given CODECS string, checked against the path the player will
// actually take: the native-HLS engine's canPlayType when supportsNativeHls
// (Safari/iOS), otherwise MediaSource.isTypeSupported (hls.js / MSE). Returns
// false when no MediaSource exists. Used to gate full-res playback on the
// recording timeline so an undecodable codec (e.g. H.265 with no hardware
// decoder) degrades to preview-only instead of a failed full-res load.
//
// Pure and element-injected like supportsNativeHls, so unit tests pass a stub
// canPlayType — no environment reads on the native branch.
export function canDecodeRecording(
  codecs: string,
  video: Pick<HTMLVideoElement, 'canPlayType'>
): boolean {
  const type = `video/mp4; codecs="${codecs}"`
  if (supportsNativeHls(video)) {
    return video.canPlayType(type) !== ''
  }
  if (typeof MediaSource !== 'undefined' && typeof MediaSource.isTypeSupported === 'function') {
    return MediaSource.isTypeSupported(type)
  }
  return false
}
