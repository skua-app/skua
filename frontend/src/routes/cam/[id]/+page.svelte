<script lang="ts">
  import { page } from '$app/state'
  import { onMount, untrack } from 'svelte'
  import { goto } from '$app/navigation'
  import { camerasStore } from '$lib/stores/cameras.svelte'
  import { prefsStore } from '$lib/stores/prefs.svelte'
  import { startWhep } from '$lib/streams/whep'
  import type { WhepHandle } from '$lib/streams/whep'
  import StreamError from '$lib/components/StreamError.svelte'
  import MobileFocus from '$lib/screens/MobileFocus.svelte'
  import DesktopFocus from '$lib/screens/DesktopFocus.svelte'
  import { ui } from '$lib/i18n/strings'
  import { lifecycle } from '$lib/lifecycle.svelte'

  const camId = $derived(page.params.id)
  const camera = $derived(camerasStore.cameras.find((c) => c.id === camId) ?? null)

  // Cache-buster for the poster <img>. The SW StaleWhileRevalidate rule
  // (Part A) strips the query when keying its cache, so each visit returns
  // the last cached tile immediately and revalidates to current. Resetting
  // on camId change refreshes the poster when navigating between cameras.
  let posterTick = $state(Date.now())
  $effect(() => {
    void camId
    posterTick = Date.now()
  })

  // Freeze-frame shown while the user has paused the live view. Captured
  // off the <video> element at pause time and cleared on resume / reconnect
  // / camera switch so a stale frame never lingers.
  let pausedFrameUrl = $state<string | null>(null)
  function captureFrame(): string | null {
    if (!videoEl) return null
    const w = videoEl.videoWidth
    const h = videoEl.videoHeight
    if (!w || !h) return null
    try {
      const canvas = document.createElement('canvas')
      canvas.width = w
      canvas.height = h
      const ctx = canvas.getContext('2d')
      if (!ctx) return null
      ctx.drawImage(videoEl, 0, 0, w, h)
      return canvas.toDataURL('image/jpeg', 0.9)
    } catch {
      // CORS-tainted or otherwise un-encodable: fall through to poster.
      return null
    }
  }

  let videoEl = $state<HTMLVideoElement | null>(null)
  let streamState = $state<WhepHandle['state']>('connecting')
  let latencyMs = $state<number | null>(null)
  let bitrateKbps = $state<number | null>(null)
  let videoCodec = $state<string | null>(null)
  let audioCodec = $state<string | null>(null)
  let resolution = $state<string | null>(null)
  let fps = $state<number | null>(null)
  let errorReason = $state<string | null>(null)
  let showTelemetry = $state(false)
  let isPaused = $state(false)
  let isFullscreen = $state(false)
  let isMuted = $state(prefsStore.mutedByDefault)
  let streamQuality = $state<'main' | 'sub'>('main')
  // A camera only has an LQ option when Frigate exposes a Sub stream for it.
  const subAvailable = $derived(!!camera?.streams.sub)
  // Effective quality clamps the persisted (global) pref to what the current
  // camera can serve: a Sub-less camera always resolves to 'main', so a
  // persisted 'sub' pref never drives a WHEP connect that the BFF would 400.
  const effectiveQuality = $derived<'main' | 'sub'>(subAvailable ? streamQuality : 'main')
  let streamHasAudio = $state(false)
  // Drives the snapshot-poster crossfade and spinner stage text. Flips true on
  // the native `playing` event (first frame painted) and back to false on
  // `emptied` (srcObject cleared in whep cleanup, i.e. disconnect / reconnect).
  let videoPlaying = $state(false)
  // Reactive so the connect $effect waits for the first prefs snapshot. Without
  // this gate, mobile (where IndexedDB-backed prefs load slowly) would issue
  // the first WHEP request against the literal default streamQuality='main'
  // before prefs-sync overwrote it — leaving LQ users on HQ until they
  // toggled the segmented manually.
  let prefsSynced = $state(false)
  let abortController: AbortController | null = null

  let width = $state(typeof window !== 'undefined' ? window.innerWidth : 0)
  const isDesktop = $derived(width >= 900)

  async function connect(cam: NonNullable<typeof camera>) {
    if (!videoEl) return
    errorReason = null
    streamState = 'connecting'
    isPaused = false
    videoPlaying = false
    // Drop any freeze-frame from a previous pause so reconnect / retry /
    // quality toggle / camera switch / foreground re-arm never re-shows
    // a stale frame.
    pausedFrameUrl = null

    const controller = new AbortController()
    abortController = controller

    try {
      await startWhep({
        camId: cam.id,
        videoEl,
        quality: effectiveQuality,
        signal: controller.signal,
        getMuted: () => isMuted,
        onStateChange(s, reason) {
          streamState = s
          if (s === 'failed') {
            errorReason = reason ?? 'unknown'
          }
        },
        onStats(stats) {
          latencyMs = stats.latencyMs
          bitrateKbps = stats.bitrateKbps
          videoCodec = stats.videoCodec
          audioCodec = stats.audioCodec
          resolution = stats.resolution
          fps = stats.fps
        },
        onAudioDetected(has) {
          streamHasAudio = has
        }
      })
    } catch {
      if (!controller.signal.aborted) {
        streamState = 'failed'
        errorReason = 'network'
      }
    }
  }

  function disconnect() {
    if (abortController) {
      abortController.abort()
      abortController = null
    }
    videoPlaying = false
  }

  function retry() {
    const cam = untrack(() => camerasStore.cameras.find((c) => c.id === camId) ?? null)
    if (!cam) return
    disconnect()
    connect(cam)
  }

  function toggleQuality() {
    // Defense in depth: the LQ segment is also disabled in the UI when the
    // camera has no Sub stream. Never persist or connect to 'sub' here.
    if (!subAvailable) return
    const next = streamQuality === 'main' ? 'sub' : 'main'
    streamQuality = next
    prefsStore.setStreamQuality(next)
    streamHasAudio = false
    const cam = untrack(() => camerasStore.cameras.find((c) => c.id === camId) ?? null)
    if (!cam) return
    disconnect()
    connect(cam)
  }

  function togglePause() {
    if (isPaused) {
      pausedFrameUrl = null
      const cam = untrack(() => camerasStore.cameras.find((c) => c.id === camId) ?? null)
      if (cam) connect(cam)
    } else {
      // Capture before disconnect — once the peer connection tears down the
      // <video> element fires `emptied` and videoWidth drops to 0.
      pausedFrameUrl = captureFrame()
      disconnect()
      isPaused = true
      streamState = 'closed'
    }
  }

  function toggleMute() {
    isMuted = !isMuted
    if (videoEl) videoEl.muted = isMuted
    prefsStore.setMutedByDefault(isMuted)
  }

  function toggleFullscreen() {
    if (!videoEl) return
    if (document.fullscreenElement) {
      document.exitFullscreen().catch(() => {})
    } else if ('webkitEnterFullscreen' in videoEl) {
      ;(videoEl as HTMLVideoElement & { webkitEnterFullscreen(): void }).webkitEnterFullscreen()
    } else {
      videoEl.requestFullscreen().catch(() => {})
    }
  }

  // PiP control. Feature-detected against both the W3C and the WebKit
  // presentation-mode APIs so desktop Chrome/Edge, iPadOS Safari, and macOS
  // Safari all work. iPhone reports webkitSupportsPresentationMode('picture-
  // in-picture') === true but cannot PiP a live MediaStream — webkitSet-
  // PresentationMode no-ops, leaving a dead button — so iPhone is excluded
  // from the WebKit path. iPad UAs report as Macintosh, not iPhone, so they
  // keep PiP support.
  type WebkitVideo = HTMLVideoElement & {
    webkitSupportsPresentationMode?: (mode: string) => boolean
    webkitSetPresentationMode?: (mode: string) => void
    webkitPresentationMode?: string
  }
  let pipSupported = $state(false)
  let pipActive = $state(false)

  function togglePip() {
    if (!videoEl) return
    const wk = videoEl as WebkitVideo
    try {
      if (document.pictureInPictureElement) {
        document.exitPictureInPicture().catch(() => {})
      } else if (typeof videoEl.requestPictureInPicture === 'function') {
        videoEl.requestPictureInPicture().catch(() => {})
      } else if (typeof wk.webkitSetPresentationMode === 'function') {
        const next =
          wk.webkitPresentationMode === 'picture-in-picture' ? 'inline' : 'picture-in-picture'
        wk.webkitSetPresentationMode(next)
      }
    } catch {
      // swallow: PiP rejection (user gesture, embedded blocked) is benign.
    }
  }

  $effect(() => {
    const el = videoEl
    if (!el) return
    const wk = el as WebkitVideo
    const isIPhone = typeof navigator !== 'undefined' && /iPhone/.test(navigator.userAgent)
    // Recent iPhone Safari exposes both APIs but still can't PiP a live
    // MediaStream, so gate the whole expression on !isIPhone — not just the
    // WebKit clause. iPad UA reports Macintosh and keeps PiP support.
    pipSupported =
      !isIPhone &&
      ((document.pictureInPictureEnabled === true &&
        typeof el.requestPictureInPicture === 'function') ||
        (typeof wk.webkitSupportsPresentationMode === 'function' &&
          wk.webkitSupportsPresentationMode('picture-in-picture') === true))

    const onEnter = () => {
      pipActive = true
    }
    const onLeave = () => {
      pipActive = false
    }
    const onWkChange = () => {
      pipActive = wk.webkitPresentationMode === 'picture-in-picture'
    }
    el.addEventListener('enterpictureinpicture', onEnter)
    el.addEventListener('leavepictureinpicture', onLeave)
    el.addEventListener('webkitpresentationmodechanged', onWkChange)
    return () => {
      el.removeEventListener('enterpictureinpicture', onEnter)
      el.removeEventListener('leavepictureinpicture', onLeave)
      el.removeEventListener('webkitpresentationmodechanged', onWkChange)
    }
  })

  // "Cameras" is a deterministic navigation to the grid, not a history pop.
  // window.history.length is unreliable in a restored standalone PWA session
  // (it can be > 1 with no in-app grid entry behind it — e.g. the iOS PWA
  // resumes the last-visited /cam/<id> directly), so history.back() would be
  // a silent no-op.
  function handleBack() {
    goto('/')
  }

  function downloadSnapshot() {
    if (!videoEl) return
    const canvas = document.createElement('canvas')
    canvas.width = videoEl.videoWidth || 1280
    canvas.height = videoEl.videoHeight || 720
    const ctx = canvas.getContext('2d')
    if (!ctx) return
    ctx.drawImage(videoEl, 0, 0, canvas.width, canvas.height)
    canvas.toBlob((blob) => {
      if (!blob) return
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = `${camId}-${Date.now()}.jpg`
      a.click()
      URL.revokeObjectURL(url)
    }, 'image/jpeg')
  }

  $effect(() => {
    // Untrack boundary: read every dep the effect should re-run on above
    // untrack(); put the side-effect and fast-changing reads inside.
    // prefsSynced gates the first connect so streamQuality/isMuted reflect
    // the persisted pref instead of the literal defaults.
    const id = camId
    const synced = prefsSynced
    if (!videoEl || !id || !synced) return

    const cam = untrack(() => camerasStore.cameras.find((c) => c.id === id) ?? null)
    if (!cam) return

    untrack(() => connect(cam))
    return () => {
      disconnect()
    }
  })

  const audioAvailable = $derived(streamHasAudio)

  $effect(() => {
    if (!audioAvailable && !isMuted) {
      isMuted = true
    }
  })

  $effect(() => {
    const el = videoEl
    if (!el) return
    const onPlaying = () => {
      videoPlaying = true
    }
    const onEmptied = () => {
      videoPlaying = false
    }
    el.addEventListener('playing', onPlaying)
    el.addEventListener('emptied', onEmptied)
    return () => {
      el.removeEventListener('playing', onPlaying)
      el.removeEventListener('emptied', onEmptied)
    }
  })

  $effect(() => {
    if (prefsStore.loaded && !prefsSynced) {
      prefsSynced = true
      isMuted = prefsStore.mutedByDefault
      streamQuality = prefsStore.streamQuality
      showTelemetry = prefsStore.showTelemetry
    }
  })

  // Tear the PC down before the iOS PWA snapshot freezes the JS context;
  // re-arm on resume. Without this, the resumed page reads a stale
  // PeerConnection and the video element never repaints (black-screen
  // failure mode). isPaused is preserved so a user-initiated pause stays
  // paused across background/foreground.
  onMount(() => {
    let wasPausedByBackground = false

    const offBg = lifecycle.onBackground(() => {
      if (streamState === 'connecting' || streamState === 'connected') {
        wasPausedByBackground = true
        disconnect()
        streamState = 'closed'
      }
    })

    const offFg = lifecycle.onForeground(() => {
      if (!wasPausedByBackground) return
      wasPausedByBackground = false
      if (isPaused) return
      const cam = camerasStore.cameras.find((c) => c.id === camId) ?? null
      if (cam) connect(cam)
    })

    // Track fullscreen state so the livebar button can swap between the
    // outward (enter) and inward (exit) diagonal-arrow glyphs. Covers the
    // standard `fullscreenchange` event plus the WebKit prefixed event for
    // iOS Safari's native video fullscreen path.
    const onFsChange = () => {
      isFullscreen = !!(
        document.fullscreenElement ??
        (document as Document & { webkitFullscreenElement?: Element }).webkitFullscreenElement
      )
    }
    document.addEventListener('fullscreenchange', onFsChange)
    document.addEventListener('webkitfullscreenchange', onFsChange)

    return () => {
      offBg()
      offFg()
      document.removeEventListener('fullscreenchange', onFsChange)
      document.removeEventListener('webkitfullscreenchange', onFsChange)
    }
  })
