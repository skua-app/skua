<script lang="ts">
  import '../app.css'
  import { onMount } from 'svelte'
  import { page } from '$app/state'
  import AppHeader from '$lib/components/AppHeader.svelte'
  import ConnectionOverlay from '$lib/components/ConnectionOverlay.svelte'
  import GlancePeek from '$lib/components/GlancePeek.svelte'
  import MobileTabBar from '$lib/components/MobileTabBar.svelte'
  import InstallPrompt from '$lib/components/InstallPrompt.svelte'
  import { registerSW } from 'virtual:pwa-register'
  import { prefsStore } from '$lib/stores/prefs.svelte'
  import { camerasStore } from '$lib/stores/cameras.svelte'
  import { configStore } from '$lib/stores/config.svelte'
  import { eventsStreamStore } from '$lib/stores/events-stream.svelte'
  import { glanceStore } from '$lib/stores/glance.svelte'
  import { groupsStore } from '$lib/stores/groups.svelte'
  import { lifecycle } from '$lib/lifecycle.svelte'

  let { children } = $props()

  let width = $state(typeof window !== 'undefined' ? window.innerWidth : 0)
  const isDesktop = $derived(width >= 900)
  // Both the single-camera focus view and the recording timeline are immersive:
  // on MOBILE they render their own top bar, so the global AppHeader must stay
  // hidden there to avoid a duplicate header row and a doubled safe-area top
  // inset. On DESKTOP these routes have no in-screen global nav, so the
  // AppHeader still renders (its grid control-bar is route-'/'-gated, so only
  // brand + tabs + theme + bell appear).
  const isImmersive = $derived(
    page.route.id === '/cam/[id]' || page.route.id === '/cam/[id]/timeline'
  )

  // All bootstrap goes through onMount, not $effect: each side-effect must
  // run exactly once per session. Previous $effect-based registerSW could
  // re-run on reactive churn and stack SW registrations; store inits had
  // their own #initStarted guards but the outer wrapper still risked
  // double-invocation. Wrapping each in try/catch ensures a single
  // unfortunate throw can't blank the entire shell — +error.svelte handles
  // SvelteKit-level errors, this handles bootstrap-time fallout.
  onMount(() => {
    // registerType: 'autoUpdate' in vite.config.ts (with skipWaiting +
    // clientsClaim) means a newer SW activates as soon as it is found.
    // registerSW({ immediate: true }) registers on load and reloads the
    // page once when the new SW takes over, so updates land on the next
    // launch (next cold start on iOS PWA when the household relaunches).
    try {
      registerSW({ immediate: true })
    } catch (err) {
      console.error('[layout] registerSW failed:', err)
    }

    // Capture the prefs load so surfaceGlance() can await it: glance.load()
    // reads prefsStore.glance* at call time, so the first surface must wait for
    // prefs to land or it fetches with the default window/max and the
    // badge/cap look reset after a restart. prefs.load() never rejects; the
    // Promise.resolve() fallback covers the synchronous-throw branch.
    let prefsReady: Promise<void> = Promise.resolve()
    try {
      prefsReady = prefsStore.load()
    } catch (err) {
      console.error('[layout] prefsStore.load failed:', err)
    }
    try {
      camerasStore.startPolling()
    } catch (err) {
      console.error('[layout] camerasStore.startPolling failed:', err)
    }
    try {
      configStore.init()
    } catch (err) {
      console.error('[layout] configStore.init failed:', err)
    }
    try {
      eventsStreamStore.init()
    } catch (err) {
      console.error('[layout] eventsStreamStore.init failed:', err)
    }
    try {
      groupsStore.init()
    } catch (err) {
      console.error('[layout] groupsStore.init failed:', err)
    }
    // Cold-open trigger: surface the peek only when the server says
    // this device is "away" AND the user is on the grid AND there are
    // unseen moments. The away verdict comes from the per-device
    // heartbeat session — an active device never trips it.
    //
    // sessionStorage survives the in-session location.reload() that
    // the PWA service worker fires on autoUpdate, but is cleared on
    // a genuine cold launch. The post-reload heartbeat would report
    // this device as no-longer-away (the pre-reload ping marked it
    // active), which would skip the surfacing gate and make the
    // peek appear to flash and vanish. Persisting the surfacing
    // intent re-surfaces the peek across that reload only.
    async function surfaceGlance(): Promise<void> {
      try {
        // Wait for prefs before any glance.load(): the fetch query is built
        // from prefsStore.glance* at call time, so racing ahead of prefs would
        // use the store defaults (max 20 / 24h) and reset the badge/cap on a
        // cold launch. On resume prefs are already loaded, so this is a no-op.
        await prefsReady
        if (glanceStore.wasSurfaced()) {
          // Post-reload re-surface: skip the heartbeat (its verdict
          // is poisoned by our own pre-reload ping; startPing keeps
          // the device active going forward).
          await glanceStore.load()
          if (page.route.id === '/' && glanceStore.unseenCount > 0) {
            glanceStore.openPeek()
          }
          return
        }
        const [away] = await Promise.all([glanceStore.heartbeat(), glanceStore.load()])
        if (away && page.route.id === '/' && glanceStore.unseenCount > 0) {
          glanceStore.markSurfaced()
          glanceStore.openPeek()
        }
      } catch (err) {
        console.error('[layout] surfaceGlance failed:', err)
      }
    }

    // Foreground heartbeat ping: while the user has the app open, keep
    // the device session active so it never trips the away threshold
    // server-side. The result is ignored — surfacing is driven by the
    // resume-time heartbeat in surfaceGlance(), not by this tick.
    let pingTimer: ReturnType<typeof setInterval> | null = null
    function startPing(): void {
      if (pingTimer !== null) return
      pingTimer = setInterval(
        () => {
          void glanceStore.heartbeat()
        },
        5 * 60 * 1000
      )
    }
    function stopPing(): void {
      if (pingTimer === null) return
      clearInterval(pingTimer)
      pingTimer = null
    }

    void surfaceGlance()
    startPing()

    lifecycle.init()

    // Cameras polling and SSE re-open on resume; pause/close on background.
    // Glance surfacing also runs on resume — the server's away verdict
    // decides whether the peek pops, so a short absence stays quiet.
    const offBg = lifecycle.onBackground(() => {
      camerasStore.stopPolling()
      eventsStreamStore.close()
      stopPing()
    })
    const offFg = lifecycle.onForeground(() => {
      camerasStore.startPolling()
      eventsStreamStore.reopen()
      void surfaceGlance()
      startPing()
    })

    return () => {
      offBg()
      offFg()
      camerasStore.stopPolling()
      stopPing()
    }
  })
