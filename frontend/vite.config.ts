import { sveltekit } from '@sveltejs/kit/vite'
import { defineConfig } from 'vite'
import tailwindcss from '@tailwindcss/vite'
import { VitePWA } from 'vite-plugin-pwa'

export default defineConfig({
  plugins: [
    tailwindcss(),
    sveltekit(),
    VitePWA({
      strategies: 'generateSW',
      base: '/',
      // 'prompt' (default) generates a registerSW.js that does NOT call
      // skipWaiting/clientsClaim — the new SW waits until every tab is
      // closed before taking over. On iOS PWA this is the launch-after-kill
      // path, exactly when a chunk-hash flip is safe. Previous 'autoUpdate'
      // could activate mid-load and break in-flight ESM imports → blank screen.
      registerType: 'prompt',
      workbox: {
        globDirectory: '.svelte-kit/output/client',
        globPatterns: ['**/*.{js,css,html,ico,png,svg,webmanifest}'],
        globIgnores: ['sw.js', 'workbox-*.js', 'registerSW.js'],
        cleanupOutdatedCaches: true,
        // SvelteKit adapter-static SPA fallback. Without this the SW
        // intercepts deep-link navigations (e.g. iOS PWA snapshot resumes
        // at /cam/cam5 or /events) and returns nothing → blank screen.
        navigateFallback: 'index.html',
        navigateFallbackDenylist: [/^\/api\//]
      },
      manifest: {
        name: 'Cams',
        short_name: 'Cams',
        description: 'Home cameras viewer',
        start_url: '/',
        scope: '/',
        display: 'standalone',
        background_color: '#0a0a0a',
        theme_color: '#0a0a0a',
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
