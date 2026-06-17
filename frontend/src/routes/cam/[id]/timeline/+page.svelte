<!-- Phase 2b-iii: the real recording-timeline screen. Wires HlsVideo +
     TimelineScrubber + timelineStore over a single fixed 3-hour window
     (scrubber window == VOD window). No wide-window / zoom / on-demand chunk
     reload yet — that is 2b-iv. Still reachable by URL only; the focus-screen
     entry link lands in 2c. -->
<script lang="ts">
  import { page } from '$app/state'
  import { onDestroy, untrack } from 'svelte'
  import { goto } from '$app/navigation'
  import { camerasStore } from '$lib/stores/cameras.svelte'
  import { timelineStore } from '$lib/stores/timeline.svelte'
  import { timelineMasterURL } from '$lib/api'
  import HlsVideo from '$lib/components/HlsVideo.svelte'
  import TimelineScrubber from '$lib/components/TimelineScrubber.svelte'
  import Icon from '$lib/components/Icon.svelte'
  import Mono from '$lib/components/Mono.svelte'
  import { ui } from '$lib/i18n/strings'

  // Decision 1: a single fixed 3-hour window per camera.
  const WINDOW_SECONDS = 3 * 3600
  // Throttle budget for heavy currentTime seeks during a continuous drag.
  const SEEK_THROTTLE_MS = 200
  // After a user seek, ignore playback's own timeupdate for a brief settle
  // window so it doesn't fight the finger right after release.
  const SEEK_SETTLE_MS = 300

  // page.params.id is typed string | undefined; the route only matches when
  // [id] is bound, so the empty fallback never surfaces in practice.
  const camId = $derived(page.params.id ?? '')
  const camera = $derived(camerasStore.cameras.find((c) => c.id === camId) ?? null)

  // The window is captured ONCE per camId (in the effect below) so the VOD
  // src stays stable and HlsVideo does not re-attach on every render. start
  // is the oldest edge of the window; end is wall-clock now at capture time.
  let windowStart = $state(0)
  let windowEnd = $state(0)

  // Wall-clock playhead in unix seconds. The scrubber consumes this; we echo
  // it back from playback (timeupdate) and from drag (onSeek).
  let position = $state(0)

  // HlsVideo error (e.g. Frigate 400 on an empty window) — shown as a text
  // overlay in the video frame; the summary can still be fine.
  let playerError = $state(false)

  // Bindable <video> from HlsVideo so this screen drives play/pause + seeks.
  let videoEl = $state<HTMLVideoElement | null>(null)
  let paused = $state(true)

  // Seek throttling + settle guard (declared before the handlers that read
  // them so closures capture live getters).
  let lastSeekAt = 0
  let lastThrottleRun = 0
  let pendingSeek: number | null = null
  let throttleTimer: ReturnType<typeof setTimeout> | null = null

  // src is empty until the window is captured, so HlsVideo's attach effect
  // (which returns early on !src) never issues a vod/0/0 fetch on mount.
  const src = $derived(windowEnd > 0 ? timelineMasterURL(camId, windowStart, windowEnd) : '')

  // On camId change: capture a fresh window, reset playhead to the oldest
  // edge (play from there forward), clear any prior player error, and pull
  // the recordings summary for the scrubber cells.
  $effect(() => {
    const id = camId
    if (!id) return
    untrack(() => {
      const end = Math.floor(Date.now() / 1000)
      windowEnd = end
      windowStart = end - WINDOW_SECONDS
      position = windowStart
      playerError = false
      void timelineStore.load(id)
    })
  })

  // Playback ↔ scrubber binding. timeupdate echoes currentTime back into the
  // wall-clock position UNLESS a user seek happened very recently — the
  // scrubber already ignores the prop mid-drag; this guard covers the brief
  // settle window after release. play/pause events keep `paused` in sync.
  $effect(() => {
    const el = videoEl
    if (!el) return
    const onTime = () => {
      if (performance.now() - lastSeekAt < SEEK_SETTLE_MS) return
      // Time mapping (decision 2) is a LINEAR approximation: Frigate's VOD
      // playlist carries no EXT-X-PROGRAM-DATE-TIME and the summary is
      // hour-granular, so master.m3u8 concatenates only existing segments —
      // within a window that contains gaps the playhead drifts from true
      // wall-clock. Gap-aware mapping is a later concern.
      position = windowStart + el.currentTime
    }
    const onPlay = () => {
      paused = false
    }
    const onPause = () => {
      paused = true
    }
    el.addEventListener('timeupdate', onTime)
    el.addEventListener('play', onPlay)
    el.addEventListener('pause', onPause)
    return () => {
      el.removeEventListener('timeupdate', onTime)
      el.removeEventListener('play', onPlay)
      el.removeEventListener('pause', onPause)
    }
  })

  // The heavy currentTime assignment. Clamp into [0, duration] so a seek into
  // a gap lands on the nearest available footage instead of overrunning the
  // concatenated duration. duration is NaN until metadata loads — the ||
  // fallback (window span) covers that.
  function applySeek(tSec: number) {
    const el = videoEl
    if (!el) return
    const dur = el.duration || windowEnd - windowStart
    const target = tSec - windowStart
    el.currentTime = target < 0 ? 0 : target > dur ? dur : target
  }

  // onSeek from the scrubber. Decision 3: at most one heavy seek per
  // ~200ms during a continuous drag, plus a trailing assignment carrying the
  // final value. position is updated immediately (cheap) so the playhead
  // stays continuous at drag release.
  function handleSeek(t: number) {
    const clamped = t < windowStart ? windowStart : t > windowEnd ? windowEnd : t
    position = clamped
    lastSeekAt = performance.now()

    pendingSeek = clamped
    const now = performance.now()
    if (now - lastThrottleRun >= SEEK_THROTTLE_MS) {
      lastThrottleRun = now
      applySeek(clamped)
      pendingSeek = null
    } else if (throttleTimer === null) {
      const wait = SEEK_THROTTLE_MS - (now - lastThrottleRun)
      throttleTimer = setTimeout(() => {
        throttleTimer = null
        lastThrottleRun = performance.now()
        if (pendingSeek !== null) {
          applySeek(pendingSeek)
          pendingSeek = null
        }
      }, wait)
    }
  }

  function togglePlay() {
    const el = videoEl
    if (!el) return
    if (el.paused) {
      void el.play().catch(() => {})
    } else {
      el.pause()
    }
  }

  function pad(n: number): string {
    return String(n).padStart(2, '0')
  }
  // Wall-clock readout of the current position (local time, HH:MM:SS).
  const clock = $derived.by(() => {
    const d = new Date(position * 1000)
    return `${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
  })

  onDestroy(() => {
    if (throttleTimer !== null) {
      clearTimeout(throttleTimer)
      throttleTimer = null
    }
  })
</script>

<div class="page">
  <header class="bar">
    <button
      type="button"
      class="back"
      onclick={() => goto(`/cam/${camId}`)}
      aria-label={ui.backLabel}
    >
      <Icon name="back" size={20} />
      <span>{ui.timelineLiveBack}</span>
    </button>
    <span class="title">{camera?.name ?? camId}</span>
  </header>

  <div class="frame">
    <HlsVideo bind:video={videoEl} {src} onError={() => (playerError = true)} />
    {#if playerError}
      <div class="frame-overlay">
        <Mono size={12} color="rgba(255,255,255,0.82)">{ui.timelineNoRecording}</Mono>
      </div>
    {/if}
  </div>

  <div class="controls">
    <button
      type="button"
      class="livebtn playpause"
      onclick={togglePlay}
      aria-label={paused ? ui.timelinePlay : ui.timelinePause}
    >
      <Icon name={paused ? 'play' : 'pause'} size={20} />
    </button>
    <span class="clock">
      <Mono size={13} weight={500} color="var(--text)" letterSpacing={0.3}>{clock}</Mono>
    </span>
  </div>

  <div class="scrub">
    <TimelineScrubber
      hours={timelineStore.hours}
      {windowStart}
      {windowEnd}
      {position}
      onSeek={handleSeek}
    />

    {#if timelineStore.loading}
      <div class="scrub-note loading">
        <Mono size={11} color="var(--text-3)">{ui.timelineLoading}</Mono>
      </div>
    {:else if timelineStore.error}
      <div class="scrub-note error">
        <Mono size={11} color="var(--warn)">{ui.timelineSummaryError}</Mono>
      </div>
    {:else if timelineStore.hours.length === 0}
      <div class="scrub-note">
        <Mono size={11} color="var(--text-3)">{ui.timelineEmpty}</Mono>
      </div>
    {/if}
  </div>
</div>

<style>
  .page {
    display: flex;
    flex-direction: column;
    gap: 14px;
    padding: calc(env(safe-area-inset-top, 0px) + 12px) 18px
      calc(env(safe-area-inset-bottom, 0px) + 24px);
    background: var(--bg);
    min-height: 100dvh;
    color: var(--text);
  }
  .bar {
    display: flex;
    align-items: center;
    gap: 12px;
  }
  .back {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    appearance: none;
    -webkit-appearance: none;
    background: var(--surface);
    color: var(--text);
    border: 1px solid var(--border);
    border-radius: var(--r-sm);
    height: var(--ctrl-h);
    padding: 0 12px;
    font-family: inherit;
    font-size: 14px;
    cursor: pointer;
  }
  .back:active {
    transform: translateY(1px);
  }
  .title {
    flex: 1 1 auto;
    min-width: 0;
    font-size: 16px;
    font-weight: 600;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .frame {
    position: relative;
    aspect-ratio: 16 / 9;
    width: 100%;
    border-radius: var(--r);
    overflow: hidden;
    background: var(--feed);
  }
  .frame-overlay {
    position: absolute;
    inset: 0;
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 12px;
    text-align: center;
    background: rgba(0, 0, 0, 0.42);
    backdrop-filter: blur(6px);
    -webkit-backdrop-filter: blur(6px);
    pointer-events: none;
  }

  .controls {
    display: flex;
    align-items: center;
    gap: 14px;
  }
  /* Mirrors MobileFocus .livebtn token styling. */
  .livebtn {
    width: 46px;
    height: 46px;
    flex: 0 0 auto;
    display: grid;
    place-items: center;
    border-radius: var(--r);
    border: 1px solid var(--border);
    background: var(--surface);
    color: var(--text);
    cursor: pointer;
    font-family: inherit;
    -webkit-appearance: none;
    appearance: none;
    transition:
      background 0.18s ease,
      color 0.18s ease,
      border-color 0.18s ease;
  }
  .livebtn:active {
    transform: translateY(1px);
  }
  /* Pin fill+stroke to currentColor so the filled-triangle play glyph and
     the two-bar pause glyph both render fully (same fix as MobileFocus). */
  .livebtn.playpause :global(svg) {
    fill: currentColor;
    stroke: currentColor;
  }
  .clock {
    display: inline-flex;
    align-items: center;
  }

  .scrub {
    display: flex;
    flex-direction: column;
    gap: 8px;
  }
  .scrub-note {
    min-height: 16px;
  }
</style>
