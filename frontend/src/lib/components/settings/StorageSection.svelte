<script lang="ts">
  import { onMount } from 'svelte'
  import Icon from '$lib/components/Icon.svelte'
  import { storageStore } from '$lib/stores/storage.svelte'
  import { camerasStore } from '$lib/stores/cameras.svelte'
  import { formatSize } from '$lib/util/size'
  import { ui } from '$lib/i18n/strings'

  onMount(() => {
    void storageStore.load()
  })

  // Clamp the bar fill to [0,100]. Frigate's used/total are independent
  // figures, so guard against a zero/garbage total producing NaN or overflow.
  function usedPct(used: number, total: number): number {
    if (!Number.isFinite(used) || !Number.isFinite(total) || total <= 0) return 0
    return Math.max(0, Math.min(100, (used / total) * 100))
  }

  // The per-camera usage_percent is already normalized 0–100 by the BFF;
  // clamp defensively before driving the bar width.
  function clampPct(pct: number): number {
    if (!Number.isFinite(pct)) return 0
    return Math.max(0, Math.min(100, pct))
  }

  // Resolve a camera's display name from the cameras store, falling back to
  // the Frigate id (same pattern as the events list).
  function camName(id: string): string {
    return camerasStore.cameras.find((c) => c.id === id)?.name ?? id
  }

  // True when the camera has a real friendly name set (present in the store
  // and its name differs from its id). Used to decide whether to surface the
  // raw id as a secondary label without duplicating "camN camN".
  function hasFriendlyName(id: string): boolean {
    const cam = camerasStore.cameras.find((c) => c.id === id)
    return cam !== undefined && cam.name !== id
  }

  const PALETTE_SIZE = 12

  // A camera's color is its stable index in the canonical Frigate order
  // (camerasStore.cameras), so it stays the same regardless of the usage
  // sort, cycling past 12. A camera present in storage but gone from the
  // store (e.g. removed) falls back to the neutral token.
  function camColor(id: string): string {
    const idx = camerasStore.cameras.findIndex((c) => c.id === id)
    if (idx >= 0) return `var(--cam-${idx % PALETTE_SIZE})`
    return 'var(--cam-other)'
  }

  // Identify the recordings mount from the data, not a hardcoded path: the
  // disk total implied by any camera's usage_mib / usage_percent matches the
  // recordings mount's total_mib. recordings + clips can share a filesystem
  // and report identical totals, so prefer a path containing "record".
  function recordingsMountPath(): string | null {
    const cam = storageStore.cameras.find((c) => c.usage_percent > 0)
    if (!cam) return null
    const denom = cam.usage_mib / (cam.usage_percent / 100)
    if (!Number.isFinite(denom) || denom <= 0) return null
    const matches = storageStore.mounts.filter(
      (m) => m.total_mib > 0 && Math.abs(m.total_mib - denom) / denom <= 0.01
    )
    const first = matches[0]
    if (!first) return null
    if (matches.length === 1) return first.path
    const rec = matches.find((m) => m.path.toLowerCase().includes('record'))
    return (rec ?? first).path
  }

  const recPath = $derived(recordingsMountPath())

  // Render the recordings mount first, then the rest in backend order. Derive
  // a local list — never mutate the store array.
  const orderedMounts = $derived.by(() => {
    if (recPath === null) return storageStore.mounts
    const rec = storageStore.mounts.find((m) => m.path === recPath)
    if (!rec) return storageStore.mounts
    return [rec, ...storageStore.mounts.filter((m) => m.path !== recPath)]
  })

  // Unaccounted "other" used space on the recordings mount (clips/db/etc.):
  // the used fraction beyond the sum of the per-camera shares. Never negative,
  // and intentionally not normalized — camera segments stay honest share-of-disk.
  function otherPct(used: number, total: number): number {
    const sum = storageStore.cameras.reduce((s, c) => s + clampPct(c.usage_percent), 0)
    return Math.max(0, usedPct(used, total) - sum)
  }
</script>

