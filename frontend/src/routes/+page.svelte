<script lang="ts">
  import DesktopGrid from '$lib/screens/DesktopGrid.svelte'
  import MobileGrid from '$lib/screens/MobileGrid.svelte'

  let width = $state(typeof window !== 'undefined' ? window.innerWidth : 0)
  const isDesktop = $derived(width >= 900)
</script>

<svelte:window bind:innerWidth={width} />

{#if isDesktop}
  <DesktopGrid />
{:else}
  <MobileGrid />
{/if}
