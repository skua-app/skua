import type { RecordingsSummary } from '$lib/api'

// TimelineHour is one wall-clock hour of recording, normalised for the
// scrubber UI. `hourStart` is local midnight + N hours, `recordedFraction`
// is duration/3600 clamped to [0,1] so the bar height never overshoots.
export type TimelineHour = {
  hourStart: Date
  recordedFraction: number
  events: number
  motion: number
}

// flattenSummary turns Frigate's per-day buckets into a single flat array
// sorted ascending by hourStart. The local-time parse is correct only when
// the upstream summary was requested with the browser's IANA timezone —
// the store is responsible for passing that timezone to the BFF.
// flattenSummary itself stays pure: no environment reads, no fetch, no
// side effects.
export function flattenSummary(summary: RecordingsSummary): TimelineHour[] {
  const out: TimelineHour[] = []
  for (const day of summary) {
    if (!day.hours || day.hours.length === 0) continue
    for (const h of day.hours) {
      const hourStart = new Date(`${day.day}T${h.hour}:00:00`)
      const fraction = h.duration / 3600
      const recordedFraction = fraction < 0 ? 0 : fraction > 1 ? 1 : fraction
      out.push({
        hourStart,
        recordedFraction,
        events: h.events,
        motion: h.motion
      })
    }
  }
  out.sort((a, b) => a.hourStart.getTime() - b.hourStart.getTime())
  return out
}
