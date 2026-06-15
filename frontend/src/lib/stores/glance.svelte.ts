import { fetchGlance, glanceHeartbeat, markAllGlanceSeen, markGlanceSeen } from '$lib/api'
import type { GlanceMoment } from '$lib/api'
import { prefsStore } from '$lib/stores/prefs.svelte'

// GLANCE_SURFACED_KEY persists the layout's "I surfaced the peek for
// this session" intent across an in-session location.reload() — most
// commonly the one the PWA service worker fires on autoUpdate. The
// flag is sessionStorage-scoped so it survives the reload but is
// cleared on a fresh PWA cold launch; without it the post-reload
// heartbeat reports the device as no-longer-away (the pre-reload
// ping marked it active), and the peek would not re-surface.
const GLANCE_SURFACED_KEY = 'skua:glance:surfaced'

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
  // user opens a moment or taps mark-all-seen. Clearing the surfaced
  // flag here means a later resume re-evaluates the away verdict
  // normally instead of immediately re-popping the peek.
  closePeek(): void {
    this.peekOpen = false
    this.clearSurfaced()
  }

  // markSeen flips seen=true on every matching moment optimistically,
  // recomputes the badge, and fires the POST without awaiting it. Errors
  // surface in the console only (fire-and-forget, same shape as prefs).
  async markSeen(ids: string[]): Promise<void> {
    if (ids.length === 0) return
    const idSet = new Set(ids)
    this.moments = this.moments.map((m) => (m.seen || !idSet.has(m.id) ? m : { ...m, seen: true }))
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
    // The user explicitly cleared the digest, so a subsequent resume
    // (or SW reload) must not re-pop the peek from a stale flag.
    this.clearSurfaced()
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

  // markSurfaced / wasSurfaced / clearSurfaced wrap the session-scoped
  // intent flag. All three are SSR-safe (sessionStorage is absent
  // during prerender) and silently swallow Storage errors (private
  // browsing on iOS Safari throws on quota or denied access). The
  // key is never exported — the methods are the only surface.
  markSurfaced(): void {
    if (typeof sessionStorage === 'undefined') return
    try {
      sessionStorage.setItem(GLANCE_SURFACED_KEY, '1')
    } catch {
      // sessionStorage is unavailable (private mode, quota); the
      // worst case is the peek does not re-surface after an
      // in-session SW reload — acceptable for a household glance UI.
    }
  }

  wasSurfaced(): boolean {
    if (typeof sessionStorage === 'undefined') return false
    try {
      return sessionStorage.getItem(GLANCE_SURFACED_KEY) === '1'
    } catch {
      return false
    }
  }

  clearSurfaced(): void {
    if (typeof sessionStorage === 'undefined') return
    try {
      sessionStorage.removeItem(GLANCE_SURFACED_KEY)
    } catch {
      // Same fail-soft rationale as markSurfaced.
    }
  }
}

export const glanceStore = new GlanceStore()
