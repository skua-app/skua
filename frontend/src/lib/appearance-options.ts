// The Appearance settings' OPTION LISTS — data only. No components, no markup,
// no state, nothing Svelte, nothing reactive.
//
// Appearance is rendered by two screens: AppearanceSection (desktop, labelled
// rows in cards) and MobileAppearance (phone, a single stacked block). They
// present the same preferences in two different layouts, so the markup is
// legitimately separate — but the lists of what each control OFFERS are not.
// They were duplicated byte for byte in both files, which meant every new
// preference had to be added twice by hand, and adding it once ships a setting
// on one form factor and not the other. This module is the single copy the two
// screens import; the layouts stay where they are.
//
// The lists are declared in the order the screens render them, which is also
// the order they are declared in below, so a new preference has one obvious
// place to go.
//
// LABELS LIVE HERE, and they are the same labels as before. Words come from
// `ui` — the app centralises language, and Below / Follow / Cyan / Yes are
// language. Unit symbols and bare numerals ('1 Hz', '6h', '10') stay literal,
// which is what the rest of the app already does with HD / ECO / HQ / LQ, and
// is why strings.ru.ts has no translation for any of them: there is nothing in
// them to translate. Reading labels from strings.ts at the point of use instead
// would put the value -> label mapping back in both screens, which is the
// duplication this module exists to remove.
//
// Every list is annotated with the domain its values come from. Four of them
// used to be, and four leaned on `as const` instead; uniform annotation is the
// stricter of the two, because it makes a value outside the domain an error
// HERE, at the list, rather than at whichever screen passes it to Segmented.
// That matters more now the list and its consumer are in different files.

import { ui } from '$lib/i18n/strings'
import type { Theme } from '$lib/stores/theme.svelte'
import type { Accent, NameStyle, TimelineMode } from '$lib/api'

// One choice in a Segmented control: the value written back and the text shown.
// Structurally the same as Segmented's own option type — Segmented declares it
// in its instance script, where it cannot be imported from — and assignable to
// it, since the `disabled` flag it also accepts is optional and no Appearance
// control uses one.
export type Option<T extends string> = { value: T; label: string }

export const themeOptions: Option<Theme>[] = [
  { value: 'auto', label: ui.themeAuto },
  { value: 'dark', label: ui.themeDark },
  { value: 'light', label: ui.themeLight }
]
export const nameStyleOptions: Option<NameStyle>[] = [
  { value: 'below', label: ui.nameStyleBelow },
  { value: 'overlay', label: ui.nameStyleOverlay },
  { value: 'off', label: ui.nameStyleOff }
]
export const showTimestampOptions: Option<'on' | 'off'>[] = [
  { value: 'on', label: ui.yes },
  { value: 'off', label: ui.no }
]
// GridFps, GlanceWindowHours and GlanceMaxMoments are NUMBERS in the prefs API.
// Segmented speaks strings, so these three lists are annotated with the string
// encoding of the domain rather than the domain itself — the same union the
// screens already cast to when they read the preference back out.
export const gridFpsOptions: Option<'1' | '2'>[] = [
  { value: '1', label: '1 Hz' },
  { value: '2', label: '2 Hz' }
]
export const timelineModeOptions: Option<TimelineMode>[] = [
  { value: 'follow', label: ui.timelineModeFollow },
  { value: 'fixed', label: ui.timelineModeFixed }
]
export const accentOptions: Option<Accent>[] = [
  { value: 'cyan', label: ui.accentCyan },
  { value: 'sage', label: ui.accentSage },
  { value: 'amber', label: ui.accentAmber },
  { value: 'violet', label: ui.accentViolet }
]
export const glanceWindowOptions: Option<'6' | '12' | '24' | '48' | '72'>[] = [
  { value: '6', label: '6h' },
  { value: '12', label: '12h' },
  { value: '24', label: '24h' },
  { value: '48', label: '48h' },
  { value: '72', label: '72h' }
]
export const glanceMaxMomentsOptions: Option<'10' | '20' | '30' | '50'>[] = [
  { value: '10', label: '10' },
  { value: '20', label: '20' },
  { value: '30', label: '30' },
  { value: '50', label: '50' }
]
