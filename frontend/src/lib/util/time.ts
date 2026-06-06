const rtf = new Intl.RelativeTimeFormat([], { numeric: 'auto' })

// relativeTime returns "now" / "5 min ago" / "yesterday" etc. relative to
// `now` (default: current wall-clock). Past times produce negative values.
export function relativeTime(iso: string, now: Date = new Date()): string {
  const then = new Date(iso).getTime()
  const diffSec = Math.round((then - now.getTime()) / 1000)
  const abs = Math.abs(diffSec)
  if (abs < 45) return 'now'
  if (abs < 90) return rtf.format(Math.round(diffSec / 60), 'minute')
  if (abs < 3600) return rtf.format(Math.round(diffSec / 60), 'minute')
  if (abs < 5400) return rtf.format(Math.round(diffSec / 3600), 'hour')
  if (abs < 86400) return rtf.format(Math.round(diffSec / 3600), 'hour')
  if (abs < 7 * 86400) return rtf.format(Math.round(diffSec / 86400), 'day')
  if (abs < 31 * 86400) return rtf.format(Math.round(diffSec / (7 * 86400)), 'week')
  return new Intl.DateTimeFormat([], { dateStyle: 'medium' }).format(then)
}

const sameDayTimeFmt = new Intl.DateTimeFormat([], {
  hour: '2-digit',
  minute: '2-digit',
  hour12: false
})

const datedTimeFmt = new Intl.DateTimeFormat([], {
  day: 'numeric',
  month: 'short',
  hour: '2-digit',
  minute: '2-digit',
  hour12: false
})

// eventTimestamp returns the wall-clock time for a recent-events row.
// Same calendar day as `now` → 24h time only (e.g. "14:32"). Any
// other calendar day → short locale-formatted date plus 24h time
// (e.g. "5 Jun, 14:32"). No seconds in either branch — recent-event
// rows are scanning UI, not forensic precision.
export function eventTimestamp(iso: string, now: Date = new Date()): string {
  const then = new Date(iso)
  if (
    then.getFullYear() === now.getFullYear() &&
    then.getMonth() === now.getMonth() &&
    then.getDate() === now.getDate()
  ) {
    return sameDayTimeFmt.format(then)
  }
  return datedTimeFmt.format(then)
}

// formatDuration — "12s", "1min 30s", "1h 5min".
export function formatDuration(seconds: number | null): string {
  if (seconds === null) return '—'
  if (seconds < 60) return `${seconds}s`
  const m = Math.floor(seconds / 60)
  const s = seconds % 60
  if (m < 60) return s === 0 ? `${m}min` : `${m}min ${s}s`
  const h = Math.floor(m / 60)
  const mm = m % 60
  return mm === 0 ? `${h}h` : `${h}h ${mm}min`
}
