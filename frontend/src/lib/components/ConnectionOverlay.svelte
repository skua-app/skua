<script lang="ts">
  import { camerasStore } from '$lib/stores/cameras.svelte'
  import { ui } from '$lib/i18n/strings'

  let retrying = $state(false)

  async function retry() {
    if (retrying) return
    retrying = true
    try {
      await camerasStore.refresh()
    } finally {
      retrying = false
    }
  }

  // Pick the user-facing message from the classified ApiError kind.
  const reasonText = $derived.by(() => {
    switch (camerasStore.errorKind) {
      case 'offline':
        return ui.connOffline
      case 'timeout':
        return ui.connTimeout
      case 'server':
        return ui.connServer
      default:
        return ui.connUnknown
    }
  })

  // hasLoaded gates the two states: a cold launch with no cached data
  // shows the full-screen card; a mid-session drop shows a slim banner
  // over the cached content.
  const showCard = $derived(!camerasStore.reachable && !camerasStore.hasLoaded)
  const showBanner = $derived(!camerasStore.reachable && camerasStore.hasLoaded)
</script>

{#if showCard}
  <div class="co-root" role="alertdialog" aria-live="assertive">
    <div class="co-card">
      <span class="co-mark" aria-hidden="true">
        <span class="logo-mark">
          <span class="bracket tl"></span>
          <span class="bracket tr"></span>
          <span class="bracket bl"></span>
          <span class="bracket br"></span>
        </span>
      </span>
      <div class="co-title">{ui.connTitle}</div>
      <div class="co-reason">{reasonText}</div>
      <div class="co-hint">{ui.connHint}</div>
      <button type="button" class="co-btn" onclick={retry} disabled={retrying}>
        {retrying ? ui.connecting : ui.retry}
      </button>
    </div>
  </div>
{:else if showBanner}
  <div class="co-banner" role="status" aria-live="polite">
    <span class="dot" aria-hidden="true"></span>
    <span class="msg">{ui.connBannerStale}</span>
    <button type="button" class="co-banner-btn" onclick={retry} disabled={retrying}>
      {retrying ? ui.connecting : ui.retry}
    </button>
  </div>
{/if}

<style>
  /* Full-screen state — mirrors +error.svelte. */
  .co-root {
    position: fixed;
    inset: 0;
    z-index: 80;
    background: var(--bg);
    color: var(--text);
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 24px;
    padding-top: calc(env(safe-area-inset-top, 0px) + 24px);
    padding-bottom: calc(env(safe-area-inset-bottom, 0px) + 24px);
  }
  .co-card {
    width: 100%;
    max-width: 380px;
    padding: 24px;
    border-radius: 12px;
    background: var(--surface);
    border: 1px solid var(--border);
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 10px;
    text-align: center;
  }
  .co-mark {
    width: 38px;
    height: 38px;
    border-radius: 11px;
    background: var(--accent-soft);
    color: var(--accent-ink);
    display: grid;
    place-items: center;
    margin-bottom: 2px;
  }
  .logo-mark {
    width: 16px;
    height: 16px;
    position: relative;
    display: inline-block;
  }
  .bracket {
    position: absolute;
    width: 6px;
    height: 6px;
  }
  .bracket.tl {
    top: 0;
    left: 0;
    border-top: 1.6px solid var(--accent);
    border-left: 1.6px solid var(--accent);
  }
  .bracket.tr {
    top: 0;
    right: 0;
    border-top: 1.6px solid var(--accent);
    border-right: 1.6px solid var(--accent);
  }
  .bracket.bl {
    bottom: 0;
    left: 0;
    border-bottom: 1.6px solid var(--accent);
    border-left: 1.6px solid var(--accent);
  }
  .bracket.br {
    bottom: 0;
    right: 0;
    border-bottom: 1.6px solid var(--accent);
    border-right: 1.6px solid var(--accent);
  }
  .co-title {
    font-size: 17px;
    font-weight: 600;
    letter-spacing: -0.2px;
    color: var(--text);
  }
  .co-reason {
    font-size: 14px;
    color: var(--text-2);
  }
  .co-hint {
    font-size: 12.5px;
    color: var(--text-3);
    line-height: 1.5;
    margin-top: 2px;
  }
  .co-btn {
    margin-top: 8px;
    padding: 10px 18px;
    border-radius: 8px;
    border: 1px solid color-mix(in oklch, var(--accent) 60%, transparent);
    background: color-mix(in oklch, var(--accent) 14%, transparent);
    color: var(--text);
    font-size: 14px;
    font-weight: 500;
    font-family: inherit;
    cursor: pointer;
    transition: background 0.15s ease;
  }
  .co-btn:hover:not(:disabled) {
    background: color-mix(in oklch, var(--accent) 22%, transparent);
  }
  .co-btn:disabled {
    opacity: 0.55;
    cursor: progress;
  }

  /* Slim stale banner pinned below the sticky header. */
  .co-banner {
    position: fixed;
    left: 50%;
    transform: translateX(-50%);
    top: calc(env(safe-area-inset-top, 0px) + var(--app-header-h, 64px) + 8px);
    z-index: 25;
    display: inline-flex;
    align-items: center;
    gap: 10px;
    padding: 8px 12px;
    border-radius: 999px;
    background: var(--surface);
    border: 1px solid var(--border-strong);
    box-shadow: var(--shadow);
    color: var(--text);
    font-size: 13px;
    max-width: calc(100% - 24px);
    animation: co-fade 0.18s ease;
  }
  .co-banner .dot {
    width: 7px;
    height: 7px;
    border-radius: 50%;
    background: var(--warn);
    flex: 0 0 auto;
  }
  .co-banner .msg {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .co-banner-btn {
    flex: 0 0 auto;
    padding: 4px 10px;
    border-radius: 999px;
    border: 1px solid color-mix(in oklch, var(--accent) 50%, transparent);
    background: transparent;
    color: var(--accent-ink);
    font-family: inherit;
    font-size: 12.5px;
    font-weight: 500;
    cursor: pointer;
  }
  .co-banner-btn:hover:not(:disabled) {
    background: var(--accent-soft);
  }
  .co-banner-btn:disabled {
    opacity: 0.55;
    cursor: progress;
  }

  @keyframes co-fade {
    from {
      opacity: 0;
      transform: translate(-50%, -6px);
    }
    to {
      opacity: 1;
      transform: translateX(-50%);
    }
  }
  @media (prefers-reduced-motion: reduce) {
    .co-banner {
      animation: none;
    }
    .co-btn {
      transition: none;
    }
  }
</style>
