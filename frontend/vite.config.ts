import { sveltekit } from '@sveltejs/kit/vite'
import { defineConfig } from 'vite'
import tailwindcss from '@tailwindcss/vite'
import { VitePWA } from 'vite-plugin-pwa'

// New revision per build so additionalManifestEntries (the shell) is treated
// as stale on every deploy — the SW re-fetches the precached shell and the
// autoUpdate flow swaps to the new build atomically.
const pwaShellRevision = String(Date.now())

// The BFF (backend/internal/api/router.go) wraps an http.FileServer over the
// embedded client FS. http.FileServer redirects GET /index.html → /, so the
// shell must be precached and looked up under '/', not 'index.html', or the
// SW install would cache the redirect.
const PWA_SHELL_URL = '/'

export default defineConfig({
  plugins: [
    tailwindcss(),
    sveltekit(),
    VitePWA({
      strategies: 'generateSW',
      base: '/',
      // autoUpdate + skipWaiting/clientsClaim: the newest SW activates
      // immediately, +layout.svelte's registerSW({ immediate: true }) reloads
      // the page once on update, and the shell / chunks always come from the
      // same build — eliminating the stale-SW blank-screen window after deploy.
      registerType: 'autoUpdate',
      workbox: {
        globDirectory: '.svelte-kit/output/client',
        // Drop 'html' here — the shell is added explicitly via
        // additionalManifestEntries with a per-build revision so navigation
        // navigateFallback always binds to a precached URL.
        globPatterns: ['**/*.{js,css,ico,png,svg,webmanifest}'],
        globIgnores: ['sw.js', 'workbox-*.js', 'registerSW.js'],
        cleanupOutdatedCaches: true,
        skipWaiting: true,
        clientsClaim: true,
        additionalManifestEntries: [{ url: PWA_SHELL_URL, revision: pwaShellRevision }],
        // SvelteKit adapter-static SPA fallback. Without this the SW
        // intercepts deep-link navigations (e.g. iOS PWA snapshot resumes
        // at /cam/cam5 or /events) and returns nothing → blank screen.
        navigateFallback: PWA_SHELL_URL,
        navigateFallbackDenylist: [/^\/api\//],
        runtimeCaching: [
          {
            // Grid camera tiles: /api/cameras/<id>/tile.jpg. The grid busts
            // the URL with ?t=<tick>, so normalise the cache key to the
            // path-only URL so each camera keeps a single latest entry that
            // serves instantly offline (StaleWhileRevalidate fetches fresh
            // in the background when online).
            urlPattern: ({ url, request }) =>
              request.method === 'GET' &&
              url.pathname.startsWith('/api/cameras/') &&
              url.pathname.endsWith('/tile.jpg'),
            handler: 'StaleWhileRevalidate',
            options: {
              cacheName: 'skua-grid-tiles',
              cacheableResponse: { statuses: [0, 200] },
              expiration: { maxEntries: 32, maxAgeSeconds: 604800 },
              plugins: [
                {
                  cacheKeyWillBeUsed: async ({ request }) => {
                    const u = new URL(request.url)
                    u.search = ''
                    return u.toString()
                  }
                }
              ]
            }
          }
        ]
      },
      manifest: {
        name: 'Skua',
        short_name: 'Skua',
        description: 'Home cameras viewer',
        start_url: '/',
        scope: '/',
        display: 'standalone',
        background_color: '#0a0b0d',
        theme_color: '#0a0b0d',
        orientation: 'any',
        icons: [
          { src: '/icons/icon-192.png', sizes: '192x192', type: 'image/png' },
          { src: '/icons/icon-512.png', sizes: '512x512', type: 'image/png' },
          {
            src: '/icons/icon-maskable-512.png',
            sizes: '512x512',
            type: 'image/png',
            purpose: 'maskable'
          }
        ]
      }
    })
  ],
  server: {
    proxy: {
      '/api': 'http://localhost:3200'
    }
  }
})
