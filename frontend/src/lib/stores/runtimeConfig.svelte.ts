import { fetchRuntimeConfig } from '$lib/api'
import type { RuntimeConfig, RuntimeConfigURLs } from '$lib/api'

const EMPTY_URLS: RuntimeConfigURLs = { frigate_url: '', frigate_ui_url: '', go2rtc_url: '' }

// runtimeConfigStore holds the /api/runtime-config payload for the
// Connection section in /settings. Lazy-inited from the /settings page
// (mirroring streamOverridesStore / go2rtcStreamsStore) so non-settings
// sessions don't pay the fetch cost. A single-flight guard prevents
// duplicate inits from the rail vs the section component both calling
// init() in the same mount.
class RuntimeConfigStore {
  effective = $state<RuntimeConfigURLs>(EMPTY_URLS)
  overlay = $state<RuntimeConfigURLs>(EMPTY_URLS)
  locked = $state({ frigate_url: false, frigate_ui_url: false, go2rtc_url: false })
  loaded = $state(false)
  loadError = $state<string | null>(null)
  #initStarted = false

  async init() {
    if (this.#initStarted) return
    this.#initStarted = true
    await this.refresh()
  }

  async refresh() {
    this.loadError = null
    try {
      const cfg = await fetchRuntimeConfig()
      this.apply(cfg)
    } catch (err) {
      this.loadError = err instanceof Error ? err.message : 'Could not load runtime config'
    } finally {
      this.loaded = true
    }
  }

  // apply lets the ConnectionSection update the store after a successful
  // save without paying a second GET round-trip — saveRuntimeConfig
  // already returns the post-save shape.
  apply(cfg: RuntimeConfig) {
    this.effective = cfg.effective
    this.overlay = cfg.overlay
    this.locked = cfg.locked
  }
}

export const runtimeConfigStore = new RuntimeConfigStore()
