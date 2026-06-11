import { fetchGlance, glanceHeartbeat, markAllGlanceSeen, markGlanceSeen } from '$lib/api'
import type { GlanceMoment } from '$lib/api'
import { prefsStore } from '$lib/stores/prefs.svelte'

class GlanceStore {
  unseenCount = $state(0)
  moments = $state<GlanceMoment[]>([])
  loaded = $state(false)
  peekOpen = $state(false)

  async load(): Promise<void> {
    try {
      const resp = await fetchGlance(prefsStore.glanceWindowHours, prefsStore.glanceMaxMoments)
      this.unseenCount = resp.unseen_count
      this.moments = resp.moments
    } catch (err) {
      console.error('[glance] load failed, using defaults:', err)
    } finally {
      this.loaded = true
    }
  }

  openPeek(): void {
    this.peekOpen = true
  }

  // closePeek closes the sheet without marking anything seen — that is
  // the explicit Model B contract: seen-state advances only when the
  // user opens a moment or taps mark-all-seen.
  closePeek(): void {
    this.peekOpen = false
  }

  // markSeen flips seen=true on every matching moment optimistically,
  // recomputes the badge, and fires the POST without awaiting it. Errors
  // surface in the console only (fire-and-forget, same shape as prefs).
  async markSeen(ids: string[]): Promise<void> {
    if (ids.length === 0) return
    const idSet = new Set(ids)
    this.moments = this.moments.map((m) =>
      m.seen || !idSet.has(m.representative_event_id) ? m : { ...m, seen: true }
    )
    this.unseenCount = this.moments.filter((m) => !m.seen).length
    try {
      await markGlanceSeen(ids)
    } catch (err) {
      console.error('[glance] markGlanceSeen failed:', err)
    }
  }

  markOneSeen(id: string): Promise<void> {
    return this.markSeen([id])
  }

  // markAllSeen advances the household watermark non-destructively: every
  // currently-listed moment flips to seen=true and the badge clears, but
  // the list stays put so the user can re-watch what they just dismissed.
  // The peek stays open — closing is a separate user action.
  async markAllSeen(): Promise<void> {
    this.moments = this.moments.map((m) => (m.seen ? m : { ...m, seen: true }))
    this.unseenCount = 0
    try {
      await markAllGlanceSeen()
    } catch (err) {
      console.error('[glance] markAllGlanceSeen failed:', err)
    }
  }

  // heartbeat pings the device session and returns the server's away
  // verdict for this device. Failures fall back to "not away" so a
  // transient error never spuriously pops the peek.
  async heartbeat(): Promise<boolean> {
    try {
      return await glanceHeartbeat()
    } catch (err) {
      console.error('[glance] heartbeat failed:', err)
      return false
    }
  }
}

export const glanceStore = new GlanceStore()
