<script lang="ts">
  import Icon from './Icon.svelte'
  import OnlineDot from './OnlineDot.svelte'
  import Mono from './Mono.svelte'
  import type { Camera } from '$lib/api'
  import { prefsStore } from '$lib/stores/prefs.svelte'
  import { ui } from '$lib/i18n/strings'
  import { untrack } from 'svelte'

  type Props = {
    camera: Camera
    index?: number
    nameStyle?: 'below' | 'overlay'
    showTimestamp?: boolean
    onclick?: () => void
  }

  let { camera, index = 0, nameStyle = 'below', showTimestamp = false, onclick }: Props = $props()

  let snapshotSrc = $state<string | null>(null)
  let imgError = $state(false)
  let currentTime = $state<string>('')

  const timeFmt = new Intl.DateTimeFormat([], {
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false
  })

  const srcBase = $derived(
    prefsStore.gridMode === 'hd'
      ? `/api/cameras/${camera.id}/snapshot.jpg`
      : `/api/cameras/${camera.id}/tile.jpg`
  )

  function refreshSnapshot() {
    if (typeof document !== 'undefined' && document.visibilityState === 'hidden') return
    imgError = false
    snapshotSrc = `${srcBase}?t=${Math.floor(Date.now() / 1000)}`
  }

  function tickTime() {
    currentTime = timeFmt.format(new Date())
  }

  // Polling: set up once on mount, runs at 1 Hz. Stagger initial fetch by
  // index * 100ms so the tiles don't all hit the BFF at the same instant.
  $effect(() => {
    const stagger = untrack(() => index * 100)
    const startTimer = setTimeout(() => {
      refreshSnapshot()
      tickTime()
    }, stagger)
    const ticker = setInterval(() => {
      refreshSnapshot()
      tickTime()
    }, 1000)

    function onVisibility() {
      if (document.visibilityState === 'visible') {
        refreshSnapshot()
        tickTime()
      }
    }
    document.addEventListener('visibilitychange', onVisibility)

    return () => {
      clearTimeout(startTimer)
      clearInterval(ticker)
      document.removeEventListener('visibilitychange', onVisibility)
    }
  })

  // Immediate refresh when HD/ECO toggles, without waiting for next tick.
  $effect(() => {
    void srcBase
    refreshSnapshot()
  })
</script>

<button
  type="button"
  {onclick}
  class="cam-tile"
  class:offline={!camera.online}
  aria-label={camera.name}
>
  <div class="tile-frame">
    {#if snapshotSrc && !imgError && camera.online}
      <img
        src={snapshotSrc}
        alt={camera.name}
        class="tile-img"
        loading="lazy"
        onerror={() => (imgError = true)}
      />
    {:else if !camera.online}
      <div class="no-signal">
        <span class="no-signal-icon" aria-hidden="true">
          <Icon name="warning" size={30} />
        </span>
        <Mono color="var(--text-2)" size={13} weight={500}>{ui.noSignal}</Mono>
      </div>
    {/if}

    <div class="tile-top-row">
      <OnlineDot online={camera.online} size={5} />
      {#if showTimestamp && camera.online}
        <span class="ts-chip">
          <Mono color="rgba(255, 255, 255, 0.7)" size={10} weight={500} letterSpacing={0.4}>
            {currentTime}
          </Mono>
        </span>
      {/if}
    </div>

    {#if nameStyle === 'overlay' && camera.online}
      <div class="name-overlay">
        <span class="name-text">{camera.name}</span>
      </div>
    {/if}
  </div>

  {#if nameStyle !== 'overlay'}
    <div class="tile-label">
      <div class="tile-label-left">
        <span class="cam-name">{camera.name}</span>
        <Mono color="var(--text-3)" size={10}>{camera.id}</Mono>
      </div>
    </div>
  {/if}
</button>

<style>
  .cam-tile {
    position: relative;
    display: block;
    width: 100%;
    padding: 0;
    background: transparent;
    border: none;
    text-align: left;
    cursor: pointer;
    opacity: 1;
    transition: opacity 200ms;
    color: inherit;
    font: inherit;
  }
  .cam-tile.offline {
    opacity: 0.8;
  }

  .tile-frame {
    position: relative;
    width: 100%;
    aspect-ratio: 16 / 9;
    border-radius: 10px;
    overflow: hidden;
    background: #0c0d0f;
    box-shadow: inset 0 0 0 1px var(--border);
  }

  .tile-img {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    object-fit: fill;
  }

  .no-signal {
    position: absolute;
    inset: 0;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 6px;
  }
  .no-signal-icon {
    display: inline-flex;
    color: var(--offline);
  }

  .tile-top-row {
    position: absolute;
    top: 8px;
    left: 10px;
    right: 10px;
    display: flex;
    justify-content: space-between;
    align-items: center;
  }

  .ts-chip {
    background: rgba(0, 0, 0, 0.35);
    backdrop-filter: blur(8px);
    padding: 2px 6px;
    border-radius: 4px;
  }

  .name-overlay {
    position: absolute;
    left: 0;
    right: 0;
    bottom: 0;
    padding: 20px 12px 10px;
    background: linear-gradient(180deg, transparent, rgba(0, 0, 0, 0.55));
    display: flex;
    align-items: flex-end;
  }
  .name-text {
    color: #fff;
    font-size: 13px;
    font-weight: 500;
    letter-spacing: -0.1px;
  }

  .tile-label {
    display: flex;
    justify-content: space-between;
    align-items: baseline;
    padding-top: 8px;
    padding-left: 2px;
    padding-right: 2px;
  }
  .tile-label-left {
    display: flex;
    align-items: baseline;
    gap: 8px;
    min-width: 0;
  }
  .cam-name {
    color: var(--text);
    font-size: 13px;
    font-weight: 500;
    letter-spacing: -0.1px;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .cam-tile:focus-visible {
    outline: 2px solid color-mix(in oklab, var(--accent) 80%, transparent);
    outline-offset: 2px;
    border-radius: 12px;
  }
</style>
