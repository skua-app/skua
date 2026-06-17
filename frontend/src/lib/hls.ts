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
