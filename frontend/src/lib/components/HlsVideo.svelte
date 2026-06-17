<script lang="ts">
  import { supportsNativeHls } from '$lib/hls'
  import type Hls from 'hls.js'
  import type { ErrorData, Events } from 'hls.js'

  interface Props {
    src: string
    muted?: boolean
    onError?: (msg: string) => void
  }

  let { src, muted = true, onError }: Props = $props()

  let videoEl = $state<HTMLVideoElement | null>(null)

  // Attach runs client-side only — $effect never fires during SSR/prerender.
  // hls.js is dynamic-imported so Safari/iOS (which take the native path)
  // never downloads the library. The `disposed` flag mirrors whep.ts's
  // `closed` guard: a teardown that races the async import must not attach
  // a stale Hls instance.
  $effect(() => {
    const video = videoEl
    if (!video || !src) return

    type HlsModule = typeof import('hls.js')
    let hls: Hls | null = null
    let HlsCtor: HlsModule['default'] | null = null
    let onHlsError: ((event: Events.ERROR, data: ErrorData) => void) | null = null
    let disposed = false

    if (supportsNativeHls(video)) {
      video.src = src
    } else {
      void import('hls.js').then((mod) => {
        if (disposed) return
        const Ctor = mod.default
        if (!Ctor.isSupported()) {
          onError?.('hls_unsupported')
          return
        }
        HlsCtor = Ctor
        const instance = new Ctor()
        hls = instance
        onHlsError = (_evt, data) => {
          if (!data.fatal) {
            console.debug('[hls] non-fatal error:', data.type, data.details)
            return
          }
          if (data.type === Ctor.ErrorTypes.NETWORK_ERROR) {
            instance.startLoad()
          } else if (data.type === Ctor.ErrorTypes.MEDIA_ERROR) {
            instance.recoverMediaError()
          } else {
            instance.destroy()
            hls = null
            onError?.(data.details ?? data.type ?? 'hls_fatal')
          }
        }
        instance.on(Ctor.Events.ERROR, onHlsError)
        instance.loadSource(src)
        instance.attachMedia(video)
      })
    }

    return () => {
      disposed = true
      if (hls) {
        if (HlsCtor && onHlsError) hls.off(HlsCtor.Events.ERROR, onHlsError)
        hls.destroy()
        hls = null
      }
      // Native path teardown: clear the URL and force the element to
      // release its loaded resource. Mirrors whep.ts's srcObject = null.
      video.removeAttribute('src')
      video.load()
    }
  })
</script>

<!-- controls=on for 2b-i: native scrub is how recordings get smoke-tested on
     iOS and Android before the custom scrubber arrives in 2b-iii.
     object-fit: contain — recordings are true aspect ratio; the fill rule
     is the live anamorphic-substream invariant only and must not leak. -->
<video bind:this={videoEl} playsinline controls {muted} class="hls-video"></video>

<style>
  .hls-video {
    width: 100%;
    height: 100%;
    object-fit: contain;
    background: var(--feed);
  }
</style>
