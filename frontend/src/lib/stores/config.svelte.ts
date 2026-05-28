import { fetchConfig } from '$lib/api'

class ConfigStore {
  frigateUIURL = $state('')
  loaded = $state(false)
  #initStarted = false

  async init() {
    if (this.#initStarted) return
    this.#initStarted = true
    try {
      const cfg = await fetchConfig()
      this.frigateUIURL = cfg.frigate_ui_url
    } catch {
      // Best-effort: deep-links degrade to no-op rather than blocking the app.
    } finally {
      this.loaded = true
    }
  }
}

export const configStore = new ConfigStore()
