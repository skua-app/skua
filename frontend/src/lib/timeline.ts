import type { CoverageSegment, RecordingsSummary } from '$lib/api'

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

// Scrubber geometry. The helpers below work in unix SECONDS, never Date,
// and never feed Date into the API. Outputs are fractions in [0,1] so the
// component can position elements with width/left percentages and stays
// free of CSS units.

// timeToFraction maps an absolute time onto the window. A degenerate
// window (endSec<=startSec) returns 0 rather than NaN so the playhead
// snaps to the left edge instead of disappearing.
export function timeToFraction(tSec: number, startSec: number, endSec: number): number {
  if (endSec <= startSec) return 0
  const f = (tSec - startSec) / (endSec - startSec)
  return f < 0 ? 0 : f > 1 ? 1 : f
}

// fractionToTime is the inverse of timeToFraction. The caller decides
// whether to round (e.g. seek targets are integer seconds).
export function fractionToTime(f: number, startSec: number, endSec: number): number {
  const clamped = f < 0 ? 0 : f > 1 ? 1 : f
  return startSec + clamped * (endSec - startSec)
}

// footageToWallclock maps an element's currentTime (footage-time, seconds into
// the concatenated VOD) back to wall-clock seconds THROUGH the coverage bands.
// The full-res VOD splices only recorded segments end-to-end — gaps are removed
// from the footage — so a naive `chunkStart + currentTime` drifts behind the
// picture by the cumulative gap size once playback crosses a gap. Walking the
// bands instead makes every gap boundary jump the wall-clock forward to the
// next band's start, keeping the playhead flag in lockstep with the footage.
//
// `chunkStart` is the chunk's wall-clock start (already a covered second, per
// the chunk-bounds logic). `footageSeconds` is el.currentTime. `coverage` is
// the loaded coverage bands (not assumed sorted).
//
// Note: the BFF coalesces Frigate's raw segments with a COVERAGE_GAP_MERGE
// threshold, so sub-threshold gaps are absorbed into a band and leave at most
// that threshold of residual error within it — acceptable, those are recording
// jitter, not real gaps.
export function footageToWallclock(
  chunkStart: number,
  footageSeconds: number,
  coverage: CoverageSegment[]
): number {
  // No coverage (not yet loaded, or a continuous-record camera reporting a
  // single full band that collapses to the same result): footage is continuous
  // wall-clock from the chunk start.
  if (coverage.length === 0) return chunkStart + footageSeconds

  const sorted = [...coverage].sort((a, b) => a.start - b.start)
  let accumulated = 0
  for (const band of sorted) {
    if (band.end <= chunkStart) continue
    const bs = Math.max(band.start, chunkStart)
    const be = band.end
    const len = be - bs
    if (len <= 0) continue
    // footageSeconds 0 maps to the first covered second at or after chunkStart;
    // each subsequent band picks up where the previous one's recorded duration
    // left off, so crossing a gap jumps wall-clock forward by the gap size.
    if (footageSeconds <= accumulated + len) {
      return bs + (footageSeconds - accumulated)
    }
    accumulated += len
  }
  // Past all loaded coverage: a late-arriving frame near the live tail. Fall
  // back to continuous mapping rather than snapping backward to the last band.
  return chunkStart + footageSeconds
}
