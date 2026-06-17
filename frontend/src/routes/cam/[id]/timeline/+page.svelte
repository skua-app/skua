<!-- Phase 2b-i scaffold: this route exists only to mount HlsVideo against
     a fixed last-hour window so the recording VOD can be device-smoke-
     tested on iOS and Android. The real scrubber screen — driven start/end,
     gap rendering, markers — replaces this file in 2b-iii. Not advertised
     from the focus screen yet (that link lands in 2c). -->
<script lang="ts">
  import { page } from '$app/state'
  import { goto } from '$app/navigation'
  import { camerasStore } from '$lib/stores/cameras.svelte'
  import { timelineMasterURL } from '$lib/api'
  import HlsVideo from '$lib/components/HlsVideo.svelte'
  import TimelineScrubber from '$lib/components/TimelineScrubber.svelte'
  import type { TimelineHour } from '$lib/timeline'

  // page.params.id is typed string | undefined by SvelteKit's $types; the
  // route only matches when [id] is bound, so the empty fallback never
  // surfaces in practice — it just satisfies the string argument type
  // on timelineMasterURL.
  const camId = $derived(page.params.id ?? '')
  const camera = $derived(camerasStore.cameras.find((c) => c.id === camId) ?? null)

  // Temporary fixed window: last hour. 2b-iii will drive these from the
  // scrubber's selected range. End is wall-clock now; Frigate decides
  // whether the range actually has recording behind it.
  const end = Math.floor(Date.now() / 1000)
  const start = end - 3600

  // 2b-ii visual scaffold: MOCK scrubber data. The window below is the
  // last 6 hours, with hand-picked recordedFraction values — some full,
  // some partial, one gap — so the cells, label decimation, and playhead
  // can be eyeballed without depending on a real Frigate. 2b-iii replaces
  // this block with the timeline store driving real summary data and
  // wires onSeek into the HlsVideo current time.
  const scrubEnd = end
  const scrubStart = scrubEnd - 6 * 3600
  function hourAt(offsetHours: number, fraction: number, events = 0): TimelineHour {
    const tSec = scrubStart + offsetHours * 3600
    const d = new Date(0)
    d.setTime(tSec * 1000)
    return { hourStart: d, recordedFraction: fraction, events, motion: 0 }
  }
  const mockHours: TimelineHour[] = [
    hourAt(0, 1.0),
    hourAt(1, 0.62, 3),
    hourAt(2, 0.0), // gap
    hourAt(3, 0.85, 8),
    hourAt(4, 1.0),
    hourAt(5, 0.35, 1)
  ]
  let playhead = $state(scrubStart + 3 * 3600 + 1800)
  function handleSeek(t: number) {
    playhead = t
    console.debug('[timeline scaffold] onSeek', t)
  }
</script>

<div class="page">
  <header class="bar">
    <button type="button" class="back" onclick={() => goto(`/cam/${camId}`)}>← Live</button>
    <span class="title">{camera?.name ?? camId} · timeline (scaffold)</span>
  </header>

  <div class="frame">
    <HlsVideo src={timelineMasterURL(camId, start, end)} />
  </div>

  <TimelineScrubber
    hours={mockHours}
    windowStart={scrubStart}
    windowEnd={scrubEnd}
    position={playhead}
    onSeek={handleSeek}
  />
</div>

<style>
  .page {
    display: flex;
    flex-direction: column;
    gap: 12px;
    padding: 12px;
    background: var(--bg);
    min-height: 100vh;
  }
  .bar {
    display: flex;
    align-items: center;
    gap: 12px;
    color: var(--text);
  }
  .back {
    appearance: none;
    background: var(--surface);
    color: var(--text);
    border: 1px solid var(--border);
    border-radius: var(--r-sm);
    height: var(--ctrl-h);
    padding: 0 12px;
    cursor: pointer;
  }
  .title {
    color: var(--text-2);
    font-size: 14px;
  }
  .frame {
    aspect-ratio: 16 / 9;
    width: 100%;
    border-radius: var(--r);
    overflow: hidden;
    background: var(--feed);
  }
</style>
