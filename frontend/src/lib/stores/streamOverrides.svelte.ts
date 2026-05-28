import { fetchStreamOverrides, setStreamOverride } from '$lib/api'
import type { Override, StreamOverridesMap } from '$lib/api'

// streamOverridesStore mirrors groupsStore's shape: lazy init on /settings
// mount, optimistic local update on save, server-revert on failure so the
// bound select snaps back to the persisted value.
class StreamOverridesStore {
  overrides = $state<StreamOverridesMap>({})
  loaded = $state(false)
  #initStarted = false

  async init() {
    if (this.#initStarted) return
    this.#initStarted = true
    await this.refresh()
  }

  async refresh() {
    try {
      this.overrides = await fetchStreamOverrides()
    } catch {
      // Keep last-known state on transient errors; the editor surfaces
      // them inline via the save() rethrow path.
    } finally {
      this.loaded = true
    }
  }

  get(camId: string): Override {
    return this.overrides[camId] ?? { main: '', sub: '' }
  }

  async save(camId: string, main: string, sub: string): Promise<void> {
    const trimmedMain = main.trim()
    const trimmedSub = sub.trim()

    // Optimistic write so the bound <select> updates immediately.
    const next = { ...this.overrides }
    if (trimmedMain === '' && trimmedSub === '') {
      delete next[camId]
    } else {
      next[camId] = { main: trimmedMain, sub: trimmedSub }
    }
    this.overrides = next

    try {
      const saved = await setStreamOverride(camId, trimmedMain, trimmedSub)
      const after = { ...this.overrides }
      if (saved.main === '' && saved.sub === '') {
        delete after[camId]
      } else {
        after[camId] = saved
      }
      this.overrides = after
    } catch (err) {
      // Revert from server so the optimistic write doesn't linger past
      // the user's intent; re-throw for the caller to display.
      await this.refresh()
      throw err
    }
  }
}

export const streamOverridesStore = new StreamOverridesStore()
