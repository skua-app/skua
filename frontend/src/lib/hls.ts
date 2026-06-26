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
  // Normalise the HEVC sample-entry tag hev1 -> hvc1 before probing. Safari's
  // canPlayType REJECTS "hev1" (returns '') and accepts only "hvc1", yet the
  // player retags hev1 -> hvc1 on the fly for actual playback — so the gate
  // must probe hvc1 to match what really plays. A device with no HEVC decoder
  // (e.g. Redmi via MediaSource.isTypeSupported) still returns false for hvc1,
  // so the gate stays correct everywhere; this only fixes Safari's hev1
  // rejection. Only the HEVC video token carries "hev1" — mp4a/avc1 never do.
  const type = `video/mp4; codecs="${codecs.replaceAll('hev1', 'hvc1')}"`
  if (supportsNativeHls(video)) {
    return video.canPlayType(type) !== ''
  }
  if (typeof MediaSource !== 'undefined' && typeof MediaSource.isTypeSupported === 'function') {
    return MediaSource.isTypeSupported(type)
  }
  return false
}
