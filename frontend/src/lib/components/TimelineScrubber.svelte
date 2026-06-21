<script lang="ts">
  import { hourCells, timeToFraction, type TimelineHour } from '$lib/timeline'

  type Props = {
    hours: TimelineHour[]
    windowStart: number
    windowEnd: number
    position: number
    // Playable-domain bounds, used only to dim the out-of-footage track regions.
    // Default to the viewport edges so no band shows when not provided.
    playbackFloor?: number
    liveEdge?: number
    onSeek: (tSec: number) => void
    // Pointer-drag lifecycle hooks. Fired only for pointer drags, NOT for
    // keyboard nudges — the consumer uses them to switch between the cheap
    // preview-scrub layer (during drag) and full-res playback (on settle).
    onScrubStart?: () => void
    onScrubEnd?: () => void
  }

  let {
    hours,
    windowStart,
    windowEnd,
    position,
    playbackFloor = windowStart,
    liveEdge = windowEnd,
    onSeek,
    onScrubStart,
    onScrubEnd
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
  // pass a drag starts / ends. Filmstrip relative-drag bookkeeping is
  // non-reactive: the pointer x and the playhead position captured at drag
  // start, so a pan applies the finger delta to startPosition and the CONTENT
  // moves under a centred playhead.
  let dragging = $state(false)
  let startClientX = 0
  let startPosition = 0

  const cells = $derived(hourCells(hours, windowStart, windowEnd))
  // The playhead follows the consumer-echoed position prop. In the filmstrip it
  // sits ~centre and slides toward an edge only when the viewport pins near live
  // or the oldest footage. No drag override — during a drag the CONTENT moves,
  // not the playhead, so the round-tripped position drives it throughout.
  const playheadFraction = $derived(timeToFraction(position, windowStart, windowEnd))
  // Out-of-footage bands: the recording floor / live edge mapped into the
  // viewport. floorFrac > 0 means the left edge runs before recording started;
  // liveFrac < 1 means the right edge runs past now. Each marks an empty span.
  const floorFrac = $derived(timeToFraction(playbackFloor, windowStart, windowEnd))
  const liveFrac = $derived(timeToFraction(liveEdge, windowStart, windowEnd))

  // Hour-tick model. Every hourStart in the window is a candidate label;
  // we drop labels until the per-label slice exceeds ~52px so HH:00 in
  // JetBrains Mono never collides with its neighbour. The first and last
  // ticks are always kept so the visible window is bracketed even after
  // decimation.
  type Tick = { tSec: number; fraction: number; label: string }
  const hourLabelFmt = new Intl.DateTimeFormat([], {
    hour: '2-digit',
    minute: '2-digit',
    hour12: false
  })
  const allTicks = $derived.by((): Tick[] => {
    if (windowEnd <= windowStart) return []
    const ticks: Tick[] = []
    // First hour boundary at or after windowStart.
    const firstHour = Math.ceil(windowStart / 3600) * 3600
    for (let t = firstHour; t <= windowEnd; t += 3600) {
      ticks.push({
        tSec: t,
        fraction: timeToFraction(t, windowStart, windowEnd),
        label: hourLabelFmt.format(new Date(t * 1000))
      })
    }
    return ticks
  })
  const ticks = $derived.by((): Tick[] => {
    if (allTicks.length === 0 || trackWidth === 0) return allTicks
    const minPx = 52
    const maxLabels = Math.max(1, Math.floor(trackWidth / minPx))
    if (allTicks.length <= maxLabels) return allTicks
    const step = Math.ceil(allTicks.length / maxLabels)
    const out: Tick[] = []
    for (let i = 0; i < allTicks.length; i += step) out.push(allTicks[i]!)
    // Always keep the right edge so the window's end hour stays labelled.
    const last = allTicks[allTicks.length - 1]!
    if (out[out.length - 1]!.tSec !== last.tSec) out.push(last)
    return out
  })

  // Pointer-Events filmstrip drag. setPointerCapture means we keep receiving
  // pointermove even after the finger/cursor leaves the track bounds;
  // touch-action: none on the track (CSS) cancels the page's horizontal-pan
  // gesture without resorting to non-passive listeners. The drag is a RELATIVE
  // pan from the press point — a tap does not seek (no jump); only movement
  // pans the content under the centred playhead.
  function onPointerDown(e: PointerEvent) {
    if (!trackEl) return
    dragging = true
    startClientX = e.clientX
    startPosition = position
    trackEl.setPointerCapture(e.pointerId)
    onScrubStart?.()
  }
  function onPointerMove(e: PointerEvent) {
    if (!dragging || !trackEl) return
    const w = trackWidth || trackEl.clientWidth
    if (w <= 0) return
    const span = windowEnd - windowStart
    // Drag right -> content moves right -> earlier time -> position decreases.
    // (If device-test shows the direction inverted, flip this one sign.)
    const target = startPosition - ((e.clientX - startClientX) / w) * span
    onSeek(Math.round(target))
  }
  function onPointerUp(e: PointerEvent) {
    if (!dragging) return
    dragging = false
    trackEl?.releasePointerCapture(e.pointerId)
    onScrubEnd?.()
  }

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
        class:has-events={cell.events > 0}
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
    <span class="playhead-handle"></span>
  </span>
</div>

<style>
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
  /* events>0: a very faint top stripe in --accent. Keep this minimal —
     real review markers land in a later phase. */
  .cell.has-events::after {
    content: '';
    position: absolute;
    top: 0;
    left: 0;
    right: 0;
    height: 2px;
    background: var(--accent);
    opacity: 0.55;
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
    font-family: 'JetBrains Mono', ui-monospace, monospace;
    font-variant-numeric: tabular-nums;
    font-size: 11px;
    color: var(--text-3);
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
  .playhead-handle {
    position: absolute;
    top: -4px;
    left: -5px;
    width: 12px;
    height: 12px;
    border-radius: 999px;
    background: var(--accent);
    box-shadow: var(--shadow);
  }
  @media (prefers-reduced-motion: reduce) {
    .playhead {
      transition: none;
    }
  }
</style>
