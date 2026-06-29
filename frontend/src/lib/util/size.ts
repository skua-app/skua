// formatSize renders a MiB figure (as reported by Frigate's stats.service.
// storage) into a human string using 1024-based units. Sub-1024 MiB stays in
// MiB; 1024 MiB or more rolls up to GiB, then TiB. Precision tightens as the
// number grows: < 10 shows one decimal, >= 10 shows none, so "8.4 GiB" and
// "512 GiB" both read cleanly. Pure — no locale/Intl dependency.
export function formatSize(mib: number): string {
  if (!Number.isFinite(mib)) return '—'
  const neg = mib < 0
  let value = Math.abs(mib)
  const units = ['MiB', 'GiB', 'TiB']
  let unit = 0
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024
    unit++
  }
  const decimals = value < 10 ? 1 : 0
  const rendered = value.toFixed(decimals)
  return `${neg ? '-' : ''}${rendered} ${units[unit]}`
}
