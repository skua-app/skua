<script lang="ts">
  import { hourCells, timeToFraction, type TimelineHour } from '$lib/timeline'
  import type { ReviewSegment, AudioMarker } from '$lib/api'

  type Props = {
    hours: TimelineHour[]
    windowStart: number
    windowEnd: number
    position: number
    // Playable-domain bounds, used only to dim the out-of-footage track regions.
    // Default to the viewport edges so no band shows when not provided.
    playbackFloor?: number
    liveEdge?: number
    // Review activity segments (grouped alert / detection) rendered as a thin
    // lane along the top of the bar. An active segment (end null) draws out to
    // the live edge. Default empty so the lane is simply absent when omitted.
    reviews?: ReviewSegment[]
    // Discrete audio-detection markers rendered as a thin lane directly under
    // the review lane. An active marker (end null) draws out to the live edge.
    // Default empty so the lane is absent when there is no audio activity.
    audioEvents?: AudioMarker[]
    onSeek: (tSec: number) => void
    // Pointer-drag lifecycle hooks. Fired only for pointer drags, NOT for
    // keyboard nudges — the consumer uses them to switch between the cheap
    // preview-scrub layer (during drag) and full-res playback (on settle).
    onScrubStart?: () => void
    onScrubEnd?: () => void
    // Zoom request: an ABSOLUTE desired viewport span in seconds (pinch
    // distance ratio or wheel step). The consumer owns viewSpan and clamps it;
    // the scrubber stays stateless about the span (reads windowEnd-windowStart
    // as the current span).
    onZoom?: (targetSpan: number) => void
  }

  let {
    hours,
    windowStart,
    windowEnd,
    position,
    playbackFloor = windowStart,
    liveEdge = windowEnd,
    reviews = [],
    audioEvents = [],
    onSeek,
    onScrubStart,
    onScrubEnd,
    onZoom
  }: Props = $props()

  // Width in CSS pixels, kept reactive via a ResizeObserver — same pattern
  // Segmented uses. Drives the label-decimation logic so narrow viewports
  // skip every other (or every Nth) hour tick instead of overlapping.
  let trackEl = $state<HTMLDivElement | null>(null)
  let trackWidth = $state(0)

  $effect(() => {
    const el = trackEl
    if (!el) return
    const ro = new ResizeObserver((entries) => {
      const w = entries[0]?.contentRect.width
      if (typeof w === 'number') trackWidth = w
    })
    ro.observe(el)
    trackWidth = el.clientWidth
    return () => ro.disconnect()
  })

  // dragging is $state so the playhead transition flips off in the same render
  // pass a drag starts / ends — it is true for the WHOLE gesture, from the
  // first pointer down to the last pointer up, covering both pan and pinch.
  // Filmstrip relative-drag bookkeeping is non-reactive: the pointer x and the
  // playhead position captured at drag start, so a pan applies the finger delta
  // to startPosition and the CONTENT moves under a centred playhead.
  let dragging = $state(false)
  let startClientX = 0
  let startPosition = 0

  // Multi-pointer model. activePointers maps pointerId -> clientX for every
  // pointer currently down on the track, supporting BOTH the 1-finger pan and
  // a 2-finger pinch zoom. pinching is a non-reactive flag (the zoom is
  // delegated to onZoom, nothing in the template branches on it). While
  // pinching, pan updates are suspended; pinchStartDist / pinchStartSpan are
  // the |x0-x1| and the windowEnd-windowStart snapshot at the pinch start, so
  // the requested span scales as pinchStartSpan * pinchStartDist / currentDist.
  const activePointers = new Map<number, number>()
  let pinching = false
  let pinchStartDist = 0
  let pinchStartSpan = 0

  const cells = $derived(hourCells(hours, windowStart, windowEnd))
  // The playhead follows the consumer-echoed position prop. In the filmstrip it
  // sits ~centre and slides toward an edge only when the viewport pins near live
  // or the oldest footage. No drag override — during a drag the CONTENT moves,
  // not the playhead, so the round-tripped position drives it throughout.
  const playheadFraction = $derived(timeToFraction(position, windowStart, windowEnd))
  // Local-time readout for the flag above the playhead — the same HH:MM:SS
  // format the controls bar used to show before the clock moved up here.
  function pad(n: number): string {
    return String(n).padStart(2, '0')
  }
  const clock = $derived.by(() => {
    const d = new Date(position * 1000)
    return `${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
  })
  // Out-of-footage bands: the recording floor / live edge mapped into the
  // viewport. floorFrac > 0 means the left edge runs before recording started;
  // liveFrac < 1 means the right edge runs past now. Each marks an empty span.
  const floorFrac = $derived(timeToFraction(playbackFloor, windowStart, windowEnd))
  const liveFrac = $derived(timeToFraction(liveEdge, windowStart, windowEnd))

  // Review-lane bands: each segment mapped onto the viewport. An active
  // segment (end null) extends to the live edge. timeToFraction clamps to
  // [0,1], so a segment fully outside the viewport collapses to zero width and
  // is dropped (x1 <= x0); left/width are kept in % so the CSS scales with the
  // track. Severity selects the colour (alert vs detection) in the template.
  type ReviewBand = { id: string; severity: ReviewSegment['severity']; x0: number; width: number }
  const reviewBands = $derived.by((): ReviewBand[] => {
    const out: ReviewBand[] = []
    for (const seg of reviews) {
      const x0 = timeToFraction(seg.start, windowStart, windowEnd)
      const x1 = timeToFraction(seg.end ?? liveEdge, windowStart, windowEnd)
      if (x1 <= x0) continue
      out.push({ id: seg.id, severity: seg.severity, x0, width: x1 - x0 })
    }
    return out
  })

  // Audio-lane bands: same viewport mapping as reviewBands, minus severity. An
  // active marker (end null) extends to the live edge; a marker fully outside
  // the viewport collapses to zero width and is dropped (x1 <= x0).
  type AudioBand = { id: string; x0: number; width: number }
  const audioBands = $derived.by((): AudioBand[] => {
    const out: AudioBand[] = []
    for (const mark of audioEvents) {
      const x0 = timeToFraction(mark.start, windowStart, windowEnd)
      const x1 = timeToFraction(mark.end ?? liveEdge, windowStart, windowEnd)
      if (x1 <= x0) continue
      out.push({ id: mark.id, x0, width: x1 - x0 })
    }
    return out
  })

  // Tick model. The label STEP adapts to the zoom span AND the available width
  // so labels stay useful at every level: a ladder of round intervals
  // (1m … 12h), choosing the smallest entry whose count across the viewport
  // (span / step) fits the pixel budget (maxLabels = trackWidth / minPx). Ticks
  // land on ABSOLUTE multiples of that step and are rendered directly — NO
  // index-based decimation. Decimating by array index would shift which
  // absolute minutes survive as the window pans, making the labels jitter
  // between e.g. :41/:43 and :40/:42; absolute marks slide smoothly instead.
  const TICK_STEPS = [60, 120, 300, 600, 900, 1800, 3600, 7200, 10800, 21600, 43200]
  const TICK_MIN_PX = 56
  function chooseTickStep(span: number, width: number): number {
    // Before the ResizeObserver fires width is 0; fall back to ~10 labels' worth
    // of budget so the step is sane rather than dividing by a zero label count.
    const maxLabels = width > 0 ? Math.max(1, Math.floor(width / TICK_MIN_PX)) : 10
    for (const step of TICK_STEPS) {
      if (span / step <= maxLabels) return step
    }
    return TICK_STEPS[TICK_STEPS.length - 1]!
  }
  type Tick = { tSec: number; fraction: number; label: string }
  const hourLabelFmt = new Intl.DateTimeFormat([], {
    hour: '2-digit',
    minute: '2-digit',
    hour12: false
  })
  const ticks = $derived.by((): Tick[] => {
    if (windowEnd <= windowStart) return []
    const out: Tick[] = []
    const step = chooseTickStep(windowEnd - windowStart, trackWidth)
    // First step boundary at or after windowStart.
    const first = Math.ceil(windowStart / step) * step
    for (let t = first; t <= windowEnd; t += step) {
      out.push({
        tSec: t,
        fraction: timeToFraction(t, windowStart, windowEnd),
        label: hourLabelFmt.format(new Date(t * 1000))
      })
    }
    return out
  })

  // Distance between the two active pointers (used by the pinch gesture).
  function pointerDistance(): number {
    const xs = [...activePointers.values()]
    if (xs.length < 2) return 0
    return Math.abs(xs[0]! - xs[1]!)
  }
  // Arm a relative pan from the given clientX: capture the press point and the
  // current playhead so onPointerMove applies the finger delta to startPosition
  // and the CONTENT moves under the centred playhead.
  function armPan(clientX: number) {
    startClientX = clientX
    startPosition = position
  }

  // Pointer-Events filmstrip drag + pinch zoom. setPointerCapture means we keep
  // receiving pointermove even after the finger/cursor leaves the track bounds;
  // touch-action: none on the track (CSS) cancels the page's pan/zoom gestures
  // without resorting to non-passive listeners. A 1-finger drag is a RELATIVE
  // pan from the press point — a tap does not seek (no jump). A 2-finger gesture
  // pinches the viewport span via onZoom while pan is suspended.
  function onPointerDown(e: PointerEvent) {
    if (!trackEl) return
    trackEl.setPointerCapture(e.pointerId)
    activePointers.set(e.pointerId, e.clientX)
    // First pointer of the gesture starts the scrub lifecycle.
    if (activePointers.size === 1) {
      dragging = true
      onScrubStart?.()
    }
    if (activePointers.size === 2) {
      // Second finger down: begin a pinch and snapshot its baseline.
      pinching = true
      pinchStartDist = pointerDistance()
      pinchStartSpan = windowEnd - windowStart
    } else if (activePointers.size === 1 && !pinching) {
      armPan(e.clientX)
    }
  }
  function onPointerMove(e: PointerEvent) {
    if (!trackEl || !activePointers.has(e.pointerId)) return
    activePointers.set(e.pointerId, e.clientX)
    if (pinching && activePointers.size === 2) {
      // Fingers apart -> currentDist up -> smaller span -> zoom in.
      const currentDist = pointerDistance()
      if (currentDist > 0) onZoom?.((pinchStartSpan * pinchStartDist) / currentDist)
      return
    }
    if (!pinching && activePointers.size === 1) {
      const w = trackWidth || trackEl.clientWidth
      if (w <= 0) return
      const span = windowEnd - windowStart
      // Drag right -> content moves right -> earlier time -> position decreases.
      // (If device-test shows the direction inverted, flip this one sign.)
      const target = startPosition - ((e.clientX - startClientX) / w) * span
      onSeek(Math.round(target))
    }
  }
  function onPointerUp(e: PointerEvent) {
    if (!activePointers.has(e.pointerId)) return
    trackEl?.releasePointerCapture(e.pointerId)
    activePointers.delete(e.pointerId)
    if (activePointers.size === 1 && pinching) {
      // Pinch dropped to one finger: end the pinch and RE-ARM the pan from the
      // remaining pointer so a pinch can flow back into a pan without a jump.
      pinching = false
      const [clientX] = activePointers.values()
      armPan(clientX ?? startClientX)
    } else if (activePointers.size === 0) {
      pinching = false
      dragging = false
      onScrubEnd?.()
    }
  }

  // Wheel zoom (desktop). Multiplicative step around the centred playhead:
  // scroll down/away -> larger span -> zoom out. The parent clamps to
  // [MIN_SPAN, MAX_SPAN]. The handler is attached non-passively in an $effect
  // below so preventDefault actually suppresses page scroll.
  function onWheel(e: WheelEvent) {
    if (!onZoom) return
    e.preventDefault()
    const span = windowEnd - windowStart
    onZoom(span * (e.deltaY > 0 ? 1.15 : 1 / 1.15))
  }
  $effect(() => {
    const el = trackEl
    if (!el) return
    el.addEventListener('wheel', onWheel, { passive: false })
    return () => el.removeEventListener('wheel', onWheel)
  })

  // Keyboard nudge: ±60s per arrow press, ±300s with Shift. Stays within
  // the window so the playhead never advertises a seek target outside the
  // VOD range.
  function onKeyDown(e: KeyboardEvent) {
    if (e.key !== 'ArrowLeft' && e.key !== 'ArrowRight') return
    e.preventDefault()
    const step = e.shiftKey ? 300 : 60
    const dir = e.key === 'ArrowLeft' ? -1 : 1
    const next = Math.max(windowStart, Math.min(windowEnd, position + dir * step))
    onSeek(Math.round(next))
  }
</script>

<!-- Non-clipping wrapper: reserves space above the track for the time flag so it
     is NOT cut off by the track's overflow:hidden. The .track stays the sole
     drag surface (all handlers, bind:this, role/aria, touch-action). -->
<div class="scrubber">
  <div
    bind:this={trackEl}
    class="track"
    role="slider"
    tabindex="0"
    aria-orientation="horizontal"
    aria-valuemin={windowStart}
    aria-valuemax={windowEnd}
    aria-valuenow={position}
    onpointerdown={onPointerDown}
    onpointermove={onPointerMove}
    onpointerup={onPointerUp}
    onpointercancel={onPointerUp}
    onkeydown={onKeyDown}
  >
    <div class="cells" aria-hidden="true">
      {#each cells as cell (cell.x0)}
        <span
          class="cell"
          style:left="{cell.x0 * 100}%"
          style:width="{(cell.x1 - cell.x0) * 100}%"
          style:--cell-fraction={cell.fraction}
        ></span>
      {/each}
    </div>

    {#if floorFrac > 0}
      <span class="void" aria-hidden="true" style:left="0" style:width="{floorFrac * 100}%"></span>
    {/if}
    {#if liveFrac < 1}
      <span class="void" aria-hidden="true" style:left="{liveFrac * 100}%" style:right="0"></span>
    {/if}

    <div class="review-lane" aria-hidden="true">
      {#each reviewBands as band (band.id)}
        <span
          class="review {band.severity}"
          style:left="{band.x0 * 100}%"
          style:width="{band.width * 100}%"
        ></span>
      {/each}
    </div>

    {#if audioEvents.length > 0}
      <div class="audio-lane" aria-hidden="true">
        {#each audioBands as band (band.id)}
          <span class="audio" style:left="{band.x0 * 100}%" style:width="{band.width * 100}%"
          ></span>
        {/each}
      </div>
    {/if}

    <div class="ticks" aria-hidden="true">
      {#each ticks as tick (tick.tSec)}
        <span class="tick" style:left="{tick.fraction * 100}%">
          <span class="tick-rule"></span>
          <span class="tick-label">{tick.label}</span>
        </span>
      {/each}
    </div>

    <span
      class="playhead"
      class:dragging
      aria-hidden="true"
      style:transform="translateX({playheadFraction * trackWidth - 1}px)"
    >
      <span class="playhead-line"></span>
    </span>
  </div>

  <!-- Time flag: a sibling of .track inside the non-clipping wrapper, so it is
       not clipped. Aligned to the playhead line with the SAME x expression the
       playhead uses; the inner bubble is centred over that x. The flag and the
       line share the drag-gated transition so they move together. -->
  <span
    class="playhead-flag"
    class:dragging
    aria-hidden="true"
    style:transform="translateX({playheadFraction * trackWidth - 1}px)"
  >
    <span class="flag-bubble">{clock}</span>
  </span>
</div>

<style>
  /* Non-clipping wrapper around the track. padding-top reserves room for the
     time flag, which sits ABOVE the overflow:hidden track. First-pass size —
     tune on device. */
  .scrubber {
    position: relative;
    width: 100%;
    padding-top: 26px;
    overflow: visible;
  }
  .track {
    position: relative;
    width: 100%;
    height: 60px;
    border-radius: var(--r-sm);
    background: var(--feed);
    border: 1px solid var(--border);
    overflow: hidden;
    cursor: pointer;
    /* Pointer-Events drag-scrub: cancel page-level horizontal pan so a
       drag moves the playhead instead of scrolling the route. */
    touch-action: none;
    user-select: none;
  }
  .track:focus-visible {
    outline: 2px solid var(--accent);
    outline-offset: 2px;
  }
  .cells {
    position: absolute;
    inset: 0;
    pointer-events: none;
  }
  /* Hour fill uses --accent-soft modulated by --cell-fraction so denser
     hours read stronger and empty hours fall back to the track. The
     opacity is the only signal — we do NOT imply WHERE within the hour
     the recording sits (the summary is hour-granular). */
  .cell {
    position: absolute;
    top: 0;
    bottom: 0;
    background: var(--accent-soft);
    opacity: calc(var(--cell-fraction) * 0.9);
  }
  /* Review activity lane: a thin strip along the top of the bar carrying the
     grouped alert / detection segments. pointer-events:none so the drag-scrub
     surface underneath is untouched; it sits below the playhead in the DOM so
     the handle still draws on top. */
  .review-lane {
    position: absolute;
    top: 0;
    left: 0;
    right: 0;
    height: 6px;
    pointer-events: none;
  }
  .review {
    position: absolute;
    top: 0;
    height: 100%;
    /* Rounded capsule. min-width 4px so the round caps read; a 1px dark rim
       separates the capsule from the cell fills behind it. */
    min-width: 4px;
    border-radius: 999px;
    box-shadow: 0 0 0 1px rgba(0, 0, 0, 0.35);
  }
  .review.alert {
    background: var(--warn);
  }
  .review.detection {
    background: var(--accent);
    opacity: 0.7;
  }
  /* Audio activity lane: a second thin strip directly under the review lane
     (review-lane is top:0 height:6px), carrying discrete audio-detection
     markers. pointer-events:none so the drag-scrub surface underneath is
     untouched; it sits below the playhead in the DOM so the handle still draws
     on top. */
  .audio-lane {
    position: absolute;
    /* top:10px leaves ~4px between this lane and the review lane (top:0,
       height:6px) so the two no longer touch. First-pass px — tune on device. */
    top: 10px;
    left: 0;
    right: 0;
    height: 5px;
    pointer-events: none;
  }
  .audio {
    position: absolute;
    top: 0;
    height: 100%;
    /* Rounded capsule. min-width 4px so the round caps read; a 1px dark rim
       separates the capsule from the cell fills behind it. */
    min-width: 4px;
    border-radius: 999px;
    box-shadow: 0 0 0 1px rgba(0, 0, 0, 0.35);
    /* Deliberate one-off violet (no design token): keeps the audio lane
       visually distinct from the amber alert and cyan detection review bands,
       which sit just above. */
    background: oklch(0.72 0.14 300);
  }
  /* Out-of-footage band: the span before recording started / beyond now. A
     faint dark wash plus a low-opacity diagonal hatch so the empty edge reads
     as "no footage here", not missing UI. Subtle — must not fight the cells or
     the playhead, and never captures pointer events. */
  .void {
    position: absolute;
    top: 0;
    bottom: 0;
    pointer-events: none;
    background:
      repeating-linear-gradient(
        -45deg,
        rgba(255, 255, 255, 0.04) 0,
        rgba(255, 255, 255, 0.04) 1px,
        rgba(255, 255, 255, 0) 1px,
        rgba(255, 255, 255, 0) 7px
      ),
      rgba(0, 0, 0, 0.28);
  }
  .ticks {
    position: absolute;
    inset: 0;
    pointer-events: none;
  }
  .tick {
    position: absolute;
    top: 0;
    bottom: 0;
    transform: translateX(-0.5px);
  }
  .tick-rule {
    position: absolute;
    top: 0;
    bottom: 18px;
    width: 1px;
    background: var(--border);
  }
  .tick-label {
    position: absolute;
    bottom: 4px;
    left: 4px;
    /* Geist (inherit), not JetBrains Mono: the mono dotted zero reads as an 8
       at this size. Keep tabular-nums so the digit columns stay aligned. */
    font-family: inherit;
    font-variant-numeric: tabular-nums;
    font-size: 12px;
    font-weight: 500;
    color: var(--text-2);
    letter-spacing: 0.2px;
  }
  .playhead {
    position: absolute;
    left: 0;
    top: 0;
    bottom: 0;
    pointer-events: none;
    /* Driven by transform: translateX so the GPU compositor handles
       movement without per-frame layout. left:% + transition: left was
       chasing the round-tripped position prop a frame or two behind the
       finger on iOS (visible ~0.5-1s lag). */
    transition: transform 120ms linear;
    will-change: transform;
  }
  .playhead.dragging {
    /* No easing while the finger drags — the element must land exactly
       under clientX on every pointermove. */
    transition: none;
  }
  .playhead-line {
    position: absolute;
    top: 0;
    bottom: 0;
    width: 2px;
    background: var(--accent);
  }
  /* Time flag — the top cap of the playhead line, sitting in the wrapper's
     reserved padding above the track. Same drag-gated transition as the
     playhead so the two move as one. First-pass sizes — tune on device. */
  .playhead-flag {
    position: absolute;
    top: 0;
    left: 0;
    pointer-events: none;
    transition: transform 120ms linear;
    will-change: transform;
  }
  .playhead-flag.dragging {
    transition: none;
  }
  .flag-bubble {
    position: relative;
    display: inline-block;
    /* Centre the bubble over the playhead-line x. */
    transform: translateX(-50%);
    height: 22px;
    padding: 0 7px;
    border-radius: var(--r-sm);
    background: var(--accent);
    color: var(--on-accent);
    font-family: 'JetBrains Mono', ui-monospace, monospace;
    font-variant-numeric: tabular-nums;
    font-size: 12px;
    font-weight: 600;
    line-height: 22px;
    white-space: nowrap;
  }
  /* Downward pointer merging the bubble into the playhead line below: its tip
     lands at the track top (22px bubble + 4px triangle == 26px padding-top). */
  .flag-bubble::after {
    content: '';
    position: absolute;
    top: 100%;
    left: 50%;
    transform: translateX(-50%);
    border-left: 4px solid transparent;
    border-right: 4px solid transparent;
    border-top: 4px solid var(--accent);
  }
  @media (prefers-reduced-motion: reduce) {
    .playhead,
    .playhead-flag {
      transition: none;
    }
  }
</style>
