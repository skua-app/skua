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

  const gridModeOptions = [
    { value: 'hd' as const, label: 'HD' },
    { value: 'eco' as const, label: 'ECO' }
  ]
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
        <IconBtn
          icon="refresh"
          label={ui.refreshCameras}
          size={32}
          onclick={() => camerasStore.refresh()}
        />
      </div>
    </div>

    <div class="dg-grid" style:--cols={prefsStore.desktopColumns}>
      {#each cameras as camera, i (camera.id)}
        <CameraTile
          {camera}
          index={i}
          nameStyle={prefsStore.nameStyle}
          showTimestamp={prefsStore.showTimestamp}
          onclick={() => goto(`/cam/${camera.id}`)}
        />
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

  .dg-grid {
    display: grid;
    grid-template-columns: repeat(var(--cols, 4), 1fr);
    gap: 18px;
  }
</style>
