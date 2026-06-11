<script lang="ts">
  import type { Snippet } from 'svelte'
  import { untrack } from 'svelte'
  import { goto } from '$app/navigation'
  import type { Camera, EventItem } from '$lib/api'
  import { fetchEvents } from '$lib/api'
  import Icon from '$lib/components/Icon.svelte'
  import IconBtn from '$lib/components/IconBtn.svelte'
  import Mono from '$lib/components/Mono.svelte'
  import OnlineDot from '$lib/components/OnlineDot.svelte'
  import Segmented from '$lib/components/Segmented.svelte'
  import EventModal from '$lib/components/EventModal.svelte'
  import { camerasStore } from '$lib/stores/cameras.svelte'
  import { eventsStreamStore } from '$lib/stores/events-stream.svelte'
  import { eventKindLabels, ui } from '$lib/i18n/strings'
  import { eventTimestamp } from '$lib/util/time'

  type Props = {
    camera: Camera | null
    effectiveQuality: 'main' | 'sub'
    subAvailable: boolean
    audioAvailable: boolean
    isMuted: boolean
    isPaused: boolean
    isLive: boolean
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
    isLive,
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

  function pad(n: number): string {
    return String(n).padStart(2, '0')
  }

  function formatNow(): string {
    const d = new Date()
    return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} · ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
  }

  let currentTime = $state(formatNow())
  let tileTick = $state(Math.floor(Date.now() / 1000))

  $effect(() => {
    const t = setInterval(() => {
      currentTime = formatNow()
      tileTick = Math.floor(Date.now() / 1000)
    }, 1000)
    return () => clearInterval(t)
  })

  const qualityOptions = $derived([
    { value: 'main' as const, label: 'HQ' },
    { value: 'sub' as const, label: 'LQ', disabled: !subAvailable }
  ])

  // Real go2rtc stream name from the camera (Frigate-truth), selected by the
  // active quality. Stream overrides are server-side only and not visible to
  // the frontend, so this is the configured Main/Sub name — never a
  // fabricated *_h264 string. Empty (shouldn't happen for Main) → render
  // nothing.
  const streamName = $derived(
    effectiveQuality === 'sub' ? (camera?.streams.sub ?? '') : (camera?.streams.main ?? '')
  )

  const RECENT_LIMIT = 6

  let recentEvents = $state<EventItem[]>([])
  let recentLastAt = $state('')
  let modalEvent = $state<EventItem | null>(null)

  // Focus → focus navigation must REPLACE the history entry so a single
  // back goes to the grid rather than walking through prior cameras.
  // The anchor stays for accessibility / right-click open-in-new-tab.
  function switchCam(e: MouseEvent, id: string) {
    e.preventDefault()
    goto(`/cam/${id}`, { replaceState: true })
  }

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
</script>

