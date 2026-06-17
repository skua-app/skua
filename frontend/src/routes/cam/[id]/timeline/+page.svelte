<!-- Phase 2b-iv-b-1: two-layer recording-timeline screen. Scrubbing rides a
     single low-res preview timelapse (instant seeking, no network per move);
     settling loads full-res only as a bounded <=5-min chunk and plays it.
     Playback STOPS at the chunk end with a hint — continuous gapless
     cross-chunk playback is 2b-iv-b-2. The 3h full-res master URL is gone:
     full-res is ever only a chunk. Still URL-reachable only (entry link is 2c). -->
<script lang="ts">
  import { page } from '$app/state'
  import { untrack } from 'svelte'
  import { goto } from '$app/navigation'
  import { camerasStore } from '$lib/stores/cameras.svelte'
  import { timelineStore } from '$lib/stores/timeline.svelte'
  import { timelineMasterURL, timelinePreviewURL } from '$lib/api'
  import { timeToFraction } from '$lib/timeline'
  import HlsVideo from '$lib/components/HlsVideo.svelte'
  import TimelineScrubber from '$lib/components/TimelineScrubber.svelte'
  import Icon from '$lib/components/Icon.svelte'
  import Mono from '$lib/components/Mono.svelte'
  import { ui } from '$lib/i18n/strings'

  // Scrubber window: a fixed 3-hour span per camera. Full-res chunk: 5 min.
  const WINDOW_SECONDS = 3 * 3600
  const CHUNK_SECONDS = 5 * 60
  // After a settle/seek, ignore the full-res timeupdate echo briefly so it
  // doesn't fight the UI right after the assignment.
  const SEEK_SETTLE_MS = 300

  // page.params.id is typed string | undefined; the route only matches when
  // [id] is bound, so the empty fallback never surfaces in practice.
  const camId = $derived(page.params.id ?? '')
  const camera = $derived(camerasStore.cameras.find((c) => c.id === camId) ?? null)

  // Window captured ONCE per camId (in the effect below). start is the oldest
  // edge; end is wall-clock now at capture time.
  let windowStart = $state(0)
  let windowEnd = $state(0)

  // Wall-clock playhead (unix sec). Driven by drag (preview) or full-res
  // timeupdate (playback); consumed by the scrubber + the clock readout.
  let position = $state(0)

  // 'scrubbing' = the low-res preview layer drives the picture; 'playback' =
  // the full-res chunk drives it.
  let mode = $state<'scrubbing' | 'playback'>('scrubbing')

  // Low-res preview layer (the whole window in one short seekable file). We
  // only ever set its currentTime — never play() it.
  let previewEl = $state<HTMLVideoElement | null>(null)
  let previewDuration = $state(0)
  const previewSrc = $derived(
    windowEnd > 0 ? timelinePreviewURL(camId, windowStart, windowEnd) : ''
  )

  // Full-res chunk layer (HlsVideo). chunkSrc is '' until a chunk is loaded,
  // so no full-res network on mount.
  let videoEl = $state<HTMLVideoElement | null>(null)
  let chunkStart = $state<number | null>(null)
  let chunkEnd = $state<number | null>(null)
  let chunkSrc = $state('')
  let paused = $state(true)
  let chunkEnded = $state(false)
  // Holds false until the freshly loaded chunk has painted its first frame —
  // until then the preview frame stays visible as a poster over the 1-3s
  // Frigate chunk generation rather than flashing the empty feed background.
  let showFullRes = $state(false)
  // Set when a chunk (re)load wants to start playing once it can: a brand-new
  // src attaches asynchronously inside HlsVideo, so we defer play() to canplay.
  let pendingPlay = $state(false)

  // HlsVideo error (e.g. Frigate 400 on an empty chunk range).
  let playerError = $state(false)

  let lastSeekAt = 0

  // Layer visibility: the preview holds until full-res has a frame, then the
  // swap happens. In scrubbing mode the preview is always the visible layer.
  const fullResVisible = $derived(mode === 'playback' && showFullRes)
  const previewVisible = $derived(!fullResVisible)

  // On camId change: capture a fresh window, reset to scrubbing at the oldest
  // edge, drop any loaded chunk, and pull the summary for the scrubber cells.
  $effect(() => {
    const id = camId
    if (!id) return
    untrack(() => {
      const end = Math.floor(Date.now() / 1000)
      windowEnd = end
      windowStart = end - WINDOW_SECONDS
      position = windowStart
      mode = 'scrubbing'
      chunkSrc = ''
      chunkStart = null
      chunkEnd = null
      previewDuration = 0
      showFullRes = false
      pendingPlay = false
      chunkEnded = false
      playerError = false
      paused = true
      void timelineStore.load(id)
    })
  })

  // Show the preview frame at the current wall-clock position. The preview's
  // own duration is much shorter than the window, so we map the window
  // fraction onto preview.currentTime. Cheap — no throttle needed.
  function seekPreview() {
    const el = previewEl
    if (!el || previewDuration <= 0) return
    el.currentTime = timeToFraction(position, windowStart, windowEnd) * previewDuration
  }

  // Preview metadata: capture duration, then show the resting frame at the
  // current position. Fires again whenever previewSrc swaps (new window).
  $effect(() => {
    const el = previewEl
    if (!el) return
    const onMeta = () => {
      previewDuration = Number.isFinite(el.duration) ? el.duration : 0
      seekPreview()
    }
    el.addEventListener('loadedmetadata', onMeta)
    return () => el.removeEventListener('loadedmetadata', onMeta)
  })

  // Full-res chunk element listeners. videoEl is stable across chunk reloads
  // (HlsVideo only swaps src), so these attach once.
  $effect(() => {
    const el = videoEl
    if (!el) return
    const onTime = () => {
      if (mode !== 'playback' || chunkStart === null) return
      if (performance.now() - lastSeekAt < SEEK_SETTLE_MS) return
      // Accurate, drift-bounded mapping: a chunk's currentTime 0 == its
      // wall-clock chunkStart, and the chunk spans at most CHUNK_SECONDS.
      position = chunkStart + el.currentTime
    }
    const onPlay = () => {
      paused = false
    }
    const onPause = () => {
      paused = true
    }
    const onReady = () => {
      // First frame of the (re)loaded chunk is available — swap the poster
      // out and honour any deferred play request.
      showFullRes = true
      if (pendingPlay) {
        pendingPlay = false
        void el.play().catch(() => {})
      }
    }
    const onEnded = () => {
      // Chunk boundary. Stop and hint; do NOT auto-advance (that is 2b-iv-b-2).
      paused = true
      chunkEnded = true
    }
    el.addEventListener('timeupdate', onTime)
    el.addEventListener('play', onPlay)
    el.addEventListener('pause', onPause)
    el.addEventListener('canplay', onReady)
    el.addEventListener('playing', onReady)
    el.addEventListener('ended', onEnded)
    return () => {
      el.removeEventListener('timeupdate', onTime)
      el.removeEventListener('play', onPlay)
      el.removeEventListener('pause', onPause)
      el.removeEventListener('canplay', onReady)
      el.removeEventListener('playing', onReady)
      el.removeEventListener('ended', onEnded)
    }
  })

  // Ensure full-res plays at wall-clock T: reuse the loaded chunk when T is
  // inside it (cheap currentTime seek), else load a fresh <=5-min chunk
  // starting at T. Either way switch to playback and start playing.
  function ensurePlaybackAt(t: number) {
    const el = videoEl
    chunkEnded = false
    const within =
      chunkStart !== null &&
      chunkEnd !== null &&
      chunkSrc !== '' &&
      t >= chunkStart &&
      t <= chunkEnd
    if (within && el) {
      lastSeekAt = performance.now()
      el.currentTime = t - chunkStart!
      void el.play().catch(() => {})
    } else {
      const cs = t
      const ce = Math.min(t + CHUNK_SECONDS, windowEnd)
      chunkStart = cs
      chunkEnd = ce
      showFullRes = false // hold the preview frame as a poster until first frame
      playerError = false
      pendingPlay = true // play() deferred to canplay (async HlsVideo attach)
      lastSeekAt = performance.now()
      chunkSrc = timelineMasterURL(camId, cs, ce)
    }
    mode = 'playback'
  }

  // Scrubber drag started: drop to the preview layer and pause full-res.
  function handleScrubStart() {
    mode = 'scrubbing'
    chunkEnded = false
    const el = videoEl
    if (el && !el.paused) el.pause()
  }

  // Continuous drag value. Preview-only — cheap, no throttle, no full-res.
  function handleSeek(t: number) {
    const clamped = t < windowStart ? windowStart : t > windowEnd ? windowEnd : t
    position = clamped
    chunkEnded = false
    if (mode === 'scrubbing') seekPreview()
  }

  // Drag released: settle to full-res at the landed position.
  function handleScrubEnd() {
    ensurePlaybackAt(position)
  }

  // Play button. From scrubbing, or when the playhead is outside the loaded
  // chunk, settle+play; otherwise toggle the loaded chunk.
  function togglePlay() {
    const el = videoEl
    const within =
      chunkStart !== null &&
      chunkEnd !== null &&
      chunkSrc !== '' &&
      position >= chunkStart &&
      position <= chunkEnd
    if (mode === 'scrubbing' || !within) {
      ensurePlaybackAt(position)
      return
    }
    if (!el) return
    if (el.paused) void el.play().catch(() => {})
    else el.pause()
  }

  function pad(n: number): string {
    return String(n).padStart(2, '0')
  }
  // Wall-clock readout of the current position (local time, HH:MM:SS).
  const clock = $derived.by(() => {
    const d = new Date(position * 1000)
    return `${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
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
    <video
      bind:this={previewEl}
      src={previewSrc || undefined}
      muted
      playsinline
      preload="auto"
      class="layer preview"
      style:opacity={previewVisible ? 1 : 0}
    ></video>

    <div class="layer fullres" style:opacity={fullResVisible ? 1 : 0}>
      <HlsVideo bind:video={videoEl} src={chunkSrc} onError={() => (playerError = true)} />
    </div>

    {#if chunkEnded}
      <div class="frame-hint">
        <Mono size={11} color="rgba(255,255,255,0.85)">{ui.timelineChunkEnd}</Mono>
      </div>
    {/if}

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
      onScrubStart={handleScrubStart}
      onScrubEnd={handleScrubEnd}
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
  /* Two stacked players. Opacity swaps between the low-res preview and the
     full-res chunk; the short fade hides the seam without a hard cut. */
  .layer {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    transition: opacity 80ms ease;
  }
  .preview {
    object-fit: contain;
    background: var(--feed);
    display: block;
  }
  @media (prefers-reduced-motion: reduce) {
    .layer {
      transition: none;
    }
  }
  .frame-hint {
    position: absolute;
    left: 50%;
    bottom: 10px;
    transform: translateX(-50%);
    padding: 5px 10px;
    border-radius: 999px;
    background: rgba(0, 0, 0, 0.5);
    backdrop-filter: blur(8px);
    -webkit-backdrop-filter: blur(8px);
    pointer-events: none;
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
