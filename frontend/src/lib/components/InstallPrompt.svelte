<script lang="ts">
  import { isIOS, isStandalone } from '$lib/utils/platform'
  import { ui } from '$lib/i18n/strings'

  let showIOSHint = $state(false)
  let showUpdateToast = $state(false)
  let deferredPrompt = $state<(Event & { prompt: () => Promise<void> }) | null>(null)
  let dismissed = $state(false)

  $effect(() => {
    if (isIOS() && !isStandalone()) {
      showIOSHint = true
    }

    const handler = (e: Event) => {
      e.preventDefault()
      deferredPrompt = e as Event & { prompt: () => Promise<void> }
    }
    window.addEventListener('beforeinstallprompt', handler)

    return () => window.removeEventListener('beforeinstallprompt', handler)
  })

  $effect(() => {
    // Listen for SW-triggered update
    if ('serviceWorker' in navigator) {
      navigator.serviceWorker.addEventListener('controllerchange', () => {
        showUpdateToast = true
        setTimeout(() => (showUpdateToast = false), 4000)
      })
    }
  })

  async function installApp() {
    if (!deferredPrompt) return
    await deferredPrompt.prompt()
    deferredPrompt = null
  }
</script>

<!-- Android/desktop install banner -->
{#if deferredPrompt && !dismissed}
  <div
    class="fixed right-4 bottom-4 left-4 z-50 flex items-center justify-between rounded-xl bg-white/10 px-4 py-3 backdrop-blur-md sm:right-4 sm:left-auto sm:w-80"
  >
    <span class="text-sm">{ui.installAppTitle}</span>
    <div class="flex gap-2">
      <button
        onclick={installApp}
        class="rounded-lg bg-white px-3 py-1 text-sm font-medium text-black"
      >
        {ui.install}
      </button>
      <button
        onclick={() => (dismissed = true)}
        class="px-2 text-white/50 hover:text-white"
        aria-label={ui.close}
      >
        ✕
      </button>
    </div>
  </div>
{/if}

<!-- iOS install hint -->
{#if showIOSHint && !dismissed}
  <div class="fixed right-4 bottom-4 left-4 z-50 rounded-xl bg-white/10 px-4 py-3 backdrop-blur-md">
    <div class="flex items-start justify-between gap-2">
      <p class="text-sm leading-relaxed">
        {ui.installIOSHintTap} <span class="font-semibold">{ui.installIOSHintShare}</span> →
        <span class="font-semibold">{ui.installIOSHintHome}</span>
      </p>
      <button
        onclick={() => (dismissed = true)}
        class="shrink-0 text-white/50 hover:text-white"
        aria-label={ui.close}
      >
        ✕
      </button>
    </div>
  </div>
{/if}

<!-- Update toast -->
{#if showUpdateToast}
  <div
    class="fixed bottom-4 left-1/2 z-50 -translate-x-1/2 rounded-lg bg-white/10 px-4 py-2 text-sm backdrop-blur-md"
  >
    {ui.appUpdatedToast}
  </div>
{/if}
