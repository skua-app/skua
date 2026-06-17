import { fetchRecordingsSummary } from '$lib/api'
import type { RecordingsSummary } from '$lib/api'
import { flattenSummary, type TimelineHour } from '$lib/timeline'

function resolveBrowserTimezone(): string | undefined {
  try {
    const tz = Intl.DateTimeFormat().resolvedOptions().timeZone
    return tz || undefined
  } catch {
    return undefined
  }
}

class TimelineStore {
  summary = $state<RecordingsSummary>([])
  hours = $state<TimelineHour[]>([])
  loading = $state(false)
  error = $state<string | null>(null)
  camId = $state<string | null>(null)

  async load(camId: string): Promise<void> {
    this.camId = camId
    this.loading = true
    this.error = null
    try {
      const tz = resolveBrowserTimezone()
      const resp = await fetchRecordingsSummary(camId, tz)
      this.summary = resp
      this.hours = flattenSummary(resp)
    } catch (err) {
      console.error('[timeline] load failed:', err)
      this.summary = []
      this.hours = []
      this.error = err instanceof Error ? err.message : 'failed to load timeline'
    } finally {
      this.loading = false
    }
  }
}

export const timelineStore = new TimelineStore()
