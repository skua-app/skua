import { fetchCameras } from '$lib/api'
import type { Camera } from '$lib/api'

class CamerasStore {
  cameras = $state<Camera[]>([])
  loading = $state(true)
  error = $state<string | null>(null)

  #interval: ReturnType<typeof setInterval> | null = null

  async refresh() {
    try {
      this.cameras = await fetchCameras()
      this.error = null
    } catch (e) {
      this.error = e instanceof Error ? e.message : 'Unknown error'
    } finally {
      this.loading = false
    }
  }

  startPolling() {
    // Idempotent so the lifecycle onForeground re-arm can't stack
    // intervals on top of an existing one.
    if (this.#interval !== null) return
    this.refresh()
    this.#interval = setInterval(() => this.refresh(), 15_000)
  }

  stopPolling() {
    if (this.#interval !== null) {
      clearInterval(this.#interval)
      this.#interval = null
    }
  }
}

export const camerasStore = new CamerasStore()
