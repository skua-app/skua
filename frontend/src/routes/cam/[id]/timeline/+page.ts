import { camerasStore } from '$lib/stores/cameras.svelte'

export async function load() {
  if (camerasStore.cameras.length === 0) {
    await camerasStore.refresh()
  }
}
