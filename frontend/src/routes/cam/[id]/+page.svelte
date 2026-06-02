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

  let videoEl = $state<HTMLVideoElement | null>(null)
  let streamState = $state<WhepHandle['state']>('connecting')
  let latencyMs = $state<number | null>(null)
  let bitrateKbps = $state<number | null>(null)
  let videoCodec = $state<string | null>(null)
  let audioCodec = $state<string | null>(null)
  let resolution = $state<string | null>(null)
  let errorReason = $state<string | null>(null)
  let showTelemetry = $state(false)
  let isPaused = $state(false)
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
      const cam = untrack(() => camerasStore.cameras.find((c) => c.id === camId) ?? null)
      if (cam) connect(cam)
    } else {
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

  // Prefer history.back() so SvelteKit restores the grid's scroll position;
  // fall back to /goto when the user opened /cam/<id> directly (no history
  // entry to pop).
  function handleBack() {
    if (typeof window !== 'undefined' && window.history.length > 1) {
      window.history.back()
    } else {
      goto('/')
    }
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

    return () => {
      offBg()
      offFg()
    }
  })
</script>

<svelte:window bind:innerWidth={width} />

{#snippet videoSnippet()}
  <img
    src="/api/cameras/{camId}/snapshot.jpg"
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
    {latencyMs}
    {bitrateKbps}
    {videoCodec}
    {audioCodec}
    {resolution}
    {togglePause}
    {toggleMute}
    {toggleQuality}
    {toggleFullscreen}
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
    {togglePause}
    {toggleMute}
    {toggleQuality}
    {toggleFullscreen}
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
