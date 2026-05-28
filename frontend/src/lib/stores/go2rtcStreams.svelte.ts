import { fetchGo2RTCStreams } from '$lib/api'
import { ui } from '$lib/i18n/strings'

// go2rtcStreamsStore is a one-shot cache of go2rtc's /api/streams list,
// inited on /settings mount. No refresh method — if go2rtc was unreachable
// during init, the user reloads the page to retry.
class Go2RTCStreamsStore {
  streams = $state<string[]>([])
  loaded = $state(false)
  error = $state<string | null>(null)
  #initStarted = false

  async init() {
    if (this.#initStarted) return
    this.#initStarted = true
    try {
      this.streams = await fetchGo2RTCStreams()
      this.error = null
    } catch (err) {
      this.streams = []
      this.error = err instanceof Error ? err.message : ui.streamsLoadError
    } finally {
      this.loaded = true
    }
  }
}

export const go2rtcStreamsStore = new Go2RTCStreamsStore()
