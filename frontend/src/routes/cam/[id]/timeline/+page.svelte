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
  import { timelineMasterURL, timelinePreviewURL, fetchPreviewBounds } from '$lib/api'
  import { clockHourBucket } from '$lib/timeline'
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

  // Low-res preview layer. Frigate's preview.mp4 only returns the first
  // clock-hour file for a multi-hour request, so the scrubber loads ONE
  // hourly bucket at a time (the bucket containing the playhead) and swaps
  // files when the finger crosses into another hour — stretching one hour
  // over the 3h window was the scrub-drift bug. We only ever set the
  // element's currentTime during scrubbing — never let it run. But a
  // <video> on iOS/mobile won't decode or paint a frame from a bare
  // currentTime assignment until it has been played at least once (preload
  // is not honoured for data on mobile), which read as a black scrub. So we
  // prime it with one muted play()/pause() cycle; previewPrimed gates that to
  // once per loaded file.
  let previewEl = $state<HTMLVideoElement | null>(null)
  let previewDuration = $state(0)
  let previewPrimed = $state(false)
  // Coalesces rapid drag seeks to one currentTime assignment per frame.
  let previewSeekRaf: number | null = null
  // The clock-hour bucket currently loaded, and its real covered wall-clock
  // span (from fetchPreviewBounds) — null when bounds are unavailable, in
  // which case seekPreview falls back to the full clock hour. boundsCache
  // keyed by bucket.start avoids refetching on re-entry to an hour.
  let bucket = $state<{ start: number; end: number } | null>(null)
  let bucketBounds = $state<{ first: number; last: number } | null>(null)
  const boundsCache = new Map<number, { first: number; last: number }>()
  const previewSrc = $derived(bucket ? timelinePreviewURL(camId, bucket.start, bucket.end) : '')

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
      // New bucket swaps previewSrc, so the new file must be re-primed.
      previewPrimed = false
      showFullRes = false
      pendingPlay = false
      chunkEnded = false
      playerError = false
      paused = true
      // Fresh camera: drop the bounds cache and the loaded bucket, then load
      // the bucket containing the oldest window edge.
      boundsCache.clear()
      bucket = null
      bucketBounds = null
      void timelineStore.load(id)
      ensureBucket(windowStart)
    })
  })

  // One-time decoder prime: a muted play()/pause() cycle so subsequent
  // currentTime seeks actually decode and paint. No-op once primed. If
  // autoplay is blocked (iOS Low-Power, no gesture) the play() promise
  // rejects and we leave previewPrimed false — handleScrubStart retries the
  // prime from a real user gesture, which is always allowed.
  function primePreview() {
    const el = previewEl
    if (previewPrimed || !el) return
    el.play()
      .then(() => {
        el.pause()
        previewPrimed = true
        seekPreview()
      })
      .catch(() => {})
  }

  // Ensure the loaded preview bucket is the clock-hour containing t. On an
  // hour crossing, swap the bucket (previewSrc reloads, the loadedmetadata
  // effect re-primes + seeks), reset the per-file prime/duration, seed
  // bucketBounds from the cache if known, and kick a bounds fetch. Cheap
  // when t stays inside the current hour — just a start comparison.
  function ensureBucket(t: number) {
    const b = clockHourBucket(t)
    if (bucket && bucket.start === b.start) return
    bucket = b
    previewDuration = 0
    previewPrimed = false
    bucketBounds = boundsCache.get(b.start) ?? null
    void loadBounds(b)
  }

  // Fetch (or reuse cached) the real covered span for bucket b. Applies the
  // result only if b is still the current bucket — an async result for an
  // hour the finger has already left must not corrupt the live mapping. On
  // failure or an empty bucket, bucketBounds stays null and seekPreview
  // falls back to the full clock hour.
  async function loadBounds(b: { start: number; end: number }) {
    const cached = boundsCache.get(b.start)
    if (cached) {
      if (bucket && bucket.start === b.start) {
        bucketBounds = cached
        seekPreview()
      }
      return
    }
    try {
      const resp = await fetchPreviewBounds(camId, b.start, b.end)
      if (resp.count > 0 && resp.end > resp.start) {
        const bounds = { first: resp.start, last: resp.end }
        boundsCache.set(b.start, bounds)
        if (bucket && bucket.start === b.start) {
          bucketBounds = bounds
          seekPreview()
        }
      }
    } catch {
      // Leave bucketBounds null — the clock-hour fallback covers it.
    }
  }

  // Show the preview frame at the current wall-clock position, mapped onto
  // the loaded bucket. The preview file covers only its clock hour and its
  // own duration is much shorter than 3600s, so the fraction is computed
  // BUCKET-relative: against the real covered span when known, else the full
  // clock hour. Cheap, but a fast drag fires many times per frame, so
  // coalesce to one assignment per animation frame using the latest position.
  function seekPreview() {
    const el = previewEl
    if (!el || previewDuration <= 0 || !bucket) return
    if (previewSeekRaf !== null) return
    previewSeekRaf = requestAnimationFrame(() => {
      previewSeekRaf = null
      const e = previewEl
      const b = bucket
      if (!e || previewDuration <= 0 || !b) return
      const lo = bucketBounds ? bucketBounds.first : b.start
      const hi = bucketBounds ? bucketBounds.last : b.end
      const raw = hi > lo ? (position - lo) / (hi - lo) : 0
      const frac = raw < 0 ? 0 : raw > 1 ? 1 : raw
      e.currentTime = frac * previewDuration
    })
  }

  // Preview metadata: capture duration, then show the resting frame at the
  // current position. Fires again whenever previewSrc swaps (new bucket).
  $effect(() => {
    const el = previewEl
    if (!el) return
    const onMeta = () => {
      previewDuration = Number.isFinite(el.duration) ? el.duration : 0
      // Best-effort prime on load so the resting frame at the current
      // position paints without waiting for a scrub. primePreview seeks
      // once primed.
      primePreview()
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
    // Prime from this user gesture — guarantees decoding on iOS/Low-Power
    // where the on-load prime's autoplay was blocked. No-op once primed.
    primePreview()
    mode = 'scrubbing'
    chunkEnded = false
    const el = videoEl
    if (el && !el.paused) el.pause()
  }

  // Continuous drag value. Preview-only — cheap, no throttle, no full-res.
  // ensureBucket swaps the hourly preview file only on an actual hour
  // crossing; otherwise it is a no-op comparison.
  function handleSeek(t: number) {
    const clamped = t < windowStart ? windowStart : t > windowEnd ? windowEnd : t
    position = clamped
    chunkEnded = false
    ensureBucket(position)
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
