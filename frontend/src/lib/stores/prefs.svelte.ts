import { fetchPrefs, updatePrefs, ACCENT_VALUES } from '$lib/api'
import type { Prefs, Accent, NameStyle, DesktopColumns, MobileColumns } from '$lib/api'

class PrefsStore {
  gridMode = $state<'hd' | 'eco'>('eco')
  mutedByDefault = $state(true)
  streamQuality = $state<'main' | 'sub'>('main')
  showTelemetry = $state(false)
  accent = $state<Accent>('cyan')
  nameStyle = $state<NameStyle>('below')
  showTimestamp = $state(false)
  desktopColumns = $state<DesktopColumns>(4)
  mobileColumns = $state<MobileColumns>(1)
  gridFilter = $state<string | null>(null)
  loaded = $state(false)

  async load() {
    try {
      const prefs = await fetchPrefs()
      this.gridMode = prefs.grid_mode
      this.mutedByDefault = prefs.muted_by_default
      this.streamQuality = prefs.stream_quality
      this.showTelemetry = prefs.show_telemetry
      this.accent = prefs.accent
      this.nameStyle = prefs.name_style
      this.showTimestamp = prefs.show_timestamp
      this.desktopColumns = prefs.desktop_columns
      this.mobileColumns = prefs.mobile_columns
      this.gridFilter = prefs.grid_filter
      this.applyAccent()
    } catch {
      // keep defaults on error
    } finally {
      this.loaded = true
    }
  }

  private applyAccent() {
    if (typeof document !== 'undefined') {
      document.documentElement.style.setProperty('--accent', ACCENT_VALUES[this.accent])
    }
  }

  async setGridMode(mode: Prefs['grid_mode']) {
    this.gridMode = mode
    try {
      await updatePrefs({ grid_mode: mode })
    } catch {
      // best-effort persist
    }
  }

  async setStreamQuality(q: Prefs['stream_quality']) {
    this.streamQuality = q
    try {
      await updatePrefs({ stream_quality: q })
    } catch {
      // best-effort persist
    }
  }

  async setMutedByDefault(v: boolean) {
    this.mutedByDefault = v
    try {
      await updatePrefs({ muted_by_default: v })
    } catch {
      // best-effort persist
    }
  }

  async setShowTelemetry(v: boolean) {
    this.showTelemetry = v
    try {
      await updatePrefs({ show_telemetry: v })
    } catch {
      // best-effort persist
    }
  }

  async setAccent(v: Accent) {
    this.accent = v
    this.applyAccent()
    try {
      await updatePrefs({ accent: v })
    } catch {
      // best-effort persist
    }
  }

  async setNameStyle(v: NameStyle) {
    this.nameStyle = v
    try {
      await updatePrefs({ name_style: v })
    } catch {
      // best-effort persist
    }
  }

  async setShowTimestamp(v: boolean) {
    this.showTimestamp = v
    try {
      await updatePrefs({ show_timestamp: v })
    } catch {
      // best-effort persist
    }
  }

  async setDesktopColumns(v: DesktopColumns) {
    this.desktopColumns = v
    try {
      await updatePrefs({ desktop_columns: v })
    } catch {
      // best-effort persist
    }
  }

  async setMobileColumns(v: MobileColumns) {
    this.mobileColumns = v
    try {
      await updatePrefs({ mobile_columns: v })
    } catch {
      // best-effort persist
    }
  }

  async setGridFilter(id: string | null) {
    this.gridFilter = id
    try {
      await updatePrefs({ grid_filter: id })
    } catch {
      // best-effort persist
    }
  }
}

export const prefsStore = new PrefsStore()
