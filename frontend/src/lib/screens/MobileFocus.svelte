<script lang="ts">
  import type { Snippet } from 'svelte'
  import { untrack } from 'svelte'
  import { goto } from '$app/navigation'
  import type { Camera, EventItem } from '$lib/api'
  import { fetchEvents } from '$lib/api'
  import { camerasStore } from '$lib/stores/cameras.svelte'
  import Icon from '$lib/components/Icon.svelte'
  import Mono from '$lib/components/Mono.svelte'
  import OnlineDot from '$lib/components/OnlineDot.svelte'
  import Segmented from '$lib/components/Segmented.svelte'
  import EventModal from '$lib/components/EventModal.svelte'
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

  function pad(n: number): string {
    return String(n).padStart(2, '0')
  }
  function formatNow(): string {
    const d = new Date()
    return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
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

  const otherCameras = $derived(camerasStore.cameras.filter((c) => c.id !== camera?.id))

  // Focus → focus must REPLACE the history entry so a single back tap returns
  // to the grid regardless of how many cameras the user hopped through.
  function switchCam(e: MouseEvent, id: string) {
    e.preventDefault()
    goto(`/cam/${id}`, { replaceState: true })
  }

  const RECENT_LIMIT = 4
  let recentEvents = $state<EventItem[]>([])
  let recentLastAt = $state('')
  let modalEvent = $state<EventItem | null>(null)

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

  const qualityOptions = $derived([
    { value: 'main' as const, label: 'HQ' },
    { value: 'sub' as const, label: 'LQ', disabled: !subAvailable }
  ])
</script>

<div class="mf-root">
  <div class="status-spacer"></div>

  <div class="mf-body">
    <div class="h-row">
      <button type="button" class="cbtn" onclick={onBack} aria-label={ui.backLabel}>
        <Icon name="back" size={21} />
      </button>
      <div class="crumb">
        <div class="title-lg">{camera?.name ?? '—'}</div>
        <div class="path">
          WEBRTC · {latencyMs !== null ? `${latencyMs} MS` : '—'}
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
      {:else}
        <span class="cbtn-spacer" aria-hidden="true"></span>
      {/if}
    </div>

    <div class="feedwrap">
      {@render videoSnippet()}

      {#if showTimestamp}
        <div class="ts-chip">
          <Mono size={10} color="rgba(255,255,255,0.85)" letterSpacing={0.4}>{currentTime}</Mono>
        </div>
      {/if}

      {#if showTelemetry && camera?.online}
        <div class="statspanel">
          <div class="sp-row">
            <span class="sp-k">LAT</span><span class="sp-v"
              >{latencyMs !== null ? `${latencyMs}ms` : '—'}</span
            >
          </div>
          <div class="sp-row">
            <span class="sp-k">BR</span><span class="sp-v"
              >{bitrateKbps !== null ? `${(bitrateKbps / 1000).toFixed(1)}Mbps` : '—'}</span
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
    </div>

    <div class="livebar">
      <button
        type="button"
        class="livebtn primary"
        onclick={togglePause}
        aria-label={isPaused ? 'Play' : 'Pause'}
      >
        <Icon name={isPaused ? 'play' : 'pause'} size={20} />
      </button>
      <button
        type="button"
        class="livebtn"
        class:active={!isMuted}
        onclick={toggleMute}
        disabled={!audioAvailable}
        aria-label={isMuted ? 'Unmute' : 'Mute'}
      >
        <Icon name={isMuted ? 'mute' : 'unmute'} size={20} />
      </button>
      <button type="button" class="livebtn" onclick={downloadSnapshot} aria-label="Snapshot">
        <Icon name="snapshot" size={20} />
      </button>
      <button
        type="button"
        class="livebtn"
        class:active={showTelemetry}
        onclick={onShowTelemetry}
        aria-label={ui.telemetryLabel}
      >
        <Icon name="activity" size={20} />
      </button>
      <button type="button" class="livebtn" onclick={toggleFullscreen} aria-label="Fullscreen">
        <Icon name="fullscreen" size={20} />
      </button>
      {#if camera?.capabilities.talk_back}
        <button type="button" class="livebtn" disabled aria-label="Talkback">
          <Icon name="mic" size={20} />
        </button>
      {/if}
    </div>

    {#if otherCameras.length > 0}
      <div class="sec-label">
        <span>{ui.otherCamerasLabel}</span>
      </div>
      <div class="hscroll" aria-label={ui.otherCamerasLabel}>
        {#each otherCameras as c (c.id)}
          <a
            href={`/cam/${c.id}`}
            class="mini"
            class:offline-tile={!c.online}
            onclick={(e) => switchCam(e, c.id)}
          >
            {#if c.online}
              <div class="cam">
                <img
                  src={`/api/cameras/${c.id}/tile.jpg?t=${tileTick}`}
                  alt={c.name}
                  class="mini-img"
                  loading="lazy"
                />
              </div>
            {:else}
              <div class="cam offline">
                <span class="off-ico" aria-hidden="true"><Icon name="warning" size={18} /></span>
              </div>
            {/if}
            <div class="lbl">
              <OnlineDot online={c.online} size={5} />
              <span class="mini-name">{c.name}</span>
            </div>
          </a>
        {/each}
      </div>
    {/if}

    <div class="h-row recent-h">
      <span class="recent-title">{ui.recentEvents}</span>
      <a href="/events" class="recent-link">{ui.viewAll}</a>
    </div>
    {#if recentEvents.length === 0}
      <div class="recent-empty">
        <Mono size={11} color="var(--text-3)">{ui.noRecentEvents}</Mono>
      </div>
    {:else}
      <div class="recent-list">
        {#each recentEvents as ev (ev.id)}
          <button type="button" class="ev" onclick={() => (modalEvent = ev)}>
            <span class="ev-ago mono">{eventTimestamp(ev.started_at)}</span>
            <div class="ev-meta">
              <div class="l1">
                <span class="ev-kind">{eventKindLabels[ev.kind] ?? ev.kind}</span>
                <span class="ev-score mono">
                  {ev.score !== null
                    ? ev.score.toFixed(2)
                    : ev.duration_seconds !== null
                      ? `${ev.duration_seconds}${ui.secondsShort}`
                      : '—'}
                </span>
              </div>
            </div>
          </button>
        {/each}
      </div>
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
  }
  .status-spacer {
    height: env(safe-area-inset-top, 0px);
    flex-shrink: 0;
  }
  .mf-body {
    display: flex;
    flex-direction: column;
    gap: 12px;
    padding: 6px 18px calc(env(safe-area-inset-bottom, 0px) + 96px);
  }

  /* Header crumb row — calm .h-row + .cbtn + .crumb + .qseg */
  .h-row {
    display: flex;
    align-items: center;
    gap: 12px;
  }
  .cbtn {
    width: 42px;
    height: 42px;
    flex: 0 0 auto;
    display: grid;
    place-items: center;
    border-radius: var(--r-sm);
    border: 1px solid var(--border);
    background: var(--surface);
    color: var(--text);
    cursor: pointer;
    font-family: inherit;
  }
  .cbtn:active {
    transform: translateY(1px);
  }
  .cbtn-spacer {
    width: 42px;
    flex: 0 0 auto;
  }
  .crumb {
    flex: 1 1 auto;
    text-align: center;
    min-width: 0;
  }
  .title-lg {
    font-size: 18px;
    font-weight: 600;
    letter-spacing: -0.2px;
    color: var(--text);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .path {
    font-family: 'JetBrains Mono', ui-monospace, monospace;
    font-size: 11px;
    color: var(--text-2);
    margin-top: 3px;
    letter-spacing: 0.2px;
    white-space: nowrap;
  }

  /* Feed — calm .feedwrap. The 16:9 frame + the video snippet sits inside.
     object-fit on the video stays `fill` deliberately (anamorphic invariant). */
  .feedwrap {
    position: relative;
    width: 100%;
    aspect-ratio: 16 / 9;
    border-radius: var(--r);
    overflow: hidden;
    background: var(--feed);
  }
  .ts-chip {
    position: absolute;
    bottom: 12px;
    right: 12px;
    padding: 4px 7px;
    border-radius: 4px;
    background: rgba(0, 0, 0, 0.42);
    backdrop-filter: blur(8px);
    -webkit-backdrop-filter: blur(8px);
  }
  .statspanel {
    position: absolute;
    top: 10px;
    left: 10px;
    min-width: 132px;
    padding: 9px 12px;
    border-radius: var(--r-sm);
    background: rgba(8, 10, 12, 0.6);
    border: 1px solid rgba(255, 255, 255, 0.14);
    box-shadow: 0 12px 28px -16px rgba(0, 0, 0, 0.7);
    backdrop-filter: blur(18px);
    -webkit-backdrop-filter: blur(18px);
    display: flex;
    flex-direction: column;
    gap: 5px;
  }
  .sp-row {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: 18px;
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
    letter-spacing: 0.2px;
    color: #fff;
  }

  /* Control bar — calm .livebar / .livebtn */
  .livebar {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 10px;
    margin-top: 2px;
  }
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
  .livebtn:active:not(:disabled) {
    transform: translateY(1px);
  }
  .livebtn:disabled {
    opacity: 0.4;
    cursor: not-allowed;
  }
  .livebtn.primary {
    background: var(--accent-soft);
    border-color: transparent;
    color: var(--accent);
  }
  .livebtn.primary :global(svg) {
    fill: var(--accent);
    stroke: var(--accent);
  }
  .livebtn.active {
    background: var(--accent-soft);
    border-color: transparent;
    color: var(--accent);
  }

  /* Others rail — calm .hscroll .mini */
  .sec-label {
    display: flex;
    align-items: center;
    justify-content: space-between;
    font-size: 15px;
    font-weight: 600;
    color: var(--text);
    margin-top: 4px;
  }
  .hscroll {
    display: flex;
    gap: 11px;
    overflow-x: auto;
    scrollbar-width: none;
    -webkit-overflow-scrolling: touch;
    padding-bottom: 4px;
    margin: 0 -18px;
    padding-left: 18px;
    padding-right: 18px;
  }
  .hscroll::-webkit-scrollbar {
    height: 0;
  }
  .mini {
    flex: 0 0 124px;
    cursor: pointer;
    text-decoration: none;
    color: inherit;
  }
  .mini .cam {
    position: relative;
    height: 74px;
    border-radius: var(--r);
    overflow: hidden;
    background: var(--feed);
    display: flex;
    align-items: center;
    justify-content: center;
    color: var(--text-3);
  }
  .mini .cam.offline {
    border: 1px dashed var(--border-strong);
    color: var(--warn);
  }
  .off-ico {
    color: var(--warn);
    display: inline-flex;
  }
  .mini-img {
    width: 100%;
    height: 100%;
    /* object-fit: cover here is fine — these tiles are decorative thumbs,
       not the live feed; the anamorphic invariant only applies to the
       active <video> + poster, not the rail miniatures. */
    object-fit: cover;
    display: block;
  }
  .lbl {
    font-size: 13px;
    color: var(--text-2);
    margin-top: 6px;
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 6px;
  }
  .mini-name {
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    max-width: 96px;
  }

  /* Recent events */
  .recent-h {
    margin-top: 4px;
  }
  .recent-title {
    font-size: 15px;
    font-weight: 600;
    color: var(--text);
    flex: 1 1 auto;
    text-align: left;
  }
  .recent-link {
    font-size: 14px;
    color: var(--text-2);
    text-decoration: none;
    flex: 0 0 auto;
  }
  .recent-link:hover {
    color: var(--text);
  }
  .recent-empty {
    padding: 12px 0;
  }
  .recent-list {
    display: flex;
    flex-direction: column;
  }
  .ev {
    width: 100%;
    display: flex;
    gap: 13px;
    align-items: center;
    padding: 11px 0;
    border: none;
    border-bottom: 1px solid var(--border);
    background: transparent;
    color: inherit;
    font-family: inherit;
    text-align: left;
    cursor: pointer;
  }
  .ev:last-child {
    border-bottom: none;
  }
  .ev-ago {
    flex: 0 0 auto;
    font-family: 'JetBrains Mono', ui-monospace, monospace;
    font-size: 12px;
    color: var(--text-2);
  }
  .ev-meta {
    flex: 1 1 auto;
    min-width: 0;
  }
  .l1 {
    display: flex;
    justify-content: space-between;
    align-items: baseline;
    gap: 8px;
    font-size: 15px;
    font-weight: 600;
    color: var(--text);
  }
  .ev-kind {
    min-width: 0;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .ev-score {
    font-family: 'JetBrains Mono', ui-monospace, monospace;
    font-size: 12px;
    font-weight: 400;
    color: var(--accent);
    white-space: nowrap;
    flex: 0 0 auto;
  }
</style>