<div class="df-root">
  <header class="df-topbar">
    <div class="df-topbar-left">
      <button type="button" onclick={onBack} class="df-back" aria-label={ui.cameras}>
        <Icon name="back" size={14} />
        <span>{ui.cameras}</span>
      </button>
      <div class="df-divider"></div>
      <span class="df-cam-name">{camera?.name ?? '—'}</span>
      {#if streamName}
        <Mono color="var(--text-3)" size={11}>{streamName}</Mono>
      {/if}
      <span class="df-status">
        <OnlineDot online={camera?.online ?? null} size={5} />
        <Mono size={10} color="var(--text-2)" letterSpacing={0.5}>
          WEBRTC · {latencyMs !== null ? `${latencyMs} MS` : '—'}
        </Mono>
      </span>
    </div>
    <div class="df-topbar-right">
      <Segmented
        value={effectiveQuality}
        options={qualityOptions}
        onChange={(v) => {
          if (v !== effectiveQuality) toggleQuality()
        }}
      />
      {#if !subAvailable}
        <Mono size={10} color="var(--text-3)" letterSpacing={0.3}>{ui.subUnavailable}</Mono>
      {/if}
      <div class="df-divider"></div>
      <IconBtn
        icon="activity"
        label={ui.telemetryLabel}
        onclick={onShowTelemetry}
        size={32}
        active={showTelemetry}
      />
    </div>
  </header>

  <div class="df-body">
    <div class="df-video-col">
      <div class="df-player-center">
        <div class="df-player">
          <div class="df-video-frame">
            {@render videoSnippet()}

            {#if showTelemetry}
              <div class="df-statspanel">
                <Mono size={10} color="rgba(255,255,255,0.45)" letterSpacing={0.4}>LATENCY</Mono>
                <Mono size={10} color="rgba(255,255,255,0.9)" weight={500}>
                  {latencyMs !== null ? `${latencyMs} ms` : '—'}
                </Mono>
                <Mono size={10} color="rgba(255,255,255,0.45)" letterSpacing={0.4}>BITRATE</Mono>
                <Mono size={10} color="rgba(255,255,255,0.9)" weight={500}>
                  {bitrateKbps !== null ? `${(bitrateKbps / 1000).toFixed(1)} Mbps` : '—'}
                </Mono>
                <Mono size={10} color="rgba(255,255,255,0.45)" letterSpacing={0.4}>VIDEO</Mono>
                <Mono size={10} color="rgba(255,255,255,0.9)" weight={500}>
                  {videoCodec ?? '—'}
                  {resolution ?? ''}
                </Mono>
                {#if audioAvailable}
                  <Mono size={10} color="rgba(255,255,255,0.45)" letterSpacing={0.4}>AUDIO</Mono>
                  <Mono size={10} color="rgba(255,255,255,0.9)" weight={500}
                    >{audioCodec ?? '—'}</Mono
                  >
                {/if}
              </div>
            {/if}

            {#if isLive}
              <div class="df-live-tag">
                <span class="df-live-dot" aria-hidden="true"></span>
                <Mono size={10} color="rgba(255,255,255,0.92)" letterSpacing={0.8} weight={600}>
                  {ui.liveTag}
                </Mono>
              </div>
            {/if}

            {#if showTimestamp}
              <div class="ts-chip">
                <Mono size={11} color="rgba(255,255,255,0.9)" letterSpacing={0.4}
                  >{currentTime}</Mono
                >
              </div>
            {/if}
          </div>

          <div class="df-controls">
            <div class="df-controls-left">
              <IconBtn
                icon={isPaused ? 'play' : 'pause'}
                label="play"
                onclick={togglePause}
                size={34}
              />
              <div class="df-ctrl-divider"></div>
              <IconBtn
                icon={isMuted ? 'mute' : 'unmute'}
                label="mute"
                onclick={toggleMute}
                size={34}
                active={!isMuted}
                disabled={!audioAvailable}
              />
              {#if camera?.capabilities.talk_back}
                <IconBtn icon="mic" label="talkback" size={34} accent disabled />
              {/if}
              <div class="df-ctrl-divider"></div>
              <IconBtn icon="snapshot" label="snap" onclick={downloadSnapshot} size={34} />
            </div>
            <div class="df-controls-right">
              <Mono size={10} color="var(--text-3)" letterSpacing={0.4}>
                {#if streamName}<span class="df-id-strong">{streamName}</span>{/if}{videoCodec
                  ? ` · ${videoCodec}`
                  : ''}{resolution ? ` · ${resolution}` : ''}
              </Mono>
              <IconBtn icon="fullscreen" label="full" onclick={toggleFullscreen} size={34} />
            </div>
          </div>
        </div>
      </div>

      {#if camerasStore.cameras.length > 1}
        <div class="df-filmstrip-wrap">
          <div class="df-filmstrip-header">
            <Mono size={10} color="var(--text-2)" letterSpacing={0.6} uppercase>
              {ui.cameras}
            </Mono>
          </div>
          <div class="df-filmstrip">
            {#each camerasStore.cameras as c (c.id)}
              {#if c.id === camera?.id}
                <div class="df-film df-film-current" class:offline={!c.online} aria-current="page">
                  <div class="df-film-frame">
                    <img
                      src={`/api/cameras/${c.id}/tile.jpg?t=${tileTick}`}
                      alt={c.name}
                      class="df-film-img"
                      loading="lazy"
                    />
                    <div class="df-film-dot">
                      <OnlineDot online={c.online} size={4} />
                    </div>
                  </div>
                  <span class="df-film-name">{c.name}</span>
                </div>
              {:else}
                <a
                  href={`/cam/${c.id}`}
                  class="df-film"
                  class:offline={!c.online}
                  onclick={(e) => switchCam(e, c.id)}
                >
                  <div class="df-film-frame">
                    <img
                      src={`/api/cameras/${c.id}/tile.jpg?t=${tileTick}`}
                      alt={c.name}
                      class="df-film-img"
                      loading="lazy"
                    />
                    <div class="df-film-dot">
                      <OnlineDot online={c.online} size={4} />
                    </div>
                  </div>
                  <span class="df-film-name">{c.name}</span>
                </a>
              {/if}
            {/each}
          </div>
        </div>
      {/if}
    </div>

    <aside class="df-sidebar">
      <div class="df-sidebar-section">
        <div class="df-sidebar-header">
          <Mono size={10} color="var(--text-2)" letterSpacing={0.6} uppercase>
            {ui.recentEvents}
          </Mono>
          <Mono size={10} color="var(--text-3)">{recentEvents.length}</Mono>
        </div>
        {#if recentEvents.length === 0}
          <div class="df-events-empty">
            <Mono size={11} color="var(--text-3)">{ui.noRecentEvents}</Mono>
          </div>
        {:else}
          {#each recentEvents as ev, i (ev.id)}
            <button
              type="button"
              class="df-event-row"
              class:active={i === 0}
              onclick={() => (modalEvent = ev)}
            >
              <div class="df-event-left">
                <Mono size={10} color="var(--text-3)">
                  {eventTimestamp(ev.started_at)}
                </Mono>
                <span class="df-event-label" class:active-label={i === 0}>
                  {eventKindLabels[ev.kind] ?? ev.kind}
                </span>
              </div>
              <Mono size={10} color="var(--text-3)">
                {ev.score !== null ? ev.score.toFixed(2) : '—'}
              </Mono>
            </button>
          {/each}
        {/if}
      </div>
    </aside>
  </div>
</div>

{#if modalEvent}
  <EventModal event={modalEvent} onClose={() => (modalEvent = null)} />
{/if}

<style>
  .df-root {
    width: 100%;
    height: 100dvh;
    background: var(--bg);
    color: var(--text);
    display: flex;
    flex-direction: column;
  }

  .df-topbar {
    padding: 14px 28px;
    display: flex;
    align-items: center;
    justify-content: space-between;
    border-bottom: 1px solid var(--border);
    flex-shrink: 0;
  }
  .df-topbar-left,
  .df-topbar-right {
    display: flex;
    align-items: center;
    gap: 14px;
  }
  .df-back {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    font-size: 13px;
    color: var(--text-2);
    background: none;
    border: none;
    padding: 0;
    cursor: pointer;
    font-family: inherit;
  }
  .df-back:hover {
    color: var(--text);
  }
  .df-divider {
    width: 1px;
    height: 14px;
    background: var(--border);
  }
  .df-cam-name {
    font-size: 15px;
    font-weight: 600;
    letter-spacing: -0.2px;
  }
  .df-status {
    display: inline-flex;
    align-items: center;
    gap: 6px;
  }

  .df-body {
    flex: 1;
    display: grid;
    grid-template-columns: minmax(0, 1fr) 348px;
    min-height: 0;
  }
  .df-video-col {
    padding: 22px 22px 22px 28px;
    display: flex;
    flex-direction: column;
    min-height: 0;
  }

  /* Sizing container for the player block. container-type:size lets the
     player width below resolve against this box's width AND height via
     cqw/cqh, so the 16:9 frame is computed deterministically instead of
     relying on the browser's aspect-ratio-vs-max-height tiebreaker (which
     was the prior bug — width:100% + max-height:100% violated the ratio
     and stretched the picture horizontally on short windows). */
  .df-player-center {
    flex: 1;
    min-height: 0;
    min-width: 0;
    container-type: size;
    display: flex;
    align-items: center;
    justify-content: center;
  }

  /* The frame + controls move together as one block, so the controls
     always sit directly under the video with no detached floating gap
     at any window height. Width = min(available width, height-derived
     16:9 width after reserving the controls bar + inter-element gap).
     --controls-reserved covers the controls bar (12px+12px padding +
     34px IconBtn + 2px border = 60px) plus the 12px gap below the frame;
     bumped to 76px so the controls are never clipped at the minimum
     usable window height. */
  .df-player {
    --controls-reserved: 76px;
    width: min(100cqw, calc((100cqh - var(--controls-reserved)) * 16 / 9));
    display: flex;
    flex-direction: column;
    gap: 12px;
  }

  /* Strict 16:9 box driven by .df-player's width — no width:100%, no
     max-height. object-fit on .poster / .video-el stays `fill`
     deliberately: the LQ sub stream is anamorphic on some cameras and
     needs the unsquashed stretch into 16:9. */
  .df-video-frame {
    position: relative;
    width: 100%;
    aspect-ratio: 16 / 9;
    border-radius: 12px;
    overflow: hidden;
    background: #000;
    box-shadow: inset 0 0 0 1px var(--border);
  }
  .ts-chip {
    position: absolute;
    bottom: 14px;
    left: 16px;
    padding: 5px 9px;
    border-radius: 5px;
    background: rgba(0, 0, 0, 0.45);
    backdrop-filter: blur(10px);
  }

  /* Top-left stats overlay (replaces the bottom-right telemetry pill).
     Translucent dark + blur, mono key/value rows. Gated by showTelemetry. */
  .df-statspanel {
    position: absolute;
    top: 14px;
    left: 16px;
    padding: 8px 12px;
    border-radius: 6px;
    background: rgba(0, 0, 0, 0.55);
    backdrop-filter: blur(10px);
    border: 1px solid rgba(255, 255, 255, 0.08);
    display: grid;
    grid-template-columns: auto auto;
    gap: 3px 14px;
    text-align: left;
  }

  /* Top-right LIVE tag with a pulsing dot. Static under reduce-motion. */
  .df-live-tag {
    position: absolute;
    top: 14px;
    right: 16px;
    display: inline-flex;
    align-items: center;
    gap: 7px;
    padding: 5px 10px;
    border-radius: 999px;
    background: rgba(0, 0, 0, 0.5);
    backdrop-filter: blur(10px);
    border: 1px solid rgba(255, 255, 255, 0.08);
  }
  .df-live-dot {
    width: 7px;
    height: 7px;
    border-radius: 50%;
    background: var(--online);
    animation: df-live-pulse 1.6s ease-in-out infinite;
  }
  @keyframes df-live-pulse {
    0% {
      box-shadow: 0 0 0 0 color-mix(in oklab, var(--online) 60%, transparent);
    }
    70% {
      box-shadow: 0 0 0 6px color-mix(in oklab, var(--online) 0%, transparent);
    }
    100% {
      box-shadow: 0 0 0 0 color-mix(in oklab, var(--online) 0%, transparent);
    }
  }
  @media (prefers-reduced-motion: reduce) {
    .df-live-dot {
      animation: none;
    }
  }

  .df-controls {
    flex-shrink: 0;
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 12px 16px;
    border: 1px solid var(--border);
    border-radius: 10px;
    background: rgba(255, 255, 255, 0.015);
  }
  .df-controls-left,
  .df-controls-right {
    display: flex;
    align-items: center;
    gap: 8px;
  }
  .df-ctrl-divider {
    width: 1px;
    height: 18px;
    background: var(--border);
    margin: 0 4px;
  }
  .df-id-strong {
    color: var(--text);
  }

  /* Bottom filmstrip: flex-none sibling so .df-player-center keeps flex:1
     and the deterministic 16:9 sizing math is unchanged. */
  .df-filmstrip-wrap {
    flex: none;
    margin-top: 16px;
  }
  .df-filmstrip-header {
    margin-bottom: 8px;
  }
  .df-filmstrip {
    display: flex;
    gap: 10px;
    overflow-x: auto;
    scrollbar-width: none;
  }
  .df-filmstrip::-webkit-scrollbar {
    display: none;
  }
  .df-film {
    flex: none;
    width: 174px;
    display: flex;
    flex-direction: column;
    gap: 6px;
    text-decoration: none;
    color: inherit;
    cursor: pointer;
  }
  .df-film.offline {
    opacity: 0.55;
  }
  .df-film-current {
    cursor: default;
  }
  .df-film-frame {
    position: relative;
    width: 174px;
    aspect-ratio: 16 / 9;
    border-radius: 6px;
    overflow: hidden;
    background: #0c0d0f;
    box-shadow: inset 0 0 0 1px var(--border);
  }
  .df-film-current .df-film-frame {
    box-shadow: 0 0 0 2px var(--accent);
  }
  .df-film-img {
    width: 100%;
    height: 100%;
    object-fit: cover;
  }
  .df-film-dot {
    position: absolute;
    top: 6px;
    left: 6px;
  }
  .df-film-name {
    text-align: center;
    font-size: 11px;
    color: var(--text-2);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .df-film-current .df-film-name {
    color: var(--accent);
    font-weight: 500;
  }

  .df-sidebar {
    border-left: 1px solid var(--border);
    padding: 22px 24px 22px 22px;
    display: flex;
    flex-direction: column;
    gap: 24px;
    overflow: auto;
  }
  .df-sidebar-section {
    display: flex;
    flex-direction: column;
  }
  .df-sidebar-header {
    display: flex;
    justify-content: space-between;
    align-items: baseline;
    margin-bottom: 12px;
  }
  .df-event-row {
    width: 100%;
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 10px 8px;
    margin: 0 -8px;
    border: none;
    border-left: 2px solid transparent;
    border-radius: 6px;
    background: transparent;
    color: inherit;
    font-family: inherit;
    padding-left: 8px;
    cursor: pointer;
    text-align: left;
  }
  .df-event-row:hover {
    background: rgba(255, 255, 255, 0.025);
  }
  .df-event-row.active {
    background: color-mix(in oklab, var(--accent) 14%, transparent);
    border-left-color: var(--accent);
    padding-left: 12px;
  }
  .df-events-empty {
    padding: 10px 0;
  }
  .df-event-left {
    display: flex;
    align-items: center;
    gap: 12px;
    min-width: 0;
  }
  .df-event-label {
    font-size: 12px;
    color: var(--text);
  }
  .df-event-label.active-label {
    font-weight: 500;
  }
</style>
