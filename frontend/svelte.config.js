import adapter from '@sveltejs/adapter-static'
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte'

/** @type {import('@sveltejs/kit').Config} */
const config = {
  preprocess: vitePreprocess(),
  kit: {
    adapter: adapter({
      fallback: 'index.html'
    }),
    serviceWorker: {
      register: false
    },
    version: {
      // SvelteKit's app version defaults to Date.now(), which it writes into
      // _app/version.json AND into one client chunk — so the same source built
      // twice produced different chunk contents, different content hashes, and
      // a different image. Pinned to a constant to make the build output a pure
      // function of the source; the PWA shell revision is derived from that
      // output (see vite.config.ts), so it cannot be deterministic while this
      // is not.
      //
      // Safe to pin here because nothing reads it: the `updated` store is not
      // used anywhere in src/, SvelteKit's own service worker registration is
      // off (above), and updates are driven by vite-plugin-pwa's autoUpdate.
      // If `updated` is ever adopted, this needs a real per-release value —
      // and version.json is not precached, so it would have to come from
      // somewhere other than the build clock to stay reproducible.
      name: 'skua'
    }
  }
}

export default config
