// SVG icon definitions. 24×24 viewBox, currentColor, stroke 1.5, round caps
// and joins by default; filled icons override fill/stroke at the root.
//
// Sourced from the Calm prototype (.design-reference/reference/calm). Icons
// are typed primitives — `paths` for stroke-only glyphs, plus optional
// `rects`/`lines`/`circles` for mixed-primitive icons (e.g. the events tab
// glyph). Icon.svelte renders each list in order.

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
  | 'skipBack'
  | 'skipForward'
  | 'rewind'
  | 'fastForward'
  | 'person'
  | 'car'
  | 'paw'
  | 'tag'

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
  fullscreen: { paths: ['M15 3h6v6', 'M21 3l-7 7', 'M9 21H3v-6', 'M3 21l7-7'] },
  exitFull: { paths: ['M4 14h6v6', 'M10 14l-7 7', 'M20 10h-6V4', 'M14 10l7-7'] },
  snapshot: {
    paths: [
      'M14.5 4h-5L7 7H4a2 2 0 0 0-2 2v9a2 2 0 0 0 2 2h16a2 2 0 0 0 2-2V9a2 2 0 0 0-2-2h-3l-2.5-3z',
      'M12 17a4 4 0 1 0 0-8 4 4 0 0 0 0 8z'
    ]
  },
  download: { paths: ['M12 4v12', 'M7 12l5 5 5-5', 'M4 20h16'] },
  events: {
    lines: [
      { x1: 8, y1: 6, x2: 20, y2: 6 },
      { x1: 8, y1: 12, x2: 20, y2: 12 },
      { x1: 8, y1: 18, x2: 20, y2: 18 }
    ],
    circles: [
      { cx: 3.8, cy: 6, r: 1.2, fill: 'currentColor', stroke: 'none' },
      { cx: 3.8, cy: 12, r: 1.2, fill: 'currentColor', stroke: 'none' },
      { cx: 3.8, cy: 18, r: 1.2, fill: 'currentColor', stroke: 'none' }
    ]
  },
  grid: { paths: ['M4 4h7v7H4z', 'M13 4h7v7h-7z', 'M4 13h7v7H4z', 'M13 13h7v7h-7z'] },
  cams: {
    rects: [
      { x: 3, y: 3, width: 7.5, height: 7.5, rx: 1.6 },
      { x: 13.5, y: 3, width: 7.5, height: 7.5, rx: 1.6 },
      { x: 3, y: 13.5, width: 7.5, height: 7.5, rx: 1.6 },
      { x: 13.5, y: 13.5, width: 7.5, height: 7.5, rx: 1.6 }
    ]
  },
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
  warning: {
    paths: ['M10.3 3.8 1.8 18a2 2 0 0 0 1.7 3h17a2 2 0 0 0 1.7-3L13.7 3.8a2 2 0 0 0-3.4 0z'],
    lines: [
      { x1: 12, y1: 9, x2: 12, y2: 13 },
      { x1: 12, y1: 17, x2: 12, y2: 17 }
    ],
    strokeWidth: 1.8
  },
  bell: {
    paths: ['M18 8a6 6 0 1 0-12 0c0 7-3 9-3 9h18s-3-2-3-9', 'M13.7 21a2 2 0 0 1-3.4 0']
  },
  history: { paths: ['M3 12a9 9 0 1 0 3-6.7L3 8', 'M3 3v5h5', 'M12 7v5l4 2'] },
  filter: { paths: 'M3 5h18l-7 9v6l-4-2v-4z' },
  chevDown: { paths: 'M6 9l6 6 6-6', strokeWidth: 2 },
  check: { paths: 'M20 6L9 17l-5-5', strokeWidth: 2.4 },
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
    lines: [
      { x1: 4, y1: 21, x2: 4, y2: 14 },
      { x1: 4, y1: 10, x2: 4, y2: 3 },
      { x1: 12, y1: 21, x2: 12, y2: 12 },
      { x1: 12, y1: 8, x2: 12, y2: 3 },
      { x1: 20, y1: 21, x2: 20, y2: 16 },
      { x1: 20, y1: 12, x2: 20, y2: 3 },
      { x1: 1, y1: 14, x2: 7, y2: 14 },
      { x1: 9, y1: 8, x2: 15, y2: 8 },
      { x1: 17, y1: 16, x2: 23, y2: 16 }
    ]
  },
  info: {
    circles: [{ cx: 12, cy: 12, r: 9 }],
    lines: [
      { x1: 12, y1: 11, x2: 12, y2: 16 },
      { x1: 12, y1: 8, x2: 12.01, y2: 8 }
    ]
  },
  link: {
    paths: [
      'M10 13a5 5 0 0 0 7.5.5l3-3a5 5 0 0 0-7-7l-1.7 1.7',
      'M14 11a5 5 0 0 0-7.5-.5l-3 3a5 5 0 0 0 7 7l1.7-1.7'
    ]
  },
  close: {
    lines: [
      { x1: 6, y1: 6, x2: 18, y2: 18 },
      { x1: 18, y1: 6, x2: 6, y2: 18 }
    ],
    strokeWidth: 2.2
  },
  chevRight: { paths: 'M9 6l6 6-6 6', strokeWidth: 2 },
  pip: {
    rects: [
      { x: 3, y: 5, width: 18, height: 14, rx: 2 },
      { x: 12, y: 11, width: 7, height: 5, rx: 1 }
    ]
  },
  // Center circle + eight rays. Ray endpoints are precomputed on a unit
  // circle at 45° steps; the inner endpoint sits at r=6, the outer at r=9.
  sun: {
    circles: [{ cx: 12, cy: 12, r: 4 }],
    lines: [
      { x1: 12, y1: 3, x2: 12, y2: 5 },
      { x1: 12, y1: 19, x2: 12, y2: 21 },
      { x1: 3, y1: 12, x2: 5, y2: 12 },
      { x1: 19, y1: 12, x2: 21, y2: 12 },
      { x1: 5.64, y1: 5.64, x2: 7.05, y2: 7.05 },
      { x1: 16.95, y1: 16.95, x2: 18.36, y2: 18.36 },
      { x1: 5.64, y1: 18.36, x2: 7.05, y2: 16.95 },
      { x1: 16.95, y1: 7.05, x2: 18.36, y2: 5.64 }
    ]
  },
  moon: {
    paths: 'M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z',
    fill: 'currentColor',
    stroke: 'none'
  },
  // Half-filled disc — appearance-toggle glyph. The circle primitive is
  // stroked (it carries its own fill='none'), while the path renders the
  // right-half disc filled with currentColor (def.fill cascades to paths).
  themeAuto: {
    paths: 'M12 3a9 9 0 0 1 0 18z',
    circles: [{ cx: 12, cy: 12, r: 9 }],
    fill: 'currentColor'
  },
  // Two-by-three dot grid — the conventional drag-handle glyph. Dots are
  // filled so the affordance reads clearly at small sizes (the row handle
  // renders at ~18 px).
  grip: {
    circles: [
      { cx: 9, cy: 6, r: 1.5, fill: 'currentColor', stroke: 'none' },
      { cx: 15, cy: 6, r: 1.5, fill: 'currentColor', stroke: 'none' },
      { cx: 9, cy: 12, r: 1.5, fill: 'currentColor', stroke: 'none' },
      { cx: 15, cy: 12, r: 1.5, fill: 'currentColor', stroke: 'none' },
      { cx: 9, cy: 18, r: 1.5, fill: 'currentColor', stroke: 'none' },
      { cx: 15, cy: 18, r: 1.5, fill: 'currentColor', stroke: 'none' }
    ]
  },
  // Feather-style pencil/edit glyph.
  edit: {
    paths: [
      'M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7',
      'M18.5 2.5a2.121 2.121 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z'
    ]
  },
  // Skip-back / skip-forward: a rounded circular arrow (counter-clockwise for
  // back, clockwise for forward), deliberately distinct from a double-triangle
  // — that glyph is reserved for the VHS rewind/fast-forward pair. Arc plus an
  // arrowhead at the open top corner.
  skipBack: { paths: ['M3 12a9 9 0 1 0 3-6.7L3 8', 'M3 3v5h5'] },
  skipForward: { paths: ['M21 12a9 9 0 1 1-3-6.7L21 8', 'M21 3v5h-5'] },
  // VHS rewind / fast-forward: filled double-triangles in the play-glyph idiom
  // (fill currentColor, stroke none). The skip arrows deliberately left room
  // for this pair. fastForward points right, rewind points left.
  fastForward: {
    paths: ['M4 5.5v13l8-6.5z', 'M12 5.5v13l8-6.5z'],
    fill: 'currentColor',
    stroke: 'none'
  },
  rewind: {
    paths: ['M20 5.5v13l-8-6.5z', 'M12 5.5v13l-8-6.5z'],
    fill: 'currentColor',
    stroke: 'none'
  },
  // Event-kind glyphs (see kindIcon below). Drawn to read at ~14px in the
  // filter chips and event rows/cards.
  // person — head circle + shoulders arc.
  person: {
    circles: [{ cx: 12, cy: 8, r: 3.5 }],
    paths: 'M5.5 21v-1a6.5 6.5 0 0 1 13 0v1'
  },
  // car — side-on body (roof arc over a chassis bar) + two wheels.
  car: {
    paths: [
      'M5 13l1.6-4.7A2 2 0 0 1 8.5 7h7a2 2 0 0 1 1.9 1.3L19 13',
      'M3 13h18v2a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1z'
    ],
    circles: [
      { cx: 7.5, cy: 17, r: 1.7 },
      { cx: 16.5, cy: 17, r: 1.7 }
    ]
  },
  // paw — filled pad + four filled toe beans; filled reads clearer at 14px.
  paw: {
    paths: 'M8.2 14.3c0-2 1.7-3.3 3.8-3.3s3.8 1.3 3.8 3.3c0 1.8-1.7 2.4-3.8 2.4s-3.8-.6-3.8-2.4z',
    circles: [
      { cx: 7.3, cy: 9, r: 1.4, fill: 'currentColor', stroke: 'none' },
      { cx: 10.4, cy: 7, r: 1.4, fill: 'currentColor', stroke: 'none' },
      { cx: 13.6, cy: 7, r: 1.4, fill: 'currentColor', stroke: 'none' },
      { cx: 16.7, cy: 9, r: 1.4, fill: 'currentColor', stroke: 'none' }
    ],
    fill: 'currentColor',
    stroke: 'none'
  },
  // tag — neutral catch-all marker (stroked tag body + filled hole dot).
  tag: {
    paths:
      'M12.6 2.6A2 2 0 0 0 11.2 2H4a2 2 0 0 0-2 2v7.2a2 2 0 0 0 .6 1.4l8.7 8.7a2.4 2.4 0 0 0 3.4 0l6.6-6.6a2.4 2.4 0 0 0 0-3.4z',
    circles: [{ cx: 7.5, cy: 7.5, r: 1.1, fill: 'currentColor', stroke: 'none' }]
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
