<script lang="ts">
  import CameraCard from '$lib/components/settings/CameraCard.svelte'
  import { RefreshApiError, refreshCameras } from '$lib/api'
  import { camerasStore } from '$lib/stores/cameras.svelte'
  import { groupsStore } from '$lib/stores/groups.svelte'
  import { go2rtcStreamsStore } from '$lib/stores/go2rtcStreams.svelte'
  import { ui } from '$lib/i18n/strings'

  let refreshing = $state(false)
  let refreshStatus = $state<string | null>(null)
  let refreshStatusKind = $state<'success' | 'error' | null>(null)
  let refreshStatusTimer: ReturnType<typeof setTimeout> | null = null

  function clearRefreshStatusSoon() {
    if (refreshStatusTimer !== null) clearTimeout(refreshStatusTimer)
    refreshStatusTimer = setTimeout(() => {
      refreshStatus = null
      refreshStatusKind = null
      refreshStatusTimer = null
    }, 5000)
  }

  function formatDiff(added: string[], removed: string[]): string {
    if (added.length === 0 && removed.length === 0) return ui.refreshNoChanges
    const parts: string[] = []
    if (added.length > 0) parts.push(`${ui.refreshDiffAdded}: ${added.join(', ')}.`)
    if (removed.length > 0) parts.push(`${ui.refreshDiffRemoved}: ${removed.join(', ')}.`)
    return parts.join(' ')
  }

  async function doRefreshCameras() {
    refreshing = true
    if (refreshStatusTimer !== null) {
      clearTimeout(refreshStatusTimer)
      refreshStatusTimer = null
    }
    refreshStatus = null
    refreshStatusKind = null
    try {
      const diff = await refreshCameras()
      refreshStatus = formatDiff(diff.added, diff.removed)
      refreshStatusKind = 'success'
      // SSE camera.added / camera.removed round-trip too; refetch here for
      // immediate UI consistency. Idempotent.
      camerasStore.refresh().catch(() => {})
      groupsStore.refresh().catch(() => {})
    } catch (err) {
      refreshStatus =
        err instanceof RefreshApiError && err.code === 'frigate_unreachable'
          ? ui.refreshFrigateDown
          : ui.refreshGenericError
      refreshStatusKind = 'error'
    } finally {
      refreshing = false
      clearRefreshStatusSoon()
    }
  }
</script>

<section id="cameras" class="settings-section">
  <header class="section-header">
    <h2 class="section-title">{ui.sectionCameras}</h2>
    <p class="section-hint">{ui.refreshHint}</p>
    <p class="section-hint section-hint-secondary">{ui.streamsHint}</p>
  </header>

  <div class="refresh-row">
    <button type="button" class="btn btn-primary" disabled={refreshing} onclick={doRefreshCameras}>
      {refreshing ? ui.refreshingCameras : ui.refreshCameras}
    </button>
    {#if refreshStatus}
      <div
        class="refresh-status"
        class:refresh-status-error={refreshStatusKind === 'error'}
        role="status"
        aria-live="polite"
      >
        {refreshStatus}
      </div>
    {/if}
  </div>

  {#if go2rtcStreamsStore.error}
    <div class="streams-error" role="alert">{ui.streamsLoadError}</div>
  {/if}

  <div class="cards">
    {#each camerasStore.cameras as cam (cam.id)}
      <CameraCard
        camera={cam}
        streams={go2rtcStreamsStore.streams}
        streamsLoaded={go2rtcStreamsStore.loaded && go2rtcStreamsStore.error === null}
        streamsDisabled={go2rtcStreamsStore.error !== null}
      />
    {/each}
  </div>
</section>

<style>
  .settings-section {
    display: block;
  }
  .section-header {
    margin-bottom: 12px;
  }
  .section-title {
    font-size: 15px;
    font-weight: 600;
    letter-spacing: -0.2px;
    margin-bottom: 6px;
  }
  .section-hint {
    font-size: 12px;
    color: var(--text-3);
    line-height: 1.4;
  }
  .section-hint-secondary {
    margin-top: 4px;
  }
  .refresh-row {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: 8px;
    margin-bottom: 18px;
  }
  @media (min-width: 600px) {
    .refresh-row {
      flex-direction: row;
      align-items: center;
      gap: 16px;
    }
  }
  .refresh-status {
    font-size: 12px;
    color: var(--text-2);
    line-height: 1.4;
  }
  .refresh-status-error {
    color: oklch(0.68 0.16 25);
  }
  .streams-error {
    margin-bottom: 12px;
    padding: 10px 12px;
    border: 1px solid color-mix(in oklab, oklch(0.68 0.16 25) 40%, transparent);
    border-radius: 8px;
    font-size: 12px;
    color: oklch(0.68 0.16 25);
    background: color-mix(in oklab, oklch(0.68 0.16 25) 8%, transparent);
    line-height: 1.4;
  }
  .cards {
    display: flex;
    flex-direction: column;
    gap: 10px;
  }
  .btn {
    padding: 6px 12px;
    font-size: 12px;
    color: var(--text);
    background: transparent;
    border: 1px solid var(--border-strong);
    border-radius: 6px;
    font-family: inherit;
    cursor: pointer;
    transition:
      background 120ms,
      border-color 120ms,
      color 120ms,
      opacity 120ms;
    flex-shrink: 0;
  }
  .btn:hover:not(:disabled) {
    background: rgba(255, 255, 255, 0.04);
  }
  .btn:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }
  .btn-primary {
    color: var(--bg);
    background: var(--accent);
    border-color: var(--accent);
  }
  .btn-primary:hover:not(:disabled) {
    background: color-mix(in oklab, var(--accent) 88%, white 12%);
  }
</style>