</script>

<svelte:window bind:innerWidth={width} />

{#snippet videoSnippet()}
  <img
    src="/api/cameras/{camId}/tile.jpg?t={posterTick}"
    alt=""
    loading="eager"
    class="poster"
    class:faded={videoPlaying}
  />

  <video
    bind:this={videoEl}
    playsinline
    muted
    autoplay
    class="video-el"
    class:hidden={streamState === 'failed'}
  ></video>

  {#if isPaused && pausedFrameUrl}
    <img src={pausedFrameUrl} alt="" class="freeze" />
  {/if}

  {#if !videoPlaying && streamState !== 'failed' && !isPaused}
    <div class="overlay-spinner">
      <div class="spinner-pill">
        <svg class="spinner-svg" viewBox="0 0 24 24" width="24" height="24" aria-hidden="true">
          <circle
            cx="12"
            cy="12"
            r="10"
            fill="none"
            stroke="currentColor"
            stroke-width="2"
            stroke-dasharray="40 60"
            stroke-linecap="round"
          />
        </svg>
        <span class="overlay-stage">
          {streamState === 'connected' ? ui.buffering : ui.connecting}
        </span>
      </div>
    </div>
  {/if}

  {#if streamState === 'failed' && errorReason !== null}
    <div class="overlay-error-dim"></div>
    <div class="overlay-error">
      <StreamError reason={errorReason} onRetry={retry} />
    </div>
  {/if}
{/snippet}

{#if isDesktop}
  <DesktopFocus
    {camera}
    {effectiveQuality}
    {subAvailable}
    {audioAvailable}
    {isMuted}
    {isPaused}
    {showTelemetry}
    showTimestamp={prefsStore.showTimestamp}
    isLive={streamState === 'connected'}
    {latencyMs}
    {bitrateKbps}
    {videoCodec}
    {audioCodec}
    {resolution}
    {fps}
    {togglePause}
    {toggleMute}
    {toggleQuality}
    {toggleFullscreen}
    {isFullscreen}
    {togglePip}
    {pipSupported}
    {pipActive}
    {downloadSnapshot}
    onShowTelemetry={() => {
      showTelemetry = !showTelemetry
      prefsStore.setShowTelemetry(showTelemetry)
    }}
    onBack={handleBack}
    {videoSnippet}
  />
{:else}
  <MobileFocus
    {camera}
    {effectiveQuality}
    {subAvailable}
    {audioAvailable}
    {isMuted}
    {isPaused}
    {showTelemetry}
    showTimestamp={prefsStore.showTimestamp}
    {latencyMs}
    {bitrateKbps}
    {videoCodec}
    {audioCodec}
    {resolution}
    {fps}
    {togglePause}
    {toggleMute}
    {toggleQuality}
    {toggleFullscreen}
    {isFullscreen}
    {togglePip}
    {pipSupported}
    {pipActive}
    {downloadSnapshot}
    onShowTelemetry={() => {
      showTelemetry = !showTelemetry
      prefsStore.setShowTelemetry(showTelemetry)
    }}
    onBack={handleBack}
    {videoSnippet}
  />
{/if}

<style>
  .poster {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    object-fit: fill;
    opacity: 1;
    transition: opacity 250ms ease;
    pointer-events: none;
  }
  .poster.faded {
    opacity: 0;
  }
  /* Freeze-frame layer: stacked above the poster and the (now emptied)
     <video>, so the user keeps seeing the exact frame at the moment of
     pause until they resume. */
  .freeze {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    object-fit: fill;
    pointer-events: none;
    z-index: 1;
  }
  .video-el {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    object-fit: fill;
    background: transparent;
  }
  .video-el.hidden {
    display: none;
  }
  .overlay-spinner {
    position: absolute;
    inset: 0;
    display: flex;
    align-items: center;
    justify-content: center;
    pointer-events: none;
  }
  .spinner-pill {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 10px;
    padding: 14px 20px;
    border-radius: 12px;
    background: rgba(0, 0, 0, 0.5);
    backdrop-filter: blur(10px);
    -webkit-backdrop-filter: blur(10px);
    color: rgba(255, 255, 255, 0.92);
  }
  .spinner-svg {
    animation: spinner-rotate 1s linear infinite;
  }
  .overlay-stage {
    font-size: 14px;
  }
  @keyframes spinner-rotate {
    to {
      transform: rotate(360deg);
    }
  }
  .overlay-error-dim {
    position: absolute;
    inset: 0;
    background: rgba(0, 0, 0, 0.55);
    pointer-events: none;
  }
  .overlay-error {
    position: absolute;
    inset: 0;
  }
</style>
