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
  import { timelineMasterURL, fetchPreviewClips, fetchPreviewFrameList } from '$lib/api'
  import type { PreviewClip, PreviewFrame } from '$lib/api'
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

  // Low-res preview layer, driven by Frigate's preview-CLIPS model — the
  // same source its own History timeline scrubs against. We load the clip
  // list for the window, play the clip whose [start,end] contains the
  // playhead, map position within that clip's real span onto its currentTime,
  // and swap files when the finger crosses a clip boundary. We only ever set
  // the element's currentTime during scrubbing — never let it run. But a
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
  // The clip list for the window (sorted by start) and the clip currently
  // loaded into previewEl. previewSrc is the active clip's BFF /preview-clip
  // URL, '' when no clip covers the playhead (blank preview for that span).
  let clips = $state<PreviewClip[]>([])
  let activeClip = $state<PreviewClip | null>(null)
  // Frigate omits the open current hour from the preview-clips list entirely —
  // its mp4 is still being assembled — so the newest available coverage ends at
  // the last clip's end (clips are sorted by start). The live tail is the span
  // after that, up to capture-time now.
  const lastClipEnd = $derived(clips.at(-1)?.end ?? null)
  // Open tail = the playhead sits past the last available clip, in the live,
  // not-yet-clipped span. Detected by POSITION, not by any clip (there is none).
  const isOpenTail = $derived(lastClipEnd != null && position > lastClipEnd)
  // Gate previewSrc to '' on the open tail (no mp4 to scrub there); the webp
  // frame layer drives the picture. Closed hours keep the clip-video src.
  const previewSrc = $derived(activeClip && !isOpenTail ? activeClip.src : '')

  // Open-tail webp frame layer. frames is the tail's frame list (sorted by ts);
  // framesKey keys which tail span's frames are loaded (load once per tail +
  // staleness guard); frameSrc is the current nearest-frame src.
  let frames = $state<PreviewFrame[]>([])
  let framesKey = $state<string | null>(null)
  let frameSrc = $state('')

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
  // The webp frame layer wins over the video preview while scrubbing the open
  // live tail; full-res still wins over both during playback.
  const framesVisible = $derived(isOpenTail && mode === 'scrubbing' && frameSrc !== '')
  const previewVisible = $derived(!fullResVisible && !framesVisible)

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
      // New clip swaps previewSrc, so the new file must be re-primed.
      previewPrimed = false
      showFullRes = false
      pendingPlay = false
      chunkEnded = false
      playerError = false
      paused = true
      // Fresh camera: drop the loaded clips/clip, then load the clip list
      // for the window and pick the clip at the oldest window edge.
      clips = []
      activeClip = null
      // Drop any open-tail frame state captured for the previous camera.
      frames = []
      framesKey = null
      frameSrc = ''
      void timelineStore.load(id)
      void loadClips(id)
    })
  })

  // Load the preview-clips list for the current window and pick the clip at
  // the playhead. Guard against a stale camId — an in-flight list for a
  // camera the user has navigated away from must not overwrite the new one.
  // On error the list stays empty (blank preview; scrubber / clock / full-res
  // still work).
  async function loadClips(id: string) {
    try {
      const list = await fetchPreviewClips(id, windowStart, windowEnd)
      if (camId !== id) return
      clips = [...list].sort((a, b) => a.start - b.start)
    } catch {
      if (camId !== id) return
      clips = []
    }
    pickClip(position)
  }

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

  // Pick the clip whose [start,end] contains t, swapping the loaded file on a
  // clip-boundary crossing. When t falls in a gap between clips, snap to the
  // nearest clip by start so the preview shows the closest available footage
  // rather than going blank mid-list. When no clip covers t at all (empty
  // list), activeClip is null and the preview blanks for that span. A new
  // clip resets the per-file prime/duration — the loadedmetadata effect
  // re-primes + seeks once the new file's metadata loads.
  function pickClip(t: number) {
    let next: PreviewClip | null = null
    if (clips.length > 0) {
      next = clips.find((c) => t >= c.start && t <= c.end) ?? null
      if (!next) {
        // Gap (or before/after the list): nearest clip by start distance.
        next = clips.reduce((best, c) =>
          Math.abs(c.start - t) < Math.abs(best.start - t) ? c : best
        )
      }
    }
    if (next?.src === activeClip?.src) return
    activeClip = next
    previewDuration = 0
    previewPrimed = false
  }

  // Show the preview frame at the current wall-clock position, mapped onto
  // the ACTIVE CLIP's real wall-clock span. clip.start/end are this file's
  // true span, so the mapping is accurate with no bounds endpoint. Cheap, but
  // a fast drag fires many times per frame, so coalesce to one assignment per
  // animation frame using the latest position.
  function seekPreview() {
    const el = previewEl
    if (!el || previewDuration <= 0 || !activeClip) return
    if (previewSeekRaf !== null) return
    previewSeekRaf = requestAnimationFrame(() => {
      previewSeekRaf = null
      const e = previewEl
      const c = activeClip
      if (!e || previewDuration <= 0 || !c) return
      const lo = c.start
      const hi = c.end
      const raw = hi > lo ? (position - lo) / (hi - lo) : 0
      const frac = raw < 0 ? 0 : raw > 1 ? 1 : raw
      e.currentTime = frac * previewDuration
    })
  }

  // Lazily load the open live tail's webp preview frame list, once per tail
  // span. The tail spans [lastClipEnd, windowEnd] — the live, not-yet-clipped
  // footage after the newest preview clip, which has no assembled mp4, so we
  // scrub it frame-by-frame. framesKey is set to String(lastClipEnd)
  // immediately so a fast drag firing this repeatedly does not spawn duplicate
  // fetches (and an error never retry-loops; the key only changes when a new
  // clip closes). On resolve, guard against a stale camId AND a stale tail span
  // (the user may have scrubbed off the tail, or a new clip closed, before the
  // list lands); on error the list stays empty (the layer shows the feed bg).
  async function loadFramesTail() {
    const end = lastClipEnd
    if (end == null) return
    const key = String(end)
    if (framesKey === key) return
    framesKey = key
    const id = camId
    try {
      const list = await fetchPreviewFrameList(id, Math.floor(end), windowEnd)
      if (camId !== id || framesKey !== key) return
      frames = [...list].sort((a, b) => a.ts - b.ts)
      pickFrame(position)
    } catch {
      if (camId !== id || framesKey !== key) return
      frames = []
      frameSrc = ''
    }
  }

  // Show the open tail's preview frame nearest the current wall-clock position.
  // frames are sorted by ts; pick the nearest by absolute ts distance. The
  // <img> swaps instantly — each immutable webp is browser-cached after first
  // fetch. No-op (keeps the current frame) when the list is empty.
  function pickFrame(t: number) {
    let best: PreviewFrame | null = null
    let bestDist = Infinity
    for (const f of frames) {
      const d = Math.abs(f.ts - t)
      if (d < bestDist) {
        best = f
        bestDist = d
      }
    }
    if (best) frameSrc = best.src
  }

  // Preview metadata: capture duration, then show the resting frame at the
  // current position. Fires again whenever previewSrc swaps (new clip).
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
  // pickClip swaps the preview file only on a clip-boundary crossing;
  // otherwise it is a no-op comparison.
  function handleSeek(t: number) {
    const clamped = t < windowStart ? windowStart : t > windowEnd ? windowEnd : t
    position = clamped
    chunkEnded = false
    pickClip(position)
    if (isOpenTail) {
      // Open live tail (past the last clip): the mp4 preview does not exist, so
      // scrub by webp frame. loadFramesTail is lazy-once; pickFrame is cheap and
      // network-free (the browser fetches+caches each nearest webp on demand).
      void loadFramesTail()
      pickFrame(position)
    } else if (mode === 'scrubbing') {
      seekPreview()
    }
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

    <img
      class="layer frames"
      src={frameSrc || undefined}
      alt=""
      style:opacity={framesVisible ? 1 : 0}
    />

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
  /* Open-hour webp frame layer. Same fit/background as .preview; the <img>
     must not capture pointer events so the scrubber/controls are unaffected. */
  .frames {
    object-fit: contain;
    background: var(--feed);
    display: block;
    pointer-events: none;
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
