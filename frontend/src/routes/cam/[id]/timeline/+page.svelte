<!-- Phase 2b-iv-b-2: continuous gapless cross-chunk recording-timeline screen.
     Scrubbing rides a single low-res preview timelapse (instant seeking, no
     network per move); settling loads full-res as a bounded 10-min chunk and
     plays it. Full-res runs on a two-element double buffer: the idle buffer
     prefetches + primes the contiguous next chunk so playback continues across
     chunk boundaries with only a soft opacity cut. Playback STOPS only at the
     live edge of the captured window. Muted-only this phase. The 3h full-res
     master URL is gone: full-res is ever only a chunk. -->
<script module lang="ts">
  // Resolved per-camera recording-decode capability, shared across mounts and
  // deep-links so re-entry on the same camera skips the codec probe. true =
  // full-res decodable on this device, false = preview-only (cannot decode the
  // recording codec, e.g. H.265 with no hardware decoder). The undetermined
  // case (probe failed / no CODECS) is never cached.
  const decodeCache = new Map<string, boolean>()
</script>

<script lang="ts">
  import { page } from '$app/state'
  import { untrack } from 'svelte'
  import { goto } from '$app/navigation'
  import { camerasStore } from '$lib/stores/cameras.svelte'
  import { prefsStore } from '$lib/stores/prefs.svelte'
  import { timelineStore } from '$lib/stores/timeline.svelte'
  import {
    timelineMasterURL,
    timelineVideoOnlyURL,
    fetchPreviewClips,
    fetchPreviewFrameList,
    fetchRecordingCodecs,
    fetchReview,
    fetchAudioEvents,
    fetchCoverage
  } from '$lib/api'
  import type {
    PreviewClip,
    PreviewFrame,
    ReviewSegment,
    AudioMarker,
    CoverageSegment,
    TimelineMode
  } from '$lib/api'
  import { footageToWallclock, previewTailKey, tailHourOpen } from '$lib/timeline'
  import {
    DEFAULT_VIEW_SPAN,
    clampViewSpan,
    centredViewStart,
    anchoredViewStart,
    pinchViewStart,
    clampViewStart,
    clampPlayhead
  } from '$lib/timeline-viewport'
  import { canDecodeRecording } from '$lib/hls'
  import HlsVideo from '$lib/components/HlsVideo.svelte'
  import TimelineScrubber from '$lib/components/TimelineScrubber.svelte'
  import ZoomPane from '$lib/components/ZoomPane.svelte'
  import Icon from '$lib/components/Icon.svelte'
  import Mono from '$lib/components/Mono.svelte'
  import Segmented from '$lib/components/Segmented.svelte'
  import { ui } from '$lib/i18n/strings'

  // Full-res chunk: 10 min. The viewport's default span and its zoom bounds
  // live in $lib/timeline-viewport with the rest of the viewport arithmetic.
  const CHUNK_SECONDS = 10 * 60
  // Max distance (seconds) between the landed time and the full-res element's
  // current playback point for which an in-buffer currentTime seek is reliable.
  // Beyond it a paused HLS element may not apply the large seek before play()
  // resumes from the old currentTime, so we re-attach a fresh chunk at the
  // landed time instead (the else-branch) rather than snap back.
  const CHUNK_REJOIN_SLACK = 30
  // Prefetch the next chunk into the idle buffer once the active playhead is
  // within this many seconds of the active chunk's end.
  const PREFETCH_LEAD_SECONDS = 10
  // After a settle/seek, ignore the full-res timeupdate echo briefly so it
  // doesn't fight the UI right after the assignment.
  const SEEK_SETTLE_MS = 300
  // Playback speeds offered by the speed dropdown, in menu order. 1 is the
  // default playbackRate and must stay present. 4x is intentionally absent —
  // it can't keep the full-res chunk pipeline fed and stutters.
  const SPEED_STEPS = [0.5, 1, 2]
  // VHS press-and-hold rush: wall-clock multipliers and the hold-duration ramp
  // that selects between them. 6x immediately, 16x after 4s held, 40x
  // after 9s held — the longer the button is held, the faster the rush.
  const RUSH_RATES = [6, 16, 40]
  const RUSH_RAMP_MS = [0, 4000, 9000]

  // page.params.id is typed string | undefined; the route only matches when
  // [id] is bound, so the empty fallback never surfaces in practice.
  const camId = $derived(page.params.id ?? '')
  const camera = $derived(camerasStore.cameras.find((c) => c.id === camId) ?? null)

  // Visible span of the scrubber viewport, in wall-clock seconds. Defaults to
  // the 1h window; the pinch / wheel gestures adjust it through setViewSpan.
  // One half of the viewport pair — viewStart below is the other.
  let viewSpan = $state(DEFAULT_VIEW_SPAN)

  // PLAYBACK-domain bounds — the recording extent full-res playback may span,
  // DISTINCT from the derived windowStart/windowEnd viewport. The viewport pans
  // freely within this domain; these bounds gate what can actually load + play.
  // liveEdge: the newest playable wall-clock second (capture-time now). Always
  // the true now, even on a deep-link whose viewport end sits in the past.
  let liveEdge = $state(0)
  // recordingFloor: the oldest recorded second, read from the global summary
  // (hours are sorted ascending). Null until this camera's summary lands, and
  // guarded against a stale camera's summary still sitting in the store.
  const recordingFloor = $derived.by(() => {
    if (timelineStore.camId !== camId) return null
    const first = timelineStore.hours[0]
    if (!first) return null
    return Math.floor(first.hourStart.getTime() / 1000)
  })
  // playbackFloor: the oldest playable wall-clock second — the true recording
  // extent once the summary loads, else a permissive 7-day fallback so a
  // far-past deep-link / early pan isn't wrongly clamped before it lands.
  const playbackFloor = $derived(recordingFloor ?? liveEdge - 7 * 24 * 3600)

  // PLAYHEAD (unix sec) — what is PLAYING. Driven by drag (preview) or full-res
  // timeupdate (playback); consumed by the scrubber's handle + clock readout.
  // It no longer decides what is DRAWN: that is the viewport below. Every write
  // goes through setPosition.
  let position = $state(0)

  // VIEWPORT — viewStart plus viewSpan, the left edge and width of the drawn
  // window in wall-clock seconds. Real state, and the only thing that decides
  // what the scrubber draws. Every write goes through setViewStart.
  // VIEWPORT-only — playback bounds are liveEdge/playbackFloor.
  let viewStart = $state(0)

  // FOLLOW — whether the viewport tracks the playhead. It IS the user's chosen
  // timeline mode and nothing else: read from the preference, written by
  // nobody. While true the viewport is kept centred on the playhead, so the
  // playhead sits dead-centre (timeToFraction(position, windowStart,
  // windowEnd) === 0.5) and a drag or playback moves the CONTENT under it —
  // exactly what the old viewport-derived-from-playhead model produced by
  // construction. While false the track is stationary and the playhead travels
  // along it.
  //
  // It was briefly hidden state that gestures mutated, and every gesture then
  // carried its own private assumption about what it meant: a drag that moved
  // the playhead instead of the track, a zoom anchor that silently became the
  // window centre, a re-engagement rule the user could not see. Making it the
  // mode is what makes each gesture answerable — the mode decides what the
  // gesture means, rather than the gesture deciding what the mode is.
  //
  // No prefsSynced gate is needed here, unlike the effects that branch on a
  // preference once: this is a derivation, so it simply flips when /api/prefs
  // lands. The entry window is placed explicitly by the capture effect rather
  // than as a side effect of follow being on, so a flip mid-load moves nothing.
  const follow = $derived(prefsStore.timelineMode === 'follow')

  // The on-timeline mode switch. The SAME server-side preference the Settings
  // control writes, so the two are never two settings — switching in either
  // place is reflected in the other the next time it is drawn. It lives here as
  // well as in Settings because the mode is switched often and Settings is a
  // different screen.
  const timelineModeOptions: { value: TimelineMode; label: string }[] = [
    { value: 'follow', label: ui.timelineModeFollow },
    { value: 'fixed', label: ui.timelineModeFixed }
  ]

  // The window the scrubber draws. Unchanged in meaning and still the exact
  // pair of props passed to TimelineScrubber — only their source moved, from
  // the playhead to the viewport state above. With follow on,
  // viewStart === position - viewSpan/2, so these are the same two numbers the
  // old `position ± viewSpan/2` produced. Near the recording's edges the
  // viewport simply extends past the playable bounds and the scrubber's dim
  // bands mark the empty span.
  const windowStart = $derived(viewStart)
  const windowEnd = $derived(viewStart + viewSpan)

  // --- viewport writers -----------------------------------------------------
  // ORDERING RULE: the playhead and the viewport are never written in separate
  // reactive passes. Re-centring the viewport from an $effect that tracks
  // position would run AFTER the render which already committed the new
  // position, so one frame would draw the new playhead against the stale
  // window — a visible jitter on every pointermove of a drag. Both writes
  // happen inside one synchronous function instead, landing in the same batch,
  // so no frame ever sees the pair half-updated.

  // The ONLY writer of `viewStart`. clampViewStart holds the viewport CENTRE at
  // the live edge — a bound that used to be emergent (the window was a function
  // of the playhead, and the playhead was clamped to liveEdge) and that a
  // viewport made of real state no longer gets for free. See
  // $lib/timeline-viewport for why the clamp is one-sided and on the centre.
  function setViewStart(start: number) {
    viewStart = clampViewStart(start, viewSpan, liveEdge)
  }

  // Re-centre the viewport on the playhead. A pure recompute from
  // (position, viewSpan), never an accumulation, so repeated calls at any rate
  // land on exactly the value the old derived window produced.
  function centreOnPlayhead() {
    setViewStart(centredViewStart(position, viewSpan))
  }

  // The ONLY writer of `position`. Drag, VHS rush, the coverage snap and the
  // full-res timeupdate all land here.
  function setPosition(t: number) {
    position = t
    if (follow) centreOnPlayhead()
  }

  // Pan the viewport WITHOUT moving the playhead — shift+wheel and shift+drag,
  // both FIXED mode only. The playhead keeps playing where it is while the
  // window shows another stretch of time, which is what the viewport/playhead
  // split was for. The mode gate lives in the scrubber, which is where the
  // gesture is recognised: follow mode never calls this, because there the
  // track is already draggable and shift has no special meaning.
  function panViewport(deltaSeconds: number) {
    setViewStart(viewStart + deltaSeconds)
  }

  // A writer of `viewSpan` — the FOLLOW mode zoom. The playhead IS the zoom
  // anchor there, so re-centring after the span change reproduces the
  // zoom-around-the-centred-playhead exactly (position does not move, so no
  // chunk refetch is triggered). Deliberately kept as the arithmetic for this
  // case rather than routing it through the anchored writer below at fraction
  // 0.5: centredViewStart is the expression the centring invariant is stated
  // in, and re-deriving it through the anchor formula would re-associate the
  // floating point.
  //
  // In fixed mode this is also what a gesture that reports NO anchor lands on:
  // the span changes and the window's left edge stays where it is, which moves
  // nothing the user is pointing at. Pinch is the only such gesture, and it is
  // deliberately untouched here — anchoring it on the finger midpoint is a
  // later change.
  //
  // Either way the left edge is re-written after the span changes. In follow
  // mode that is the re-centring the mode is defined by. In fixed mode the
  // window stays exactly where it is and the write exists only to re-apply the
  // live-edge bound, which TIGHTENS as the span grows: a viewport parked at the
  // bound and then zoomed out would otherwise be left overhanging live by more
  // than half a span, which is the one thing that bound exists to prevent.
  function setViewSpan(span: number) {
    viewSpan = span
    if (follow) centreOnPlayhead()
    else setViewStart(viewStart)
  }

  // The other writer of `viewSpan` — the FIXED mode zoom. Holds the wall-clock
  // time under `fraction` (a position across the drawn window) fixed while the
  // span changes, which is what a stationary track requires: the moment under
  // the cursor is the moment you are zooming into. Never called in follow mode,
  // where the playhead is the anchor and setViewSpan above is the arithmetic.
  function setViewSpanAnchored(span: number, fraction: number) {
    const target = anchoredViewStart(viewStart, viewSpan, span, fraction)
    viewSpan = span
    setViewStart(target)
  }

  // The third writer of `viewSpan` — the FIXED mode two-finger gesture, which
  // zooms and pans at the same time. anchorTime is the wall-clock point the
  // fingers took hold of, frozen when the second finger landed; fraction is
  // where their midpoint is now. Both halves of the viewport are written from
  // those frozen inputs in one synchronous pass, so the pair is never seen
  // half-updated and repeated calls at pointermove rate cannot drift.
  function setViewSpanPinched(span: number, anchorTime: number, fraction: number) {
    const target = pinchViewStart(anchorTime, span, fraction)
    viewSpan = span
    setViewStart(target)
  }

  // Switching the mode back to FOLLOW re-centres the window on the playhead at
  // once. Without this the viewport would keep whatever the fixed-mode session
  // left it with until the next write to position — which during playback is
  // the next timeupdate, but while paused is never. "Follow" that visibly does
  // not follow until something else happens is not a mode.
  //
  // This is where it belongs, and NOT in the mode control's click handler.
  // `follow` is derived from the preference, so the preference changing is the
  // event; a handler would miss a change made in Settings, or on another device.
  // The effect READS follow and WRITES the viewport, which is the one direction
  // of causality the mode model allows — nothing here writes follow, and
  // reintroducing a gesture-driven write to it is exactly what the model exists
  // to prevent.
  //
  // TRANSITION only, tracked by a plain non-reactive flag. Fixed → follow
  // re-centres; follow → fixed leaves the window exactly where it is, which is
  // right — the ruler should start from what you were already looking at. The
  // first run only records the starting mode, so mounting never snaps.
  //
  // The ORDERING RULE above does not bite here. It forbids re-centring from an
  // effect that tracks POSITION, because that lands a frame after the render
  // which already drew the new position. This tracks the mode alone, which
  // changes at most once per user action and never mid-drag, and the write sits
  // inside untrack so position and viewSpan cannot become dependencies.
  let wasFollowing = true
  $effect(() => {
    const f = follow
    untrack(() => {
      if (f && !wasFollowing) centreOnPlayhead()
      wasFollowing = f
    })
  })

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
  // Bumped on every activeClip swap — the generation token the async prime
  // compares itself against. The prime is a play()/pause() round trip, and a
  // drag that crosses a clip boundary and comes straight back can swap the clip
  // out from under one that is mid-flight: the element's play() promise has
  // already been resolved by the engine, the swap then runs the media load
  // algorithm, and the .then lands afterwards against a file it does not
  // describe. Without the token it sets previewPrimed for a clip that was never
  // played, the new clip's own on-metadata prime early-returns on that flag, and
  // its currentTime seek never decodes — a bare seek paints nothing on mobile
  // until the element has been played once. That is a black preview with no way
  // out: pickClip early-returns on re-entering the same clip, and previewReady
  // is deliberately sticky across clip swaps, so nothing retries. This is the
  // activeClip equivalent of the camId guard loadClipsAround / loadFramesTail
  // already carry.
  let previewGen = 0
  // Coalesces rapid drag seeks to one currentTime assignment per frame.
  let previewSeekRaf: number | null = null
  // Same coalescing for the open-tail webp <img> swap (one src per frame).
  let frameRaf: number | null = null
  // The clip list for the window (sorted by start) and the clip currently
  // loaded into previewEl. previewSrc is the active clip's BFF /preview-clip
  // URL, '' when no clip covers the playhead (blank preview for that span).
  let clips = $state<PreviewClip[]>([])
  let activeClip = $state<PreviewClip | null>(null)
  // Review activity segments (grouped alert / detection) for the scrubber's
  // top lane. Loaded on the SAME trigger and over the SAME span as the
  // preview clips (capture reset + the debounced clip-follow), so the lane
  // stays correct as the playhead pans without a separate effect.
  let reviews = $state<ReviewSegment[]>([])
  // Audio-detection events (speech, bark, ...) for the scrubber's audio lane.
  // Loaded on the SAME trigger and over the SAME span as reviews / preview
  // clips, so the lane stays correct as the playhead pans.
  let audioEvents = $state<AudioMarker[]>([])
  // Recorded-coverage ranges for the scrubber's base fill layer. Loaded on the
  // SAME trigger and over the SAME span as reviews / audio / preview clips, so
  // the recorded/gap fill stays correct as the playhead pans.
  let coverage = $state<CoverageSegment[]>([])
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

  // The preview-clip list FOLLOWS the playhead as it pans: we fetch a span
  // around the playhead (CLIP_LOAD_SPAN wide) and refetch — debounced — when the
  // playhead nears a loaded edge, so the scrub preview stays correct anywhere in
  // the recording without refetching on every move. loadedClipsStart/End bracket
  // the last fetched list (null until the first load). clipFollowTimer is the
  // single trailing-edge debounce handle.
  const clipLoadSpan = $derived(viewSpan * 2)
  let loadedClipsStart = $state<number | null>(null)
  let loadedClipsEnd = $state<number | null>(null)
  let clipFollowTimer: ReturnType<typeof setTimeout> | null = null

  // Open-tail webp frame layer. frames is the tail's frame list (sorted by ts);
  // framesKey is previewTailKey(tail start, clock hour) — which tail's frames
  // are loaded and which hour they were fetched in, so the list is loaded once
  // per tail and stops being trusted when the hour it covers closes (see
  // loadFramesTail); frameSrc is the current nearest-frame src.
  let frames = $state<PreviewFrame[]>([])
  let framesKey = $state<string | null>(null)
  let frameSrc = $state('')
  // True once the preview-clip <video> has actually PAINTED a frame for the
  // currently entered camera at the current position (set on its 'seeked'
  // event). Until then a preview element may still be holding a previous
  // camera's / pre-seek frame, so the visibility gate hides it and the .frame
  // feed background shows instead. Reset ONLY on a camId change — NOT per clip
  // swap, which would flicker the feed background at clip boundaries mid-drag.
  let previewReady = $state(false)

  // Full-res playback runs on a TWO-element double buffer so playback
  // continues across chunk boundaries without a visible stall: while the
  // ACTIVE buffer plays its chunk, the IDLE buffer pre-loads and primes the
  // contiguous next chunk, then a soft opacity cut swaps them on 'ended'.
  // Each buffer owns an element ref + src + [start,end). Both srcs are ''
  // until a chunk loads, so no full-res network on mount. Muted-only this
  // phase — audio continuity across the seam is a later phase.
  let videoElA = $state<HTMLVideoElement | null>(null)
  let videoElB = $state<HTMLVideoElement | null>(null)
  let srcA = $state('')
  let srcB = $state('')
  let startA = $state<number | null>(null)
  let startB = $state<number | null>(null)
  let endA = $state<number | null>(null)
  let endB = $state<number | null>(null)
  // Per-buffer "degraded to video-only" flag. Set true only when a buffer's
  // chunk was reloaded as the audio-less index-v1.m3u8 rendition after an
  // audio-flap 503 (see degradeActiveToVideoOnly). A fresh chunk, a prefetch,
  // and a cleared idle buffer all load with audio, so every set*/clearIdle
  // helper resets the flag — it is only ever raised by the degrade path.
  let degradedA = $state(false)
  let degradedB = $state(false)
  // Which buffer currently drives the picture. The other is the prefetch
  // target. A swap flips this flag; all the active/idle derivations follow.
  let active = $state<'a' | 'b'>('a')
  // True once the idle buffer's prefetched chunk has decoded its first frame
  // (the prime play()/pause() landed) and is ready for an instant swap. Reset
  // whenever the idle buffer is cleared or reloaded.
  let idlePrimed = $state(false)

  // Active/idle views over the two buffers. Reads only — writes go through the
  // set*/clearIdle helpers so they target the correct physical buffer.
  const activeEl = $derived(active === 'a' ? videoElA : videoElB)
  const idleEl = $derived(active === 'a' ? videoElB : videoElA)
  const activeStart = $derived(active === 'a' ? startA : startB)
  const activeEnd = $derived(active === 'a' ? endA : endB)
  const activeSrc = $derived(active === 'a' ? srcA : srcB)
  const activeDegraded = $derived(active === 'a' ? degradedA : degradedB)
  const idleStart = $derived(active === 'a' ? startB : startA)
  const idleSrc = $derived(active === 'a' ? srcB : srcA)

  function setActiveChunk(start: number, end: number, src: string) {
    if (active === 'a') {
      startA = start
      endA = end
      srcA = src
      degradedA = false
    } else {
      startB = start
      endB = end
      srcB = src
      degradedB = false
    }
  }
  function setIdleChunk(start: number, end: number, src: string) {
    if (active === 'a') {
      startB = start
      endB = end
      srcB = src
      degradedB = false
    } else {
      startA = start
      endA = end
      srcA = src
      degradedA = false
    }
    idlePrimed = false
  }
  // Clear the idle buffer (drop its src/range) and the prefetch guard. Any
  // settle invalidates a prefetched continuation, and after a swap the old
  // active becomes idle and must be freed for the N+2 prefetch.
  function clearIdle() {
    if (active === 'a') {
      srcB = ''
      startB = null
      endB = null
      degradedB = false
    } else {
      srcA = ''
      startA = null
      endA = null
      degradedA = false
    }
    idlePrimed = false
  }

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

  // Preview-only mode: this device cannot decode the camera's recording codec
  // (e.g. H.265 with no hardware decoder), so full-res is never attempted —
  // the scrubber + low-res preview stay fully functional and an honest hint
  // replaces the misleading "No recording" overlay. Resolved proactively from
  // the master playlist's CODECS (primary) with the HlsVideo decode error as a
  // backstop. scrubActive is true during an active drag so the hint clears to
  // reveal the preview frame underneath.
  let previewOnly = $state(false)
  let scrubActive = $state(false)
  // True once full-res has been PROVEN decodable on this device for the
  // currently entered camera this session (cached/probed decodable, or frames
  // confirmed advancing). Once proven, a later decode-class error is a
  // transient glitch (e.g. MEDIA_ERR_DECODE while scrubbing or near the live
  // edge), NOT a real codec incompatibility — the backstop must ignore it
  // rather than latch into preview-only.
  let decodeProven = $state(false)
  // canPlayType/native-vs-MSE probe element. Decode capability is engine-wide,
  // not per-instance, so a detached element answers identically to the live
  // player and is always available regardless of bind timing.
  const probeVideo = typeof document !== 'undefined' ? document.createElement('video') : null

  // Full-res mute. Muted by default so the one-shot entry autoplay is never
  // blocked; the user opts into audio via the control button. HlsVideo's
  // element is muted by default too — we drive el.muted directly.
  let fullResMuted = $state(true)

  // Full-res playback rate, cycled by the speed chip. Applied to whichever
  // buffer is active (and re-applied on confirmed start + after each swap,
  // since a fresh chunk src resets the element's rate). A fresh camera starts
  // at normal speed.
  let playbackRate = $state(1)

  // VHS press-and-hold rewind / fast-forward. vhsDir: 0 idle, 1 fast-forward,
  // -1 rewind. vhsRate is the current wall-clock multiplier shown on the side
  // indicator (0 when idle). The rush rides the SAME low-res preview path a manual
  // drag uses (handleScrubStart / handleSeek / handleScrubEnd) — no separate
  // seek logic.
  let vhsDir = $state<0 | 1 | -1>(0)
  let vhsRate = $state(0)
  // Non-reactive rush bookkeeping: press timestamp (drives the ramp), last rAF
  // timestamp (drives dt), and the rAF handle.
  let vhsHoldStart = 0
  let vhsLastTs = 0
  let vhsRaf: number | null = null

  let lastSeekAt = 0

  // Layer visibility: the settled picture is the poster until full-res paints.
  const fullResVisible = $derived(mode === 'playback' && showFullRes)
  // Open-tail webp frame: visible whenever full-res has not yet painted (not
  // just during the drag), so the released webp frame holds as the poster
  // through the full-res load instead of flashing an empty preview-video frame.
  const framesVisible = $derived(!fullResVisible && isOpenTail && frameSrc !== '')
  // Preview <video>: only the fallback when neither full-res nor the webp frame
  // is showing, AND it actually holds the current camera's painted frame at the
  // settled position (previewSrc set + previewReady) — otherwise a stale
  // previous-camera / pre-seek frame is hidden and the .frame feed background
  // shows as the neutral poster until the correct frame paints.
  const previewVisible = $derived(
    !fullResVisible && !framesVisible && previewSrc !== '' && previewReady
  )

  // On camId change: capture the playable bounds, re-arm follow, place the
  // playhead, reset to scrubbing, drop any loaded chunk, and pull the summary
  // for the recording floor. The entry window is centred on the playhead in
  // BOTH modes: an optional ?t=<unix> deep-link (from an event's "See on
  // timeline") lands the playhead at t and the viewport centred on it; without
  // it the playhead rests centred in the last viewSpan up to live (so the entry
  // window is the last viewSpan ending at live), then autoplays forward. Read
  // the raw t param in the tracked scope so a same-camera deep-link with a new
  // t re-captures.
  const tParam = $derived(page.url.searchParams.get('t'))
  // Fires the heavy reset + one-shot autoplay once per real (camId, t) change.
  // The effect also re-runs on unrelated page.url churn (tParam is derived from
  // page.url); the key compare keeps a spurious re-run from wiping the
  // buffers/mode mid-playback, which would freeze on the preview poster.
  let lastCaptureKey: string | null = null
  $effect(() => {
    const id = camId
    const rawT = tParam
    if (!id) return
    const key = `${id}|${rawT ?? ''}`
    if (key === lastCaptureKey) return
    untrack(() => {
      lastCaptureKey = key
      const now = Math.floor(Date.now() / 1000)
      // PLAYBACK live edge is the true capture-time now REGARDLESS of the
      // deep-link branch: a deep-link's derived viewport end may sit in the
      // past, but full-res must still play/stop at real now.
      liveEdge = now
      // Defensive parse: a malformed ?t= (empty, non-numeric) falls back to
      // the default last-1h window, never NaN.
      const tSec = rawT !== null ? Number.parseInt(rawT, 10) : Number.NaN
      if (Number.isFinite(tSec)) {
        // Deep-link: a real event time is always a valid past second, so just
        // land the playhead at t (never past live). playbackFloor is derived and
        // may be the permissive fallback until the summary lands, so we do NOT
        // clamp the entry position by it — it governs panning once loaded.
        setPosition(tSec > liveEdge ? liveEdge : tSec)
      } else {
        // Default: rest the playhead centred in the last viewSpan up to live so
        // the viewport shows the last 1h; autoplay then runs it forward to live.
        setPosition(liveEdge - viewSpan / 2)
      }
      // Place the entry window explicitly rather than relying on setPosition
      // having done it: in fixed mode the viewport does not track the playhead,
      // so nothing else would put it anywhere, and it would still be sitting on
      // the previous camera's window (or on unix 0 for a first mount). In
      // follow mode this is the same pure recompute setPosition just made, so
      // it lands on exactly the same second and changes nothing.
      centreOnPlayhead()
      mode = 'scrubbing'
      // Reset BOTH buffers and the swap state.
      active = 'a'
      srcA = ''
      srcB = ''
      startA = null
      startB = null
      endA = null
      endB = null
      degradedA = false
      degradedB = false
      idlePrimed = false
      previewDuration = 0
      // New clip swaps previewSrc, so the new file must be re-primed.
      previewPrimed = false
      showFullRes = false
      pendingPlay = false
      chunkEnded = false
      playerError = false
      paused = true
      // Fresh camera starts at normal speed.
      playbackRate = 1
      // Reset decode-capability gating for the (re-)entered camera.
      previewOnly = false
      scrubActive = false
      decodeProven = false
      // Fresh camera: drop the loaded clips/clip, then load the clip list
      // around the playhead and pick the clip there.
      clips = []
      // Same retirement as pickClip's: a prime still in flight for the previous
      // camera's clip must not mark the newly entered camera's clip primed.
      previewGen++
      activeClip = null
      // Drop the review lane for the previous camera; it reloads below over
      // the same span as the clips.
      reviews = []
      // Same for the audio lane.
      audioEvents = []
      // Same for the recorded-coverage base fill.
      coverage = []
      // Reset the clip-follow window + cancel any pending debounced refetch.
      loadedClipsStart = null
      loadedClipsEnd = null
      if (clipFollowTimer !== null) {
        clearTimeout(clipFollowTimer)
        clipFollowTimer = null
      }
      // Drop any open-tail frame state captured for the previous camera.
      frames = []
      framesKey = null
      frameSrc = ''
      // Hide the preview <video> until it paints the new camera's frame at the
      // settled position, so a stale previous-camera frame can't flash.
      previewReady = false
      void timelineStore.load(id)
      void loadClipsAround(position)
      void loadReviewAround(position)
      void loadAudioAround(position)
      void loadCoverageAround(position)
      // Resolve decode capability, then gate the one-shot entry autoplay on it:
      // play full-res only when this device is known to decode the recording
      // codec. While capability is still pending we stay on the preview layer
      // (scrubber already works) so there is no failed-load flash.
      void resolveCapability(id, position)
    })
  })

  // Decide whether full-res is decodable on this device for the entered camera
  // and, when so, kick the one-shot entry autoplay. Cache hit applies instantly
  // (no fetch). On a miss, probe the master playlist's CODECS for a recent
  // covered window and check it against the path the player will take. A null
  // codecs string is undetermined: stay previewOnly=false and attempt full-res
  // anyway, letting the reactive decode-error backstop catch a real mismatch.
  // Guards against the camera changing during the await before applying.
  async function resolveCapability(id: string, t: number) {
    const cached = decodeCache.get(id)
    if (cached !== undefined) {
      previewOnly = !cached
      if (cached) {
        decodeProven = true
        ensurePlaybackAt(t)
      }
      return
    }
    const codecs = await fetchRecordingCodecs(id, windowEnd - 3600, windowEnd)
    if (camId !== id) return
    if (codecs === null) {
      // Undetermined: do NOT cache; rely on the reactive backstop. Attempt
      // full-res so that backstop can actually fire on a true decode failure.
      ensurePlaybackAt(t)
      return
    }
    const canDecode = probeVideo ? canDecodeRecording(codecs, probeVideo) : true
    decodeCache.set(id, canDecode)
    previewOnly = !canDecode
    if (canDecode) {
      decodeProven = true
      ensurePlaybackAt(t)
    }
  }

  // Load the preview-clips list spanning CLIP_LOAD_SPAN around center (clamped
  // to the playable domain) and pick the clip at the playhead. Guard against a
  // stale camId — an in-flight list for a camera the user has navigated away
  // from must not overwrite the new one. On resolve, record the loaded span so
  // the follow effect knows when the playhead nears an edge. On error the list
  // stays empty (blank preview; scrubber / clock / full-res still work).
  async function loadClipsAround(center: number) {
    const id = camId
    const half = clipLoadSpan / 2
    // The BFF preview-clips route requires integer unix seconds
    // (validUnixSeconds), same as review/audio/coverage — floor the bounds so a
    // fractional playhead (position is fractional after playback) can't 400.
    const lo = Math.floor(Math.max(playbackFloor, center - half))
    const hi = Math.floor(Math.min(liveEdge, center + half))
    try {
      const list = await fetchPreviewClips(id, lo, hi)
      if (camId !== id) return
      clips = [...list].sort((a, b) => a.start - b.start)
      loadedClipsStart = lo
      loadedClipsEnd = hi
    } catch {
      if (camId !== id) return
      clips = []
    }
    pickClip(position)
  }

  // Load the review-activity segments spanning the same clipLoadSpan window
  // around center (clamped to the playable domain) as loadClipsAround. Guard a
  // stale camId — an in-flight list for a camera the user has navigated away
  // from must not overwrite the new one. On error the lane stays empty (no
  // markers; the scrubber still works). No pickClip equivalent — the scrubber
  // renders the whole list against the viewport.
  async function loadReviewAround(center: number) {
    const id = camId
    const half = clipLoadSpan / 2
    const lo = Math.max(playbackFloor, center - half)
    const hi = Math.min(liveEdge, center + half)
    try {
      const list = await fetchReview(id, Math.floor(lo), Math.floor(hi))
      if (camId !== id) return
      reviews = list
    } catch {
      if (camId !== id) return
      reviews = []
    }
  }

  // Load the audio-detection events spanning the same clipLoadSpan window
  // around center (clamped to the playable domain) as loadReviewAround. Guard a
  // stale camId — an in-flight list for a camera the user has navigated away
  // from must not overwrite the new one. On error the lane stays empty (no
  // markers; the lane simply hides).
  async function loadAudioAround(center: number) {
    const id = camId
    const half = clipLoadSpan / 2
    const lo = Math.max(playbackFloor, center - half)
    const hi = Math.min(liveEdge, center + half)
    try {
      const list = await fetchAudioEvents(id, Math.floor(lo), Math.floor(hi))
      if (camId !== id) return
      audioEvents = list
    } catch {
      if (camId !== id) return
      audioEvents = []
    }
  }

  // Load the recorded-coverage ranges spanning the same clipLoadSpan window
  // around center (clamped to the playable domain) as loadReviewAround. Guard a
  // stale camId — an in-flight list for a camera the user has navigated away
  // from must not overwrite the new one. On error the layer stays empty (no
  // recorded fill; the track shows as an empty bar).
  async function loadCoverageAround(center: number) {
    const id = camId
    const half = clipLoadSpan / 2
    const lo = Math.max(playbackFloor, center - half)
    const hi = Math.min(liveEdge, center + half)
    try {
      const list = await fetchCoverage(id, Math.floor(lo), Math.floor(hi))
      if (camId !== id) return
      coverage = list
    } catch {
      if (camId !== id) return
      coverage = []
    }
  }

  // Position-following clip refetch (debounced, trailing edge). When the
  // playhead pans within viewSpan/2 of either loaded edge — and that edge isn't
  // already pinned at the playable bound — schedule a refetch around the new
  // position after a short settle. Tracks position; a cheap no-op until near an
  // edge, so it does NOT refetch on every timeupdate. Each new schedule clears
  // the prior timer (single trailing debounce), so a pan/rush coalesces into one
  // fetch when the finger settles.
  $effect(() => {
    const pos = position
    const lo = loadedClipsStart
    const hi = loadedClipsEnd
    if (lo === null || hi === null) return
    const margin = viewSpan / 2
    const nearStart = pos - lo < margin && lo > playbackFloor
    const nearEnd = hi - pos < margin && hi < liveEdge
    if (!nearStart && !nearEnd) return
    if (clipFollowTimer !== null) clearTimeout(clipFollowTimer)
    clipFollowTimer = setTimeout(() => {
      clipFollowTimer = null
      void loadClipsAround(position)
      void loadReviewAround(position)
      void loadAudioAround(position)
      void loadCoverageAround(position)
    }, 250)
  })

  // One-time decoder prime: a muted play()/pause() cycle so subsequent
  // currentTime seeks actually decode and paint. No-op once primed. If
  // autoplay is blocked (iOS Low-Power, no gesture) the play() promise
  // rejects and we leave previewPrimed false — handleScrubStart retries the
  // prime from a real user gesture, which is always allowed.
  function primePreview() {
    const el = previewEl
    if (previewPrimed || !el) return
    // The clip this prime is for. Every caller runs in a task (a media event, a
    // pointerdown, the readiness poll), and Svelte flushes the src in a
    // microtask, so the element already holds activeClip's file here — only the
    // RESOLUTION below can land against a different one.
    const gen = previewGen
    el.play()
      .then(() => {
        // A stale resolution touches nothing at all: the swap that bumped the
        // generation ran the media load algorithm, which already stopped this
        // playback, and pausing across generations could abort the new clip's
        // own prime.
        if (gen !== previewGen) return
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
    // Retire any prime still in flight for the outgoing clip before the swap,
    // so its resolution cannot claim the incoming one as primed.
    previewGen++
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

  // Snap a settle target onto recorded footage. Frigate's VOD only plays
  // forward and silently skips a gap to the first real segment, but our
  // wall-clock anchor (position = chunkStart + el.currentTime) counts from the
  // chunk's start second — so a chunk that starts inside a gap makes the flag
  // clock lag the real footage by the gap size. Snapping a gap target forward
  // to the first recorded second keeps chunkStart on footage and the anchor
  // honest. Settle-only (never during a drag): scrubbing must follow the finger.
  function snapToCoverage(t: number): number {
    // No coverage data (not yet loaded, or a continuous-record camera reporting
    // full coverage): degrade to current behaviour, no snap.
    if (coverage.length === 0) return t
    // Already on footage: leave it.
    for (const b of coverage) {
      if (t >= b.start && t <= b.end) return t
    }
    // In a gap: snap to the FORWARD-nearest band (smallest start >= t). Do not
    // assume coverage is sorted; scan for the minimum qualifying start.
    let best: number | null = null
    for (const b of coverage) {
      if (b.start >= t && (best === null || b.start < best)) best = b.start
    }
    // No forward band — t is past the last loaded coverage. Near the live edge
    // coverage reaches ~now, so this is a sub-segment tail; a BACKWARD snap
    // would rewind the user surprisingly and Frigate VOD only plays forward
    // anyway. Forward-only: leave t unchanged.
    return best ?? t
  }

  // Lazily load the open live tail's webp preview frame list, once per tail
  // span AND per clock hour. The tail spans [lastClipEnd, windowEnd] — the
  // live, not-yet-clipped footage after the newest preview clip, which has no
  // assembled mp4, so we scrub it frame-by-frame. framesKey is set to
  // previewTailKey(...) immediately so a fast drag firing this repeatedly does
  // not spawn duplicate fetches (and an error never retry-loops).
  //
  // The hour is half the key because Frigate DELETES the tail's frames the
  // moment that hour closes and its assembled mp4 appears — a list keyed on the
  // span alone keeps requesting webps that no longer exist for the rest of the
  // session, since lastClipEnd only moves when the clip list is refetched and
  // that has its own trigger. The clock is read here, on demand: every open-tail
  // seek already calls this, so a rollover is noticed the next time the tail is
  // actually touched, with no timer and no work while nobody is scrubbing.
  //
  // On resolve, guard against a stale camId AND a stale key (the user may have
  // scrubbed off the tail, or the hour may have rolled, before the list lands);
  // on error the list stays empty (the layer shows the feed bg).
  async function loadFramesTail() {
    const end = lastClipEnd
    if (end == null) return
    const nowSec = Date.now() / 1000
    const key = previewTailKey(end, nowSec)
    if (framesKey === key) return
    framesKey = key
    // The hour this tail was open in has closed. There are no frames to fetch
    // for it any more, and the honest source for the span is the preview mp4
    // Frigate has just assembled — so drop the dead list and refetch the clips,
    // which advances lastClipEnd and takes the playhead off the tail entirely.
    // frameSrc is deliberately left alone: the already-decoded webp holds as the
    // poster (pickFrame no-ops on an empty list) until the clip preview paints,
    // instead of blanking the picture. Guarded by framesKey, so this runs once
    // per (tail, hour) and cannot loop.
    if (!tailHourOpen(end, nowSec)) {
      frames = []
      void loadClipsAround(position)
      return
    }
    const id = camId
    try {
      // Both bounds must be integer unix seconds (validUnixSeconds), same as
      // the clip / review / coverage routes — a VHS rush or a full-res
      // timeupdate leaves position fractional, which makes windowEnd fractional.
      // The end bound also takes the playhead into account: the tail is reached
      // BY POSITION, and in fixed mode the viewport routinely sits entirely
      // before it — the track does not move while the playhead runs on, so this
      // is the ordinary case there rather than a corner one. Asking for an
      // inverted range 400s, and framesKey would then pin the empty list for
      // the rest of the hour: a live tail that never paints again.
      const tailEnd = Math.floor(Math.max(windowEnd, position))
      const list = await fetchPreviewFrameList(id, Math.floor(end), tailEnd)
      if (camId !== id || framesKey !== key) return
      frames = [...list].sort((a, b) => a.ts - b.ts)
      pickFrame()
    } catch {
      if (camId !== id || framesKey !== key) return
      frames = []
      frameSrc = ''
    }
  }

  // Show the open tail's preview frame nearest the current wall-clock position.
  // frames are sorted by ts; pick the nearest by absolute ts distance. The
  // <img> swaps instantly — each immutable webp is browser-cached after first
  // fetch. A fast drag fires this many times per frame, so coalesce to one src
  // swap per animation frame reading the LATEST position (symmetric with
  // seekPreview). No-op (keeps the current frame) when the list is empty.
  function pickFrame() {
    if (frameRaf !== null) return
    frameRaf = requestAnimationFrame(() => {
      frameRaf = null
      let best: PreviewFrame | null = null
      let bestDist = Infinity
      for (const f of frames) {
        const d = Math.abs(f.ts - position)
        if (d < bestDist) {
          best = f
          bestDist = d
        }
      }
      if (best) frameSrc = best.src
    })
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
    // 'seeked' fires after a currentTime assignment has actually painted the
    // correct position — the moment the preview is the picture at the settled
    // position and safe to reveal. ('loadeddata' would mark ready on frame 0,
    // the wrong position.)
    const onSeeked = () => {
      previewReady = true
    }
    // Some Frigate preview clips report duration NaN/Infinity at
    // 'loadedmetadata' (onMeta strands previewDuration at 0, so seekPreview
    // early-returns forever and the frame never paints). The usable duration
    // arrives later on 'durationchange'/'loadeddata' — pick it up there and
    // seek. Engines differ on which event carries it, so listen on both.
    const onDuration = () => {
      const d = el.duration
      if (Number.isFinite(d) && d > 0 && d !== previewDuration) {
        previewDuration = d
        seekPreview()
      }
    }
    el.addEventListener('loadedmetadata', onMeta)
    el.addEventListener('seeked', onSeeked)
    el.addEventListener('durationchange', onDuration)
    el.addEventListener('loadeddata', onDuration)
    return () => {
      el.removeEventListener('loadedmetadata', onMeta)
      el.removeEventListener('seeked', onSeeked)
      el.removeEventListener('durationchange', onDuration)
      el.removeEventListener('loadeddata', onDuration)
    }
  })

  // Chunk-scoped readiness poll handle (see the effect below). Its lifetime is
  // the current chunk, NOT a timeout — it retries play() until playback truly
  // begins, then stops.
  let readyPoll: ReturnType<typeof setInterval> | null = null
  function stopReadyPoll() {
    if (readyPoll !== null) {
      clearInterval(readyPoll)
      readyPoll = null
    }
  }

  // Clip/settle-scoped preview readiness poll handle (see the effect below). It
  // retries the preview seek until the frame actually paints, then stops — the
  // safety net for a closed-hour frame that didn't seek/paint on the first try
  // (late duration, a dropped seek). Mirrors readyPoll's lifecycle.
  let previewReadyPoll: ReturnType<typeof setInterval> | null = null
  function stopPreviewReadyPoll() {
    if (previewReadyPoll !== null) {
      clearInterval(previewReadyPoll)
      previewReadyPoll = null
    }
  }

  // Reveal the full-res layer and (re)attempt the deferred play once the loaded
  // chunk has a decodable frame. REVEAL is unconditional: as soon as a frame
  // exists the layer is shown, independent of whether playback has actually
  // started. PLAY-START is best-effort and retry-safe: pendingPlay is cleared
  // ONLY on a confirmed start (the play() success, or the 'playing' event) —
  // a rejected/interrupted early play() leaves pendingPlay true so the poll
  // retries. Calling play() on a paused native element is also what makes a
  // slow manifest fetch its next segment and advance, so retrying past the old
  // ~3s cap is what breaks the native paused-deadlock.
  function markReadyIfPlayable() {
    const el = activeEl
    if (!el || mode !== 'playback') return
    showFullRes = true
    if (pendingPlay && el.paused) {
      void el
        .play()
        .then(() => {
          pendingPlay = false
        })
        .catch(() => {})
    }
  }

  // Prefetch the contiguous next chunk into the IDLE buffer and prime it, so
  // an 'ended' swap can reveal a decoded frame instantly. Fired from the
  // active element's timeupdate once the playhead is within
  // PREFETCH_LEAD_SECONDS of the active chunk's end. No-op once the idle
  // buffer already holds the contiguous next chunk (idle start === activeEnd),
  // or at the live edge of the window (no next chunk).
  function runPrefetchCheck() {
    if (mode !== 'playback') return
    const aEnd = activeEnd
    if (aEnd === null) return
    // Scale the lead by the playback rate so the real-time prefetch window
    // stays constant: at 4x the playhead eats the chunk 4x faster, so the
    // prefetch must start 4x further from the end to keep the seam gapless.
    if (aEnd - position > PREFETCH_LEAD_SECONDS * playbackRate) return
    // PLAYBACK live-edge guard: no chunk past the recording's live edge.
    if (aEnd >= liveEdge) return
    if (idleStart === aEnd) return
    const ns = aEnd
    // Next chunk's end is a PLAYBACK bound — cap it at the live edge.
    const ne = Math.min(aEnd + CHUNK_SECONDS, liveEdge)
    setIdleChunk(ns, ne, timelineMasterURL(camId, ns, ne))
    primeIdle()
  }

  // Prime the idle buffer: once its element can play, seek to 0 and run one
  // muted play()/pause() so the first frame decodes and a little buffers, then
  // leave it paused and hidden. Captures the primed src so a settle/clear that
  // reassigns the idle buffer mid-prime (or a swap that turns this element
  // ACTIVE) cancels the pause()/flag without touching what the buffer now
  // holds — guarded by the idleSrc compare before any side effect.
  function primeIdle() {
    const el = idleEl
    const src = idleSrc
    if (!el || !src) return
    idlePrimed = false
    const finish = () => {
      el.removeEventListener('canplay', finish)
      el.removeEventListener('loadeddata', finish)
      if (idleSrc !== src) return
      try {
        el.currentTime = 0
      } catch {
        // some elements reject a pre-metadata seek; play() below still primes
      }
      el.muted = true
      void el
        .play()
        .then(() => {
          el.pause()
          if (idleSrc === src) idlePrimed = true
        })
        .catch(() => {})
    }
    el.addEventListener('canplay', finish)
    el.addEventListener('loadeddata', finish)
  }

  // Active chunk reached its end. At the window's live edge there is no next
  // chunk — stop and hint. Otherwise swap to the idle buffer: a clean swap
  // when it holds the primed contiguous chunk, else a fallback that loads the
  // next chunk and starts it through pendingPlay (a brief stall, not a freeze).
  function handleActiveEnded() {
    const aEnd = activeEnd
    // Live edge of the recording extent — no next chunk past it.
    if (aEnd === null || aEnd >= liveEdge) {
      paused = true
      chunkEnded = true
      return
    }
    if (idleStart === aEnd && idlePrimed) {
      cleanSwap()
    } else {
      fallbackSwap(aEnd)
    }
  }

  // Clean swap: the idle buffer holds the primed contiguous chunk. Flip
  // active, reveal the primed (already-decoded) buffer and play it; the old
  // active becomes idle and is cleared so the next timeupdate can prefetch
  // N+2 into it. Soft opacity cut via the .layer opacity transition.
  function cleanSwap() {
    active = active === 'a' ? 'b' : 'a'
    chunkEnded = false
    showFullRes = true
    pendingPlay = false
    playerError = false
    const el = activeEl
    applyPlaybackRate()
    if (el) void el.play().catch(() => {})
    clearIdle()
  }

  // Fallback swap: the prefetch did not land (slow camera) or the idle buffer
  // is not contiguous. Ensure the idle buffer holds [nextStart, …), flip to
  // it, and start it through the pendingPlay + readiness-poll path while the
  // preview poster holds. A brief stall is the accepted fallback.
  function fallbackSwap(nextStart: number) {
    if (idleStart !== nextStart || idleSrc === '') {
      const ne = Math.min(nextStart + CHUNK_SECONDS, liveEdge)
      setIdleChunk(nextStart, ne, timelineMasterURL(camId, nextStart, ne))
    }
    active = active === 'a' ? 'b' : 'a'
    chunkEnded = false
    showFullRes = false
    pendingPlay = true
    playerError = false
    idlePrimed = false
    lastSeekAt = performance.now()
    applyPlaybackRate()
    clearIdle()
  }

  // Attach the full-res listeners to ONE buffer element. Every handler is a
  // no-op unless its element is currently the ACTIVE one, so the idle buffer's
  // own play/pause/timeupdate churn (from priming) is ignored. videoElA/B are
  // stable across chunk reloads (HlsVideo only swaps src), so this attaches
  // once per element.
  function attachFullRes(el: HTMLVideoElement, which: 'a' | 'b') {
    const isActive = () => active === which
    const onTime = () => {
      if (!isActive() || mode !== 'playback') return
      const start = which === 'a' ? startA : startB
      if (start === null) return
      if (performance.now() - lastSeekAt < SEEK_SETTLE_MS) return
      // Map footage-time to wall-clock THROUGH the coverage bands: the VOD
      // splices only recorded segments, so a naive start + currentTime drifts
      // behind the picture once playback crosses a gap. Walking the bands makes
      // the flag jump each gap in lockstep with the footage.
      setPosition(footageToWallclock(start, el.currentTime, coverage))
      runPrefetchCheck()
    }
    const onPlay = () => {
      if (isActive()) paused = false
    }
    const onPause = () => {
      if (isActive()) paused = true
    }
    const onReady = () => {
      if (isActive()) markReadyIfPlayable()
    }
    // Definitive "started" signal: clear the deferred-play flag and stop the
    // readiness poll. 'play' only means play() was requested; 'playing' means
    // frames are actually advancing.
    const onPlaying = () => {
      if (!isActive()) return
      // Frames are actually advancing: full-res decodes on this device, so any
      // later decode-class error is a transient glitch, not a codec mismatch.
      decodeProven = true
      showFullRes = true
      pendingPlay = false
      paused = false
      // A fresh chunk src starts at rate 1; re-apply the chosen rate on the
      // confirmed start so the seam doesn't reset to normal speed.
      applyPlaybackRate()
      stopReadyPoll()
    }
    const onEnded = () => {
      if (isActive()) handleActiveEnded()
    }
    el.addEventListener('timeupdate', onTime)
    el.addEventListener('play', onPlay)
    el.addEventListener('pause', onPause)
    el.addEventListener('canplay', onReady)
    el.addEventListener('playing', onPlaying)
    el.addEventListener('ended', onEnded)
    return () => {
      el.removeEventListener('timeupdate', onTime)
      el.removeEventListener('play', onPlay)
      el.removeEventListener('pause', onPause)
      el.removeEventListener('canplay', onReady)
      el.removeEventListener('playing', onPlaying)
      el.removeEventListener('ended', onEnded)
    }
  }
  $effect(() => {
    const el = videoElA
    if (!el) return
    return attachFullRes(el, 'a')
  })
  $effect(() => {
    const el = videoElB
    if (!el) return
    return attachFullRes(el, 'b')
  })

  // Chunk-scoped readiness poll, rekeyed on the ACTIVE buffer's src. A fresh
  // chunk attaches inside HlsVideo (native el.src, or after the async hls.js
  // import), so canplay/playing can fire outside the window our listeners
  // observe — and on a paused native element a slow manifest may never advance
  // to canplay on its own. Its lifetime is the CHUNK, not a timeout: while the
  // chunk is loaded and playback is still pending, keep revealing + retrying
  // play() until the element actually starts. markReadyIfPlayable is a no-op
  // once playing (el.paused is false), and the 'playing' listener clears
  // pendingPlay + stops this poll. After a clean swap the new active is already
  // primed so this usually no-ops; it stays the safety net for the fallback
  // path. A fresh active src cancels the prior poll, so only one runs at a time.
  $effect(() => {
    const src = activeSrc
    stopReadyPoll()
    if (!src) return
    readyPoll = setInterval(() => {
      // Terminal: playback started (pendingPlay cleared) or we left playback.
      if (mode !== 'playback' || !pendingPlay) {
        stopReadyPoll()
        return
      }
      // A not-yet-bound element just skips this tick (it binds shortly); do
      // NOT stop the poll, or initial entry would strand before the bind.
      const el = activeEl
      if (el && el.readyState >= el.HAVE_CURRENT_DATA) markReadyIfPlayable()
    }, 250)
    return () => stopReadyPoll()
  })

  // Clip/settle-scoped preview readiness poll, rekeyed on the active clip src +
  // settled position + mode. A closed-hour preview frame is expected to paint
  // (previewSrc set, not yet ready, not the open webp tail, while scrubbing),
  // but the seek can be stranded — duration arrived late, or a currentTime
  // assignment didn't paint. Retry primePreview + seekPreview (existing calls,
  // existing guards) on a bounded schedule until 'seeked' flips previewReady,
  // which the next tick observes and stops on. previewReady is set ONLY by
  // 'seeked' (real paint), never here. A settle into playback flips mode and
  // re-runs this effect, stopping the poll. No-op when the frame paints first
  // try — the first tick's terminal check stops it almost immediately.
  $effect(() => {
    void activeClip?.src
    void position
    const m = mode
    stopPreviewReadyPoll()
    if (!(previewSrc !== '' && !previewReady && !isOpenTail && m === 'scrubbing')) return
    let attempts = 0
    previewReadyPoll = setInterval(() => {
      if (
        previewReady ||
        previewSrc === '' ||
        isOpenTail ||
        mode !== 'scrubbing' ||
        attempts >= 8
      ) {
        stopPreviewReadyPoll()
        return
      }
      attempts++
      primePreview()
      seekPreview()
    }, 300)
    return () => stopPreviewReadyPoll()
  })

  // Ensure full-res plays at wall-clock T on the ACTIVE buffer: reuse the
  // loaded chunk when T is inside it (cheap currentTime seek), else load a
  // fresh <=10-min chunk starting at T. Either way switch to playback, start
  // playing, and clear the idle buffer — any settle invalidates a prefetched
  // continuation, so the next timeupdate re-prefetches afresh.
  function ensurePlaybackAt(t: number) {
    // Preview-only device: never touch full-res — the scrub/preview path is
    // the whole experience here.
    if (previewOnly) return
    // Settle landed in a gap: snap forward to the first recorded second so the
    // chunk starts on real footage and the wall-clock anchor holds (otherwise
    // the flag clock lags by the gap size). Move the playhead too — the flag /
    // cursor jump to the first existing record is the point of the fix, and
    // setPosition re-centres the viewport on it in the same mutation while
    // follow is on. Keep the load-time poster on the snapped footage rather
    // than the gap. Runs for every caller (handleScrubEnd, togglePlay,
    // entry/deep-link autoplay) by living here.
    const snapped = snapToCoverage(t)
    if (snapped !== t) {
      t = snapped
      setPosition(snapped)
      pickClip(snapped)
      seekPreview()
    }
    chunkEnded = false
    const el = activeEl
    const aStart = activeStart
    const aEnd = activeEnd
    const within = aStart !== null && aEnd !== null && activeSrc !== '' && t >= aStart && t <= aEnd
    // The element's current wall-clock playback point (null until bound). A
    // large jump from it on a PAUSED HLS element often isn't applied before
    // play() resumes from the old currentTime, so restrict the cheap in-buffer
    // seek to small jumps near it; bigger jumps fall through to a fresh chunk.
    const cur = el && aStart !== null ? aStart! + el.currentTime : null
    const nearCurrent = cur !== null && Math.abs(t - cur) <= CHUNK_REJOIN_SLACK
    // A degraded (video-only) active chunk must not be reused on a settle —
    // re-attach master so audio is retried at the new position; on the bad-
    // boundary second it will simply degrade again, elsewhere audio returns.
    if (within && el && nearCurrent && !activeDegraded) {
      lastSeekAt = performance.now()
      el.currentTime = t - aStart!
      void el.play().catch(() => {})
    } else {
      // Chunk bounds MUST be integer unix seconds: the BFF's VOD route slots
      // are integer-only (validUnixSeconds rejects a fractional path as
      // invalid_range). A VHS rush / full-res timeupdate leaves t fractional,
      // so floor it. Clamp the start to at most end-1 so a settle at the live
      // edge can't build an empty (cs==ce) range, and not below the window
      // start. ce derives from the now-integer cs, so activeStart stays integer
      // and the position->currentTime mapping does not drift.
      const end = Math.floor(liveEdge)
      const cs = Math.max(Math.floor(playbackFloor), Math.min(Math.floor(t), end - 1))
      const ce = Math.min(cs + CHUNK_SECONDS, end)
      showFullRes = false // hold the preview frame as a poster until first frame
      playerError = false
      pendingPlay = true // play() deferred to canplay (async HlsVideo attach)
      lastSeekAt = performance.now()
      setActiveChunk(cs, ce, timelineMasterURL(camId, cs, ce))
    }
    clearIdle()
    mode = 'playback'
  }

  // Scrubber drag started: drop to the preview layer and pause full-res.
  function handleScrubStart() {
    // Prime from this user gesture — guarantees decoding on iOS/Low-Power
    // where the on-load prime's autoplay was blocked. No-op once primed.
    primePreview()
    scrubActive = true
    mode = 'scrubbing'
    chunkEnded = false
    const el = activeEl
    if (el && !el.paused) el.pause()
  }

  // Continuous drag value. Preview-only — cheap, no throttle, no full-res.
  // pickClip swaps the preview file only on a clip-boundary crossing;
  // otherwise it is a no-op comparison.
  function handleSeek(t: number) {
    // The drag still SEEKS — it moves the playhead, and with follow on the
    // viewport is re-centred on it inside setPosition, which is what makes the
    // filmstrip pan under a fixed handle. So the playhead is bounded by what's
    // PLAYABLE, never by the viewport (which follows it).
    setPosition(clampPlayhead(t, playbackFloor, liveEdge))
    chunkEnded = false
    pickClip(position)
    if (isOpenTail) {
      // Open live tail (past the last clip): the mp4 preview does not exist, so
      // scrub by webp frame. loadFramesTail is lazy-once; pickFrame is cheap and
      // network-free (the browser fetches+caches each nearest webp on demand).
      void loadFramesTail()
      pickFrame()
    } else if (mode === 'scrubbing') {
      seekPreview()
    }
  }

  // Drag released: settle to full-res at the landed position. The full-res
  // settle is gated off inside ensurePlaybackAt when previewOnly is true, so
  // here we only clear the active-drag flag (re-showing the hint).
  function handleScrubEnd() {
    scrubActive = false
    ensurePlaybackAt(position)
  }

  // Zoom: the scrubber requests an absolute target span (pinch distance ratio
  // or wheel step) plus WHERE the gesture happened, as a position across the
  // drawn window. clampViewSpan bounds and rounds the span; the MODE decides
  // what the zoom holds in place, which is the whole reason the scrubber
  // reports the anchor rather than applying it.
  //
  // follow: the playhead is the centre of the track and the zoom is about it,
  // exactly as before — the reported anchor is ignored outright.
  // fixed: the track is stationary, so the zoom holds the time under the
  // pointer. anchorFraction is null when the gesture expressed no anchor
  // (pinch, which this change does not touch), and then the span simply
  // changes without moving the window.
  function onZoom(targetSpan: number, anchorFraction: number | null) {
    const span = clampViewSpan(targetSpan)
    if (follow || anchorFraction === null) {
      setViewSpan(span)
      return
    }
    setViewSpanAnchored(span, anchorFraction)
  }

  // Two-finger gesture, fixed mode only. The scrubber has already resolved the
  // separation into a target span and the midpoint into a fraction; here the
  // span is bounded and the whole viewport recomputed from the anchor. Follow
  // mode never reaches this — its pinch reports no anchor and goes through
  // onZoom above, exactly as it always has.
  function onPinch(targetSpan: number, anchorTime: number, midFraction: number) {
    setViewSpanPinched(clampViewSpan(targetSpan), anchorTime, midFraction)
  }

  // Play button. From scrubbing, or when the playhead is outside the loaded
  // chunk, settle+play; otherwise toggle the loaded chunk.
  function togglePlay() {
    // Preview-only device: no full-res to play (the button is hidden too).
    if (previewOnly) return
    const el = activeEl
    const aStart = activeStart
    const aEnd = activeEnd
    const within =
      aStart !== null && aEnd !== null && activeSrc !== '' && position >= aStart && position <= aEnd
    if (mode === 'scrubbing' || !within) {
      ensurePlaybackAt(position)
      return
    }
    if (!el) return
    if (el.paused) void el.play().catch(() => {})
    else el.pause()
  }

  // Apply the current playback rate to the active buffer's element. A fresh
  // chunk src resets an element's rate to 1, so this is re-applied on confirmed
  // start (onPlaying) and after each buffer swap, not just on a speed change.
  function applyPlaybackRate() {
    const el = activeEl
    if (el) el.playbackRate = playbackRate
  }

  // Speed dropdown (opens upward — the controls sit below the video, above the
  // scrubber). Mirrors the AppHeader group-filter popover idiom: a bound
  // container, an open flag, close on Escape / outside pointerdown / selection.
  let speedOpen = $state(false)
  let speedMenuEl: HTMLDivElement | undefined = $state()
  function selectSpeed(s: number) {
    playbackRate = s
    applyPlaybackRate()
    speedOpen = false
  }
  $effect(() => {
    if (!speedOpen) return
    function onKey(e: KeyboardEvent) {
      if (e.key === 'Escape') speedOpen = false
    }
    function onPointer(e: PointerEvent) {
      const t = e.target as Node | null
      if (speedMenuEl && t && !speedMenuEl.contains(t)) speedOpen = false
    }
    window.addEventListener('keydown', onKey)
    window.addEventListener('pointerdown', onPointer)
    return () => {
      window.removeEventListener('keydown', onKey)
      window.removeEventListener('pointerdown', onPointer)
    }
  })

  // Rush multiplier for an elapsed hold: walk the ramp thresholds and return
  // the highest rate whose hold-time has been reached.
  function rushRateFor(heldMs: number): number {
    let rate = RUSH_RATES[0] ?? 60
    for (let i = 0; i < RUSH_RAMP_MS.length; i++) {
      const threshold = RUSH_RAMP_MS[i]
      const r = RUSH_RATES[i]
      if (threshold !== undefined && r !== undefined && heldMs >= threshold) rate = r
    }
    return rate
  }

  // Press-and-hold start. dir: 1 fast-forward, -1 rewind. No-op on preview-only
  // (the buttons are hidden there too) and when a rush is already running.
  // handleScrubStart does the exact rush setup: prime the preview from this
  // user gesture, set scrubActive + mode='scrubbing', and pause full-res. Then
  // run the rAF rush loop.
  function vhsStart(dir: 1 | -1) {
    if (previewOnly || vhsDir !== 0) return
    vhsHoldStart = vhsLastTs = performance.now()
    vhsDir = dir
    handleScrubStart()
    vhsRaf = requestAnimationFrame(vhsTick)
  }

  // The rush loop. Advance the playhead by dir * rate * dt of wall-clock and
  // feed it to the SAME preview path the scrubber's onSeek uses, so closed-hour
  // clip seeks and open-tail webp frames behave identically to a manual drag.
  // dt is capped at 0.1s so a stutter / backgrounded tab can't fling the
  // playhead. Auto-stops when the playhead reaches the PLAYABLE edge it travels
  // toward (the centred viewport keeps following, so the empty span past the
  // edge just shows the dim band).
  function vhsTick(ts: number) {
    if (vhsDir === 0) return
    const dt = Math.min(Math.max((ts - vhsLastTs) / 1000, 0), 0.1)
    vhsLastTs = ts
    const rate = rushRateFor(ts - vhsHoldStart)
    vhsRate = rate
    const next = position + vhsDir * rate * dt
    if (vhsDir > 0 ? next >= liveEdge : next <= playbackFloor) {
      handleSeek(vhsDir > 0 ? liveEdge : playbackFloor)
      vhsStop()
      return
    }
    handleSeek(next)
    vhsRaf = requestAnimationFrame(vhsTick)
  }

  // Release. Idempotent (pointerup + pointercancel, or the auto-stop, can each
  // fire). Cancel the loop and settle full-res at the landed position via
  // handleScrubEnd (gated off internally when previewOnly).
  function vhsStop() {
    if (vhsDir === 0) return
    if (vhsRaf !== null) {
      cancelAnimationFrame(vhsRaf)
      vhsRaf = null
    }
    vhsDir = 0
    vhsRate = 0
    // Behave exactly like a manual scrub-release: leave the preview <video> on
    // its loaded clip and just seek it to the landed position, then settle.
    // (Nulling the clip / resetting duration / reloading the element only broke
    // it — seekPreview early-returns while previewDuration is 0, so the landed
    // frame never paints.) The one extra step over scrub-release: a rush almost
    // always has a coalesced seekPreview rAF in flight at release, and its guard
    // (previewSeekRaf !== null) would swallow the final landed seek — so clear
    // the stale guard first. On closed hours this seeks the already-loaded
    // <video>; on the open tail handleSeek drives the webp <img>.
    if (previewSeekRaf !== null) {
      cancelAnimationFrame(previewSeekRaf)
      previewSeekRaf = null
    }
    handleSeek(position)
    handleScrubEnd()
  }

  // Pointer-capture release helper for the VHS buttons: a finger that slid off
  // the button still counts as held (capture), so release only when this
  // element actually holds the capture (pointercancel may have released it).
  function vhsRelease(btn: HTMLButtonElement, pointerId: number) {
    if (btn.hasPointerCapture(pointerId)) btn.releasePointerCapture(pointerId)
    vhsStop()
  }

  // Mute toggle for the full-res element. The first tap is a user gesture, so
  // subsequent programmatic play() on the same element stays allowed. Flip the
  // state only — the muted={fullResMuted} prop on HlsVideo drives el.muted
  // reactively, so the choice survives chunk swaps / re-renders (an imperative
  // el.muted write here would be reset by the next render's prop).
  function toggleFullResMute() {
    fullResMuted = !fullResMuted
  }

  // Fullscreen — mirrors cam/[id]/+page.svelte. Desktop/iPad take the whole
  // .frame composite fullscreen (Element.requestFullscreen). iPhone has no
  // Element.requestFullscreen, so it falls back to webkitEnterFullscreen on the
  // currently-visible <video>: the full-res buffer when it is the shown layer,
  // else the preview element.
  let frameEl: HTMLDivElement | undefined = $state()
  let isFullscreen = $state(false)

  function toggleFullscreen() {
    if (document.fullscreenElement) {
      document.exitFullscreen().catch(() => {})
      return
    }
    if (frameEl?.requestFullscreen) {
      frameEl.requestFullscreen().catch(() => {})
      return
    }
    // iPhone path: native fullscreen on the visible video element.
    const vis = mode === 'playback' && showFullRes && activeSrc ? activeEl : previewEl
    if (vis && 'webkitEnterFullscreen' in vis) {
      ;(vis as HTMLVideoElement & { webkitEnterFullscreen(): void }).webkitEnterFullscreen()
    }
  }

  // Track fullscreen state so the button swaps the enter↔exit glyph. iPhone's
  // native single-video fullscreen may not fire these events (and loses the
  // scrubber/overlays) — the same known limitation as the focus view, and the
  // only iOS path available.
  $effect(() => {
    const onFsChange = () => {
      isFullscreen = !!(
        document.fullscreenElement ??
        (document as Document & { webkitFullscreenElement?: Element }).webkitFullscreenElement
      )
    }
    document.addEventListener('fullscreenchange', onFsChange)
    document.addEventListener('webkitfullscreenchange', onFsChange)
    return () => {
      document.removeEventListener('fullscreenchange', onFsChange)
      document.removeEventListener('webkitfullscreenchange', onFsChange)
    }
  })

  // Decode-class HlsVideo errors: the player CAN fetch the stream but cannot
  // decode it. Native path emits media_error_<code> (MEDIA_ERR_DECODE=3,
  // MEDIA_ERR_SRC_NOT_SUPPORTED=4); the hls.js path emits its two
  // incompatible-codecs details. This set matches EXACTLY what HlsVideo
  // forwards as decode-class — transient hls.js media errors (bufferAppendError
  // / fragParsingError) are swallowed by recoverMediaError and never arrive
  // here. Everything else (empty range / 400 / manifest or level load failure)
  // is a genuine "no recording" error.
  const DECODE_ERROR_STRINGS = new Set<string>([
    'media_error_3',
    'media_error_4',
    'manifestIncompatibleCodecsError',
    'bufferIncompatibleCodecsError'
  ])

  // Drop into preview-only: tear down both full-res buffers, cache the device
  // as undecodable for this camera, and stay on the scrub/preview layer.
  function enterPreviewOnly() {
    previewOnly = true
    decodeCache.set(camId, false)
    stopReadyPoll()
    const a = videoElA
    const b = videoElB
    if (a && !a.paused) a.pause()
    if (b && !b.paused) b.pause()
    active = 'a'
    srcA = ''
    srcB = ''
    startA = null
    startB = null
    endA = null
    endB = null
    degradedA = false
    degradedB = false
    idlePrimed = false
    showFullRes = false
    pendingPlay = false
    chunkEnded = false
    playerError = false
    paused = true
    mode = 'scrubbing'
  }

  // Audio-flap 503 fallback. Reload the ACTIVE buffer's chunk over the SAME
  // [start,end] range from the audio-less index-v1.m3u8 rendition. Some cam5
  // (H.264-record) windows have adjacent recording segments whose audio-track
  // counts differ (the camera's RTSP audio flapping); Frigate's nginx-vod
  // refuses to splice them and 503s the combined video+audio segment while the
  // video-only rendition serves 200. We swap the src in place (NOT through
  // setActiveChunk — keep start/end as-is) and mark the buffer degraded so the
  // no-audio glyph shows. The reload restarts the chunk from its start; for a
  // seek-triggered 503 that start is ≈ the seeked second, so the jump is
  // negligible and currentTime is intentionally not preserved. The activeSrc-
  // keyed readyPoll re-keys on the new src and drives play() once it is ready;
  // onPlaying clears pendingPlay. Returns false when there is no active range
  // to reload — the caller takes the terminal "No recording" path instead.
  function degradeActiveToVideoOnly(): boolean {
    const aStart = activeStart
    const aEnd = activeEnd
    if (aStart === null || aEnd === null) return false
    const url = timelineVideoOnlyURL(camId, aStart, aEnd)
    if (active === 'a') {
      srcA = url
      degradedA = true
    } else {
      srcB = url
      degradedB = true
    }
    showFullRes = false // hold the preview poster over the reload
    playerError = false
    pendingPlay = true
    lastSeekAt = performance.now()
    return true
  }

  // Backstop for the proactive codec gate: classify an HlsVideo error and
  // either fall back to preview-only (decode-class), retry the active chunk
  // video-only (audio-flap 503), or surface the genuine "No recording" overlay.
  function handleFullResError(msg: string) {
    if (DECODE_ERROR_STRINGS.has(msg)) {
      // Not yet proven decodable → a genuine undecodable codec (e.g. H.265 with
      // no hardware decoder): drop to preview-only and stay there.
      if (!decodeProven) {
        enterPreviewOnly()
        return
      }
      // decodeProven here. Safari surfaces an audio-flap 503 on the v+a segment
      // as a DECODE-class MediaError (media_error_4 / DEMUXER_COULD_NOT_PARSE),
      // not a network error, so it lands in this branch too. pendingPlay tells
      // the two apart: true means the chunk never STARTED — the seek→seg-1
      // splice 503 masquerading as a decode error, recoverable video-only, so
      // fall through to the fallback ladder. false means a transient decode
      // blip DURING active playback (the case this proven-decode ignore exists
      // for) — keep ignoring it.
      if (!pendingPlay) return
      // else fall through to the single-sourced fallback ladder below.
    }
    // Fallback ladder, reached by any non-decode error OR a pre-start
    // decodeProven decode-class error. On a chunk that still carries audio, try
    // the same range video-only first. A genuine empty range fails this too and
    // lands on the terminal branch below; an audio-flap 503 recovers here.
    if (!activeDegraded && activeStart !== null && activeEnd !== null) {
      if (degradeActiveToVideoOnly()) return
    }
    // Terminal: already video-only, or no range to reload. Surface "No
    // recording" and clear the playback hang — pendingPlay + the readiness
    // poll would otherwise keep retrying play() against a dead chunk forever.
    playerError = true
    pendingPlay = false
    stopReadyPoll()
  }

  // Smart back: the timeline is entered from focus, grid, AND events, so return
  // to wherever the user came from when there is in-app history; fall back to
  // this camera's focus screen on a fresh/direct load (no history to pop).
  function goBack() {
    if (typeof history !== 'undefined' && history.length > 1) history.back()
    else goto(`/cam/${camId}`)
  }
</script>

<div class="page">
  <header class="bar">
    <button type="button" class="back" onclick={goBack} aria-label={ui.backLabel}>
      <Icon name="back" size={20} />
      <span>{ui.backLabel}</span>
    </button>
    <span class="title">{camera?.name ?? camId}</span>
  </header>

  <div class="frame" bind:this={frameEl}>
    <!-- Picture zoom (wheel / two-finger pinch / double-tap reset), same as the
         live focus view. Only the media layers scale; the overlays below stay
         fixed siblings outside the pane. resetKey={camId} resets zoom on camera
         switch, but NOT on scrub↔playback within one camera. -->
    <ZoomPane resetKey={camId}>
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

      <div class="layer fullres" style:opacity={active === 'a' && fullResVisible ? 1 : 0}>
        <HlsVideo
          bind:video={videoElA}
          src={srcA}
          muted={fullResMuted}
          onError={handleFullResError}
        />
      </div>
      <div class="layer fullres" style:opacity={active === 'b' && fullResVisible ? 1 : 0}>
        <HlsVideo
          bind:video={videoElB}
          src={srcB}
          muted={fullResMuted}
          onError={handleFullResError}
        />
      </div>
    </ZoomPane>

    {#if chunkEnded}
      <div class="frame-hint">
        <Mono size={11} color="rgba(255,255,255,0.85)">{ui.timelineLiveEnd}</Mono>
      </div>
    {/if}

    {#if previewOnly && !scrubActive}
      <div class="frame-codec">
        <Mono size={12} color="rgba(255,255,255,0.82)" wrap>{ui.timelineCodecUnsupported}</Mono>
      </div>
    {/if}

    {#if playerError}
      <div class="frame-overlay">
        <Mono size={12} color="rgba(255,255,255,0.82)" wrap>{ui.timelineNoRecording}</Mono>
      </div>
    {/if}

    {#if vhsDir !== 0}
      <!-- Large side indicator while rushing: anchored to the side the playhead
           is travelling (rewind left, fast-forward right), vertically centred,
           showing the direction glyph and the live rate. -->
      <div class="rush-osd {vhsDir > 0 ? 'ff' : 'rew'}" aria-hidden="true">
        <Icon name={vhsDir > 0 ? 'fastForward' : 'rewind'} size={36} />
        <Mono size={15} weight={600} color="rgba(255,255,255,0.95)">{vhsRate}×</Mono>
      </div>
    {/if}
  </div>

  <div class="controls">
    {#if !previewOnly}
      <!-- Centred transport cluster: the VHS rewind/fast-forward pair (C-ii)
           flanks play/pause inside .transport. -->
      <div class="transport">
        <button
          type="button"
          class="livebtn vhs"
          aria-label={ui.timelineRewind}
          onpointerdown={(e) => {
            e.currentTarget.setPointerCapture(e.pointerId)
            vhsStart(-1)
          }}
          onpointerup={(e) => vhsRelease(e.currentTarget, e.pointerId)}
          onpointercancel={(e) => vhsRelease(e.currentTarget, e.pointerId)}
          onkeydown={(e) => {
            if ((e.key === 'Enter' || e.key === ' ') && !e.repeat) {
              e.preventDefault()
              vhsStart(-1)
            }
          }}
          onkeyup={(e) => {
            if (e.key === 'Enter' || e.key === ' ') {
              e.preventDefault()
              vhsStop()
            }
          }}
        >
          <Icon name="rewind" size={18} />
        </button>
        <button
          type="button"
          class="livebtn playpause"
          onclick={togglePlay}
          aria-label={paused ? ui.timelinePlay : ui.timelinePause}
        >
          <Icon name={paused ? 'play' : 'pause'} size={20} />
        </button>
        <button
          type="button"
          class="livebtn vhs"
          aria-label={ui.timelineFastForward}
          onpointerdown={(e) => {
            e.currentTarget.setPointerCapture(e.pointerId)
            vhsStart(1)
          }}
          onpointerup={(e) => vhsRelease(e.currentTarget, e.pointerId)}
          onpointercancel={(e) => vhsRelease(e.currentTarget, e.pointerId)}
          onkeydown={(e) => {
            if ((e.key === 'Enter' || e.key === ' ') && !e.repeat) {
              e.preventDefault()
              vhsStart(1)
            }
          }}
          onkeyup={(e) => {
            if (e.key === 'Enter' || e.key === ' ') {
              e.preventDefault()
              vhsStop()
            }
          }}
        >
          <Icon name="fastForward" size={18} />
        </button>
      </div>
    {/if}
    <div class="meta">
      {#if !previewOnly}
        <div class="speed-drop" bind:this={speedMenuEl}>
          <button
            type="button"
            class="livebtn speed"
            class:open={speedOpen}
            aria-haspopup="listbox"
            aria-expanded={speedOpen}
            aria-label={ui.timelineSpeed}
            onclick={() => (speedOpen = !speedOpen)}
          >
            <Mono size={13} weight={500} color="inherit">{playbackRate}×</Mono>
            <Icon name="chevDown" size={14} />
          </button>
          {#if speedOpen}
            <ul class="speed-menu" role="listbox">
              {#each SPEED_STEPS as s (s)}
                <li>
                  <button
                    type="button"
                    class="speed-opt"
                    class:on={playbackRate === s}
                    role="option"
                    aria-selected={playbackRate === s}
                    onclick={() => selectSpeed(s)}
                  >
                    <span class="check" aria-hidden="true">
                      {#if playbackRate === s}<Icon name="check" size={15} />{/if}
                    </span>
                    <Mono size={13} weight={500} color="inherit">{s}×</Mono>
                  </button>
                </li>
              {/each}
            </ul>
          {/if}
        </div>
        <!-- Mute toggle. When the active chunk is video-only (audio-flap 503
             fallback) it carries no audio, so the button shows the struck-
             speaker icon and goes disabled — same button, same place, only the
             icon + disabled state swap, so the row never reflows on toggle. -->
        <button
          type="button"
          class="livebtn"
          onclick={toggleFullResMute}
          disabled={activeDegraded}
          aria-label={activeDegraded
            ? ui.timelineAudioUnavailable
            : fullResMuted
              ? ui.timelineUnmute
              : ui.timelineMute}
          title={activeDegraded
            ? ui.timelineAudioUnavailable
            : fullResMuted
              ? ui.timelineUnmute
              : ui.timelineMute}
        >
          <Icon name={fullResMuted ? 'mute' : 'unmute'} size={20} />
        </button>
      {/if}
      <!-- Fullscreen is available even in preview-only mode, so it sits OUTSIDE
           the !previewOnly guard as the last meta button. -->
      <button type="button" class="livebtn" onclick={toggleFullscreen} aria-label="Fullscreen">
        <Icon name={isFullscreen ? 'exitFull' : 'fullscreen'} size={20} />
      </button>
    </div>
  </div>

  <div class="scrub">
    <!-- Mode switch, directly above the track it governs. Trailing-aligned in
         its own row rather than folded into .controls, which would push the
         transport cluster off centre. It is nowhere near the LIVE capsule,
         which lives INSIDE the track at the live-edge fraction — a different
         concept in a different place. Visual treatment is a first pass. -->
    <div class="mode-row" role="group" aria-label={ui.timelineModeAria}>
      <Segmented
        value={prefsStore.timelineMode}
        options={timelineModeOptions}
        onChange={(v) => prefsStore.setTimelineMode(v)}
      />
    </div>

    <TimelineScrubber
      {windowStart}
      {windowEnd}
      {position}
      mode={prefsStore.timelineMode}
      {playbackFloor}
      {liveEdge}
      {coverage}
      {reviews}
      {audioEvents}
      onSeek={handleSeek}
      onScrubStart={handleScrubStart}
      onScrubEnd={handleScrubEnd}
      {onZoom}
      onPan={panViewport}
      {onPinch}
      onGoLive={() => goto(`/cam/${camId}`)}
    />

    {#if timelineStore.loading}
      <div class="scrub-note loading">
        <Mono size={11} color="var(--text-3)">{ui.timelineLoading}</Mono>
      </div>
    {:else if timelineStore.error}
      <div class="scrub-note error">
        <Mono size={11} color="var(--warn)" wrap>{ui.timelineSummaryError}</Mono>
      </div>
    {:else if timelineStore.hours.length === 0}
      <div class="scrub-note">
        <Mono size={11} color="var(--text-3)" wrap>{ui.timelineEmpty}</Mono>
      </div>
    {/if}

    <!-- Lane colour legend. Desktop only (hidden < 900px); a quiet inline row of
         swatch+label pairs so the scrubber's lane colours are readable. The
         swatch colours mirror TimelineScrubber's lanes exactly. -->
    <div class="legend" aria-hidden="true">
      <span class="legend-item">
        <span class="swatch alert"></span>
        <span class="legend-label">{ui.legendAlert}</span>
      </span>
      <span class="legend-item">
        <span class="swatch detection"></span>
        <span class="legend-label">{ui.legendDetection}</span>
      </span>
      <span class="legend-item">
        <span class="swatch audio"></span>
        <span class="legend-label">{ui.legendAudio}</span>
      </span>
      <span class="legend-item">
        <span class="swatch recorded"></span>
        <span class="legend-label">{ui.legendRecorded}</span>
      </span>
    </div>
  </div>
</div>

<style>
  .page {
    display: flex;
    flex-direction: column;
    gap: 14px;
    /* Mobile (< 900px) renders the fixed bottom tab bar, which would overlap
       the scrubber + its note. Reserve safe-area + 96px at the bottom for it,
       mirroring MobileFocus's .mf-body clearance. Desktop resets this below. */
    padding: calc(env(safe-area-inset-top, 0px) + 12px) 18px
      calc(env(safe-area-inset-bottom, 0px) + 96px);
    background: var(--bg);
    /* --screen-h, not 100dvh: body reserves the bottom safe-area inset out of
       its own content box, so a full 100dvh here overflows it by that inset and
       the screen scrolls a little with nothing to scroll to. See app.css. */
    min-height: var(--screen-h);
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
  /* Fullscreen: let the composite fill the viewport. The media layers already
     object-fit:contain (preview/frames + HlsVideo), so they letterbox cleanly. */
  .frame:fullscreen,
  .frame:-webkit-full-screen {
    width: 100%;
    height: 100%;
    max-height: none;
    aspect-ratio: auto;
    border-radius: 0;
  }
  /* Desktop (matches the layout's 900px isDesktop breakpoint): the whole player
     fits exactly one viewport with no scroll. The column is pinned to one
     screen with overflow hidden; header, controls and scrubber keep their natural
     height (flex:0 0 auto) and the video frame grows to fill the leftover
     space. The frame drops its fixed 16:9 aspect-ratio so it takes whatever
     height is left — the media layers already object-fit:contain, so the
     picture letterboxes cleanly. */
  @media (min-width: 900px) {
    /* No bottom tab bar on desktop — drop the mobile tab-bar clearance, pin the
       column to the viewport, and clip any overflow so there is no page scroll. */
    .page {
      /* Subtract the sticky AppHeader so the column fits under it without
         overflowing. 64px is .dk-top's desktop height; the fallback covers
         desktop, where --app-header-h is not published (only the mobile header
         publishes it via ResizeObserver). */
      height: calc(var(--screen-h) - var(--app-header-h, 64px));
      min-height: 0;
      overflow: hidden;
      padding-bottom: calc(env(safe-area-inset-bottom, 0px) + 24px);
    }
    .bar,
    .controls,
    .scrub {
      flex: 0 0 auto;
    }
    .frame {
      flex: 1 1 auto;
      min-height: 0;
      height: auto;
      width: 100%;
      aspect-ratio: auto;
      align-self: stretch;
    }
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
  /* Preview-only (undecodable codec) hint. A centred pill — distinct from the
     bottom live-edge .frame-hint and not a full dim like .frame-overlay, so the
     preview frame underneath stays readable while scrubbing. Never captures
     pointer events (the scrubber/controls underneath must stay reachable). */
  .frame-codec {
    position: absolute;
    left: 50%;
    top: 50%;
    transform: translate(-50%, -50%);
    max-width: 80%;
    padding: 8px 14px;
    border-radius: var(--r-sm);
    text-align: center;
    background: rgba(0, 0, 0, 0.55);
    backdrop-filter: blur(8px);
    -webkit-backdrop-filter: blur(8px);
    pointer-events: none;
  }
  /* Large rush indicator: anchored to the side the playhead is travelling
     (rewind left, fast-forward right), vertically centred. Direction glyph
     above the live rush rate. Sits above the media layers but below the
     controls (it lives inside .frame). Never captures pointer events. */
  .rush-osd {
    position: absolute;
    top: 50%;
    transform: translateY(-50%);
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 4px;
    padding: 14px 16px;
    border-radius: var(--r);
    background: rgba(0, 0, 0, 0.5);
    backdrop-filter: blur(8px);
    -webkit-backdrop-filter: blur(8px);
    color: rgba(255, 255, 255, 0.95);
    pointer-events: none;
  }
  .rush-osd.rew {
    left: 16px;
  }
  .rush-osd.ff {
    right: 16px;
  }

  /* One centred bar: the transport cluster, a gap, then the meta cluster, both
     centred as a single group (justify-content:center). A max-width keeps the
     group from stretching across a wide desktop; margin:0 auto centres the
     capped bar in the column. */
  .controls {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: center;
    column-gap: 14px;
    row-gap: 10px;
    width: 100%;
    max-width: 720px;
    margin: 0 auto;
  }
  /* Centred transport cluster: rewind, play/pause, fast-forward. Tight internal
     gap so the three read as one unit. */
  .transport {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 8px;
    flex: 0 1 auto;
  }
  /* Meta cluster: speed chip, mute, fullscreen. Sized to content so it reads as
     part of the one centred group. The clock now lives in the scrubber's time
     flag, not here. */
  .meta {
    flex: 0 0 auto;
    display: flex;
    align-items: center;
    gap: 14px;
  }
  /* Five transport buttons plus the meta cluster genuinely overflow a phone-
     width bar, so below this width the meta cluster wraps to its own centred
     second row (justify-content:center on .controls keeps each row centred).
     Desktop keeps the single centred row. */
  @media (max-width: 540px) {
    .controls {
      column-gap: 10px;
    }
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
  /* Don't run the press dip on a disabled control (the degraded mute button). */
  .livebtn:active:not(:disabled) {
    transform: translateY(1px);
  }
  /* Disabled control (the mute button while the active chunk is video-only and
     carries no audio): dimmed and non-interactive, but still the same 46px
     button in place — the icon swaps to the struck speaker with no reflow. */
  .livebtn:disabled {
    color: var(--text-3);
    cursor: default;
  }
  /* VHS rewind/fast-forward buttons sit a touch smaller than play/pause — they
     flank it. */
  .livebtn.vhs {
    width: 40px;
    height: 40px;
  }
  /* Disable the tap-highlight / text-select on a held VHS button so a long
     touch-hold reads as a transport press, not a selection. */
  .livebtn.vhs {
    touch-action: none;
    -webkit-user-select: none;
    user-select: none;
    -webkit-touch-callout: none;
  }
  /* Speed dropdown trigger: width follows the mono label (e.g. "0.5×") + chev
     instead of a fixed square. */
  .livebtn.speed {
    width: auto;
    min-width: 46px;
    padding: 0 12px;
    display: inline-flex;
    align-items: center;
    gap: 6px;
    color: var(--text);
  }
  .livebtn.speed.open {
    background: var(--accent-soft);
    border-color: transparent;
    color: var(--accent-ink);
  }
  /* Speed popover: anchored to the trigger and grown UPWARD (controls sit below
     the video, above the scrubber). Raised z-index + own stacking context so it
     is not painted under the later .scrub sibling. */
  .speed-drop {
    position: relative;
    z-index: 40;
  }
  .speed-menu {
    list-style: none;
    margin: 0;
    position: absolute;
    bottom: calc(100% + 8px);
    left: 0;
    z-index: 40;
    min-width: 104px;
    background: var(--elev);
    border: 1px solid var(--border-strong);
    border-radius: var(--r-sm);
    box-shadow: var(--shadow);
    padding: 6px;
  }
  .speed-opt {
    display: flex;
    align-items: center;
    gap: 8px;
    width: 100%;
    padding: 8px 10px;
    border: none;
    background: transparent;
    border-radius: 8px;
    color: var(--text);
    font-family: inherit;
    text-align: left;
    cursor: pointer;
  }
  .speed-opt:active {
    background: var(--surface-2);
  }
  .speed-opt.on {
    background: var(--accent-soft);
    color: var(--accent-ink);
  }
  .speed-opt .check {
    width: 16px;
    flex: 0 0 16px;
    color: var(--accent-ink);
    display: inline-flex;
  }
  .scrub {
    display: flex;
    flex-direction: column;
    gap: 8px;
  }
  /* Mode switch row. Trailing-aligned so it sits over the newest end of the
     track, away from the earliest-time edge the eye reads first. .scrub is a
     flex column with its own gap, so no margin is needed here. */
  .mode-row {
    display: flex;
    justify-content: flex-end;
  }
  .scrub-note {
    min-height: 16px;
  }
  /* Lane colour legend — desktop only. Hidden below the 900px breakpoint where
     the player fits exactly one viewport and any extra row would force a scroll;
     it's revealed in the desktop @media below. A quiet inline row. */
  .legend {
    display: none;
    flex-wrap: wrap;
    align-items: center;
    gap: 6px 14px;
  }
  .legend-item {
    display: inline-flex;
    align-items: center;
    gap: 6px;
  }
  .legend-label {
    /* Normal sans (NOT mono), small and quiet. */
    font-size: 11px;
    color: var(--text-3);
  }
  /* Tiny rounded chips matching the lane capsule shape (border-radius:999px). */
  .swatch {
    width: 14px;
    height: 6px;
    border-radius: 999px;
    flex: 0 0 auto;
  }
  /* Each swatch mirrors a TimelineScrubber lane colour exactly. */
  .swatch.alert {
    background: var(--warn);
  }
  .swatch.detection {
    background: var(--accent);
    opacity: 0.7;
  }
  .swatch.audio {
    /* SYNC: duplicates TimelineScrubber's .audio one-off violet literal (no
       app.css token on this branch) — keep both in step. */
    background: oklch(0.72 0.14 300);
  }
  .swatch.recorded {
    background: var(--border-strong);
  }
  @media (min-width: 900px) {
    .legend {
      display: flex;
    }
  }
</style>
