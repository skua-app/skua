<script lang="ts">
  import Icon from './Icon.svelte'
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

  // Stagger first fetch by index so all tiles don't hit the BFF at once;
  // then poll at 1 Hz. Visibility wake-up forces a refresh on resume.
  $effect(() => {
    const stagger = untrack(() => index * 100)
    const start = setTimeout(() => {
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
      clearTimeout(start)
      clearInterval(ticker)
      document.removeEventListener('visibilitychange', onVisibility)
    }
  })

  // Immediate refresh when HD/ECO toggles.
  $effect(() => {
    void srcBase
    refreshSnapshot()
  })
</script>

<button
  type="button"
  {onclick}
  class="cam-tile"
  class:name-below={nameStyle === 'below'}
  class:offline-tile={!camera.online}
  aria-label={camera.name}
>
  {#if camera.online}
    <div class="cam">
      {#if snapshotSrc && !imgError}
        <img
          src={snapshotSrc}
          alt={camera.name}
          class="cam-img"
          loading="lazy"
          onerror={() => (imgError = true)}
        />
      {/if}
      <span class="livedot" aria-hidden="true"></span>
      {#if showTimestamp}
        <span class="ts">{currentTime}</span>
      {/if}
      <div class="cam-hover" aria-hidden="true">
        <span class="pchip">
          <Icon name="play" size={15} />
          <span>{ui.openLive}</span>
        </span>
      </div>
    </div>
  {:else}
    <div class="cam offline">
      <span class="off-ico" aria-hidden="true"><Icon name="warning" size={22} /></span>
      <span class="off-txt">{ui.offline}</span>
    </div>
  {/if}

  <div class="cam-name">
    <span class="cn-name">{camera.name}</span>
    <span class="cn-sub">{camera.id}</span>
  </div>
</button>

<style>
  /* Tile root mirrors calm .cam-tile. In overlay mode it clips the gradient
     name overlay (border-radius + overflow hidden); in name-below mode it
     stays unclipped so the name sits underneath the framed feed. */
  .cam-tile {
    position: relative;
    display: block;
    width: 100%;
    padding: 0;
    background: transparent;
    border: none;
    text-align: left;
    cursor: pointer;
    color: inherit;
    font: inherit;
  }
  .cam-tile:not(.name-below) {
    border-radius: var(--r);
    overflow: hidden;
  }
  .cam-tile:active {
    transform: translateY(1px);
  }
  .cam-tile:focus-visible {
    outline: 2px solid color-mix(in oklab, var(--accent) 80%, transparent);
    outline-offset: 2px;
    border-radius: var(--r);
  }

  /* Feed frame — calm .cam */
  .cam {
    position: relative;
    aspect-ratio: 16 / 9;
    border-radius: var(--r);
    overflow: hidden;
    background: var(--feed);
    display: flex;
    align-items: center;
    justify-content: center;
    color: var(--text-3);
    transition:
      transform 160ms ease,
      box-shadow 200ms ease,
      border-color 160ms ease;
  }
  .cam-img {
    width: 100%;
    height: 100%;
    /* DO NOT change object-fit: load-bearing for our anamorphic snapshots. */
    object-fit: fill;
    display: block;
  }
  .livedot {
    position: absolute;
    top: 9px;
    right: 9px;
    width: 9px;
    height: 9px;
    border-radius: 50%;
    background: var(--online);
    box-shadow: 0 0 0 3px rgba(0, 0, 0, 0.28);
    z-index: 2;
  }
  .ts {
    position: absolute;
    top: 10px;
    left: 12px;
    z-index: 2;
    font-family: 'JetBrains Mono', ui-monospace, monospace;
    font-size: 11px;
    color: rgba(255, 255, 255, 0.82);
    background: rgba(8, 10, 12, 0.4);
    padding: 2px 7px;
    border-radius: 6px;
    backdrop-filter: blur(4px);
    -webkit-backdrop-filter: blur(4px);
    letter-spacing: 0.2px;
  }

  /* Offline state — dashed border, warn glyph, "Offline" text */
  .cam.offline {
    flex-direction: column;
    gap: 6px;
    color: var(--warn);
    border: 1px dashed var(--border-strong);
  }
  .cam-tile:not(.name-below) .cam.offline {
    padding-bottom: 24px;
  }
  .off-ico {
    color: var(--warn);
    display: inline-flex;
  }
  .off-txt {
    font-size: 13px;
    font-weight: 500;
  }

  /* Name — overlay default, static-below variant when .name-below set */
  .cam-name {
    position: absolute;
    left: 0;
    right: 0;
    bottom: 0;
    padding: 30px 14px 12px;
    background: linear-gradient(transparent, rgba(6, 8, 10, 0.82));
    display: flex;
    flex-direction: column;
    gap: 1px;
    z-index: 2;
  }
  .cn-name {
    font-size: 16px;
    font-weight: 600;
    color: #fff;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .cn-sub {
    font-size: 12px;
    color: rgba(255, 255, 255, 0.66);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .cam-tile.name-below .cam-name {
    position: static;
    padding: 9px 2px 0;
    background: none;
  }
  .cam-tile.name-below .cn-name {
    color: var(--text);
    font-size: 14px;
    font-weight: 500;
    letter-spacing: -0.1px;
  }
  .cam-tile.name-below .cn-sub {
    color: var(--text-2);
    font-family: 'JetBrains Mono', ui-monospace, monospace;
    font-size: 11px;
  }
  .cam-tile.name-below.offline-tile .cn-name {
    color: var(--text-2);
  }

  /* Desktop hover affordance — lift + "Open live" pill. Gated to hover-capable
     pointing devices so touchscreens (mobile grid) get nothing. */
  .cam-hover {
    position: absolute;
    inset: 0;
    z-index: 3;
    display: grid;
    place-items: center;
    background: rgba(8, 10, 12, 0.34);
    opacity: 0;
    pointer-events: none;
    transition: opacity 160ms ease;
  }
  .pchip {
    display: inline-flex;
    align-items: center;
    gap: 7px;
    padding: 9px 15px;
    border-radius: 999px;
    background: rgba(12, 14, 16, 0.6);
    border: 1px solid rgba(255, 255, 255, 0.22);
    color: #fff;
    font-size: 13px;
    font-weight: 600;
    backdrop-filter: blur(6px);
    -webkit-backdrop-filter: blur(6px);
  }
  .pchip :global(svg) {
    fill: #fff;
    stroke: none;
  }
  @media (hover: hover) and (pointer: fine) {
    .cam-tile:not(.offline-tile):hover .cam {
      transform: translateY(-2px);
      border: 1px solid var(--border-strong);
      box-shadow: 0 16px 34px -20px rgba(0, 0, 0, 0.7);
    }
    .cam-tile:not(.offline-tile):hover .cam-hover {
      opacity: 1;
    }
    .cam-tile.offline-tile {
      cursor: default;
    }
  }
</style>
