import { fetchStorage, type StorageCamera, type StorageMount } from '$lib/api'
import { ui } from '$lib/i18n/strings'

// storageStore holds the disk usage from GET /api/storage: per-mount figures
// plus per-camera recordings usage. It is not auto-inited anywhere —
// StorageSection calls load() on mount, and the Refresh button re-calls it.
// Static load, no polling.
class StorageStore {
  mounts = $state<StorageMount[]>([])
  cameras = $state<StorageCamera[]>([])
  loading = $state(false)
  error = $state<string | null>(null)

  async load() {
    this.loading = true
    this.error = null
    try {
      const info = await fetchStorage()
      this.mounts = info.mounts
      this.cameras = info.cameras
    } catch (err) {
      this.mounts = []
      this.cameras = []
      this.error = err instanceof Error ? err.message : ui.storageLoadError
    } finally {
      this.loading = false
    }
  }
}

export const storageStore = new StorageStore()
