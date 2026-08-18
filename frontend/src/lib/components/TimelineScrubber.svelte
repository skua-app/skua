<script lang="ts">
  import { timeToFraction, fractionToTime } from '$lib/timeline'
  import { resolveGesture, exceedsClickSlop, type Gesture } from '$lib/timeline-gestures'
  import { playheadEdge } from '$lib/timeline-viewport'
  import type { ReviewSegment, AudioMarker, CoverageSegment, TimelineMode } from '$lib/api'
  import Icon from '$lib/components/Icon.svelte'
  import { ui } from '$lib/i18n/strings'

  type Props = {
    windowStart: number
    windowEnd: number
    position: number
    // Which interaction model is active. It is not a style flag: it decides
    // what a press on the track MEANS, so the scrubber has to know it rather
    // than be handed one callback per possible meaning. Nothing here ever
    // writes it — it is the user's preference, echoed down. Defaults to the
    // behaviour the track has always had.
    mode?: TimelineMode
    // Playable-domain bounds, used only to dim the out-of-footage track regions.
    // Default to the viewport edges so no band shows when not provided.
    playbackFloor?: number
    liveEdge?: number
    // Recorded-coverage ranges rendered as a full-height neutral base fill. Each
    // range is footage-present; the unfilled track between ranges is a real
    // recording gap. Default empty so the layer is simply absent when omitted.
    coverage?: CoverageSegment[]
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
    //
    // anchorFraction says WHERE the gesture wants to zoom about, as a position
    // across the drawn window (0 = left edge, 1 = right edge), or null for "no
    // anchor expressed". The scrubber only reports where the gesture happened;
    // whether to honour it is the consumer's decision, because the timeline
    // mode lives there and not here.
    //
    // The wheel passes its pointer position. Pinch DELIBERATELY passes null:
    // its natural anchor in fixed mode is the midpoint between the fingers, but
    // no touch handler changes here, so it keeps behaving exactly as it did.
    // The parameter is shaped so pinch can start passing that midpoint later
    // without another signature change.
    onZoom?: (targetSpan: number, anchorFraction: number | null) => void
    // Pan request: how far to move the viewport, in seconds, positive = later.
    // Pointer-only (shift+wheel and shift+drag), and fixed mode only — the mode
    // gates it, not the presence of the prop, so both pan gestures answer to
    // one source of truth.
    onPan?: (deltaSeconds: number) => void
    // Two-finger request, fixed mode only: an absolute desired span PLUS where
    // to put the wall-clock time the gesture was anchored on. anchorTime is
    // captured once, from the midpoint at the moment the second finger landed;
    // midFraction is where that midpoint is NOW, across the drawn window.
    //
    // One callback rather than an onZoom and an onPan, because the two are one
    // gesture and one viewport write: reporting them separately would apply two
    // writes per frame and accumulate per-frame deltas, which a pinch fed
    // hundreds of pointermove events would drift on. The consumer recomputes
    // the whole viewport from these frozen inputs instead.
    onPinch?: (targetSpan: number, anchorTime: number, midFraction: number) => void
    // Live control: when set, a "LIVE" capsule is drawn at the live-edge
    // position on the track and clicking it navigates away (the consumer routes
    // to the camera's live focus view). Omitted → no capsule.
    onGoLive?: () => void
  }

  let {
    windowStart,
    windowEnd,
    position,
    mode = 'follow',
    playbackFloor = windowStart,
    liveEdge = windowEnd,
    coverage = [],
    reviews = [],
    audioEvents = [],
    onSeek,
    onScrubStart,
    onScrubEnd,
    onZoom,
    onPan,
    onPinch,
    onGoLive
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
  // The wall-clock time under the MIDPOINT at the moment the second finger
  // landed. Frozen for the gesture: it is the point the two fingers hold, and
  // everything the gesture draws is recomputed from it rather than accumulated,
  // so a long pinch cannot drift. Fixed mode only.
  let pinchAnchorTime = 0

  // What the CURRENT press means, settled at pointerdown by resolveGesture and
  // then read — never recomputed — for the rest of the gesture. Non-reactive:
  // no template branches on it. Reset to 'inert' when the last pointer lifts.
  let gesture: Gesture = 'inert'
  // Press bookkeeping for the click / drag distinction. pressClientX is where
  // the press landed (the click's target time is read from it, not from the
  // release point — see clickSeek); movedBeyondSlop is LATCHED, so a press that
  // wanders past the slop and comes back is a drag for good; pressIsMouse gates
  // the click, because a touch tap deliberately does not seek.
  let pressClientX = 0
  let movedBeyondSlop = false
  let pressIsMouse = false
  // How far the press landed from the centre of the playhead line, in pixels.
  // The grab zone is wider than the line, so a press can be up to
  // PLAYHEAD_GRAB_PX off it; carrying that offset through the drag keeps the
  // playhead exactly where it was grabbed instead of snapping it under the
  // cursor on the first move.
  let grabOffsetPx = 0
  // Is a scrub lifecycle currently open? Tracked separately from `gesture`,
  // because a gesture can change identity mid-flight — a finger resting on the
  // playhead becomes a two-finger gesture the moment a second finger lands —
  // and onScrubStart / onScrubEnd must still pair exactly. An unpaired start
  // would strand the route on the preview layer.
  let scrubOpen = false
  // Previous pointer x of a shift+drag pan. The pan writer is RELATIVE, so
  // each move contributes only its own step; accumulating from the press point
  // instead would fight the live-edge clamp, which can absorb part of a pan and
  // must not then be re-applied from scratch on the next move.
  let panLastX = 0

  // Coverage bands: each recorded range mapped onto the viewport, same idiom as
  // reviewBands but with NO severity and — crucially — NO min-width clamp, so a
  // recorded range renders at its true width and a real gap between ranges stays
  // visible as empty track. timeToFraction clamps to [0,1], so a range fully
  // outside the viewport collapses to zero width and is dropped (x1 <= x0).
  type CoverageBand = { id: string; x0: number; width: number }
  const coverageBands = $derived.by((): CoverageBand[] => {
    const out: CoverageBand[] = []
    for (const seg of coverage) {
      const x0 = timeToFraction(seg.start, windowStart, windowEnd)
      const x1 = timeToFraction(seg.end, windowStart, windowEnd)
      if (x1 <= x0) continue
      out.push({ id: `${seg.start}-${seg.end}`, x0, width: x1 - x0 })
    }
    return out
  })
  // The playhead follows the consumer-echoed position prop. In the filmstrip it
  // sits ~centre and slides toward an edge only when the viewport pins near live
  // or the oldest footage. No drag override — during a drag the CONTENT moves,
  // not the playhead, so the round-tripped position drives it throughout.
  const playheadFraction = $derived(timeToFraction(position, windowStart, windowEnd))
  // Which edge the playhead left on, or null while it is drawn. Fixed mode
  // only: in follow mode the window is centred on the playhead by construction,
  // so the marker could never appear and asking is pointless. An INDICATOR —
  // it says which way the playhead went and does nothing else.
  const offEdge = $derived(
    mode === 'fixed' ? playheadEdge(position, windowStart, windowEnd - windowStart) : null
  )
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
  // RAW, unclamped live fraction — unlike liveFrac (clamped to [0,1] for the
  // void band) this can fall outside [0,1]. The in-track Live capsule renders
  // only while it is within [0,1]; once the live edge pans off the right
  // (frac > 1) or left (frac < 0) the capsule is simply not drawn, and the
  // track's overflow:hidden clips the partial case near the edge.
  const liveFracRaw = $derived(
    windowEnd > windowStart ? (liveEdge - windowStart) / (windowEnd - windowStart) : 0
  )

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
  // The major (labelled) step is computed ONCE here; both the labelled ticks
  // and the micro-ticks derive from this single value, so the two ladders can
  // never diverge as the window pans (calling chooseTickStep twice could).
  const majorStep = $derived(chooseTickStep(windowEnd - windowStart, trackWidth))
  const ticks = $derived.by((): Tick[] => {
    if (windowEnd <= windowStart) return []
    const out: Tick[] = []
    const step = majorStep
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

  // Micro-ticks: finer unlabelled marks subdividing each labelled interval.
  // MICRO_DIVISIONS=4 quarters each major interval — a first-pass subdivision
  // count, tunable. microStep stays integer for every TICK_STEPS entry, so the
  // "is this also a major?" modulo test below is exact.
  const MICRO_DIVISIONS = 4
  // Hide micro-ticks once their pixel spacing drops below this, so wide zooms
  // stay clean instead of crowding. ~9px is a first pass — tune on device.
  const MIN_MICRO_PX = 9
  const microTicks = $derived.by((): { fraction: number }[] => {
    if (windowEnd <= windowStart) return []
    const span = windowEnd - windowStart
    const microStep = majorStep / MICRO_DIVISIONS
    // Pixel-spacing guard: drop the whole micro layer when it would crowd.
    if ((microStep / span) * trackWidth < MIN_MICRO_PX) return []
    const out: { fraction: number }[] = []
    const first = Math.ceil(windowStart / microStep) * microStep
    for (let t = first; t <= windowEnd; t += microStep) {
      // Skip marks that are also major boundaries — those get the labelled tick.
      if (t % majorStep === 0) continue
      out.push({ fraction: timeToFraction(t, windowStart, windowEnd) })
    }
    return out
  })

  // Approximate half-width of a labelled tick in px. The first/last labels are
  // clamped so their centre stays this far inside the track, keeping them from
  // clipping under .track's overflow:hidden. ~22px is a tune-on-device guess.
  const LABEL_HALF_W = 22
  // Clamp a tick's label centre (in px across the track) to stay fully visible.
  // The tick MARK itself stays at the true fraction; only the label clamps.
  function clampLabelPx(fraction: number): number {
    const x = fraction * trackWidth
    const hi = trackWidth - LABEL_HALF_W
    if (hi < LABEL_HALF_W) return x // track too narrow to clamp meaningfully
    return Math.min(Math.max(x, LABEL_HALF_W), hi)
  }

  // Hour-boundary dividers, computed from TIME on a fixed 3600s grid (NOT from
  // any band geometry, whose straddling-edge x0/x1 are clamped to [0,1] and so
  // are not real boundaries). Walks whole-hour marks across the window like ticks.
  // Keep only STRICTLY interior lines so a boundary landing exactly on a window
  // edge does not draw a spurious divider pinned at 0 or 1.
  const HOUR = 3600
  const hourLines = $derived.by((): number[] => {
    if (windowEnd <= windowStart) return []
    const out: number[] = []
    const first = Math.ceil(windowStart / HOUR) * HOUR
    for (let t = first; t <= windowEnd; t += HOUR) {
      const fraction = timeToFraction(t, windowStart, windowEnd)
      if (fraction > 0 && fraction < 1) out.push(fraction)
    }
    return out
  })

  // Distance between the two active pointers (used by the pinch gesture).
  function pointerDistance(): number {
    const xs = [...activePointers.values()]
    if (xs.length < 2) return 0
    return Math.abs(xs[0]! - xs[1]!)
  }
  // Midpoint between the two active pointers, in client x. The pan half of the
  // two-finger gesture is entirely this value moving.
  function pointerMidX(): number {
    const xs = [...activePointers.values()]
    if (xs.length < 2) return xs[0] ?? 0
    return (xs[0]! + xs[1]!) / 2
  }
  // Arm a relative pan from the given clientX: capture the press point and the
  // current playhead so onPointerMove applies the finger delta to startPosition
  // and the CONTENT moves under the centred playhead.
  function armPan(clientX: number) {
    startClientX = clientX
    startPosition = position
  }

  // Where a client x sits across the track, in CSS pixels from its left edge.
  // Unclamped — a press that begins inside the track can travel outside it
  // under pointer capture, and the gesture arithmetic wants the true offset.
  function trackX(clientX: number): number {
    const el = trackEl
    if (!el) return 0
    return clientX - el.getBoundingClientRect().left
  }

  // Seek to where a click landed — fixed mode only, and only once the press has
  // been confirmed not to be a drag.
  //
  // The target is read from the PRESS point rather than the release point. They
  // are within the slop of each other by definition, but the press point is the
  // one the user aimed at, and using it makes the landed time independent of
  // any sub-threshold drift while the button was held.
  //
  // The whole scrub lifecycle runs here, in order, so the route sees exactly
  // what a drag of zero duration would produce: engage the preview layer, seek,
  // then settle to full-res at the landed time.
  function clickSeek() {
    const f = pointerFraction(pressClientX)
    if (f === null) return
    onScrubStart?.()
    onSeek(Math.round(fractionToTime(f, windowStart, windowEnd)))
    onScrubEnd?.()
  }

  // Pointer-Events filmstrip drag + pinch zoom. setPointerCapture means we keep
  // receiving pointermove even after the finger/cursor leaves the track bounds;
  // touch-action: none on the track (CSS) cancels the page's pan/zoom gestures
  // without resorting to non-passive listeners. In follow mode a 1-finger drag
  // is a RELATIVE pan from the press point — a tap does not seek (no jump). A
  // 2-finger gesture pinches the viewport span via onZoom while pan is
  // suspended.
  //
  // In fixed mode the press is routed by resolveGesture instead, and the scrub
  // lifecycle is fired only for the gestures that actually MOVE THE PLAYHEAD.
  // A pan must not pause full-res and re-settle it, and a press that is still
  // deciding whether it is a click must not either — a click opens and closes
  // its own lifecycle on release.
  function onPointerDown(e: PointerEvent) {
    if (!trackEl) return
    trackEl.setPointerCapture(e.pointerId)
    activePointers.set(e.pointerId, e.clientX)
    // First pointer of the gesture: settle what it means, once.
    if (activePointers.size === 1) {
      dragging = true
      gesture = resolveGesture({
        mode,
        shiftKey: e.shiftKey,
        pressX: trackX(e.clientX),
        playheadX: playheadFraction * trackWidth,
        pointerType: e.pointerType
      })
      pressClientX = e.clientX
      movedBeyondSlop = false
      pressIsMouse = e.pointerType === 'mouse'
      grabOffsetPx = trackX(e.clientX) - playheadFraction * trackWidth
      panLastX = e.clientX
      if (gesture === 'tape' || gesture === 'playhead') {
        scrubOpen = true
        onScrubStart?.()
      }
    }
    if (activePointers.size === 2) {
      // Second finger down: begin a two-finger gesture and snapshot its
      // baseline. Two fingers always mean the two-finger gesture, whatever the
      // first was doing — the same handoff ZoomPane makes, and the only one
      // that lets the track be zoomed while a finger rests on the playhead.
      pinching = true
      pinchStartDist = pointerDistance()
      pinchStartSpan = windowEnd - windowStart
      // The point the fingers are holding, captured once.
      const midF = pointerFraction(pointerMidX())
      pinchAnchorTime = midF === null ? position : fractionToTime(midF, windowStart, windowEnd)
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
      if (currentDist <= 0) return
      const nextSpan = (pinchStartSpan * pinchStartDist) / currentDist
      if (mode === 'fixed') {
        // Fixed mode: the separation zooms and the midpoint pans, both from the
        // same two fingers and both at once. Nothing discriminates between them
        // because there is nothing to discriminate — they are independent
        // readings of one gesture, so fingers that do a bit of each (which they
        // always do) get a bit of each.
        const f = pointerFraction(pointerMidX())
        if (f === null) return
        onPinch?.(nextSpan, pinchAnchorTime, f)
      } else {
        // Follow mode: the playhead is the anchor and the track is not pannable,
        // so the midpoint is discarded and this stays the span-only zoom it has
        // always been. null = no anchor expressed.
        onZoom?.(nextSpan, null)
      }
      return
    }
    if (!pinching && activePointers.size === 1) {
      // Latch the click / drag distinction before anything acts on the move.
      // This is the ONLY thing that separates a click from a track pan, and it
      // is one-way: below the slop nothing has panned yet and a release still
      // seeks; the instant it flips, the click is off the table for good and
      // every later move pans.
      if (!movedBeyondSlop && exceedsClickSlop(pressClientX, e.clientX)) {
        movedBeyondSlop = true
        // Re-baseline the pan at the crossing point so the slop distance is not
        // replayed as a jolt on the first panning frame. The first few pixels of
        // travel are deliberately absorbed — that is what the threshold costs,
        // and it is under the width of the playhead line. Only for 'track':
        // shift+drag pans from its first pixel and must keep doing so.
        if (gesture === 'track') panLastX = e.clientX
      }
      const w = trackWidth || trackEl.clientWidth
      if (w <= 0) return
      const span = windowEnd - windowStart
      if (gesture === 'tape') {
        // Drag right -> content moves right -> earlier time -> position
        // decreases. (If device-test shows the direction inverted, flip this
        // one sign.)
        const target = startPosition - ((e.clientX - startClientX) / w) * span
        onSeek(Math.round(target))
      } else if (gesture === 'playhead') {
        // Fixed mode: the track is stationary, so the PLAYHEAD follows the
        // cursor — the opposite of the tape drag above, where the cursor drags
        // the content past a fixed playhead. Absolute, minus the grab offset,
        // so the line stays exactly where it was picked up.
        //
        // pointerFraction clamps to the drawn window, so dragging off either
        // end parks the playhead at that edge rather than running the window
        // along with it. The track not moving is the whole of fixed mode; the
        // route then bounds the seek by what is actually playable.
        const f = pointerFraction(e.clientX - grabOffsetPx)
        if (f === null) return
        onSeek(Math.round(fractionToTime(f, windowStart, windowEnd)))
      } else if (gesture === 'pan' || (gesture === 'track' && movedBeyondSlop)) {
        // Grab the track and move it. The playhead is not touched and keeps
        // playing where it is. Reached two ways, which are the same gesture:
        // shift+drag from the first pixel, and a plain track drag once it has
        // travelled far enough to stop being a click.
        //
        // Drag right -> the window shows EARLIER time, so the content appears
        // to travel with the cursor. This is the opposite sign to shift+WHEEL,
        // deliberately: a wheel is a scrollbar (scroll right, go later) and a
        // drag is a grab (pull right, bring earlier content into view). Both
        // are the conventions of their own idiom, and every map and canvas
        // application pairs them the same way.
        onPan?.(((panLastX - e.clientX) / w) * span)
        panLastX = e.clientX
      }
      // 'track' under the slop moves nothing yet — it is still a candidate
      // click. 'inert' never moves anything.
    }
  }
  function onPointerUp(e: PointerEvent) {
    if (!activePointers.has(e.pointerId)) return
    trackEl?.releasePointerCapture(e.pointerId)
    activePointers.delete(e.pointerId)
    if (activePointers.size === 1 && pinching) {
      pinching = false
      const [clientX] = activePointers.values()
      if (mode === 'fixed') {
        // Fixed mode: the remaining finger goes INERT rather than resuming
        // whatever the first finger was doing. It is somewhere new after the
        // two-finger gesture, so resuming a playhead drag from it would jump
        // the playhead, and resuming a track pan would jump the window.
        gesture = 'inert'
      } else {
        // Follow mode: RE-ARM the pan from the remaining pointer so a pinch can
        // flow back into a drag without a jump. Unchanged.
        armPan(clientX ?? startClientX)
      }
    } else if (activePointers.size === 0) {
      pinching = false
      dragging = false
      if (scrubOpen) {
        scrubOpen = false
        onScrubEnd?.()
      } else if (gesture === 'track' && !movedBeyondSlop && pressIsMouse) {
        // A fixed-mode track press that never travelled, made with a mouse: a
        // click, which seeks. Nothing has panned, because the pan branch only
        // runs once the same flag has flipped — the two are mutually exclusive
        // by construction, not by ordering.
        //
        // The pointerType gate is what keeps a touch TAP inert: an accidental
        // tap must not seek, and that decision predates the mode work. It also
        // means a pinch can never end as a click, since a mouse cannot raise a
        // second pointer.
        clickSeek()
      }
      gesture = 'inert'
    }
  }

  // Where the pointer sits across the track, as a position in the drawn window
  // (0 = left edge, 1 = right edge) — the coordinate onZoom's anchor is
  // expressed in. Read from the live rect rather than trackWidth so a scrolled
  // or resized page can't shift the anchor. Clamped: a wheel event can arrive
  // with the cursor a hair outside the bounds. null when there is no measurable
  // track, which the consumer reads as "no anchor expressed".
  function pointerFraction(clientX: number): number | null {
    const el = trackEl
    if (!el) return null
    const r = el.getBoundingClientRect()
    if (r.width <= 0) return null
    const f = (clientX - r.left) / r.width
    return f < 0 ? 0 : f > 1 ? 1 : f
  }

  // A wheel event's scroll distance in CSS pixels. Browsers disagree about
  // which axis a shift+wheel lands on — several report it as deltaX — so take
  // whichever axis moved, and normalise the unit, since deltaMode can be lines
  // or pages rather than pixels. WHEEL_LINE_PX is the usual ~1 line of text.
  const WHEEL_LINE_PX = 16
  function wheelPixels(e: WheelEvent): number {
    const raw = e.deltaX !== 0 ? e.deltaX : e.deltaY
    if (e.deltaMode === 1) return raw * WHEEL_LINE_PX
    if (e.deltaMode === 2) return raw * (trackWidth || 0)
    return raw
  }

  // Wheel (desktop, pointer only). Plain wheel zooms; shift+wheel pans when the
  // consumer binds it. The handler is attached non-passively in an $effect below
  // so preventDefault actually suppresses page scroll.
  function onWheel(e: WheelEvent) {
    const span = windowEnd - windowStart
    // Shift, not ctrl/cmd — those are the browser's own page zoom — and not
    // alt, which differs across platforms. Shift+wheel is the web's
    // horizontal-scroll convention, so it is what a pan should be bound to.
    //
    // Gated on the MODE, the same way shift+drag is: in follow mode shift has
    // no special meaning, so the event is left alone entirely — no
    // preventDefault — and the page keeps its own scroll rather than having
    // the gesture swallowed to do nothing.
    if (e.shiftKey) {
      if (mode !== 'fixed' || !onPan) return
      e.preventDefault()
      const w = trackWidth || trackEl?.clientWidth || 0
      if (w <= 0) return
      // Scrolled distance read as a distance across the track: one track width
      // of scroll pans by one whole viewport. Positive scrolls right/down and
      // moves the window LATER, as a horizontal scrollbar would.
      onPan((wheelPixels(e) / w) * span)
      return
    }
    // Zoom. Multiplicative step: scroll down/away -> larger span -> zoom out.
    // The parent clamps to [MIN_SPAN, MAX_SPAN] and decides what to anchor on —
    // we just report the pointer position with it.
    if (!onZoom) return
    e.preventDefault()
    onZoom(span * (e.deltaY > 0 ? 1.15 : 1 / 1.15), pointerFraction(e.clientX))
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
    <div class="coverage" aria-hidden="true">
      {#each coverageBands as band (band.id)}
        <span class="covered" style:left="{band.x0 * 100}%" style:width="{band.width * 100}%"
        ></span>
      {/each}
    </div>

    <div class="hourlines" aria-hidden="true">
      {#each hourLines as fraction (fraction)}
        <span class="hourline" style:left="{fraction * 100}%"></span>
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
      {#each microTicks as mt (mt.fraction)}
        <span class="microtick" style:left="{mt.fraction * 100}%"></span>
      {/each}
      {#each ticks as tick (tick.tSec)}
        <span class="tick" style:left="{tick.fraction * 100}%">
          <span class="tick-rule"></span>
        </span>
      {/each}
      {#each ticks as tick (tick.tSec)}
        <span class="tick-label" style:left="{clampLabelPx(tick.fraction)}px">{tick.label}</span>
      {/each}
    </div>

    <!-- Live control: a CHILD of .track (the bar itself), placed with its LEFT
         edge at the live-edge fraction so the capsule lies just inside the right
         hatch band and rides with the content as the user pans/zooms. Rendered
         only while the live edge is on-screen ([0,1]); the track's
         overflow:hidden clips the partial case near the right edge — no
         right-edge parking. Sits BEFORE .playhead in DOM order so the playhead
         line still draws on top of it. onpointerdown stopPropagation keeps a
         press on the capsule from starting a track scrub. -->
    {#if onGoLive && liveEdge > 0 && liveFracRaw >= 0 && liveFracRaw <= 1}
      <button
        type="button"
        class="live-jump-btn"
        style:left="{liveFracRaw * 100}%"
        onpointerdown={(e) => e.stopPropagation()}
        onclick={onGoLive}
        aria-label={ui.timelineGoLive}
      >
        <span class="live-jump-label">{ui.liveTag}</span>
        <Icon name="chevRight" size={15} />
      </button>
    {/if}

    <!-- Off-screen playhead marker (fixed mode only). Deliberately NOT a
         control: no handler, no hit area, no cursor, pointer-events:none. It is
         a piece of the playhead line showing through the edge it went out of,
         which is why it is a bare accent bar with a fade rather than anything
         with a border, a label or a background — the LIVE capsule is the
         bordered pill that MEANS something when pressed, and the two must not
         be mistaken for each other. Pinned to the track edge rather than placed
         at a content fraction, so it never rides with a pan. -->
    {#if offEdge}
      <span class="off-edge {offEdge}" aria-hidden="true"></span>
    {/if}

    <span
      class="playhead"
      class:dragging
      aria-hidden="true"
      style:transform="translateX({playheadFraction * trackWidth - 1}px)"
    >
      <span class="playhead-line"></span>
      <!-- Grip: the VISIBLE part of the handle, and FIXED MODE ONLY. The hit
           area is far wider and is computed arithmetically at pointerdown (44px
           on touch, 16px for a cursor), so this only has to be seen, not caught
           — which is what lets it stay narrow enough to sit over the coverage
           fill without hiding it, and in the band below the review and audio
           lanes rather than across them.
           Follow mode does not draw it: there the whole track is the drag
           surface, so a grip would advertise a distinction that does not exist
           and invite a grab that means nothing in particular. The cursor
           affordance goes with it, since there is no handle to promise. -->
      {#if mode === 'fixed'}
        <span class="playhead-grip"></span>
      {/if}
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
    style:transform="translateX({playheadFraction * trackWidth + 1}px)"
  >
    <!-- +1 (vs the playhead's -1) compensates the track's 1px left border: the
         flag lives in the borderless .scrubber frame, the playhead inside the
         bordered .track frame, so the flag's centre lands on the line. -->
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
  /* Taller track for fingers. Two-finger pan and pinch need somewhere to put
     two fingers, and 60px is a thin strip to land them in — especially since
     the fingers then cover most of what they are aiming at.
     Gated on BOTH conditions, because the height has to come out of a real
     budget: the timeline screen is `min-height: 100dvh` with no overflow clip
     below 900px, so anything that overflows scrolls, and it must not.
       pointer: coarse   only a finger needs this; a cursor does not, and on
                         desktop the page is height-capped with the video
                         flexing, so every pixel here is taken from the picture.
       min-height: 780   the screen can afford it. Measured against the mobile
                         stack (safe-area + 12, bar 38, gap 14, 16:9 frame,
                         gap 14, controls 46 or 102 wrapped, gap 14, mode row
                         38 + 8 + flag 26 + track, safe-area + 96 for the tab
                         bar): at 88px the tightest case in range — 375px wide,
                         wrapped controls, notched insets, summary still loading
                         — clears by 31px, and a 375x812 phone by 63px. Below
                         the threshold a 375x667 phone has only ~10px spare
                         while loading, so it keeps 60px rather than being given
                         a few pixels that would cost it a scroll.
     In an installed PWA the viewport is stable, so this does not flip about;
     in a browser tab with chrome showing, a shorter viewport correctly gets
     the shorter track. */
  @media (pointer: coarse) and (min-height: 780px) {
    .track {
      height: 88px;
    }
  }
  .track:focus-visible {
    outline: 2px solid var(--accent);
    outline-offset: 2px;
  }
  /* Recorded-coverage base fill: the bottom layer (first in DOM order), a
     full-height neutral grey marking footage-present spans so the amber/cyan
     review and violet audio lanes above it stand out. Gaps are simply left
     unfilled — the .track --feed background shows through as empty track. */
  .coverage {
    position: absolute;
    inset: 0;
    pointer-events: none;
  }
  /* Token-based neutral grey (no hex literal): a faint white wash reading as
     plain "footage here". Colour/opacity are a first pass — tune on device. */
  .covered {
    position: absolute;
    top: 0;
    bottom: 0;
    background: var(--border-strong);
  }
  /* Hour-boundary dividers: a crisp vertical line at each whole hour, the
     primary cue separating hours. Stronger than the faint --border label ticks.
     Width/colour are a first pass to tune on device. */
  .hourlines {
    position: absolute;
    inset: 0;
    pointer-events: none;
  }
  .hourline {
    position: absolute;
    top: 0;
    bottom: 0;
    width: 1px;
    background: var(--border-strong);
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
       which sit just above. SYNC: the timeline legend swatch
       (cam/[id]/timeline/+page.svelte, .swatch.audio) hard-codes this same
       literal — keep both in step until a shared app.css token exists. */
    background: oklch(0.72 0.14 300);
  }
  /* Out-of-footage band: the span before recording started / beyond now. A
     faint dark wash plus a low-opacity diagonal hatch so the empty edge reads
     as "no footage here", not missing UI. Subtle — must not fight the coverage
     fill or the playhead, and never captures pointer events. */
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
  /* Major (labelled) ruler tick: a short mark rising from the BOTTOM edge.
     Taller/brighter than the micro-ticks, shorter than the full-height
     .hourline so LENGTH distinguishes a ruler tick from an hour boundary.
     Exact height/colour are a first pass to tune on device. */
  .tick-rule {
    position: absolute;
    bottom: 0;
    height: 8px;
    width: 1px;
    background: var(--border-strong);
  }
  /* Micro-tick: a tiny faint mark from the bottom edge between the labelled
     ticks; shorter + fainter than .tick-rule. First-pass px — tune on device. */
  .microtick {
    position: absolute;
    bottom: 0;
    height: 4px;
    width: 1px;
    background: var(--border);
    transform: translateX(-0.5px);
    pointer-events: none;
  }
  .tick-label {
    position: absolute;
    /* Centred over its tick (left is a clamped px set inline) and sitting just
       ABOVE the 8px tick mark. First-pass bottom — tune on device. */
    bottom: 12px;
    transform: translateX(-50%);
    /* Geist (inherit), not JetBrains Mono: the mono dotted zero reads as an 8
       at this size. Keep tabular-nums so the digit columns stay aligned. */
    font-family: inherit;
    font-variant-numeric: tabular-nums;
    font-size: 12px;
    font-weight: 500;
    color: var(--text-2);
    letter-spacing: 0.2px;
    white-space: nowrap;
  }
  /* Off-screen playhead marker: a full-height accent bar hard against the track
     edge, with a short gradient fading inward — the playhead line seen edge-on
     through the side it left by. No border, no background plate, no label, so
     it cannot read as the LIVE capsule (a bordered, pressable pill sitting at a
     content fraction mid-track). Never interactive. First-pass sizes — tune on
     device. */
  .off-edge {
    position: absolute;
    top: 0;
    bottom: 0;
    width: 22px;
    pointer-events: none;
  }
  .off-edge.before {
    left: 0;
    background: linear-gradient(to right, var(--accent), transparent);
  }
  .off-edge.after {
    right: 0;
    background: linear-gradient(to left, var(--accent), transparent);
  }
  /* The hard 3px stripe at the very edge: the line itself, as opposed to the
     glow that points back toward it. */
  .off-edge::before {
    content: '';
    position: absolute;
    top: 0;
    bottom: 0;
    width: 3px;
    background: var(--accent);
  }
  .off-edge.before::before {
    left: 0;
  }
  .off-edge.after::before {
    right: 0;
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
  /* Visible grip on the playhead line. Sized and placed to be seen rather than
     caught — the hit area is arithmetic and much wider — so it can stay narrow
     enough not to hide the coverage fill it sits over, and low enough to clear
     the review lane (top 6px) and the audio lane (top 10px, 5px tall)
     completely. It stops above the tick labels, so the ruler stays readable
     through it. First-pass sizes — tune on device. */
  .playhead-grip {
    position: absolute;
    top: 17px;
    /* Centre the 12px grip on the 2px line, whose centre sits at x=1. */
    left: -5px;
    width: 12px;
    height: 16px;
    border-radius: 6px;
    background: var(--accent);
    /* Same hairline the hourlines use, so the grip reads as sitting ON the
       track rather than floating above it. */
    box-shadow: 0 0 0 1px rgba(0, 0, 0, 0.28);
    /* Only rendered in fixed mode, where the playhead IS what a drag moves, so
       the grip can say so unconditionally. Taking pointer events is purely for
       the cursor — the press bubbles to .track and is resolved by
       resolveGesture exactly as a press on the line would be. */
    pointer-events: auto;
    cursor: ew-resize;
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
  /* Live control: a compact BUTTON (the .back idiom, scaled down) living INSIDE
     the track — plain Geist "LIVE" + chevron on --surface with a --border
     hairline, reading as actionable rather than a passive badge. `left` is set
     inline at the live-edge fraction; a small margin-left nudges the button a
     few px right of that edge so it does not butt against the hatch band. It
     rides with the content on pan/zoom and is clipped by the track's overflow
     when the edge scrolls off. Vertically centred on the 60px track row, which
     keeps it clear of the review/audio lanes at the top. Tokens only. */
  .live-jump-btn {
    position: absolute;
    top: 50%;
    /* Gap from the live edge — nudge right so the button clears the hatch.
       First-pass px — tune on device. */
    margin-left: 6px;
    transform: translateY(-50%);
    display: inline-flex;
    align-items: center;
    gap: 5px;
    height: 26px;
    padding: 0 8px 0 10px;
    border-radius: var(--r-sm);
    border: 1px solid var(--border);
    background: var(--surface);
    color: var(--text);
    cursor: pointer;
    -webkit-appearance: none;
    appearance: none;
    font-family: inherit;
    white-space: nowrap;
    backdrop-filter: blur(8px);
    -webkit-backdrop-filter: blur(8px);
    transition:
      color 0.18s ease,
      border-color 0.18s ease;
  }
  .live-jump-label {
    font-family: inherit;
    font-size: 13px;
    font-weight: 600;
    line-height: 1;
  }
  .live-jump-btn:hover {
    border-color: var(--border-strong);
  }
  .live-jump-btn:hover :global(svg) {
    color: var(--text);
  }
  .live-jump-btn:active {
    /* Press dip composes with the vertical-centring translate. */
    transform: translateY(calc(-50% + 1px));
  }
  .live-jump-btn:focus-visible {
    outline: 2px solid var(--accent);
    outline-offset: 2px;
  }
  .live-jump-btn :global(svg) {
    color: var(--text-2);
  }
  @media (prefers-reduced-motion: reduce) {
    .playhead,
    .playhead-flag {
      transition: none;
    }
  }
</style>
