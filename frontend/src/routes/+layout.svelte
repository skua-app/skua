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
  const isFocus = $derived(page.route.id === '/cam/[id]')

  // All bootstrap goes through onMount, not $effect: each side-effect must
  // run exactly once per session. Previous $effect-based registerSW could
  // re-run on reactive churn and stack SW registrations; store inits had
  // their own #initStarted guards but the outer wrapper still risked
  // double-invocation. Wrapping each in try/catch ensures a single
  // unfortunate throw can't blank the entire shell — +error.svelte handles
  // SvelteKit-level errors, this handles bootstrap-time fallout.
  onMount(() => {
    // SW registration is deliberately silent: registerType: 'prompt' in
    // vite.config.ts means a newer SW waits in the background and only
    // takes over on the next cold start. No prompt UI; the household app
    // gets the next version when iOS relaunches the PWA.
    try {
      registerSW({ immediate: true })
    } catch (err) {
      console.error('[layout] registerSW failed:', err)
    }

    try {
      prefsStore.load()
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
    async function surfaceGlance(): Promise<void> {
      try {
        const [away] = await Promise.all([glanceStore.heartbeat(), glanceStore.load()])
        if (away && page.route.id === '/' && glanceStore.unseenCount > 0) {
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

<div class="min-h-screen bg-[var(--bg)] text-[var(--text)]">
  {#if !isFocus}
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
