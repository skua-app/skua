<script lang="ts">
  import { onMount } from 'svelte'
  import Icon from '$lib/components/Icon.svelte'
  import { storageStore } from '$lib/stores/storage.svelte'
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
      {#each storageStore.mounts as m (m.path)}
        <div class="st-mount">
          <div class="st-row1">
            <span class="st-path mono">{m.path}</span>
            {#if m.type}<span class="st-type">{m.type}</span>{/if}
          </div>
          <div class="st-bar" aria-hidden="true">
            <span class="st-fill" style:width={`${usedPct(m.used_mib, m.total_mib)}%`}></span>
          </div>
          <div class="st-row2">
            <span class="st-usage mono">{formatSize(m.used_mib)} / {formatSize(m.total_mib)}</span>
            <span class="st-free mono">{formatSize(m.free_mib)} {ui.storageFree}</span>
          </div>
        </div>
      {/each}
    </div>
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
  .st-type {
    flex: 0 0 auto;
    font-size: 11px;
    color: var(--text-2);
    padding: 2px 7px;
    border: 1px solid var(--border);
    border-radius: 999px;
  }
  .st-bar {
    height: 7px;
    margin: 10px 0 8px;
    border-radius: 999px;
    background: var(--surface-2, rgba(255, 255, 255, 0.06));
    overflow: hidden;
  }
  .st-fill {
    display: block;
    height: 100%;
    border-radius: 999px;
    background: var(--accent);
    transition: width 0.2s ease;
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
