import { ackGlance, fetchGlance } from '$lib/api'
import type { Moment } from '$lib/api'
import { pickSeenThrough } from '$lib/glance'

class GlanceStore {
  lastSeen = $state<string | null>(null)
  unseenCount = $state(0)
  moments = $state<Moment[]>([])
  loaded = $state(false)
  peekOpen = $state(false)

  async load() {
    try {
      const resp = await fetchGlance()
      this.lastSeen = resp.last_seen
      this.unseenCount = resp.unseen_count
      this.moments = resp.moments
    } catch (err) {
      console.error('[glance] load failed, using defaults:', err)
    } finally {
      this.loaded = true
    }
  }

  openPeek() {
    this.peekOpen = true
  }

  // dismiss is the single path that marks everything seen. The sheet
  // closes and the badge clears immediately (optimistic) so the user
  // never sees stale UI; the ack POST is fire-and-forget on the
  // optimistic path, errors surface in the console (same pattern as
  // prefs persistence).
  async dismiss() {
    this.peekOpen = false
    this.unseenCount = 0
    const seenThrough = pickSeenThrough(this.moments)
    if (seenThrough === null) return
    try {
      const resp = await ackGlance(seenThrough)
      this.lastSeen = resp.last_seen
    } catch (err) {
      console.error('[glance] ack failed:', err)
    }
  }
}

export const glanceStore = new GlanceStore()
