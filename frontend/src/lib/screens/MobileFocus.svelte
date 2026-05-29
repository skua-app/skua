<script lang="ts">
  import type { Snippet } from 'svelte'
  import { untrack } from 'svelte'
  import { goto } from '$app/navigation'
  import type { Camera, EventItem } from '$lib/api'
  import { fetchEvents } from '$lib/api'
  import { camerasStore } from '$lib/stores/cameras.svelte'
  import Icon from '$lib/components/Icon.svelte'
  import IconBtn from '$lib/components/IconBtn.svelte'
  import Mono from '$lib/components/Mono.svelte'
  import OnlineDot from '$lib/components/OnlineDot.svelte'
  import Segmented from '$lib/components/Segmented.svelte'
  import EventModal from '$lib/components/EventModal.svelte'
  import { eventsStreamStore } from '$lib/stores/events-stream.svelte'
  import { eventKindLabels, ui } from '$lib/i18n/strings'

  type Props = {
    camera: Camera | null
    effectiveQuality: 'main' | 'sub'
    subAvailable: boolean
    audioAvailable: boolean
    isMuted: boolean
    isPaused: boolean
    showTelemetry: boolean
    showTimestamp: boolean
    latencyMs: number | null
    bitrateKbps: number | null
    videoCodec: string | null
    audioCodec: string | null
    resolution: string | null
    togglePause: () => void
    toggleMute: () => void
    toggleQuality: () => void
    toggleFullscreen: () => void
    downloadSnapshot: () => void
    onShowTelemetry: () => void
    onBack: () => void
    videoSnippet: Snippet
  }

  let {
    camera,
    effectiveQuality,
    subAvailable,
    audioAvailable,
    isMuted,
    isPaused,
    showTelemetry,
    showTimestamp,
    latencyMs,
    bitrateKbps,
    videoCodec,
    audioCodec,
    resolution,
    togglePause,
    toggleMute,
    toggleQuality,
    toggleFullscreen,
    downloadSnapshot,
    onShowTelemetry,
    onBack,
    videoSnippet
  }: Props = $props()

  const eventTimeFmt = new Intl.DateTimeFormat([], {
    hour: '2-digit',
    minute: '2-digit',
    hour12: false
  })

  function pad(n: number): string {
    return String(n).padStart(2, '0')
  }

  function formatNow(): string {
    const d = new Date()
    return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
  }

  let currentTime = $state(formatNow())
  // Bump every second to cache-bust the rail's tile.jpg <img> srcs — same
  // pattern as DesktopFocus's sidebar tiles.
  let tileTick = $state(Math.floor(Date.now() / 1000))

  $effect(() => {
    const t = setInterval(() => {
      currentTime = formatNow()
      tileTick = Math.floor(Date.now() / 1000)
    }, 1000)
    return () => clearInterval(t)
  })

  const otherCameras = $derived(camerasStore.cameras.filter((c) => c.id !== camera?.id))

  // Focus → focus navigation must REPLACE the history entry so a single
  // back-tap returns to the grid no matter how many cameras the user
  // hopped through. Native <a href> would push every hop. We keep the
  // anchor for accessibility / right-click-open-in-new-tab and intercept.
  function switchCam(e: MouseEvent, id: string) {
    e.preventDefault()
    goto(`/cam/${id}`, { replaceState: true })
  }

  const RECENT_LIMIT = 4

  let recentEvents = $state<EventItem[]>([])
  let recentLastAt = $state('')
  let modalEvent = $state<EventItem | null>(null)

  // Fetch the latest events for this camera whenever cam.id changes.
  // The cam-id read is tracked outside untrack so route navigation triggers
  // a clean refetch; the fetch itself runs untracked.
  $effect(() => {
    const id = camera?.id
    if (!id) {
      recentEvents = []
      recentLastAt = ''
      return
    }
    untrack(async () => {
      try {
        const resp = await fetchEvents({ cameras: [id], limit: RECENT_LIMIT })
        recentEvents = resp.items
        recentLastAt = resp.items[0]?.started_at ?? new Date().toISOString()
      } catch {
        recentEvents = []
      }
    })
  })

  // Merge SSE event.new for this camera into the local list.
  $effect(() => {
    const id = camera?.id
    if (!id) return
    const stream = eventsStreamStore.latest
    const newer = stream.filter((e) => e.cam_id === id && e.started_at > recentLastAt)
    if (newer.length === 0) return
    untrack(() => {
      const existing = new Set(recentEvents.map((e) => e.id))
      const fresh = newer.filter((e) => !existing.has(e.id))
      const head = fresh[0]
      if (!head) return
      recentEvents = [...fresh, ...recentEvents].slice(0, RECENT_LIMIT)
      recentLastAt = head.started_at
    })
  })

  const qualityOptions = $derived([
    { value: 'main' as const, label: 'HQ' },
    { value: 'sub' as const, label: 'LQ', disabled: !subAvailable }
  ])
