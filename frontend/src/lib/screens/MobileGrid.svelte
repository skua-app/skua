<script lang="ts">
  import { goto } from '$app/navigation'
  import CameraTile from '$lib/components/CameraTile.svelte'
  import { camerasStore } from '$lib/stores/cameras.svelte'
  import { prefsStore } from '$lib/stores/prefs.svelte'
  import { groupsStore } from '$lib/stores/groups.svelte'

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
</script>

<div class="mobile-grid">
  <div class="grid" style:--cols={prefsStore.mobileColumns}>
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
</div>

<style>
  .mobile-grid {
    width: 100%;
    display: flex;
    flex-direction: column;
  }
  .grid {
    flex: 1;
    display: grid;
    grid-template-columns: repeat(var(--cols, 1), 1fr);
    gap: 14px;
    margin-top: 2px;
    padding: 14px 14px calc(env(safe-area-inset-bottom, 0px) + 96px);
  }
</style>
