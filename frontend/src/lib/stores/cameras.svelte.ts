import { fetchCameras, ApiError, type ApiErrorKind } from '$lib/api'
import type { Camera } from '$lib/api'

const POLL_OK_MS = 15_000
const POLL_RETRY_MS = 5_000

class CamerasStore {
  cameras = $state<Camera[]>([])
  loading = $state(true)
  error = $state<string | null>(null)
  // Reachability signal consumed by ConnectionOverlay.
  reachable = $state(true)
  hasLoaded = $state(false)
  errorKind = $state<ApiErrorKind | null>(null)

  #interval: ReturnType<typeof setInterval> | null = null
  #intervalMs = POLL_OK_MS

  async refresh() {
    try {
      this.cameras = await fetchCameras()
      this.error = null
      this.reachable = true
      this.hasLoaded = true
      this.errorKind = null
      this.#ensureCadence(POLL_OK_MS)
    } catch (e) {
      this.error = e instanceof Error ? e.message : 'Unknown error'
      this.reachable = false
      this.errorKind = e instanceof ApiError ? e.kind : 'unknown'
      // While failing, retry faster so a transient blip clears quickly
      // without stacking timers.
      this.#ensureCadence(POLL_RETRY_MS)
    } finally {
      this.loading = false
    }
  }

  // Idempotent cadence switch — only restarts the timer when the interval
  // actually changes, so a string of failures doesn't pile up timers.
  #ensureCadence(nextMs: number) {
    if (this.#interval === null) return
    if (this.#intervalMs === nextMs) return
    clearInterval(this.#interval)
    this.#intervalMs = nextMs
    this.#interval = setInterval(() => this.refresh(), nextMs)
  }

  startPolling() {
    // Idempotent so the lifecycle onForeground re-arm can't stack
    // intervals on top of an existing one.
    if (this.#interval !== null) return
    this.refresh()
    this.#intervalMs = POLL_OK_MS
    this.#interval = setInterval(() => this.refresh(), this.#intervalMs)
  }

  stopPolling() {
    if (this.#interval !== null) {
      clearInterval(this.#interval)
      this.#interval = null
    }
  }

  // Reorders `cameras` in place to match the supplied id list. Used by
  // the Settings → Cameras drag list to make the grid follow a drop
  // instantly, before the PUT /api/camera-order round-trip resolves. Ids
  // not present in the supplied list keep their relative position at the
  // end so we never silently drop a camera if the caller is stale.
  applyLocalOrder(orderedIds: string[]) {
    const byID = new Map(this.cameras.map((c) => [c.id, c] as const))
    const placed = new Set<string>()
    const next: Camera[] = []
    for (const id of orderedIds) {
      const cam = byID.get(id)
      if (cam && !placed.has(id)) {
        next.push(cam)
        placed.add(id)
      }
    }
    for (const cam of this.cameras) {
      if (!placed.has(cam.id)) next.push(cam)
    }
    this.cameras = next
  }
}

export const camerasStore = new CamerasStore()