</script>

<div class="mf-root">
  <div class="status-spacer"></div>

  <header class="mf-header">
    <button type="button" onclick={onBack} class="mf-icon-btn" aria-label={ui.backLabel}>
      <Icon name="back" size={20} />
    </button>
    <div class="mf-title-col">
      <span class="mf-cam-name">{camera?.name ?? '—'}</span>
      <span class="mf-status-row">
        <OnlineDot online={camera?.online ?? null} size={5} />
        <Mono size={10} color="var(--text-3)" letterSpacing={0.4}>
          WEBRTC · {latencyMs !== null ? `${latencyMs} MS` : '—'}
        </Mono>
      </span>
    </div>
    <button
      type="button"
      onclick={onShowTelemetry}
      class="mf-icon-btn"
      aria-label={ui.telemetryLabel}
    >
      <Icon name="activity" size={20} />
    </button>
  </header>

  <div class="mf-video-frame">
    {@render videoSnippet()}

    {#if showTimestamp}
      <div class="ts-chip">
        <Mono size={10} color="rgba(255,255,255,0.85)" letterSpacing={0.4}>{currentTime}</Mono>
      </div>
    {/if}

    {#if showTelemetry}
      <div class="telemetry-pill">
        <Mono size={9} color="rgba(255,255,255,0.4)">LAT</Mono>
        <Mono size={9} color="rgba(255,255,255,0.85)" weight={500}>
          {latencyMs !== null ? `${latencyMs}ms` : '—'}
        </Mono>
        <Mono size={9} color="rgba(255,255,255,0.4)">BR</Mono>
        <Mono size={9} color="rgba(255,255,255,0.85)" weight={500}>
          {bitrateKbps !== null ? `${(bitrateKbps / 1000).toFixed(1)}Mbps` : '—'}
        </Mono>
        <Mono size={9} color="rgba(255,255,255,0.4)">VID</Mono>
        <Mono size={9} color="rgba(255,255,255,0.85)" weight={500}>{videoCodec ?? '—'}</Mono>
        {#if audioAvailable}
          <Mono size={9} color="rgba(255,255,255,0.4)">AUD</Mono>
          <Mono size={9} color="rgba(255,255,255,0.85)" weight={500}>{audioCodec ?? '—'}</Mono>
        {/if}
      </div>
    {/if}
  </div>

  {#if otherCameras.length > 0}
    <div class="mf-others-rail" aria-label={ui.otherCamerasLabel}>
      {#each otherCameras as c (c.id)}
        <a
          href={`/cam/${c.id}`}
          class="mf-other-tile"
          class:offline={!c.online}
          onclick={(e) => switchCam(e, c.id)}
        >
          <img
            src={`/api/cameras/${c.id}/tile.jpg?t=${tileTick}`}
            alt={c.name}
            class="mf-other-img"
            loading="lazy"
          />
          <span class="mf-other-name">{c.name}</span>
        </a>
      {/each}
    </div>
  {/if}

  <div class="mf-meta">
    <div class="mf-meta-left">
      <Mono size={10} color="var(--text-3)" letterSpacing={0.5} uppercase>{ui.streamLabel}</Mono>
      <Segmented
        value={effectiveQuality}
        options={qualityOptions}
        onChange={(v) => {
          if (v !== effectiveQuality) toggleQuality()
        }}
      />
      {#if !subAvailable}
        <Mono size={9} color="var(--text-3)" letterSpacing={0.3}>{ui.subUnavailable}</Mono>
      {/if}
    </div>
    <div class="mf-meta-right">
      <Mono size={10} color="var(--text-3)" letterSpacing={0.5} uppercase>{ui.qualityLabel}</Mono>
      <Mono size={12} color="var(--text)" weight={500}>{resolution ?? '—'}</Mono>
    </div>
  </div>

  <div class="mf-controls">
    <div class="mf-controls-group">
      <IconBtn icon={isPaused ? 'play' : 'pause'} label="play" onclick={togglePause} size={40} />
      <IconBtn
        icon={isMuted ? 'mute' : 'unmute'}
        label="mute"
        onclick={toggleMute}
        size={40}
        active={!isMuted}
        disabled={!audioAvailable}
      />
      <IconBtn icon="fullscreen" label="fullscreen" onclick={toggleFullscreen} size={40} />
    </div>
    <div class="mf-controls-group">
      <IconBtn icon="snapshot" label="snap" onclick={downloadSnapshot} size={40} />
      {#if camera?.capabilities.talk_back}
        <IconBtn icon="mic" label="talkback" size={40} accent disabled />
      {/if}
    </div>
  </div>

  <div class="mf-events">
    <div class="mf-events-header">
      <Mono size={10} color="var(--text-3)" letterSpacing={0.6} uppercase>{ui.recentEvents}</Mono>
      <a href="/events" class="mf-events-link">
        <Mono size={10} color="var(--text-2)">{ui.viewAll}</Mono>
      </a>
    </div>
    {#if recentEvents.length === 0}
      <div class="mf-events-empty">
        <Mono size={11} color="var(--text-3)">{ui.noRecentEvents}</Mono>
      </div>
    {:else}
      {#each recentEvents as ev (ev.id)}
        <button type="button" class="mf-event-row" onclick={() => (modalEvent = ev)}>
          <div class="mf-event-left">
            <Mono size={11} color="var(--text-2)"
              >{eventTimeFmt.format(new Date(ev.started_at))}</Mono
            >
            <span class="mf-event-label">{eventKindLabels[ev.kind] ?? ev.kind}</span>
          </div>
          <Mono size={10} color="var(--text-3)"
            >{ev.duration_seconds !== null ? `${ev.duration_seconds}${ui.secondsShort}` : '—'}</Mono
          >
        </button>
      {/each}
    {/if}
  </div>
</div>

{#if modalEvent}
  <EventModal event={modalEvent} onClose={() => (modalEvent = null)} />
{/if}

<style>
  .mf-root {
    width: 100%;
    min-height: 100dvh;
    background: var(--bg);
    color: var(--text);
    display: flex;
    flex-direction: column;
    overflow: hidden;
  }
  .status-spacer {
    height: env(safe-area-inset-top, 0px);
    flex-shrink: 0;
  }
  .mf-header {
    padding: 4px 16px 12px;
    display: flex;
    align-items: center;
    justify-content: space-between;
  }
  .mf-icon-btn {
    background: none;
    border: none;
    color: var(--text-2);
    padding: 4px;
    display: flex;
    align-items: center;
    cursor: pointer;
  }
  .mf-icon-btn:hover {
    color: var(--text);
  }
  .mf-title-col {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 2px;
  }
  .mf-cam-name {
    font-size: 15px;
    font-weight: 600;
    letter-spacing: -0.2px;
  }
  .mf-status-row {
    display: inline-flex;
    align-items: center;
    gap: 5px;
  }
  .mf-video-frame {
    position: relative;
    width: 100%;
    aspect-ratio: 16 / 9;
    background: #000;
    overflow: hidden;
  }
  .ts-chip {
    position: absolute;
    top: 10px;
    right: 12px;
    padding: 4px 7px;
    border-radius: 4px;
    background: rgba(0, 0, 0, 0.42);
    backdrop-filter: blur(8px);
  }
  .telemetry-pill {
    position: absolute;
    bottom: 10px;
    right: 12px;
    padding: 6px 10px;
    border-radius: 6px;
    background: rgba(0, 0, 0, 0.5);
    backdrop-filter: blur(8px);
    display: grid;
    grid-template-columns: auto auto;
    gap: 2px 10px;
    text-align: right;
  }
  /* Horizontal scroll-rail of other cameras' ECO tiles, polling 1 Hz via
     tileTick. Fixed tile size keeps ~4 visible on a stock iPhone width;
     the rest scrolls. Scrollbar hidden to match native iOS feel. */
  .mf-others-rail {
    display: flex;
    gap: 6px;
    overflow-x: auto;
    padding: 8px 16px;
    scrollbar-width: none;
    -webkit-overflow-scrolling: touch;
    scroll-snap-type: x mandatory;
  }
  .mf-others-rail::-webkit-scrollbar {
    display: none;
  }
  .mf-other-tile {
    flex-shrink: 0;
    width: 96px;
    display: flex;
    flex-direction: column;
    gap: 4px;
    text-decoration: none;
    color: inherit;
    scroll-snap-align: start;
    opacity: 1;
    transition: opacity 200ms;
  }
  .mf-other-tile.offline {
    opacity: 0.7;
  }
  .mf-other-img {
    width: 96px;
    height: 54px;
    object-fit: cover;
    border-radius: 4px;
    background: #000;
    box-shadow: inset 0 0 0 1px var(--border);
  }
  .mf-other-name {
    font-size: 10px;
    color: var(--text-3);
    text-align: center;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .mf-meta {
    padding: 14px 18px 10px;
    display: flex;
    justify-content: space-between;
    align-items: center;
    border-bottom: 1px solid var(--border);
  }
  .mf-meta-left,
  .mf-meta-right {
    display: flex;
    flex-direction: column;
    gap: 4px;
  }
  .mf-meta-right {
    align-items: flex-end;
  }
  .mf-controls {
    padding: 18px 18px 14px;
    display: flex;
    align-items: center;
    justify-content: space-between;
  }
  .mf-controls-group {
    display: flex;
    gap: 8px;
  }
  .mf-events {
    padding: 6px 18px;
    /* Clear the fixed MobileTabBar (~56px tall + its own safe-area-inset)
       plus a small breathing gap so the last event row never hides
       behind the bar at the bottom of scroll. */
    padding-bottom: calc(env(safe-area-inset-bottom, 0px) + 72px);
    flex: 1;
    overflow: auto;
  }
  .mf-events-header {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    margin-bottom: 10px;
  }
  .mf-events-link {
    text-decoration: none;
  }
  .mf-event-row {
    width: 100%;
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 12px 0;
    border: none;
    border-bottom: 1px solid var(--border);
    background: none;
    color: inherit;
    font-family: inherit;
    cursor: pointer;
    text-align: left;
  }
  .mf-event-row:hover {
    background: rgba(255, 255, 255, 0.02);
  }
  .mf-events-empty {
    padding: 12px 0;
  }
  .mf-event-left {
    display: flex;
    align-items: center;
    gap: 12px;
  }
  .mf-event-label {
    font-size: 13px;
    color: var(--text);
  }
</style>
