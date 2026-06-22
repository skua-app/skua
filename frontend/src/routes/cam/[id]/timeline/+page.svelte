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
  import { timelineStore } from '$lib/stores/timeline.svelte'
  import {
    timelineMasterURL,
    fetchPreviewClips,
    fetchPreviewFrameList,
    fetchRecordingCodecs,
    fetchReview,
    fetchAudioEvents
  } from '$lib/api'
  import type { PreviewClip, PreviewFrame, ReviewSegment, AudioMarker } from '$lib/api'
  import { canDecodeRecording } from '$lib/hls'
  import HlsVideo from '$lib/components/HlsVideo.svelte'
  import TimelineScrubber from '$lib/components/TimelineScrubber.svelte'
  import Icon from '$lib/components/Icon.svelte'
  import Mono from '$lib/components/Mono.svelte'
  import { ui } from '$lib/i18n/strings'

  // Scrubber window: a fixed 3-hour span per camera. Full-res chunk: 10 min.
  const WINDOW_SECONDS = 3 * 3600
  const CHUNK_SECONDS = 10 * 60
  // Zoom bounds for the scrubber viewport span (pinch / wheel). 10 min keeps a
  // useful minimum context; 12 h is the widest pan that still resolves cells.
  const MIN_SPAN = 10 * 60
  const MAX_SPAN = 12 * 3600
  // Prefetch the next chunk into the idle buffer once the active playhead is
  // within this many seconds of the active chunk's end.
  const PREFETCH_LEAD_SECONDS = 10
  // After a settle/seek, ignore the full-res timeupdate echo briefly so it
  // doesn't fight the UI right after the assignment.
  const SEEK_SETTLE_MS = 300
  // Playback-speed cycle for the speed chip. Cycling from 1x runs
  // 1 -> 2 -> 4 -> 0.5 -> 1 (advance by one, wrapping).
  const SPEED_STEPS = [0.5, 1, 2, 4]
  // Transport skip step, in wall-clock seconds, for the back/forward buttons.
  const SKIP_SECONDS = 10
  // VHS press-and-hold rush: wall-clock multipliers and the hold-duration ramp
  // that selects between them. 60x immediately, 240x after 1.2s held, 480x
  // after 2.5s held — the longer the button is held, the faster the rush.
  const RUSH_RATES = [60, 240, 480]
  const RUSH_RAMP_MS = [0, 1200, 2500]

  // page.params.id is typed string | undefined; the route only matches when
  // [id] is bound, so the empty fallback never surfaces in practice.
  const camId = $derived(page.params.id ?? '')
  const camera = $derived(camerasStore.cameras.find((c) => c.id === camId) ?? null)

  // Visible span of the scrubber viewport, in wall-clock seconds. Fixed at the
  // 3h window this phase; A-2 makes it adjustable (zoom). The viewport itself
  // (windowStart/windowEnd) is DERIVED from the playhead + this span below, so
  // the filmstrip pans under a centred playhead instead of the playhead
  // chasing a fixed window.
  let viewSpan = $state(WINDOW_SECONDS)

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

  // Wall-clock playhead (unix sec). Driven by drag (preview) or full-res
  // timeupdate (playback); consumed by the scrubber + the clock readout. In the
  // filmstrip model the playhead is the anchor: the viewport is derived FROM it.
  let position = $state(0)

  // Scrubber VIEWPORT, DERIVED from the playhead: a pure-centred filmstrip. The
  // playhead is ALWAYS dead-centre (timeToFraction(position, windowStart,
  // windowEnd) === 0.5); a drag and playback both move the CONTENT under it,
  // never the handle. Near the recording's edges the viewport simply extends
  // past the playable bounds and the scrubber's dim bands mark the empty span.
  // VIEWPORT-only — playback bounds are liveEdge/playbackFloor.
  const windowStart = $derived(position - viewSpan / 2)
  const windowEnd = $derived(position + viewSpan / 2)

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
  // framesKey keys which tail span's frames are loaded (load once per tail +
  // staleness guard); frameSrc is the current nearest-frame src.
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
  const idleStart = $derived(active === 'a' ? startB : startA)
  const idleSrc = $derived(active === 'a' ? srcB : srcA)

  function setActiveChunk(start: number, end: number, src: string) {
    if (active === 'a') {
      startA = start
      endA = end
      srcA = src
    } else {
      startB = start
      endB = end
      srcB = src
    }
  }
  function setIdleChunk(start: number, end: number, src: string) {
    if (active === 'a') {
      startB = start
      endB = end
      srcB = src
    } else {
      startA = start
      endA = end
      srcA = src
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
    } else {
      srcA = ''
      startA = null
      endA = null
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
  // -1 rewind. vhsRate is the current wall-clock multiplier shown on the OSD
  // badge (0 when idle). The rush rides the SAME low-res preview path a manual
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

  // On camId change: capture the playable bounds, place the playhead, reset to
  // scrubbing, drop any loaded chunk, and pull the summary for the scrubber
  // cells. The viewport is derived from the playhead, so we only set position:
  // an optional ?t=<unix> deep-link (from an event's "See on timeline") lands
  // the playhead at t (the centred viewport then sits t dead-centre); without it
  // the playhead rests centred in the last viewSpan up to live, then autoplays
  // forward. Read the raw t param in the tracked scope so a same-camera
  // deep-link with a new t re-captures.
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
      // the default last-3h window, never NaN.
      const tSec = rawT !== null ? Number.parseInt(rawT, 10) : Number.NaN
      if (Number.isFinite(tSec)) {
        // Deep-link: a real event time is always a valid past second, so just
        // land the playhead at t (never past live). playbackFloor is derived and
        // may be the permissive fallback until the summary lands, so we do NOT
        // clamp the entry position by it — it governs panning once loaded.
        position = tSec > liveEdge ? liveEdge : tSec
      } else {
        // Default: rest the playhead centred in the last viewSpan up to live so
        // the viewport shows the last 3h; autoplay then runs it forward to live.
        position = liveEdge - viewSpan / 2
      }
      mode = 'scrubbing'
      // Reset BOTH buffers and the swap state.
      active = 'a'
      srcA = ''
      srcB = ''
      startA = null
      startB = null
      endA = null
      endB = null
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
      activeClip = null
      // Drop the review lane for the previous camera; it reloads below over
      // the same span as the clips.
      reviews = []
      // Same for the audio lane.
      audioEvents = []
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
    const lo = Math.max(playbackFloor, center - half)
    const hi = Math.min(liveEdge, center + half)
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
    el.addEventListener('loadedmetadata', onMeta)
    el.addEventListener('seeked', onSeeked)
    return () => {
      el.removeEventListener('loadedmetadata', onMeta)
      el.removeEventListener('seeked', onSeeked)
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
      // Accurate, drift-bounded mapping: a chunk's currentTime 0 == its
      // wall-clock start, and the chunk spans at most CHUNK_SECONDS.
      position = start + el.currentTime
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

  // Ensure full-res plays at wall-clock T on the ACTIVE buffer: reuse the
  // loaded chunk when T is inside it (cheap currentTime seek), else load a
  // fresh <=10-min chunk starting at T. Either way switch to playback, start
  // playing, and clear the idle buffer — any settle invalidates a prefetched
  // continuation, so the next timeupdate re-prefetches afresh.
  function ensurePlaybackAt(t: number) {
    // Preview-only device: never touch full-res — the scrub/preview path is
    // the whole experience here.
    if (previewOnly) return
    chunkEnded = false
    const el = activeEl
    const aStart = activeStart
    const aEnd = activeEnd
    const within = aStart !== null && aEnd !== null && activeSrc !== '' && t >= aStart && t <= aEnd
    if (within && el) {
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
    // Filmstrip: the viewport is derived from position, so clamping position to
    // the viewport would be circular. The playhead is bounded by what's
    // PLAYABLE; the viewport then follows it.
    const clamped = t < playbackFloor ? playbackFloor : t > liveEdge ? liveEdge : t
    position = clamped
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
  // or wheel step). We clamp to [MIN_SPAN, MAX_SPAN] and round to a whole
  // second. Because windowStart/windowEnd are derived as position ± viewSpan/2
  // and the playhead is centred, changing viewSpan zooms around the playhead
  // for free — position does not move, so no chunk refetch is triggered.
  function onZoom(targetSpan: number) {
    viewSpan = Math.round(Math.max(MIN_SPAN, Math.min(MAX_SPAN, targetSpan)))
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

  // Speed chip: advance to the next entry in SPEED_STEPS (wrapping) and apply
  // it to the active element. From 1x: 1 -> 2 -> 4 -> 0.5 -> 1.
  function cycleSpeed() {
    const i = SPEED_STEPS.indexOf(playbackRate)
    playbackRate = SPEED_STEPS[(i + 1) % SPEED_STEPS.length] ?? 1
    applyPlaybackRate()
  }

  // Transport skip by deltaSeconds (called with +/-SKIP_SECONDS). The skip is a
  // full-res PLAYBACK seek, so clamp the target to the playback extent
  // [playbackFloor, liveEdge], not the scrubber viewport. When the target stays
  // inside the loaded active chunk
  // and we're in playback, a cheap currentTime seek preserves the current
  // play/pause state (no forced play, no pause). When it crosses the active
  // chunk boundary or we're not in playback, settle a fresh chunk at the target
  // and play. Full-res control: a no-op on preview-only devices.
  function skip(deltaSeconds: number) {
    if (previewOnly) return
    const raw = position + deltaSeconds
    const target = raw < playbackFloor ? playbackFloor : raw > liveEdge ? liveEdge : raw
    const el = activeEl
    const aStart = activeStart
    const aEnd = activeEnd
    const within =
      aStart !== null && aEnd !== null && activeSrc !== '' && target >= aStart && target <= aEnd
    if (mode === 'playback' && within && el) {
      el.currentTime = target - aStart!
      lastSeekAt = performance.now()
      position = target
      // Preserve the current play/pause state — the element keeps playing if it
      // was playing and stays paused if it was paused.
    } else {
      ensurePlaybackAt(target)
    }
  }

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
    idlePrimed = false
    showFullRes = false
    pendingPlay = false
    chunkEnded = false
    playerError = false
    paused = true
    mode = 'scrubbing'
  }

  // Backstop for the proactive codec gate: classify an HlsVideo error and
  // either fall back to preview-only (decode-class) or surface the genuine
  // "No recording" overlay (everything else).
  function handleFullResError(msg: string) {
    if (DECODE_ERROR_STRINGS.has(msg)) {
      // Already proven decodable this session → this is a transient glitch
      // (decode error while scrubbing / near the live edge), not a real codec
      // incompatibility. Ignore it: do not latch preview-only, do not error.
      if (decodeProven) return
      enterPreviewOnly()
      return
    }
    playerError = true
  }

  // Smart back: the timeline is entered from focus, grid, AND events, so return
  // to wherever the user came from when there is in-app history; fall back to
  // this camera's focus screen on a fresh/direct load (no history to pop).
  function goBack() {
    if (typeof history !== 'undefined' && history.length > 1) history.back()
    else goto(`/cam/${camId}`)
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
    <button type="button" class="back" onclick={goBack} aria-label={ui.backLabel}>
      <Icon name="back" size={20} />
      <span>{ui.backLabel}</span>
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

    {#if chunkEnded}
      <div class="frame-hint">
        <Mono size={11} color="rgba(255,255,255,0.85)">{ui.timelineLiveEnd}</Mono>
      </div>
    {/if}

    {#if previewOnly && !scrubActive}
      <div class="frame-codec">
        <Mono size={12} color="rgba(255,255,255,0.82)">{ui.timelineCodecUnsupported}</Mono>
      </div>
    {/if}

    {#if playerError}
      <div class="frame-overlay">
        <Mono size={12} color="rgba(255,255,255,0.82)">{ui.timelineNoRecording}</Mono>
      </div>
    {/if}

    {#if vhsDir !== 0}
      <!-- Light CRT texture + a top-centre direction/rate badge while rushing. -->
      <div class="vhs-shimmer" aria-hidden="true"></div>
      <div class="vhs-badge">
        <Icon name={vhsDir > 0 ? 'fastForward' : 'rewind'} size={13} />
        <Mono size={11} weight={500} color="rgba(255,255,255,0.92)">{vhsRate}×</Mono>
      </div>
    {/if}
  </div>

  <div class="controls">
    {#if !previewOnly}
      <!-- Left flex spacer balances the trailing meta cluster so the transport
           stays centred in the bar. The VHS rewind/fast-forward pair (C-ii)
           will flank play/pause inside .transport. -->
      <span class="edge" aria-hidden="true"></span>
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
          class="livebtn skip"
          onclick={() => skip(-SKIP_SECONDS)}
          aria-label={ui.timelineSkipBack}
        >
          <Icon name="skipBack" size={18} />
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
          class="livebtn skip"
          onclick={() => skip(SKIP_SECONDS)}
          aria-label={ui.timelineSkipForward}
        >
          <Icon name="skipForward" size={18} />
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
        <button
          type="button"
          class="livebtn speed"
          onclick={cycleSpeed}
          aria-label={ui.timelineSpeed}
        >
          <Mono size={13} weight={500} color="var(--text)">{playbackRate}×</Mono>
        </button>
        <button
          type="button"
          class="livebtn"
          onclick={toggleFullResMute}
          aria-label={fullResMuted ? ui.timelineUnmute : ui.timelineMute}
        >
          <Icon name={fullResMuted ? 'mute' : 'unmute'} size={20} />
        </button>
      {/if}
      <span class="clock">
        <Mono size={13} weight={500} color="var(--text)" letterSpacing={0.3}>{clock}</Mono>
      </span>
    </div>
  </div>

  <div class="scrub">
    <TimelineScrubber
      hours={timelineStore.hours}
      {windowStart}
      {windowEnd}
      {position}
      {playbackFloor}
      {liveEdge}
      {reviews}
      {audioEvents}
      onSeek={handleSeek}
      onScrubStart={handleScrubStart}
      onScrubEnd={handleScrubEnd}
      {onZoom}
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
  /* Light CRT shimmer over the preview while a VHS rush is held. Sits above the
     media layers but below the controls (it lives inside .frame). Subtle
     scanlines plus a faint rolling sheen — texture, not an obstruction. */
  .vhs-shimmer {
    position: absolute;
    inset: 0;
    pointer-events: none;
    background: repeating-linear-gradient(
      to bottom,
      rgba(0, 0, 0, 0) 0px,
      rgba(0, 0, 0, 0) 1px,
      rgba(0, 0, 0, 0.07) 1px,
      rgba(0, 0, 0, 0.07) 2px
    );
  }
  .vhs-shimmer::after {
    content: '';
    position: absolute;
    inset: 0;
    background: linear-gradient(
      to bottom,
      rgba(255, 255, 255, 0) 0%,
      rgba(255, 255, 255, 0.05) 50%,
      rgba(255, 255, 255, 0) 100%
    );
    background-size: 100% 40%;
    background-repeat: no-repeat;
    animation: vhs-roll 2.4s linear infinite;
  }
  @keyframes vhs-roll {
    from {
      background-position: 0 -40%;
    }
    to {
      background-position: 0 140%;
    }
  }
  @media (prefers-reduced-motion: reduce) {
    .vhs-shimmer::after {
      animation: none;
    }
  }
  /* VHS OSD badge: top-centre pill, mirrors .frame-hint styling. Direction
     glyph + the live rush rate. Never captures pointer events. */
  .vhs-badge {
    position: absolute;
    left: 50%;
    top: 10px;
    transform: translateX(-50%);
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 5px 10px;
    border-radius: 999px;
    background: rgba(0, 0, 0, 0.5);
    backdrop-filter: blur(8px);
    -webkit-backdrop-filter: blur(8px);
    color: rgba(255, 255, 255, 0.92);
    pointer-events: none;
  }
  .vhs-badge :global(svg) {
    fill: currentColor;
    stroke: currentColor;
  }

  .controls {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: center;
    column-gap: 14px;
    row-gap: 10px;
  }
  /* Left spacer (= the trailing meta cluster's flex weight) keeps the transport
     cluster centred in the bar on the single-row (desktop) layout. */
  .edge {
    flex: 1 1 0;
  }
  /* Centred transport cluster: rewind, skip-back, play/pause, skip-forward,
     fast-forward. Tight internal gap so the five read as one unit. */
  .transport {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 8px;
    flex: 0 1 auto;
  }
  /* Trailing meta cluster: speed chip, mute, clock — pinned to the right. */
  .meta {
    flex: 1 1 0;
    display: flex;
    align-items: center;
    justify-content: flex-end;
    gap: 14px;
  }
  /* Five transport buttons plus the meta cluster genuinely overflow a phone-
     width bar, so below this width the meta cluster wraps to its own centred
     second row and the flex spacer is dropped (it would otherwise push the
     transport off-centre on the wrapped row). Desktop keeps the single centred
     row. */
  @media (max-width: 540px) {
    .controls {
      column-gap: 10px;
    }
    .edge {
      display: none;
    }
    .meta {
      flex: 0 0 auto;
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
  .livebtn:active {
    transform: translateY(1px);
  }
  /* Skip and VHS buttons sit a touch smaller than play/pause — they flank it. */
  .livebtn.skip,
  .livebtn.vhs {
    width: 40px;
    height: 40px;
  }
  /* Pin fill+stroke to currentColor so the filled double-triangle VHS glyphs
     render fully (same fix as the play glyph). */
  .livebtn.vhs :global(svg) {
    fill: currentColor;
    stroke: currentColor;
  }
  /* Disable the tap-highlight / text-select on a held VHS button so a long
     touch-hold reads as a transport press, not a selection. */
  .livebtn.vhs {
    touch-action: none;
    -webkit-user-select: none;
    user-select: none;
    -webkit-touch-callout: none;
  }
  /* Speed chip: width follows the mono label (e.g. "0.5×") instead of a fixed
     square. */
  .livebtn.speed {
    width: auto;
    min-width: 46px;
    padding: 0 12px;
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
