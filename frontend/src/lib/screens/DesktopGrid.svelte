<script lang="ts">
  import { goto } from '$app/navigation'
  import CameraTile from '$lib/components/CameraTile.svelte'
  import Icon from '$lib/components/Icon.svelte'
  import Mono from '$lib/components/Mono.svelte'
  import Segmented from '$lib/components/Segmented.svelte'
  import { camerasStore } from '$lib/stores/cameras.svelte'
  import { prefsStore } from '$lib/stores/prefs.svelte'
  import { groupsStore } from '$lib/stores/groups.svelte'
  import { ui } from '$lib/i18n/strings'
  import type { Camera, DesktopColumns } from '$lib/api'

  // Drop a stale gridFilter (group deleted on another device) silently.
  $effect(() => {
    if (!groupsStore.loaded) return
    const id = prefsStore.gridFilter
    if (id !== null && !groupsStore.groups.some((g) => g.id === id)) {
      prefsStore.setGridFilter(null)
    }
  })

  const cameras = $derived(
    prefsStore.gridFilter
      ? camerasStore.cameras.filter((c) => c.groups.includes(prefsStore.gridFilter!))
      : camerasStore.cameras
  )
  const allCameras = $derived(camerasStore.cameras)
  const onlineCount = $derived(allCameras.filter((c) => c.online).length)
  const offlineCount = $derived(allCameras.length - onlineCount)

  const activeGroup = $derived(
    prefsStore.gridFilter ? groupsStore.groups.find((g) => g.id === prefsStore.gridFilter) : null
  )
  function cameraCount(groupId: string): number {
    return groupsStore.byId(groupId)?.camera_ids.length ?? 0
  }

  let filterOpen = $state(false)
  function selectGroup(id: string | null) {
    prefsStore.setGridFilter(id)
    filterOpen = false
  }
  $effect(() => {
    if (!filterOpen) return
    function onKey(e: KeyboardEvent) {
      if (e.key === 'Escape') filterOpen = false
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  })

  type GridItem =
    | { kind: 'label'; key: string; name: string }
    | { kind: 'tile'; key: string; camera: Camera; index: number }

  // When a filter is active, render a flat wall; when not, group tiles by
  // room (groups in store order, then a final "ungrouped" bucket). Every
  // tile carries a running flat index so CameraTile's stagger stays unique
  // and stable across rooms.
  const gridItems = $derived.by<GridItem[]>(() => {
    if (prefsStore.gridFilter !== null) {
      return cameras.map((c, i) => ({ kind: 'tile' as const, key: c.id, camera: c, index: i }))
    }
    const out: GridItem[] = []
    let idx = 0
    for (const g of groupsStore.groups) {
      const tiles = camerasStore.cameras.filter((c) => c.groups.includes(g.id))
      if (tiles.length === 0) continue
      out.push({ kind: 'label', key: `label:${g.id}`, name: g.name })
      for (const c of tiles) {
        out.push({ kind: 'tile', key: c.id, camera: c, index: idx++ })
      }
    }
    const ungrouped = camerasStore.cameras.filter((c) => c.groups.length === 0)
    if (ungrouped.length > 0) {
      out.push({ kind: 'label', key: 'label:__ungrouped', name: ui.ungrouped })
      for (const c of ungrouped) {
        out.push({ kind: 'tile', key: c.id, camera: c, index: idx++ })
      }
    }
    return out
  })

  const gridModeOptions = [
    { value: 'hd' as const, label: 'HD' },
    { value: 'eco' as const, label: 'ECO' }
  ]

  type Density = 'cozy' | 'compact' | 'dense'
  const densityOptions = [
    { value: 'cozy' as const, label: ui.densityCozy },
    { value: 'compact' as const, label: ui.densityCompact },
    { value: 'dense' as const, label: ui.densityDense }
  ]
  const density = $derived<Density>(
    prefsStore.desktopColumns <= 3 ? 'cozy' : prefsStore.desktopColumns === 4 ? 'compact' : 'dense'
  )
  // Match the prototype's DENSITY map (cozy 360 / compact 288 / dense 224)
  // — tile min-width drives the auto-fill grid.
  const tilePx = $derived(density === 'cozy' ? 360 : density === 'compact' ? 288 : 224)
  function setDensity(v: Density) {
    const n = v === 'cozy' ? 3 : v === 'compact' ? 4 : 5
    prefsStore.setDesktopColumns(n as DesktopColumns)
  }
</script>

<div class="desktop-grid">
  <main class="dg-main">
    <div class="dk-pagehead">
      <div class="dk-titlewrap">
        <div class="dk-h1">{ui.cameras}</div>
        <div class="dk-sub">
          <span class="gd" aria-hidden="true"></span>
          {onlineCount}
          {ui.online} · {offlineCount}
          {ui.offline}
        </div>
      </div>
      <div class="dk-controls">
        <div class="dk-drop">
          <button
            type="button"
            class="dk-dropbtn"
            class:open={filterOpen}
            aria-label={ui.groupFilterLabel}
            aria-haspopup="listbox"
            aria-expanded={filterOpen}
            onclick={() => (filterOpen = !filterOpen)}
          >
            <span class="db-label">{activeGroup ? activeGroup.name : ui.filterAllCams}</span>
            <span class="ck" aria-hidden="true"><Icon name="chevDown" size={16} /></span>
          </button>
          {#if filterOpen}
            <button
              type="button"
              class="dk-catch"
              tabindex={-1}
              aria-label={ui.close}
              onclick={() => (filterOpen = false)}
            ></button>
            <ul class="dk-menu" role="listbox">
              <li>
                <button
                  type="button"
                  class="dk-opt"
                  class:on={prefsStore.gridFilter === null}
                  role="option"
                  aria-selected={prefsStore.gridFilter === null}
                  onclick={() => selectGroup(null)}
                >
                  <span class="ck" aria-hidden="true">
                    {#if prefsStore.gridFilter === null}<Icon name="check" size={15} />{/if}
                  </span>
                  <span class="nm">{ui.filterAllCams}</span>
                  <Mono size={11} color="inherit" class="ct">{allCameras.length}</Mono>
                </button>
              </li>
              {#each groupsStore.groups as g (g.id)}
                <li>
                  <button
                    type="button"
                    class="dk-opt"
                    class:on={prefsStore.gridFilter === g.id}
                    role="option"
                    aria-selected={prefsStore.gridFilter === g.id}
                    onclick={() => selectGroup(g.id)}
                  >
                    <span class="ck" aria-hidden="true">
                      {#if prefsStore.gridFilter === g.id}<Icon name="check" size={15} />{/if}
                    </span>
                    <span class="nm">{g.name}</span>
                    <Mono size={11} color="inherit" class="ct">{cameraCount(g.id)}</Mono>
                  </button>
                </li>
              {/each}
            </ul>
          {/if}
        </div>
        <Segmented
          value={prefsStore.gridMode}
          options={gridModeOptions}
          onChange={(v) => prefsStore.setGridMode(v)}
        />
        <div class="dg-density" aria-label={ui.gridDensityLabel}>
          <Segmented value={density} options={densityOptions} onChange={setDensity} />
        </div>
      </div>
    </div>

    <div
      class="dg-wall"
      class:name-below={prefsStore.nameStyle === 'below'}
      style:--tile="{tilePx}px"
    >
      {#each gridItems as item (item.key)}
        {#if item.kind === 'label'}
          <div class="dg-roomlabel">
            <span class="rl-text">{item.name}</span>
            <span class="ln" aria-hidden="true"></span>
          </div>
        {:else}
          <CameraTile
            camera={item.camera}
            index={item.index}
            nameStyle={prefsStore.nameStyle}
            showTimestamp={prefsStore.showTimestamp}
            onclick={() => goto(`/cam/${item.camera.id}`)}
          />
        {/if}
      {/each}
    </div>
  </main>
</div>

<style>
  .desktop-grid {
    width: 100%;
    display: flex;
    flex-direction: column;
  }
  .dg-main {
    flex: 1;
    padding: 24px 28px;
  }

  /* dk-pagehead — large title + online/offline sub on the left; controls right */
  .dk-pagehead {
    display: flex;
    align-items: flex-end;
    justify-content: space-between;
    gap: 18px;
    margin-bottom: 20px;
  }
  .dk-titlewrap {
    display: flex;
    align-items: baseline;
    gap: 12px;
    min-width: 0;
  }
  .dk-h1 {
    font-size: 28px;
    font-weight: 600;
    letter-spacing: -0.6px;
    line-height: 1.05;
    color: var(--text);
  }
  .dk-sub {
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: 14px;
    color: var(--text-2);
  }
  .dk-sub .gd {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: var(--online);
    display: inline-block;
  }
  .dk-controls {
    display: flex;
    align-items: center;
    gap: 10px;
    flex-wrap: wrap;
  }
  .dg-density {
    display: inline-flex;
  }

  /* dk-drop — group dropdown */
  .dk-drop {
    position: relative;
  }
  .dk-dropbtn {
    display: inline-flex;
    align-items: center;
    gap: 9px;
    height: 40px;
    padding: 0 17px;
    border-radius: 999px;
    border: 1px solid var(--border);
    background: var(--surface);
    color: var(--text);
    font-size: 14px;
    font-weight: 500;
    font-family: inherit;
    cursor: pointer;
    transition:
      border-color 0.15s ease,
      background 0.15s ease;
  }
  .dk-dropbtn:hover {
    border-color: var(--border-strong);
  }
  .dk-dropbtn.open {
    background: var(--accent-soft);
    border-color: transparent;
    color: var(--accent-ink);
  }
  .dk-dropbtn .db-label {
    min-width: 0;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .dk-dropbtn .ck {
    color: var(--text-3);
    display: inline-flex;
    transition: transform 0.2s ease;
  }
  .dk-dropbtn.open .ck {
    color: var(--accent-ink);
    transform: rotate(180deg);
  }
  .dk-dropbtn:active {
    transform: translateY(1px);
  }

  .dk-catch {
    position: fixed;
    inset: 0;
    z-index: 24;
    background: transparent;
    border: none;
    padding: 0;
    cursor: default;
  }
  .dk-menu {
    list-style: none;
    margin: 0;
    position: absolute;
    top: calc(100% + 8px);
    left: 0;
    z-index: 25;
    width: 250px;
    max-height: 340px;
    overflow-y: auto;
    background: var(--elev);
    border: 1px solid var(--border-strong);
    border-radius: 14px;
    box-shadow: var(--shadow);
    padding: 6px;
    animation: dkpop 0.15s ease;
  }
  @keyframes dkpop {
    from {
      transform: translateY(-6px);
      opacity: 0;
    }
    to {
      transform: none;
      opacity: 1;
    }
  }
  .dk-opt {
    display: flex;
    align-items: center;
    gap: 10px;
    width: 100%;
    padding: 10px 11px;
    border: none;
    background: transparent;
    border-radius: 10px;
    font-size: 14.5px;
    font-family: inherit;
    color: var(--text);
    text-align: left;
    cursor: pointer;
  }
  .dk-opt:hover {
    background: var(--surface-2);
  }
  .dk-opt.on {
    background: var(--accent-soft);
  }
  .dk-opt .ck {
    width: 16px;
    flex: 0 0 16px;
    color: var(--accent-ink);
    display: inline-flex;
  }
  .dk-opt .nm {
    flex: 1 1 auto;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .dk-opt.on .nm {
    color: var(--accent-ink);
    font-weight: 500;
  }
  .dk-opt :global(.ct) {
    flex: 0 0 auto;
    color: var(--text-2);
    background: var(--surface-2);
    border-radius: 999px;
    padding: 2px 9px;
  }
  .dk-opt.on :global(.ct) {
    color: var(--accent-ink);
    background: transparent;
    box-shadow: inset 0 0 0 1px var(--accent-soft);
  }

  /* Calm desktop wall — auto-fill with tile min-width driven by density. */
  .dg-wall {
    display: grid;
    gap: 14px;
    grid-template-columns: repeat(auto-fill, minmax(var(--tile, 280px), 1fr));
  }
  .dg-roomlabel {
    grid-column: 1 / -1;
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 14px 2px 2px;
    font-family: 'JetBrains Mono', ui-monospace, monospace;
    font-size: 11px;
    font-weight: 500;
    letter-spacing: 1.4px;
    text-transform: uppercase;
    color: var(--text-3);
  }
  .dg-roomlabel:first-child {
    padding-top: 0;
  }
  .dg-roomlabel .ln {
    flex: 1 1 auto;
    height: 1px;
    background: var(--border);
  }
  .rl-text {
    flex: 0 0 auto;
  }
</style>
