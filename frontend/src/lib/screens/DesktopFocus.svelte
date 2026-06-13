<script lang="ts">
  import type { Snippet } from 'svelte'
  import { untrack } from 'svelte'
  import { goto } from '$app/navigation'
  import type { Camera, EventItem } from '$lib/api'
  import { fetchEvents } from '$lib/api'
  import Icon from '$lib/components/Icon.svelte'
  import Mono from '$lib/components/Mono.svelte'
  import OnlineDot from '$lib/components/OnlineDot.svelte'
  import Segmented from '$lib/components/Segmented.svelte'
  import EventModal from '$lib/components/EventModal.svelte'
  import ZoomPane from '$lib/components/ZoomPane.svelte'
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
    fps: number | null
    togglePause: () => void
    toggleMute: () => void
    toggleQuality: () => void
    toggleFullscreen: () => void
    togglePip: () => void
    pipSupported: boolean
    pipActive: boolean
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
    fps,
    togglePause,
    toggleMute,
    toggleQuality,
    toggleFullscreen,
    togglePip,
    pipSupported,
    pipActive,
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

  const streamName = $derived(
    effectiveQuality === 'sub' ? (camera?.streams.sub ?? '') : (camera?.streams.main ?? '')
  )

  const RECENT_LIMIT = 6
  let recentEvents = $state<EventItem[]>([])
  let recentLastAt = $state('')
  let modalEvent = $state<EventItem | null>(null)

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
  <div class="df-body">
    <div class="df-live-main">
      <div class="dk-crumb">
        <button type="button" onclick={onBack} class="dk-back" aria-label={ui.cameras}>
          <Icon name="back" size={21} />
        </button>
        <div class="dk-crumb-t">
          <div class="nm">
            {camera?.name ?? '—'}
            <OnlineDot online={camera?.online ?? null} size={6} />
          </div>
          <div class="pth">
            {#if camera?.online}
              WEBRTC · WHEP · {latencyMs !== null ? `${latencyMs} MS` : '—'}
            {:else}
              STREAM OFFLINE
            {/if}
            {#if streamName}
              · {streamName}
            {/if}
          </div>
        </div>
        {#if camera?.online}
          <Segmented
            value={effectiveQuality}
            options={qualityOptions}
            variant="mono"
            onChange={(v) => {
              if (v !== effectiveQuality) toggleQuality()
            }}
          />
        {/if}
      </div>

      <div class="df-video-frame dk-feed">
        <ZoomPane resetKey={camera?.id}>
          {@render videoSnippet()}
        </ZoomPane>

        {#if isLive}
          <div class="dk-live-tag">
            <span class="pulse" aria-hidden="true"></span>
            <span class="live-text">{ui.liveTag}</span>
          </div>
        {/if}

        {#if showTelemetry}
          <div class="dk-statspanel">
            <div class="sp-row">
              <span class="sp-k">LAT</span><span class="sp-v"
                >{latencyMs !== null ? `${latencyMs} ms` : '—'}</span
              >
            </div>
            <div class="sp-row">
              <span class="sp-k">BR</span><span class="sp-v"
                >{bitrateKbps !== null ? `${(bitrateKbps / 1000).toFixed(1)} Mbps` : '—'}</span
              >
            </div>
            <div class="sp-row">
              <span class="sp-k">FPS</span><span class="sp-v"
                >{fps !== null ? `${fps} fps` : '—'}</span
              >
            </div>
            <div class="sp-row">
              <span class="sp-k">VID</span><span class="sp-v">{videoCodec ?? '—'}</span>
            </div>
            <div class="sp-row">
              <span class="sp-k">RES</span><span class="sp-v">{resolution ?? '—'}</span>
            </div>
            {#if audioAvailable}
              <div class="sp-row">
                <span class="sp-k">AUD</span><span class="sp-v">{audioCodec ?? '—'}</span>
              </div>
            {/if}
          </div>
        {/if}

        {#if showTimestamp}
          <div class="ts-chip">
            <Mono size={11} color="rgba(255,255,255,0.9)" letterSpacing={0.4}>{currentTime}</Mono>
          </div>
        {/if}
      </div>

      <div class="dk-livebar">
        {#if camera?.online}
          <button type="button" class="dk-livebtn primary" onclick={togglePause}>
            <Icon name={isPaused ? 'play' : 'pause'} size={20} />
            <span>{isPaused ? 'Play' : 'Pause'}</span>
          </button>
          <button
            type="button"
            class="dk-livebtn"
            class:active={!isMuted}
            onclick={toggleMute}
            disabled={!audioAvailable}
            aria-label={isMuted ? 'Unmute' : 'Mute'}
          >
            <Icon name={isMuted ? 'mute' : 'unmute'} size={20} />
          </button>
          <button
            type="button"
            class="dk-livebtn"
            class:active={showTelemetry}
            onclick={onShowTelemetry}
          >
            <Icon name="activity" size={20} />
            <span>{ui.statsLabel}</span>
          </button>
          {#if camera?.capabilities.talk_back}
            <button type="button" class="dk-livebtn" disabled aria-label="Talkback">
              <Icon name="mic" size={20} />
            </button>
          {/if}
          <span class="grow"></span>
          <button type="button" class="dk-livebtn" onclick={downloadSnapshot}>
            <Icon name="snapshot" size={20} />
            <span>Snapshot</span>
          </button>
          {#if pipSupported}
            <button
              type="button"
              class="dk-livebtn"
              class:active={pipActive}
              onclick={togglePip}
              aria-label={ui.pipLabel}
            >
              <Icon name="pip" size={20} />
              <span>{ui.pipLabel}</span>
            </button>
          {/if}
          <button type="button" class="dk-livebtn" onclick={toggleFullscreen}>
            <Icon name="fullscreen" size={20} />
            <span>Fullscreen</span>
          </button>
        {:else}
          <button type="button" class="dk-livebtn" onclick={onBack}>
            <Icon name="back" size={20} />
            <span>{ui.cameras}</span>
          </button>
        {/if}
      </div>

      {#if camerasStore.cameras.length > 1}
        <div class="dk-filmstrip-wrap">
          <div class="dk-sec-label">{ui.otherCamerasLabel}</div>
          <div class="dk-filmstrip">
            {#each camerasStore.cameras as c (c.id)}
              {@const current = c.id === camera?.id}
              {#if current}
                <div class="dk-film on" class:off={!c.online} aria-current="page">
                  <div class="thumb">
                    {#if c.online}
                      <div class="cam">
                        <img
                          src={`/api/cameras/${c.id}/tile.jpg?t=${tileTick}`}
                          alt={c.name}
                          class="film-img"
                          loading="lazy"
                        />
                      </div>
                    {:else}
                      <div class="cam offline">
                        <span class="off-ico" aria-hidden="true"
                          ><Icon name="warning" size={18} /></span
                        >
                      </div>
                    {/if}
                  </div>
                  <div class="flbl">
                    <OnlineDot online={c.online} size={5} />
                    <span class="flbl-name">{c.name}</span>
                  </div>
                </div>
              {:else}
                <a
                  href={`/cam/${c.id}`}
                  class="dk-film"
                  class:off={!c.online}
                  onclick={(e) => switchCam(e, c.id)}
                >
                  <div class="thumb">
                    {#if c.online}
                      <div class="cam">
                        <img
                          src={`/api/cameras/${c.id}/tile.jpg?t=${tileTick}`}
                          alt={c.name}
                          class="film-img"
                          loading="lazy"
                        />
                      </div>
                    {:else}
                      <div class="cam offline">
                        <span class="off-ico" aria-hidden="true"
                          ><Icon name="warning" size={18} /></span
                        >
                      </div>
                    {/if}
                  </div>
                  <div class="flbl">
                    <OnlineDot online={c.online} size={5} />
                    <span class="flbl-name">{c.name}</span>
                  </div>
                </a>
              {/if}
            {/each}
          </div>
        </div>
      {/if}
    </div>

    <aside class="dk-live-side">
      <div class="dk-panel">
        <div class="dk-panel-h">
          <span class="ttl">{ui.recentEvents}{camera?.name ? ` · ${camera.name}` : ''}</span>
          <a href="/events" class="lnk">{ui.viewAll}</a>
        </div>
        <div class="dk-panel-body">
          {#if recentEvents.length === 0}
            <div class="dk-events-empty">
              <Mono size={11} color="var(--text-3)">{ui.noRecentEvents}</Mono>
            </div>
          {:else}
            {#each recentEvents as ev (ev.id)}
              <button type="button" class="dk-evrow" onclick={() => (modalEvent = ev)}>
                <div class="meta">
                  <div class="l1">
                    <span>{eventKindLabels[ev.kind] ?? ev.kind}</span>
                    <span class="mono">{eventTimestamp(ev.started_at)}</span>
                  </div>
                  <div class="l2">
                    <span>{ui.score}</span>
                    <span class="mono">{ev.score !== null ? ev.score.toFixed(2) : '—'}</span>
                  </div>
                </div>
              </button>
            {/each}
          {/if}
        </div>
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
    min-height: 100dvh;
    background: var(--bg);
    color: var(--text);
    display: flex;
    flex-direction: column;
  }

  /* dk-live two-column layout — top-aligned, page scrolls naturally */
  .df-body {
    display: grid;
    grid-template-columns: minmax(0, 1fr) 348px;
    gap: 22px;
    padding: 22px 28px;
    align-items: start;
  }
  .df-live-main {
    display: flex;
    flex-direction: column;
    gap: 14px;
    min-width: 0;
  }

  /* dk-crumb */
  .dk-crumb {
    display: flex;
    align-items: center;
    gap: 14px;
    flex: none;
  }
  .dk-back {
    width: 42px;
    height: 42px;
    flex: 0 0 auto;
    border-radius: 11px;
    border: 1px solid var(--border);
    background: var(--surface);
    color: var(--text);
    display: grid;
    place-items: center;
    cursor: pointer;
    font-family: inherit;
    transition:
      border-color 0.15s ease,
      background 0.15s ease;
  }
  .dk-back:hover {
    border-color: var(--border-strong);
    background: var(--surface-2);
  }
  .dk-crumb-t {
    flex: 1 1 auto;
    min-width: 0;
  }
  .dk-crumb-t .nm {
    font-size: 22px;
    font-weight: 600;
    letter-spacing: -0.3px;
    display: flex;
    align-items: center;
    gap: 10px;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .dk-crumb-t .pth {
    font-family: 'JetBrains Mono', ui-monospace, monospace;
    font-size: 11px;
    color: var(--text-2);
    margin-top: 3px;
    letter-spacing: 0.3px;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .df-video-frame {
    position: relative;
    width: 100%;
    aspect-ratio: 16 / 9;
    /* dk-feed look: rounded, --feed bg, --border ring */
    border-radius: var(--r);
    overflow: hidden;
    background: var(--feed);
    border: 1px solid var(--border);
  }

  /* Overlays — dk-statspanel + dk-live-tag */
  .dk-statspanel {
    position: absolute;
    top: 14px;
    left: 14px;
    min-width: 150px;
    padding: 11px 14px;
    border-radius: var(--r-sm);
    background: rgba(8, 10, 12, 0.6);
    border: 1px solid rgba(255, 255, 255, 0.14);
    backdrop-filter: blur(18px);
    -webkit-backdrop-filter: blur(18px);
    display: flex;
    flex-direction: column;
    gap: 6px;
  }
  .sp-row {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: 22px;
    white-space: nowrap;
  }
  .sp-k {
    font-family: 'JetBrains Mono', ui-monospace, monospace;
    font-size: 11px;
    font-weight: 600;
    letter-spacing: 0.6px;
    color: rgba(255, 255, 255, 0.45);
  }
  .sp-v {
    font-family: 'JetBrains Mono', ui-monospace, monospace;
    font-size: 12px;
    font-weight: 600;
    color: #fff;
  }
  .dk-live-tag {
    position: absolute;
    top: 14px;
    right: 14px;
    display: inline-flex;
    align-items: center;
    gap: 7px;
    padding: 5px 10px;
    border-radius: 999px;
    background: rgba(8, 10, 12, 0.46);
    border: 1px solid rgba(255, 255, 255, 0.16);
    backdrop-filter: blur(6px);
    -webkit-backdrop-filter: blur(6px);
    color: #fff;
    font-family: 'JetBrains Mono', ui-monospace, monospace;
    font-size: 11px;
    font-weight: 600;
    letter-spacing: 0.4px;
  }
  .dk-live-tag .pulse {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: var(--online);
    box-shadow: 0 0 0 0 color-mix(in oklab, var(--online) 50%, transparent);
    animation: dk-pulse 2s infinite;
  }
  @keyframes dk-pulse {
    0% {
      box-shadow: 0 0 0 0 color-mix(in oklab, var(--online) 50%, transparent);
    }
    70% {
      box-shadow: 0 0 0 7px color-mix(in oklab, var(--online) 0%, transparent);
    }
    100% {
      box-shadow: 0 0 0 0 color-mix(in oklab, var(--online) 0%, transparent);
    }
  }
  @media (prefers-reduced-motion: reduce) {
    .dk-live-tag .pulse {
      animation: none;
    }
  }
  .ts-chip {
    position: absolute;
    bottom: 14px;
    left: 16px;
    padding: 5px 9px;
    border-radius: 5px;
    background: rgba(0, 0, 0, 0.45);
    backdrop-filter: blur(10px);
    -webkit-backdrop-filter: blur(10px);
  }

  /* dk-livebar */
  .dk-livebar {
    display: flex;
    align-items: center;
    gap: 10px;
    flex-shrink: 0;
  }
  .dk-livebar .grow {
    flex: 1 1 auto;
  }
  .dk-livebtn {
    height: 46px;
    min-width: 46px;
    padding: 0 14px;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    gap: 9px;
    border-radius: 12px;
    border: 1px solid var(--border);
    background: var(--surface);
    color: var(--text);
    cursor: pointer;
    font-family: inherit;
    font-size: 14px;
    font-weight: 600;
    -webkit-appearance: none;
    appearance: none;
    transition:
      background 0.16s ease,
      color 0.16s ease,
      border-color 0.16s ease;
  }
  .dk-livebtn:hover:not(:disabled) {
    border-color: var(--border-strong);
    background: var(--surface-2);
  }
  .dk-livebtn:disabled {
    opacity: 0.4;
    cursor: not-allowed;
  }
  .dk-livebtn.primary {
    background: var(--accent-soft);
    border-color: transparent;
    color: var(--accent-ink);
  }
  .dk-livebtn.primary :global(svg) {
    fill: var(--accent-ink);
    stroke: var(--accent-ink);
  }
  .dk-livebtn.active {
    background: var(--accent-soft);
    border-color: transparent;
    color: var(--accent-ink);
  }

  /* Filmstrip — dk-filmstrip-wrap / dk-filmstrip / dk-film */
  .dk-filmstrip-wrap {
    display: flex;
    flex-direction: column;
    gap: 11px;
    margin-top: 2px;
  }
  .dk-sec-label {
    font-size: 14px;
    font-weight: 600;
    color: var(--text);
  }
  .dk-filmstrip {
    display: flex;
    gap: 12px;
    overflow-x: auto;
    padding-bottom: 6px;
    scrollbar-width: thin;
  }
  .dk-film {
    flex: 0 0 174px;
    cursor: pointer;
    text-decoration: none;
    color: inherit;
  }
  .dk-film.off {
    cursor: default;
  }
  .dk-film .thumb {
    position: relative;
    aspect-ratio: 16 / 9;
    border-radius: var(--r-sm);
    overflow: hidden;
    border: 1px solid var(--border);
    background: var(--feed);
    transition:
      border-color 0.15s ease,
      box-shadow 0.15s ease;
  }
  .dk-film:hover .thumb {
    border-color: var(--border-strong);
  }
  .dk-film.on .thumb {
    border-color: transparent;
    box-shadow: 0 0 0 2px var(--accent);
  }
  .dk-film .thumb .cam {
    position: absolute;
    inset: 0;
    border-radius: 0;
    display: flex;
    align-items: center;
    justify-content: center;
  }
  .dk-film .thumb .cam.offline {
    border: none;
    color: var(--warn);
  }
  .off-ico {
    color: var(--warn);
    display: inline-flex;
  }
  .film-img {
    width: 100%;
    height: 100%;
    object-fit: cover;
    display: block;
  }
  .dk-film .flbl {
    font-size: 13px;
    color: var(--text-2);
    margin-top: 8px;
    display: flex;
    align-items: center;
    gap: 7px;
    justify-content: center;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .dk-film.on .flbl {
    color: var(--accent-ink);
  }
  .flbl-name {
    overflow: hidden;
    text-overflow: ellipsis;
  }

  /* Events-only sidebar — dk-live-side / dk-panel */
  .dk-live-side {
    display: flex;
    flex-direction: column;
    gap: 18px;
    min-width: 0;
  }
  .dk-panel {
    border: 1px solid var(--border);
    border-radius: var(--r);
    background: var(--surface);
    overflow: hidden;
    display: flex;
    flex-direction: column;
  }
  .dk-panel-h {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 10px;
    padding: 14px 16px;
    border-bottom: 1px solid var(--border);
    flex: none;
  }
  .dk-panel-h .ttl {
    font-size: 14px;
    font-weight: 600;
    color: var(--text);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .dk-panel-h .lnk {
    font-size: 13px;
    color: var(--accent-ink);
    cursor: pointer;
    text-decoration: none;
    flex: 0 0 auto;
  }
  .dk-panel-h .lnk:hover {
    text-decoration: underline;
  }
  .dk-panel-body {
    padding: 8px;
    overflow-y: auto;
  }
  .dk-events-empty {
    padding: 10px 8px;
  }
  .dk-evrow {
    width: 100%;
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 9px 8px;
    border: none;
    border-radius: 11px;
    background: transparent;
    color: inherit;
    font-family: inherit;
    cursor: pointer;
    text-align: left;
    transition: background 0.14s ease;
  }
  .dk-evrow:hover {
    background: var(--surface-2);
  }
  .dk-evrow .meta {
    flex: 1 1 auto;
    min-width: 0;
  }
  .dk-evrow .meta .l1 {
    font-size: 14px;
    font-weight: 500;
    display: flex;
    justify-content: space-between;
    gap: 8px;
  }
  .dk-evrow .meta .l1 .mono {
    color: var(--text-2);
    font-weight: 400;
    font-family: 'JetBrains Mono', ui-monospace, monospace;
    font-size: 11px;
  }
  .dk-evrow .meta .l2 {
    font-size: 12.5px;
    color: var(--text-2);
    display: flex;
    justify-content: space-between;
    gap: 8px;
    margin-top: 2px;
  }
  .dk-evrow .meta .l2 .mono {
    color: var(--accent-ink);
    font-family: 'JetBrains Mono', ui-monospace, monospace;
    font-size: 11px;
  }
</style>
