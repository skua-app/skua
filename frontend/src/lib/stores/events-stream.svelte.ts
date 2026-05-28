import { fetchCameraNames } from '$lib/api'
import type { EventItem } from '$lib/api'
import { camerasStore } from '$lib/stores/cameras.svelte'
import { groupsStore } from '$lib/stores/groups.svelte'

// Cap the in-memory ring. The /events page paginates independently; this
// ring exists for focus-sidebar "Recent events" reactivity without re-fetch.
const RING_CAP = 50

type EndPayload = {
  id: string
  ended_at: string | null
  duration_seconds: number | null
}

class EventsStreamStore {
  latest = $state<EventItem[]>([])
  connected = $state(false)
  #source: EventSource | null = null
  #initStarted = false
  #backoffMs = 1000
  #reconnectTimer: ReturnType<typeof setTimeout> | null = null

  init() {
    if (this.#initStarted) return
    this.#initStarted = true
    this.#open()
  }

  // Tear the EventSource down when the PWA backgrounds. iOS Safari
  // freezes the JS context while the tab is hidden; a captured-mid-frame
  // EventSource resurfaces in a broken state on resume and never reconnects.
  close() {
    if (this.#reconnectTimer !== null) {
      clearTimeout(this.#reconnectTimer)
      this.#reconnectTimer = null
    }
    if (this.#source !== null) {
      this.#source.close()
      this.#source = null
    }
    this.connected = false
  }

  reopen() {
    // Reset backoff so a long-absence resume gets a fresh attempt.
    this.#backoffMs = 1000
    if (!this.#initStarted) {
      this.init()
      return
    }
    if (this.#source === null) {
      this.#open()
    }
  }

  #open() {
    if (typeof window === 'undefined' || typeof EventSource === 'undefined') return
    const es = new EventSource('/api/stream')
    this.#source = es

    es.addEventListener('open', () => {
      this.connected = true
      this.#backoffMs = 1000
    })

    es.addEventListener('error', () => {
      this.connected = false
      // EventSource auto-reconnects unless we're CLOSED — in which case
      // it never reopens on its own. Drive an exponential-backoff reopen
      // matching the backend's upstream loop (1, 2, 4, 8, 16, 30s cap).
      if (es.readyState === EventSource.CLOSED) {
        this.#scheduleReopen()
      }
    })

    es.addEventListener('event.new', (ev: MessageEvent) => {
      const item = this.#parse<EventItem>(ev.data)
      if (!item) return
      this.#prepend(item)
    })

    es.addEventListener('event.end', (ev: MessageEvent) => {
      const end = this.#parse<EndPayload>(ev.data)
      if (!end) return
      this.#applyEnd(end)
    })

    // camera.online / camera.offline are handled by camerasStore polling.
    // camera.added / camera.removed trigger a full re-fetch of the camera
    // set so refreshes initiated on another client surface within seconds.
    // reconnected is informational; no consumers yet.
    es.addEventListener('camera.added', () => this.#refetchCameraSet())
    es.addEventListener('camera.removed', () => this.#refetchCameraSet())
  }

  // Triggered by SSE camera.added / camera.removed. Each fetch is fire-and-
  // forget and isolated so one failure does not stall the others. The
  // payload is intentionally ignored — we always refetch the full set so
  // a missed event can't desync the local view.
  #refetchCameraSet() {
    camerasStore.refresh().catch(() => {})
    groupsStore.refresh().catch(() => {})
    fetchCameraNames().catch(() => {})
  }

  #scheduleReopen() {
    if (this.#reconnectTimer !== null) return
    const delay = this.#backoffMs
    this.#backoffMs = Math.min(this.#backoffMs * 2, 30_000)
    this.#reconnectTimer = setTimeout(() => {
      this.#reconnectTimer = null
      this.#source?.close()
      this.#source = null
      this.#open()
    }, delay)
  }

  #parse<T>(raw: string): T | null {
    try {
      return JSON.parse(raw) as T
    } catch {
      return null
    }
  }

  #prepend(item: EventItem) {
    // Dedupe: if a "new" arrives twice (e.g. after a brief reconnect),
    // keep the most recent copy in the head.
    const filtered = this.latest.filter((e) => e.id !== item.id)
    const next = [item, ...filtered]
    if (next.length > RING_CAP) next.length = RING_CAP
    this.latest = next
  }

  #applyEnd(end: EndPayload) {
    const idx = this.latest.findIndex((e) => e.id === end.id)
    if (idx === -1) return
    const current = this.latest[idx]
    if (!current) return
    const next = [...this.latest]
    next[idx] = {
      ...current,
      ended_at: end.ended_at,
      duration_seconds: end.duration_seconds
    }
    this.latest = next
  }
}

export const eventsStreamStore = new EventsStreamStore()
