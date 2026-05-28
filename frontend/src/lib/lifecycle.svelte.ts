// App-wide visibility/lifecycle hub. Consumers subscribe with onBackground
// (tab hidden / PWA snapshotted) and onForeground (resumed) callbacks; the
// hub owns the document listeners and the long-absence reload heuristic so
// each store doesn't reinvent it.
//
// Why this exists: iOS Safari freezes a PWA's JS context when backgrounded.
// Live WHEP PeerConnections and the SSE EventSource keep references that
// the webview snapshot captures in a broken state — on resume the page
// reads stale objects and silently fails (white/black screen). Tearing
// streams down before the freeze and re-opening on resume avoids that.

type Callback = () => void

// If the PWA was hidden for longer than this on resume, force a full
// reload instead of trusting the resumed JS context. Empirically iPhones
// can hold a snapshot for hours; modules, fetch caches, and SSE buffers
// past 30 min are not worth resurrecting.
const STALE_THRESHOLD_MS = 30 * 60 * 1000

class Lifecycle {
  #bgSubs = new Set<Callback>()
  #fgSubs = new Set<Callback>()
  #initStarted = false
  #hiddenAt = 0

  init() {
    if (this.#initStarted) return
    if (typeof document === 'undefined') return
    this.#initStarted = true
    document.addEventListener('visibilitychange', this.#onVisibility)
    // pagehide is the only reliable "going away" signal on iOS Safari —
    // it fires when the page enters bfcache or the tab is killed, whereas
    // unload often doesn't fire on mobile.
    window.addEventListener('pagehide', this.#onPageHide)
    window.addEventListener('pageshow', this.#onPageShow)
  }

  onBackground(fn: Callback): () => void {
    this.#bgSubs.add(fn)
    return () => this.#bgSubs.delete(fn)
  }

  onForeground(fn: Callback): () => void {
    this.#fgSubs.add(fn)
    return () => this.#fgSubs.delete(fn)
  }

  #onVisibility = () => {
    if (document.visibilityState === 'hidden') {
      this.#emitBackground()
    } else if (document.visibilityState === 'visible') {
      this.#emitForeground()
    }
  }

  #onPageHide = () => {
    this.#emitBackground()
  }

  #onPageShow = (ev: PageTransitionEvent) => {
    // persisted=true means restored from bfcache — JS context is stale by
    // definition. Treat the same as a long-absence resume.
    if (ev.persisted) {
      this.#checkStaleAndReload(true)
      return
    }
    this.#emitForeground()
  }

  #emitBackground() {
    if (this.#hiddenAt === 0) {
      this.#hiddenAt = Date.now()
    }
    for (const fn of this.#bgSubs) {
      try {
        fn()
      } catch (err) {
        console.error('[lifecycle] background subscriber threw:', err)
      }
    }
  }

  #emitForeground() {
    if (this.#checkStaleAndReload(false)) return
    this.#hiddenAt = 0
    for (const fn of this.#fgSubs) {
      try {
        fn()
      } catch (err) {
        console.error('[lifecycle] foreground subscriber threw:', err)
      }
    }
  }

  #checkStaleAndReload(forceBfcache: boolean): boolean {
    const hiddenMs = this.#hiddenAt === 0 ? 0 : Date.now() - this.#hiddenAt
    if (forceBfcache || hiddenMs > STALE_THRESHOLD_MS) {
      // Use replace so the user doesn't accumulate a history entry per
      // long-absence resume.
      window.location.reload()
      return true
    }
    return false
  }
}

export const lifecycle = new Lifecycle()
