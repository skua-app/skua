<script lang="ts">
  import { goto } from '$app/navigation'
  import CameraTile from '$lib/components/CameraTile.svelte'
  import IconBtn from '$lib/components/IconBtn.svelte'
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

  type GridItem =
    | { kind: 'label'; key: string; name: string }
    | { kind: 'tile'; key: string; camera: Camera; index: number }

  // When a filter is active, render a flat grid; when not, group tiles by
  // room (groups in store order, then a final "ungrouped" bucket). Every
  // tile carries a running flat index so CameraTile's stagger stays
  // unique and stable across rooms.
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
  function setDensity(v: Density) {
    const n = v === 'cozy' ? 3 : v === 'compact' ? 4 : 5
    prefsStore.setDesktopColumns(n as DesktopColumns)
  }
</script>

<div class="desktop-grid">
  <main class="dg-main">
    <div class="dg-section-header">
      <div class="dg-section-title">
        <Mono size={13} weight={500} color="var(--text-2)" letterSpacing={0.3} uppercase>
          {ui.cameras}
        </Mono>
        <Mono color="var(--text-3)" size={11}>· {cameras.length}</Mono>
      </div>
      <div class="dg-section-controls">
        <div class="dg-filters">
          <button
            type="button"
            class="filter-chip"
            class:active={prefsStore.gridFilter === null}
            onclick={() => prefsStore.setGridFilter(null)}
          >
            {ui.filterAll}
          </button>
          {#each groupsStore.groups as g (g.id)}
            <button
              type="button"
              class="filter-chip"
              class:active={prefsStore.gridFilter === g.id}
              onclick={() => prefsStore.setGridFilter(g.id)}
            >
              {g.name}
            </button>
          {/each}
        </div>
        <div class="dg-divider"></div>
        <Segmented
          value={prefsStore.gridMode}
          options={gridModeOptions}
          onChange={(v) => prefsStore.setGridMode(v)}
        />
        <div class="dg-density" aria-label={ui.gridDensityLabel}>
          <Segmented value={density} options={densityOptions} onChange={setDensity} />
        </div>
        <IconBtn
          icon="refresh"
          label={ui.refreshCameras}
          size={32}
          onclick={() => camerasStore.refresh()}
        />
      </div>
    </div>

    <div class="dg-grid" style:--cols={prefsStore.desktopColumns}>
      {#each gridItems as item (item.key)}
        {#if item.kind === 'label'}
          <div class="dg-room-label">
            <Mono size={11} weight={500} color="var(--text-3)" letterSpacing={1.4} uppercase>
              {item.name}
            </Mono>
            <span class="dg-room-rule" aria-hidden="true"></span>
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

  .dg-section-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 18px;
    gap: 16px;
  }
  .dg-section-title {
    display: flex;
    align-items: baseline;
    gap: 10px;
  }
  .dg-section-controls {
    display: flex;
    align-items: center;
    gap: 14px;
  }

  .dg-filters {
    display: flex;
    gap: 6px;
  }
  .filter-chip {
    padding: 4px 10px;
    font-size: 11px;
    color: var(--text-3);
    background: transparent;
    border: 1px solid transparent;
    border-radius: 6px;
    cursor: pointer;
    font-family: inherit;
    transition:
      color 120ms,
      border-color 120ms,
      background 120ms;
  }
  .filter-chip:hover {
    color: var(--text-2);
  }
  .filter-chip.active {
    color: var(--accent);
    border-color: color-mix(in oklab, var(--accent) 50%, transparent);
    background: color-mix(in oklab, var(--accent) 14%, transparent);
  }

  .dg-divider {
    width: 1px;
    height: 14px;
    background: var(--border);
  }
  .dg-density {
    display: inline-flex;
  }

  .dg-grid {
    display: grid;
    grid-template-columns: repeat(var(--cols, 4), 1fr);
    gap: 18px;
  }
  .dg-room-label {
    grid-column: 1 / -1;
    display: flex;
    align-items: center;
    gap: 12px;
    margin-top: 6px;
  }
  .dg-room-label:first-child {
    margin-top: 0;
  }
  .dg-room-rule {
    flex: 1;
    height: 1px;
    background: var(--border);
  }
</style>
