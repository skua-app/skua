import { ackGlance, fetchGlance } from '$lib/api'
import type { Moment } from '$lib/api'
import { pickSeenThrough } from '$lib/glance'

class GlanceStore {
  lastSeen = $state<string | null>(null)
  unseenCount = $state(0)
  moments = $state<Moment[]>([])
  loaded = $state(false)
  peekOpen = $state(false)
  // Session-local set of representative event ids the user has already
  // tapped in the current peek. Used to dim rows after they've been
  // opened, so the user can tell which ones they've already watched in
  // this sitting. Not persisted and never sent to the server — server
  // seen-state is household-wide and only advances on dismiss().
  viewed = $state<Set<string>>(new Set<string>())

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

  // closePeek closes the sheet without acking the seen-state. Used when
  // the user navigates to a live focus view from a moment row — the
  // route change closes the sheet, but the household last_seen only
  // advances when the user explicitly dismisses the peek itself.
  closePeek() {
    this.peekOpen = false
  }

  // markViewed records that the user has tapped a row in this session.
  // A fresh Set is assigned so Svelte's $state proxy fires reactivity;
  // mutating the existing Set in place would not trigger updates.
  markViewed(eventId: string) {
    if (this.viewed.has(eventId)) return
    this.viewed = new Set([...this.viewed, eventId])
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
