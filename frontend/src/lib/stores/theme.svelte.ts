// Device-local theme preference. Deliberately a documented exception to the
// otherwise server-backed, no-localStorage prefs model — theme is per-device
// (light on a desktop, dark on a phone), NOT household-shared, so it does not
// go through prefsStore or the BFF.

export type Theme = 'auto' | 'dark' | 'light'

const STORAGE_KEY = 'skua-theme'
const THEMES: readonly Theme[] = ['auto', 'dark', 'light'] as const

function isTheme(v: unknown): v is Theme {
  return typeof v === 'string' && (THEMES as readonly string[]).includes(v)
}

class ThemeStore {
  theme = $state<Theme>('auto')

  constructor() {
    if (typeof localStorage === 'undefined') return
    try {
      const stored = localStorage.getItem(STORAGE_KEY)
      this.theme = isTheme(stored) ? stored : 'auto'
    } catch {
      this.theme = 'auto'
    }
    this.#applyClass(this.theme)
  }

  setTheme(v: Theme) {
    this.theme = v
    if (typeof localStorage !== 'undefined') {
      try {
        localStorage.setItem(STORAGE_KEY, v)
      } catch {
        // localStorage may be unavailable (private mode quota, etc.) — class
        // still applies so the current session reflects the choice.
      }
    }
    this.#applyClass(v)
  }

  cycle() {
    const next: Theme = this.theme === 'auto' ? 'dark' : this.theme === 'dark' ? 'light' : 'auto'
    this.setTheme(next)
  }

  #applyClass(v: Theme) {
    if (typeof document === 'undefined') return
    const root = document.documentElement.classList
    root.remove('theme-auto', 'theme-dark', 'theme-light')
    root.add(`theme-${v}`)
  }
}

export const themeStore = new ThemeStore()
