// SVG icon definitions. 24×24 viewBox, currentColor, stroke 1.5, round caps
// and joins by default; filled icons override fill/stroke at the root.
//
// Glyphs come from Lucide (lucide.dev) via lucide-static v1.21.0, ISC-licensed.
// Each glyph's inner elements are copied into the typed primitive system here:
// `paths` for stroke-only glyphs, plus optional `rects`/`lines`/`circles` for
// mixed-primitive icons. Icon.svelte renders each list in order.
//
// cols1/cols2 are deliberate hand-rolled exceptions, kept off Lucide on purpose:
// the column-density toggle has no Lucide equivalent. The four-bracket brand mark
// lives in AppHeader, not here.

import type { EventKind } from '$lib/api'

export type IconRect = { x: number; y: number; width: number; height: number; rx?: number }
export type IconLine = { x1: number; y1: number; x2: number; y2: number }
export type IconCircle = {
  cx: number
  cy: number
  r: number
  fill?: 'currentColor' | 'none'
  stroke?: 'currentColor' | 'none'
}

export type IconDef = {
  paths?: string | string[]
  rects?: IconRect[]
  lines?: IconLine[]
  circles?: IconCircle[]
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
  | 'cams'
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
  | 'chevDown'
  | 'check'
  | 'cols1'
  | 'cols2'
  | 'sliders'
  | 'info'
  | 'link'
  | 'close'
  | 'chevRight'
  | 'pip'
  | 'sun'
  | 'moon'
  | 'themeAuto'
  | 'grip'
  | 'edit'
  | 'rewind'
  | 'fastForward'
  | 'person'
  | 'car'
  | 'paw'
  | 'tag'
  | 'hardDrive'

export const ICONS: Record<IconName, IconDef> = {
  back: { paths: 'm15 18-6-6 6-6' },
  play: {
    paths: 'M5 5a2 2 0 0 1 3.008-1.728l11.997 6.998a2 2 0 0 1 .003 3.458l-12 7A2 2 0 0 1 5 19z'
  },
  pause: {
    rects: [
      { x: 14, y: 3, width: 5, height: 18, rx: 1 },
      { x: 5, y: 3, width: 5, height: 18, rx: 1 }
    ]
  },
  mute: {
    paths:
      'M11 4.702a.705.705 0 0 0-1.203-.498L6.413 7.587A1.4 1.4 0 0 1 5.416 8H3a1 1 0 0 0-1 1v6a1 1 0 0 0 1 1h2.416a1.4 1.4 0 0 1 .997.413l3.383 3.384A.705.705 0 0 0 11 19.298z',
    lines: [
      { x1: 22, y1: 9, x2: 16, y2: 15 },
      { x1: 16, y1: 9, x2: 22, y2: 15 }
    ]
  },
  unmute: {
    paths: [
      'M11 4.702a.705.705 0 0 0-1.203-.498L6.413 7.587A1.4 1.4 0 0 1 5.416 8H3a1 1 0 0 0-1 1v6a1 1 0 0 0 1 1h2.416a1.4 1.4 0 0 1 .997.413l3.383 3.384A.705.705 0 0 0 11 19.298z',
      'M16 9a5 5 0 0 1 0 6',
      'M19.364 18.364a9 9 0 0 0 0-12.728'
    ]
  },
  mic: {
    paths: ['M12 19v3', 'M19 10v2a7 7 0 0 1-14 0v-2'],
    rects: [{ x: 9, y: 2, width: 6, height: 13, rx: 3 }]
  },
  micOff: {
    paths: [
      'M12 19v3',
      'M15 9.34V5a3 3 0 0 0-5.68-1.33',
      'M16.95 16.95A7 7 0 0 1 5 12v-2',
      'M18.89 13.23A7 7 0 0 0 19 12v-2',
      'm2 2 20 20',
      'M9 9v3a3 3 0 0 0 5.12 2.12'
    ]
  },
  fullscreen: { paths: ['M15 3h6v6', 'm21 3-7 7', 'm3 21 7-7', 'M9 21H3v-6'] },
  exitFull: { paths: ['m14 10 7-7', 'M20 10h-6V4', 'm3 21 7-7', 'M4 14h6v6'] },
  snapshot: {
    paths:
      'M13.997 4a2 2 0 0 1 1.76 1.05l.486.9A2 2 0 0 0 18.003 7H20a2 2 0 0 1 2 2v9a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V9a2 2 0 0 1 2-2h1.997a2 2 0 0 0 1.759-1.048l.489-.904A2 2 0 0 1 10.004 4z',
    circles: [{ cx: 12, cy: 13, r: 3 }]
  },
  download: { paths: ['M12 15V3', 'M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4', 'm7 10 5 5 5-5'] },
  events: {
    paths: ['M3 5h.01', 'M3 12h.01', 'M3 19h.01', 'M8 5h13', 'M8 12h13', 'M8 19h13']
  },
  grid: {
    paths: ['M12 3v18', 'M3 12h18'],
    rects: [{ x: 3, y: 3, width: 18, height: 18, rx: 2 }]
  },
  cams: {
    rects: [
      { x: 3, y: 3, width: 7, height: 7, rx: 1 },
      { x: 14, y: 3, width: 7, height: 7, rx: 1 },
      { x: 14, y: 14, width: 7, height: 7, rx: 1 },
      { x: 3, y: 14, width: 7, height: 7, rx: 1 }
    ]
  },
  settings: {
    paths:
      'M9.671 4.136a2.34 2.34 0 0 1 4.659 0 2.34 2.34 0 0 0 3.319 1.915 2.34 2.34 0 0 1 2.33 4.033 2.34 2.34 0 0 0 0 3.831 2.34 2.34 0 0 1-2.33 4.033 2.34 2.34 0 0 0-3.319 1.915 2.34 2.34 0 0 1-4.659 0 2.34 2.34 0 0 0-3.32-1.915 2.34 2.34 0 0 1-2.33-4.033 2.34 2.34 0 0 0 0-3.831A2.34 2.34 0 0 1 6.35 6.051a2.34 2.34 0 0 0 3.319-1.915',
    circles: [{ cx: 12, cy: 12, r: 3 }]
  },
  refresh: {
    paths: [
      'M3 12a9 9 0 0 1 9-9 9.75 9.75 0 0 1 6.74 2.74L21 8',
      'M21 3v5h-5',
      'M21 12a9 9 0 0 1-9 9 9.75 9.75 0 0 1-6.74-2.74L3 16',
      'M8 16H3v5'
    ]
  },
  more: {
    circles: [
      { cx: 12, cy: 12, r: 1 },
      { cx: 19, cy: 12, r: 1 },
      { cx: 5, cy: 12, r: 1 }
    ]
  },
  activity: {
    paths:
      'M22 12h-2.48a2 2 0 0 0-1.93 1.46l-2.35 8.36a.25.25 0 0 1-.48 0L9.24 2.18a.25.25 0 0 0-.48 0l-2.35 8.36A2 2 0 0 1 4.49 12H2'
  },
  expand: {
    paths: [
      'M8 3H5a2 2 0 0 0-2 2v3',
      'M21 8V5a2 2 0 0 0-2-2h-3',
      'M3 16v3a2 2 0 0 0 2 2h3',
      'M16 21h3a2 2 0 0 0 2-2v-3'
    ]
  },
  ptz: {
    paths: [
      'M12 2v20',
      'm15 19-3 3-3-3',
      'm19 9 3 3-3 3',
      'M2 12h20',
      'm5 9-3 3 3 3',
      'm9 5 3-3 3 3'
    ]
  },
  warning: {
    paths: [
      'm21.73 18-8-14a2 2 0 0 0-3.48 0l-8 14A2 2 0 0 0 4 21h16a2 2 0 0 0 1.73-3',
      'M12 9v4',
      'M12 17h.01'
    ],
    strokeWidth: 1.8
  },
  bell: {
    paths: [
      'M10.268 21a2 2 0 0 0 3.464 0',
      'M3.262 15.326A1 1 0 0 0 4 17h16a1 1 0 0 0 .74-1.673C19.41 13.956 18 12.499 18 8A6 6 0 0 0 6 8c0 4.499-1.411 5.956-2.738 7.326'
    ]
  },
  history: {
    paths: ['M3 12a9 9 0 1 0 9-9 9.75 9.75 0 0 0-6.74 2.74L3 8', 'M3 3v5h5', 'M12 7v5l4 2']
  },
  filter: {
    paths:
      'M10 20a1 1 0 0 0 .553.895l2 1A1 1 0 0 0 14 21v-7a2 2 0 0 1 .517-1.341L21.74 4.67A1 1 0 0 0 21 3H3a1 1 0 0 0-.742 1.67l7.225 7.989A2 2 0 0 1 10 14z'
  },
  chevDown: { paths: 'm6 9 6 6 6-6', strokeWidth: 2 },
  check: { paths: 'M20 6 9 17l-5-5', strokeWidth: 2.4 },
  cols1: {
    rects: [
      { x: 4, y: 4, width: 16, height: 7, rx: 1.6 },
      { x: 4, y: 13, width: 16, height: 7, rx: 1.6 }
    ],
    fill: 'currentColor',
    stroke: 'none'
  },
  cols2: {
    rects: [
      { x: 4, y: 4, width: 7, height: 7, rx: 1.5 },
      { x: 13, y: 4, width: 7, height: 7, rx: 1.5 },
      { x: 4, y: 13, width: 7, height: 7, rx: 1.5 },
      { x: 13, y: 13, width: 7, height: 7, rx: 1.5 }
    ],
    fill: 'currentColor',
    stroke: 'none'
  },
  sliders: {
    paths: [
      'M10 8h4',
      'M12 21v-9',
      'M12 8V3',
      'M17 16h4',
      'M19 12V3',
      'M19 21v-5',
      'M3 14h4',
      'M5 10V3',
      'M5 21v-7'
    ]
  },
  info: {
    circles: [{ cx: 12, cy: 12, r: 10 }],
    paths: ['M12 16v-4', 'M12 8h.01']
  },
  link: {
    paths: [
      'M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71',
      'M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71'
    ]
  },
  close: {
    paths: ['M18 6 6 18', 'm6 6 12 12'],
    strokeWidth: 2.2
  },
  chevRight: { paths: 'm9 18 6-6-6-6', strokeWidth: 2 },
  pip: {
    paths: 'M21 9V6a2 2 0 0 0-2-2H4a2 2 0 0 0-2 2v10c0 1.1.9 2 2 2h4',
    rects: [{ x: 12, y: 13, width: 10, height: 7, rx: 2 }]
  },
  sun: {
    circles: [{ cx: 12, cy: 12, r: 4 }],
    paths: [
      'M12 2v2',
      'M12 20v2',
      'm4.93 4.93 1.41 1.41',
      'm17.66 17.66 1.41 1.41',
      'M2 12h2',
      'M20 12h2',
      'm6.34 17.66-1.41 1.41',
      'm19.07 4.93-1.41 1.41'
    ]
  },
  moon: {
    paths:
      'M20.985 12.486a9 9 0 1 1-9.473-9.472c.405-.022.617.46.402.803a6 6 0 0 0 8.268 8.268c.344-.215.825-.004.803.401',
    fill: 'currentColor',
    stroke: 'none'
  },
  themeAuto: {
    rects: [{ x: 2, y: 3, width: 20, height: 14, rx: 2 }],
    lines: [
      { x1: 8, y1: 21, x2: 16, y2: 21 },
      { x1: 12, y1: 17, x2: 12, y2: 21 }
    ]
  },
  grip: {
    circles: [
      { cx: 9, cy: 12, r: 1 },
      { cx: 9, cy: 5, r: 1 },
      { cx: 9, cy: 19, r: 1 },
      { cx: 15, cy: 12, r: 1 },
      { cx: 15, cy: 5, r: 1 },
      { cx: 15, cy: 19, r: 1 }
    ]
  },
  edit: {
    paths: [
      'M12 3H5a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7',
      'M18.375 2.625a1 1 0 0 1 3 3l-9.013 9.014a2 2 0 0 1-.853.505l-2.873.84a.5.5 0 0 1-.62-.62l.84-2.873a2 2 0 0 1 .506-.852z'
    ]
  },
  rewind: {
    paths: [
      'M12 6a2 2 0 0 0-3.414-1.414l-6 6a2 2 0 0 0 0 2.828l6 6A2 2 0 0 0 12 18z',
      'M22 6a2 2 0 0 0-3.414-1.414l-6 6a2 2 0 0 0 0 2.828l6 6A2 2 0 0 0 22 18z'
    ]
  },
  fastForward: {
    paths: [
      'M12 6a2 2 0 0 1 3.414-1.414l6 6a2 2 0 0 1 0 2.828l-6 6A2 2 0 0 1 12 18z',
      'M2 6a2 2 0 0 1 3.414-1.414l6 6a2 2 0 0 1 0 2.828l-6 6A2 2 0 0 1 2 18z'
    ]
  },
  // Event-kind glyphs (see kindIcon below). Drawn to read at ~14px in the
  // filter chips and event rows/cards.
  person: {
    paths: 'M19 21v-2a4 4 0 0 0-4-4H9a4 4 0 0 0-4 4v2',
    circles: [{ cx: 12, cy: 7, r: 4 }]
  },
  car: {
    paths: [
      'M19 17h2c.6 0 1-.4 1-1v-3c0-.9-.7-1.7-1.5-1.9C18.7 10.6 16 10 16 10s-1.3-1.4-2.2-2.3c-.5-.4-1.1-.7-1.8-.7H5c-.6 0-1.1.4-1.4.9l-1.4 2.9A3.7 3.7 0 0 0 2 12v4c0 .6.4 1 1 1h2',
      'M9 17h6'
    ],
    circles: [
      { cx: 7, cy: 17, r: 2 },
      { cx: 17, cy: 17, r: 2 }
    ]
  },
  paw: {
    circles: [
      { cx: 11, cy: 4, r: 2 },
      { cx: 18, cy: 8, r: 2 },
      { cx: 20, cy: 16, r: 2 }
    ],
    paths:
      'M9 10a5 5 0 0 1 5 5v3.5a3.5 3.5 0 0 1-6.84 1.045Q6.52 17.48 4.46 16.84A3.5 3.5 0 0 1 5.5 10Z'
  },
  tag: {
    paths:
      'M12.586 2.586A2 2 0 0 0 11.172 2H4a2 2 0 0 0-2 2v7.172a2 2 0 0 0 .586 1.414l8.704 8.704a2.426 2.426 0 0 0 3.42 0l6.58-6.58a2.426 2.426 0 0 0 0-3.42z',
    circles: [{ cx: 7.5, cy: 7.5, r: 0.5, fill: 'currentColor', stroke: 'none' }]
  },
  hardDrive: {
    paths:
      'M5.45 5.11 2 12v6a2 2 0 0 0 2 2h16a2 2 0 0 0 2-2v-6l-3.45-6.89A2 2 0 0 0 16.76 4H7.24a2 2 0 0 0-1.79 1.11z',
    lines: [
      { x1: 22, y1: 12, x2: 2, y2: 12 },
      { x1: 6, y1: 16, x2: 6.01, y2: 16 },
      { x1: 10, y1: 16, x2: 10.01, y2: 16 }
    ]
  }
}

// kindIcon maps each EventKind to the glyph that represents it. Audio-detection
// events carry no snapshot and are excluded from the Events list by the
// has_snapshot filter, so no audio kind/icon is needed here — audio activity
// has its own lane on the recording timeline.
export const kindIcon: Record<EventKind, IconName> = {
  person: 'person',
  vehicle: 'car',
  animal: 'paw',
  other: 'tag'
}
