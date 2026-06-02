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
    } catch (err) {
      console.error('[prefs] load failed, using defaults:', err)
    } finally {
      this.loaded = true
    }
  }

  private applyAccent() {
    if (typeof document !== 'undefined') {
      document.documentElement.style.setProperty('--accent', ACCENT_VALUES[this.accent])
    }
  }

  // Optimistic-local pattern: every setter assigns local state first, then
  // funnels the PUT through #persist so a failed write surfaces in the
  // console instead of vanishing into a silent catch.
  async #persist(partial: Partial<Prefs>, label: string) {
    try {
      await updatePrefs(partial)
    } catch (err) {
      console.error(`[prefs] persist ${label} failed:`, err)
    }
  }

  async setGridMode(mode: Prefs['grid_mode']) {
    this.gridMode = mode
    await this.#persist({ grid_mode: mode }, 'grid_mode')
  }

  async setStreamQuality(q: Prefs['stream_quality']) {
    this.streamQuality = q
    await this.#persist({ stream_quality: q }, 'stream_quality')
  }

  async setMutedByDefault(v: boolean) {
    this.mutedByDefault = v
    await this.#persist({ muted_by_default: v }, 'muted_by_default')
  }

  async setShowTelemetry(v: boolean) {
    this.showTelemetry = v
    await this.#persist({ show_telemetry: v }, 'show_telemetry')
  }

  async setAccent(v: Accent) {
    this.accent = v
    this.applyAccent()
    await this.#persist({ accent: v }, 'accent')
  }

  async setNameStyle(v: NameStyle) {
    this.nameStyle = v
    await this.#persist({ name_style: v }, 'name_style')
  }

  async setShowTimestamp(v: boolean) {
    this.showTimestamp = v
    await this.#persist({ show_timestamp: v }, 'show_timestamp')
  }

  async setDesktopColumns(v: DesktopColumns) {
    this.desktopColumns = v
    await this.#persist({ desktop_columns: v }, 'desktop_columns')
  }

  async setMobileColumns(v: MobileColumns) {
    this.mobileColumns = v
    await this.#persist({ mobile_columns: v }, 'mobile_columns')
  }

  async setGridFilter(id: string | null) {
    this.gridFilter = id
    await this.#persist({ grid_filter: id }, 'grid_filter')
  }
}

export const prefsStore = new PrefsStore()
