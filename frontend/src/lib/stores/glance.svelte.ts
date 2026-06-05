import { fetchGlance, markGlanceSeen } from '$lib/api'
import type { GlanceMoment } from '$lib/api'

class GlanceStore {
  unseenCount = $state(0)
  moments = $state<GlanceMoment[]>([])
  loaded = $state(false)
  peekOpen = $state(false)
  // Session-local set of representative event ids that have already been
  // surfaced through the peek in this app session. We add ids on every
  // openPeek() so the same moment doesn't auto-resurface on every resume
  // until the user marks it seen — only genuinely new (post-open) unseen
  // moments trigger the lifecycle re-open.
  presentedIds = $state<Set<string>>(new Set<string>())

  async load(): Promise<void> {
    try {
      const resp = await fetchGlance()
      this.unseenCount = resp.unseen_count
      this.moments = resp.moments
    } catch (err) {
      console.error('[glance] load failed, using defaults:', err)
    } finally {
      this.loaded = true
    }
  }

  // hasNewUnseen returns true when there is at least one unseen moment
  // the current session has not already surfaced through the peek. Used
  // both by the cold-open trigger and by the iOS-resume re-open path.
  hasNewUnseen(): boolean {
    return this.moments.some((m) => !m.seen && !this.presentedIds.has(m.representative_event_id))
  }

  openPeek(): void {
    this.peekOpen = true
    // Remember every currently-unseen moment as already-presented so it
    // doesn't auto-reopen later in the same session if the user closes
    // without marking it seen.
    const next = new Set(this.presentedIds)
    for (const m of this.moments) {
      if (!m.seen) next.add(m.representative_event_id)
    }
    this.presentedIds = next
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

  // markAllSeen marks every currently-unseen moment as seen in one batch
  // then closes the peek. Computed from the moment list so callers don't
  // need to enumerate ids themselves.
  async markAllSeen(): Promise<void> {
    const ids = this.moments.filter((m) => !m.seen).map((m) => m.representative_event_id)
    if (ids.length > 0) {
      await this.markSeen(ids)
    }
    this.closePeek()
  }
}

export const glanceStore = new GlanceStore()
