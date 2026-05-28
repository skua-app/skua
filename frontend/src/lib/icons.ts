// SVG icon path data. All icons are designed for 24×24 viewBox, currentColor,
// stroke 1.5, round line caps and joins. Filled icons override fill/stroke.
//
// Sourced from the design handoff (atoms.jsx — object `I`). Paths are
// vector-identical; only the rendering wrapper differs (Icon.svelte).

export type IconDef = {
  paths: string | string[]
  fill?: 'currentColor' | 'none'
  stroke?: 'currentColor' | 'none'
  strokeWidth?: number
}

export type IconName =
  | 'back'
  | 'play'
  | 'pause'
  | 'mute'
  | 'unmute'
  | 'mic'
  | 'micOff'
  | 'fullscreen'
  | 'exitFull'
  | 'snapshot'
  | 'download'
  | 'events'
  | 'grid'
  | 'settings'
  | 'refresh'
  | 'more'
  | 'activity'
  | 'expand'
  | 'ptz'
  | 'warning'
  | 'bell'
  | 'history'
  | 'filter'

export const ICONS: Record<IconName, IconDef> = {
  back: { paths: 'M15 18l-6-6 6-6' },
  play: { paths: 'M7 5.5v13l11-6.5z', fill: 'currentColor', stroke: 'none' },
  pause: { paths: 'M7 5h3v14H7zM14 5h3v14h-3z', fill: 'currentColor', stroke: 'none' },
  mute: { paths: ['M11 5L6 9H3v6h3l5 4z', 'M22 9l-6 6', 'M16 9l6 6'] },
  unmute: { paths: ['M11 5L6 9H3v6h3l5 4z', 'M16 8a5 5 0 010 8', 'M19.5 4.5a9 9 0 010 15'] },
  mic: {
    paths: ['M12 3a3 3 0 00-3 3v6a3 3 0 006 0V6a3 3 0 00-3-3z', 'M19 11a7 7 0 01-14 0', 'M12 18v3']
  },
  micOff: {
    paths: [
      'M9 9V6a3 3 0 015.83-1',
      'M15 12V6',
      'M9 12a3 3 0 003 3',
      'M19 11a7 7 0 01-7 7',
      'M5 5l14 14',
      'M12 18v3'
    ]
  },
  fullscreen: { paths: ['M4 9V4h5', 'M20 9V4h-5', 'M4 15v5h5', 'M20 15v5h-5'] },
  exitFull: { paths: ['M9 4v5H4', 'M15 4v5h5', 'M9 20v-5H4', 'M15 20v-5h5'] },
  snapshot: {
    paths: [
      'M14.5 4h-5L7 7H4a2 2 0 0 0-2 2v9a2 2 0 0 0 2 2h16a2 2 0 0 0 2-2V9a2 2 0 0 0-2-2h-3l-2.5-3z',
      'M12 17a4 4 0 1 0 0-8 4 4 0 0 0 0 8z'
    ]
  },
  download: { paths: ['M12 4v12', 'M7 12l5 5 5-5', 'M4 20h16'] },
  events: { paths: ['M5 4h14v16H5z', 'M9 9h6', 'M9 13h6', 'M9 17h4'] },
  grid: { paths: ['M4 4h7v7H4z', 'M13 4h7v7h-7z', 'M4 13h7v7H4z', 'M13 13h7v7h-7z'] },
  settings: {
    paths: [
      'M12 8.5a3.5 3.5 0 100 7 3.5 3.5 0 000-7z',
      'M19.4 15a1.6 1.6 0 00.32 1.76l.06.06a2 2 0 11-2.83 2.83l-.06-.06a1.6 1.6 0 00-1.76-.32 1.6 1.6 0 00-.97 1.47V21a2 2 0 11-4 0v-.09a1.6 1.6 0 00-1.05-1.47 1.6 1.6 0 00-1.76.32l-.06.06a2 2 0 11-2.83-2.83l.06-.06a1.6 1.6 0 00.32-1.76A1.6 1.6 0 003.09 14H3a2 2 0 110-4h.09A1.6 1.6 0 004.56 9a1.6 1.6 0 00-.32-1.76l-.06-.06a2 2 0 112.83-2.83l.06.06A1.6 1.6 0 008.83 4.8 1.6 1.6 0 009.8 3.33V3a2 2 0 114 0v.09c0 .65.4 1.23 1.05 1.47.65.24 1.39.11 1.76-.32l.06-.06a2 2 0 112.83 2.83l-.06.06a1.6 1.6 0 00-.32 1.76c.24.65.82 1.05 1.47 1.05H21a2 2 0 110 4h-.09a1.6 1.6 0 00-1.47.97z'
    ]
  },
  refresh: {
    paths: ['M3 12a9 9 0 0115-6.7L21 8', 'M21 3v5h-5', 'M21 12a9 9 0 01-15 6.7L3 16', 'M3 21v-5h5']
  },
  more: { paths: ['M5 12h.01', 'M12 12h.01', 'M19 12h.01'], strokeWidth: 3 },
  activity: { paths: 'M3 12h4l3-9 4 18 3-9h4' },
  expand: { paths: ['M9 3H3v6', 'M15 3h6v6', 'M9 21H3v-6', 'M15 21h6v-6'] },
  ptz: {
    paths: [
      'M12 3v18',
      'M3 12h18',
      'M9 6l3-3 3 3',
      'M15 18l-3 3-3-3',
      'M6 9l-3 3 3 3',
      'M18 9l3 3-3 3'
    ]
  },
  warning: { paths: ['M12 4l10 17H2z', 'M12 10v5', 'M12 18.5v.01'] },
  bell: { paths: ['M6 8a6 6 0 0112 0c0 7 3 7 3 9H3c0-2 3-2 3-9z', 'M10 21a2 2 0 004 0'] },
  history: { paths: ['M3 12a9 9 0 1 0 3-6.7L3 8', 'M3 3v5h5', 'M12 7v5l4 2'] },
  filter: { paths: 'M3 5h18l-7 9v6l-4-2v-4z' }
}
