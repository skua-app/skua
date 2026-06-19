<script lang="ts">
  import { supportsNativeHls } from '$lib/hls'
  import type Hls from 'hls.js'
  import type { ErrorData, Events } from 'hls.js'

  interface Props {
    src: string
    muted?: boolean
    // controls defaults OFF: 2b-i relied on native controls for smoke
    // testing, but the real timeline screen (2b-iii) supplies its own
    // play/pause + scrubber, so the element renders chromeless by default.
    controls?: boolean
    // Bindable so a parent can drive the element directly (currentTime
    // seeks, play/pause, timeupdate listeners). The attach/teardown effect
    // below reads this same element.
    video?: HTMLVideoElement | null
    onError?: (msg: string) => void
  }

  let { src, muted = true, controls = false, video = $bindable(null), onError }: Props = $props()

  // Attach runs client-side only — $effect never fires during SSR/prerender.
  // hls.js is dynamic-imported so Safari/iOS (which take the native path)
  // never downloads the library. The `disposed` flag mirrors whep.ts's
  // `closed` guard: a teardown that races the async import must not attach
  // a stale Hls instance.
  $effect(() => {
    const el = video
    if (!el || !src) return

    type HlsModule = typeof import('hls.js')
    let hls: Hls | null = null
    let HlsCtor: HlsModule['default'] | null = null
    let onHlsError: ((event: Events.ERROR, data: ErrorData) => void) | null = null
    let onNativeError: (() => void) | null = null
    let disposed = false

    if (supportsNativeHls(el)) {
      el.src = src
      // Native path had no error surfacing: a 404 / unsupported manifest
      // would hang silently with the parent stuck on its poster. Report it.
      onNativeError = () => {
        if (disposed) return
        onError?.(el.error ? `media_error_${el.error.code}` : 'media_error')
      }
      el.addEventListener('error', onNativeError)
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
        // A fatal NETWORK_ERROR on the manifest/level (e.g. a 404 on an empty
        // chunk range) will never recover by reloading, and a bare
        // startLoad() loop retries it forever without ever surfacing. Treat
        // manifest/level load failures as terminal, and cap transient
        // (fragment) network retries before giving up.
        let networkRetries = 0
        const MAX_NETWORK_RETRIES = 2
        const terminalNetworkDetails = new Set<string>([
          Ctor.ErrorDetails.MANIFEST_LOAD_ERROR,
          Ctor.ErrorDetails.MANIFEST_LOAD_TIMEOUT,
          Ctor.ErrorDetails.MANIFEST_PARSING_ERROR,
          Ctor.ErrorDetails.LEVEL_LOAD_ERROR,
          Ctor.ErrorDetails.LEVEL_LOAD_TIMEOUT
        ])
        onHlsError = (_evt, data) => {
          if (!data.fatal) {
            console.debug('[hls] non-fatal error:', data.type, data.details)
            return
          }
          if (data.type === Ctor.ErrorTypes.NETWORK_ERROR) {
            const terminal =
              (data.details != null && terminalNetworkDetails.has(data.details)) ||
              networkRetries >= MAX_NETWORK_RETRIES
            if (terminal) {
              instance.destroy()
              hls = null
              onError?.(data.details ?? data.type ?? 'hls_network_fatal')
              return
            }
            networkRetries++
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
        instance.attachMedia(el)
      })
    }

    return () => {
      disposed = true
      // Remove the native error listener BEFORE clearing src + load(), so the
      // teardown's own emptied/error churn can't fire a spurious onError.
      if (onNativeError) {
        el.removeEventListener('error', onNativeError)
        onNativeError = null
      }
      if (hls) {
        if (HlsCtor && onHlsError) hls.off(HlsCtor.Events.ERROR, onHlsError)
        hls.destroy()
        hls = null
      }
      // Native path teardown: clear the URL and force the element to
      // release its loaded resource. Mirrors whep.ts's srcObject = null.
      el.removeAttribute('src')
      el.load()
    }
  })
</script>

<!-- controls default OFF (see Props): the timeline screen supplies its own
     chrome. object-fit: contain — recordings are true aspect ratio; the fill
     rule is the live anamorphic-substream invariant only and must not leak. -->
<video bind:this={video} playsinline {controls} {muted} class="hls-video"></video>

<style>
  .hls-video {
    width: 100%;
    height: 100%;
    object-fit: contain;
    background: var(--feed);
  }
</style>
