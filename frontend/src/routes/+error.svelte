<script lang="ts">
  import { page } from '$app/state'
  import { ui } from '$lib/i18n/strings'

  function reload() {
    window.location.reload()
  }
</script>

<div class="error-root">
  <div class="error-card">
    <div class="error-title">{ui.errorTitle}</div>
    <div class="error-subtitle">{ui.errorSubtitle}</div>
    {#if page.error?.message}
      <div class="error-detail">{page.error.message}</div>
    {/if}
    <button type="button" class="error-btn" onclick={reload}>{ui.errorReload}</button>
  </div>
</div>

<style>
  .error-root {
    min-height: var(--screen-h);
    background: var(--bg);
    color: var(--text);
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 24px;
  }
  .error-card {
    width: 100%;
    max-width: 360px;
    padding: 24px;
    border-radius: 12px;
    background: var(--surface);
    border: 1px solid var(--border);
    display: flex;
    flex-direction: column;
    gap: 12px;
    text-align: center;
  }
  .error-title {
    font-size: 17px;
    font-weight: 600;
    letter-spacing: -0.2px;
  }
  .error-subtitle {
    font-size: 14px;
    color: var(--text-2);
  }
  .error-detail {
    font-family: 'JetBrains Mono', ui-monospace, monospace;
    font-size: 11px;
    color: var(--text-3);
    word-break: break-word;
    padding: 8px 10px;
    border-radius: 6px;
    background: var(--surface);
    border: 1px solid var(--border);
  }
  .error-btn {
    margin-top: 4px;
    padding: 10px 16px;
    border-radius: 8px;
    border: 1px solid color-mix(in oklch, var(--accent) 60%, transparent);
    background: color-mix(in oklch, var(--accent) 14%, transparent);
    color: var(--text);
    font-size: 14px;
    font-weight: 500;
    cursor: pointer;
  }
  .error-btn:hover {
    background: color-mix(in oklch, var(--accent) 22%, transparent);
  }
</style>