<section id="storage" class="settings-section dk-set-section">
  <h2 class="dk-set-h2">{ui.sectionStorage}</h2>
  <p class="dk-set-desc">{ui.storageDescription}</p>

  <div class="st-head">
    <button
      type="button"
      class="st-refresh"
      onclick={() => void storageStore.load()}
      disabled={storageStore.loading}
    >
      <Icon name="refresh" size={15} />
      {storageStore.loading ? ui.storageRefreshing : ui.storageRefresh}
    </button>
  </div>

  {#if storageStore.error && storageStore.mounts.length === 0}
    <div class="dk-card st-state">
      <span>{ui.storageLoadError}</span>
      <button type="button" class="st-retry" onclick={() => void storageStore.load()}
        >{ui.retry}</button
      >
    </div>
  {:else if storageStore.loading && storageStore.mounts.length === 0}
    <div class="dk-card st-state">{ui.storageLoading}</div>
  {:else if storageStore.mounts.length === 0}
    <div class="dk-card st-state">{ui.storageEmpty}</div>
  {:else}
    <div class="dk-card">
      {#each orderedMounts as m (m.path)}
        <div class="st-mount">
          <div class="st-row1">
            <span class="st-path mono">{m.path}</span>
            {#if m.type}<span class="st-type">{m.type}</span>{/if}
          </div>
          <div class="st-bar" aria-hidden="true">
            {#if m.path === recPath}
              {#each storageStore.cameras as c (c.id)}
                <span
                  class="st-seg"
                  style:width={`${clampPct(c.usage_percent)}%`}
                  style:background={camColor(c.id)}
                  title={`${camName(c.id)} · ${formatSize(c.usage_mib)}`}
                ></span>
              {/each}
              <span
                class="st-seg"
                style:width={`${otherPct(m.used_mib, m.total_mib)}%`}
                style:background="var(--cam-other)"
              ></span>
            {:else}
              <span class="st-fill" style:width={`${usedPct(m.used_mib, m.total_mib)}%`}></span>
            {/if}
          </div>
          <div class="st-row2">
            <span class="st-usage mono">{formatSize(m.used_mib)} / {formatSize(m.total_mib)}</span>
            <span class="st-free mono">{formatSize(m.free_mib)} {ui.storageFree}</span>
          </div>
        </div>
      {/each}
    </div>

    <h3 class="st-subhead">{ui.storageCameras}</h3>
    {#if storageStore.cameras.length === 0}
      <div class="dk-card st-state">{ui.storagePerCameraEmpty}</div>
    {:else}
      <div class="dk-card">
        {#each storageStore.cameras as c (c.id)}
          <div class="st-mount">
            <div class="st-row1">
              <span class="st-swatch" style:background={camColor(c.id)} aria-hidden="true"></span>
              <span class="st-camline">
                <span class="st-cam">{camName(c.id)}</span>
                {#if hasFriendlyName(c.id)}<span class="st-cam-id mono">{c.id}</span>{/if}
              </span>
            </div>
            <div class="st-bar" aria-hidden="true">
              <span
                class="st-fill"
                style:width={`${clampPct(c.usage_percent)}%`}
                style:background={camColor(c.id)}
              ></span>
            </div>
            <div class="st-row2">
              <span class="st-usage mono"
                >{formatSize(c.usage_mib)} · {c.usage_percent.toFixed(1)}%</span
              >
              <span class="st-free mono">{formatSize(c.bandwidth_mib_per_hr)}{ui.storagePerHr}</span
              >
            </div>
          </div>
        {/each}
      </div>
    {/if}
  {/if}
</section>

<style>
  .settings-section {
    display: block;
  }
  .dk-set-section {
    max-width: 720px;
  }
  .dk-set-h2 {
    font-size: 20px;
    font-weight: 600;
    letter-spacing: -0.3px;
    margin: 0 0 4px;
    color: var(--text);
  }
  /* Mobile drill-head already shows the section title; hide the in-section h2
     below the master/detail breakpoint, matching the other sections. */
  @media (max-width: 899.98px) {
    .dk-set-h2 {
      display: none;
    }
  }
  .dk-set-desc {
    font-size: 13.5px;
    color: var(--text-2);
    margin: 0 0 20px;
    line-height: 1.5;
  }

  .st-head {
    display: flex;
    justify-content: flex-end;
    margin-bottom: 12px;
  }
  .st-refresh {
    display: inline-flex;
    align-items: center;
    gap: 7px;
    padding: 7px 13px;
    font-size: 13px;
    font-weight: 500;
    color: var(--text);
    background: var(--surface);
    border: 1px solid var(--border-strong);
    border-radius: var(--r-sm);
    cursor: pointer;
    font-family: inherit;
    transition:
      color 0.14s ease,
      border-color 0.14s ease;
  }
  .st-refresh:hover:not(:disabled) {
    border-color: var(--text-3);
  }
  .st-refresh:disabled {
    color: var(--text-3);
    cursor: default;
  }
  .st-retry {
    align-self: flex-start;
    padding: 6px 12px;
    font-size: 12px;
    color: var(--text);
    background: transparent;
    border: 1px solid var(--border-strong);
    border-radius: var(--r-sm);
    cursor: pointer;
    font-family: inherit;
  }

  /* prototype .dk-card — bordered surface card */
  .dk-card {
    border: 1px solid var(--border);
    border-radius: var(--r);
    background: var(--surface);
    padding: 6px 18px;
    margin-bottom: 16px;
  }
  .st-state {
    display: flex;
    flex-direction: column;
    gap: 10px;
    padding: 18px;
    font-size: 13px;
    color: var(--text-2);
  }

  .st-mount {
    padding: 16px 0;
    border-bottom: 1px solid var(--border);
  }
  .st-mount:last-child {
    border-bottom: none;
  }
  .st-row1 {
    display: flex;
    align-items: center;
    gap: 10px;
    min-width: 0;
  }
  .st-path {
    font-size: 13px;
    color: var(--text);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    min-width: 0;
  }
  /* Name + raw id share one baseline so the differing Geist/mono metrics don't
     make the id appear to float; the swatch stays centered in .st-row1. */
  .st-camline {
    display: inline-flex;
    align-items: baseline;
    gap: 8px;
    min-width: 0;
  }
  .st-cam {
    font-size: 14px;
    font-weight: 500;
    color: var(--text);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    min-width: 0;
  }
  /* Raw Frigate id shown after a friendly name as a dim mono secondary. */
  .st-cam-id {
    flex: 0 0 auto;
    font-size: 11px;
    color: var(--text-3);
  }
  .st-subhead {
    font-size: 13px;
    font-weight: 600;
    letter-spacing: -0.2px;
    color: var(--text);
    margin: 4px 0 12px;
  }
  .st-type {
    flex: 0 0 auto;
    font-size: 11px;
    color: var(--text-2);
    padding: 2px 7px;
    border: 1px solid var(--border);
    border-radius: 999px;
  }
  .st-bar {
    display: flex;
    height: 7px;
    margin: 10px 0 8px;
    border-radius: 999px;
    background: var(--surface-2, rgba(255, 255, 255, 0.06));
    overflow: hidden;
  }
  .st-fill {
    height: 100%;
    border-radius: 999px;
    background: var(--accent);
    transition: width 0.2s ease;
  }
  /* Stacked per-camera segments in the recordings mount bar. The track clips
     the ends via overflow:hidden, so segments carry no individual radius. A
     2px inset shadow on the right edge separates adjacent colors so they don't
     blend; it draws inside the box (no layout/width shift). The last segment
     (the neutral "other" span) is excluded so there's no trailing line against
     the free track. */
  .st-seg {
    flex: 0 0 auto;
    height: 100%;
  }
  .st-seg:not(:last-child) {
    box-shadow: inset -2px 0 0 var(--bg);
  }
  /* Per-camera color swatch before the camera name. */
  .st-swatch {
    flex: 0 0 auto;
    width: 9px;
    height: 9px;
    border-radius: 3px;
  }
  .st-row2 {
    display: flex;
    justify-content: space-between;
    gap: 10px;
    font-size: 12px;
    color: var(--text-2);
  }
  .st-free {
    color: var(--text-3);
  }
  .mono {
    font-family: 'JetBrains Mono', ui-monospace, monospace;
    font-feature-settings: 'ss01', 'cv11';
  }
</style>