</script>

<svelte:window bind:innerWidth={width} />

<div class="shell bg-[var(--bg)] text-[var(--text)]">
  {#if !isImmersive || isDesktop}
    <AppHeader {isDesktop} />
  {/if}

  {@render children()}

  {#if !isDesktop}
    <MobileTabBar />
  {/if}

  <GlancePeek />
  <ConnectionOverlay />
</div>

<InstallPrompt />

<style>
  /* Two display modes, two viewports, two answers.

     In a browser tab the shell is sized in `dvh`, via --screen-h. A bare `vh`
     is the viewport measured with the mobile toolbar retracted, so while that
     toolbar is showing it overstates the space: the document came out taller
     than the screen on every route and could be dragged up by the toolbar's
     height to reveal nothing. `dvh` tracks the toolbar instead of ignoring it.
     See app.css.

     In the installed app there is no such toolbar, and iOS does the opposite
     — it withholds the last stretch of the viewport from a document that
     cannot scroll. A page that fits is handed a viewport shorter than the
     screen by the top safe-area inset (812 against 874 physical on a notched
     device) and only gets the rest once the document becomes scrollable. A
     fixed, bottom-anchored element is anchored to that short viewport, so the
     tab bar floats an inset clear of the physical bottom edge. The shell is
     therefore kept a full screen height tall in standalone, which is what
     holds the bar against the bottom edge from the very first frame.

     A document only barely taller than the viewport does not do it. The
     expansion then lands lazily, after first paint, and on a cold start the
     bar is drawn against the short viewport and visibly slides down into
     place; a full screen height gets the expansion during first layout.

     `lvh` rather than `vh`: identical in standalone, but it names what is
     meant — the viewport at its largest — and it leaves a repo-wide grep for
     a bare `vh` meaningful as a guard against the browser-tab regression
     coming back.

     Both are minimums, so a page taller than the screen grows and scrolls as
     before. */
  .shell {
    min-height: var(--screen-h);
  }

  @media (display-mode: standalone) {
    .shell {
      min-height: 100lvh;
    }
  }
</style>
