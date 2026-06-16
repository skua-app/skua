// SvelteKit ambient types. The `App` namespace lets us extend
// framework-typed objects (locals, page data, page state) project-wide.
// See https://kit.svelte.dev/docs/types#app for the full surface.

declare global {
  namespace App {
    // PageState is the typed shape of history-entry state attached via
    // shallow routing (pushState / replaceState from $app/navigation)
    // and read back through `page.state` in $app/state. We use it for
    // mobile Settings drill-in so the native back gesture / browser
    // back pops the drill instead of leaving the /settings route.
    interface PageState {
      settingsSection?: 'cameras' | 'groups' | 'connection' | 'about'
    }
  }
}

export {}
