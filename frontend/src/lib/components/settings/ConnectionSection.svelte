<script lang="ts">
  import { onDestroy } from 'svelte'
  import {
    RuntimeConfigApiError,
    restartRuntimeConfig,
    saveRuntimeConfig,
    testRuntimeConfig
  } from '$lib/api'
  import type { ProbeReport } from '$lib/api'
  import { runtimeConfigStore } from '$lib/stores/runtimeConfig.svelte'
  import { ui } from '$lib/i18n/strings'

  // Draft state mirrors the overlay file values (what an unconfigured /
  // env-unlocked field can be edited to). Locked fields are surfaced
  // read-only from store.effective so the operator sees what's actually
  // running.
  let frigateDraft = $state('')
  let go2rtcDraft = $state('')
  let frigateUIDraft = $state('')
  let initialised = $state(false)

  // Initialise from the store when it loads; thereafter the local drafts
  // are the source of truth until the user saves or applies.
  $effect(() => {
    if (initialised || !runtimeConfigStore.loaded) return
    frigateDraft = runtimeConfigStore.overlay.frigate_url
    go2rtcDraft = runtimeConfigStore.overlay.go2rtc_url
    frigateUIDraft = runtimeConfigStore.overlay.frigate_ui_url
    initialised = true
  })

  let testing = $state(false)
  let saving = $state(false)
  let report = $state<ProbeReport | null>(null)
  let saveError = $state<string | null>(null)
  let testError = $state<string | null>(null)

  // Save / apply dirty-state: after a successful save, savedAt is set and
  // the Apply button becomes available. If the user edits the form again,
  // dirtyAfterSave flips and Apply is disabled until the user saves again.
  let savedAt = $state<number | null>(null)
  let dirtyAfterSave = $state(false)

  let confirmingApply = $state(false)
  let applying = $state(false)
  let applyFallbackVisible = $state(false)
  let pollHandle: ReturnType<typeof setInterval> | null = null
  let fallbackTimer: ReturnType<typeof setTimeout> | null = null

  function markDirty() {
    if (savedAt !== null) dirtyAfterSave = true
    report = null
  }

  function onFrigateInput(e: Event) {
    frigateDraft = (e.currentTarget as HTMLInputElement).value
    markDirty()
  }
  function onGo2rtcInput(e: Event) {
    go2rtcDraft = (e.currentTarget as HTMLInputElement).value
    markDirty()
  }
  function onFrigateUIInput(e: Event) {
    frigateUIDraft = (e.currentTarget as HTMLInputElement).value
    markDirty()
  }

  async function runTest() {
    testing = true
    testError = null
    try {
      const res = await testRuntimeConfig(frigateDraft.trim(), go2rtcDraft.trim())
      report = res
    } catch (err) {
      testError = err instanceof RuntimeConfigApiError ? err.message : ui.connectionTestErrorGeneric
    } finally {
      testing = false
    }
  }

  async function runSave() {
    saving = true
    saveError = null
    try {
      const next = await saveRuntimeConfig({
        frigate_url: frigateDraft.trim(),
        frigate_ui_url: frigateUIDraft.trim(),
        go2rtc_url: go2rtcDraft.trim()
      })
      runtimeConfigStore.apply(next)
      savedAt = Date.now()
      dirtyAfterSave = false
    } catch (err) {
      saveError = err instanceof RuntimeConfigApiError ? err.message : ui.connectionSaveErrorGeneric
    } finally {
      saving = false
    }
  }

  function requestApply() {
    confirmingApply = true
  }
  function cancelApply() {
    confirmingApply = false
  }
  async function confirmApply() {
    confirmingApply = false
    applying = true
    saveError = null
    try {
      await restartRuntimeConfig()
    } catch (err) {
      saveError = err instanceof RuntimeConfigApiError ? err.message : ui.connectionSaveErrorGeneric
      applying = false
      return
    }
    startHealthPoll()
  }

  function startHealthPoll() {
    if (pollHandle !== null) return
    pollHandle = setInterval(() => {
      void pollHealth()
    }, 2000)
    fallbackTimer = setTimeout(() => {
      applyFallbackVisible = true
    }, 30_000)
  }

  async function pollHealth() {
    try {
      const res = await fetch('/healthz', { cache: 'no-store' })
      if (res.ok) {
        stopHealthPoll()
        location.reload()
      }
    } catch {
      // mid-restart; keep polling
    }
  }

  function stopHealthPoll() {
    if (pollHandle !== null) {
      clearInterval(pollHandle)
      pollHandle = null
    }
    if (fallbackTimer !== null) {
      clearTimeout(fallbackTimer)
      fallbackTimer = null
    }
  }

  onDestroy(() => stopHealthPoll())

  const canApply = $derived(savedAt !== null && !dirtyAfterSave && !applying)
  // Frigate URL is required unless env-locked (in which case the server
  // holds it and the PUT body's frigate_url field is dropped server-side).
  const canSave = $derived(runtimeConfigStore.locked.frigate_url || frigateDraft.trim().length > 0)
</script>

<section id="connection" class="settings-section">
  <header class="section-header">
    <h2 class="section-title">{ui.sectionConnection}</h2>
  </header>

  {#if runtimeConfigStore.loadError}
    <div class="form-error">{ui.connectionLoadError}</div>
  {/if}

  <div class="card">
    <div class="row">
      <label class="row-label" for="connection-frigate">{ui.connectionFrigateURLLabel}</label>
      <div class="row-control">
        {#if runtimeConfigStore.locked.frigate_url}
          <div class="locked-value">{runtimeConfigStore.effective.frigate_url}</div>
        {:else}
          <input
            id="connection-frigate"
            class="text-input"
            type="url"
            placeholder="http://frigate:5000"
            value={frigateDraft}
            oninput={onFrigateInput}
            disabled={applying}
            autocomplete="off"
            spellcheck="false"
          />
        {/if}
      </div>
    </div>
    <div class="hint">
      {runtimeConfigStore.locked.frigate_url
        ? ui.connectionEnvLockedFrigate
        : ui.connectionFrigateURLHint}
    </div>

    <div class="row">
      <label class="row-label" for="connection-go2rtc">{ui.connectionGo2rtcURLLabel}</label>
      <div class="row-control">
        {#if runtimeConfigStore.locked.go2rtc_url}
          <div class="locked-value">{runtimeConfigStore.effective.go2rtc_url || '—'}</div>
        {:else}
          <input
            id="connection-go2rtc"
            class="text-input"
            type="url"
            placeholder="http://frigate:1984"
            value={go2rtcDraft}
            oninput={onGo2rtcInput}
            disabled={applying}
            autocomplete="off"
            spellcheck="false"
          />
        {/if}
      </div>
    </div>
    <div class="hint">
      {runtimeConfigStore.locked.go2rtc_url
        ? ui.connectionEnvLockedGo2rtc
        : ui.connectionGo2rtcURLHint}
    </div>

    <div class="row">
      <label class="row-label" for="connection-ui">{ui.connectionFrigateUIURLLabel}</label>
      <div class="row-control">
        {#if runtimeConfigStore.locked.frigate_ui_url}
          <div class="locked-value">{runtimeConfigStore.effective.frigate_ui_url}</div>
        {:else}
          <input
            id="connection-ui"
            class="text-input"
            type="url"
            placeholder="http://frigate:8971"
            value={frigateUIDraft}
            oninput={onFrigateUIInput}
            disabled={applying}
            autocomplete="off"
            spellcheck="false"
          />
        {/if}
      </div>
    </div>
    <div class="hint">
      {runtimeConfigStore.locked.frigate_ui_url
        ? ui.connectionEnvLockedFrigateUI
        : ui.connectionFrigateUIURLHint}
    </div>

    {#if report}
      <div class="result">
        <div class="result-row">
          <span class="result-label">{ui.connectionFrigateLabel}</span>
          {#if report.frigate.ok}
            <span class="result-status ok">{ui.connectionResultOK}</span>
          {:else}
            <span class="result-status bad">{ui.connectionResultFailed}</span>
            {#if report.frigate.error}
              <span class="result-detail">{report.frigate.error}</span>
            {/if}
          {/if}
        </div>
        <div class="result-row">
          <span class="result-label">{ui.connectionGo2rtcLabel}</span>
          {#if report.go2rtc.skipped}
            <span class="result-status skip">{ui.connectionResultSkipped}</span>
          {:else if report.go2rtc.ok}
            <span class="result-status ok">{ui.connectionResultOK}</span>
          {:else}
            <span class="result-status bad">{ui.connectionResultFailed}</span>
            {#if report.go2rtc.error}
              <span class="result-detail">{report.go2rtc.error}</span>
            {/if}
          {/if}
        </div>
      </div>
    {/if}

    {#if testError}
      <div class="form-error">{testError}</div>
    {/if}
    {#if saveError}
      <div class="form-error">{saveError}</div>
    {/if}
    {#if savedAt !== null && !dirtyAfterSave && !applying}
      <div class="save-hint">{ui.connectionSavedRestartHint}</div>
    {/if}

    <div class="policy-note">{ui.connectionRestartPolicyNote}</div>

    {#if applying}
      <div class="restart-status" aria-live="polite">
        <span class="dot" aria-hidden="true"></span>
        <span>{ui.connectionRestartingNote}</span>
      </div>
      {#if applyFallbackVisible}
        <div class="hint">{ui.connectionRestartFallback}</div>
      {/if}
    {:else if confirmingApply}
      <div class="confirm">
        <p class="confirm-text">{ui.connectionConfirmApply}</p>
        <div class="actions">
          <button type="button" class="btn" onclick={cancelApply}>{ui.cancel}</button>
          <button type="button" class="btn btn-danger" onclick={confirmApply}>
            {ui.connectionApplyConfirm}
          </button>
        </div>
      </div>
    {:else}
      <div class="actions">
        <button
          type="button"
          class="btn"
          onclick={runTest}
          disabled={testing || saving || !canSave}
        >
          {testing ? ui.connectionTesting : ui.connectionTest}
        </button>
        <button
          type="button"
          class="btn btn-primary"
          onclick={runSave}
          disabled={saving || testing || !canSave}
        >
          {saving ? ui.connectionSaving : ui.connectionSave}
        </button>
        <button type="button" class="btn btn-danger" onclick={requestApply} disabled={!canApply}>
          {ui.connectionApply}
        </button>
      </div>
    {/if}
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
  .card {
    border: 1px solid var(--border);
    border-radius: 10px;
    padding: 14px 16px;
    background: var(--surface);
    display: flex;
    flex-direction: column;
    gap: 12px;
  }
  .row {
    display: flex;
    flex-direction: column;
    gap: 4px;
  }
  .row-label {
    font-size: 12px;
    color: var(--text-3);
  }
  .row-control {
    display: flex;
    align-items: center;
    gap: 8px;
    min-width: 0;
    flex: 1;
  }
  .text-input {
    width: 100%;
    padding: 8px 10px;
    font-size: 13px;
    color: var(--text);
    background: rgba(0, 0, 0, 0.25);
    border: 1px solid var(--border);
    border-radius: 8px;
    font-family: inherit;
    min-width: 0;
  }
  .text-input:focus {
    outline: none;
    border-color: var(--border-strong);
  }
  .text-input:disabled {
    opacity: 0.6;
  }
  .locked-value {
    flex: 1;
    padding: 8px 10px;
    font-size: 13px;
    color: var(--text-2);
    background: rgba(0, 0, 0, 0.35);
    border: 1px solid var(--border);
    border-radius: 8px;
    font-family: 'JetBrains Mono', ui-monospace, monospace;
    overflow-x: auto;
    white-space: nowrap;
  }
  .hint {
    font-size: 12px;
    color: var(--text-3);
    margin-top: -6px;
    line-height: 1.45;
  }
  .save-hint {
    font-size: 12px;
    color: var(--text-2);
  }
  .policy-note {
    font-size: 12px;
    color: var(--text-2);
    line-height: 1.45;
  }
  .form-error {
    font-size: 12px;
    color: oklch(0.68 0.16 25);
    line-height: 1.45;
  }
  .result {
    border: 1px solid var(--border);
    border-radius: 8px;
    padding: 10px 12px;
    font-size: 12px;
    color: var(--text-2);
    display: flex;
    flex-direction: column;
    gap: 6px;
  }
  .result-row {
    display: flex;
    align-items: baseline;
    gap: 8px;
    flex-wrap: wrap;
  }
  .result-label {
    min-width: 60px;
  }
  .result-status {
    font-weight: 500;
  }
  .result-status.ok {
    color: oklch(0.74 0.14 145);
  }
  .result-status.bad {
    color: oklch(0.68 0.16 25);
  }
  .result-status.skip {
    color: var(--text-3);
  }
  .result-detail {
    color: var(--text-3);
    font-family: 'JetBrains Mono', ui-monospace, monospace;
    font-size: 11px;
  }
  .confirm {
    border: 1px solid color-mix(in oklab, oklch(0.68 0.16 25) 35%, var(--border));
    background: color-mix(in oklab, oklch(0.68 0.16 25) 6%, transparent);
    border-radius: 8px;
    padding: 12px;
    display: flex;
    flex-direction: column;
    gap: 10px;
  }
  .confirm-text {
    font-size: 13px;
    color: var(--text);
    line-height: 1.45;
  }
  .restart-status {
    display: flex;
    align-items: center;
    gap: 10px;
    font-size: 13px;
    color: var(--text-2);
  }
  .dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: var(--accent);
    animation: pulse 1.4s ease-in-out infinite;
  }
  @keyframes pulse {
    0%,
    100% {
      opacity: 0.35;
      transform: scale(0.92);
    }
    50% {
      opacity: 0.95;
      transform: scale(1.1);
    }
  }
  .actions {
    display: flex;
    justify-content: flex-end;
    gap: 8px;
    flex-wrap: wrap;
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
  .btn-danger {
    color: oklch(0.68 0.16 25);
    border-color: color-mix(in oklab, oklch(0.68 0.16 25) 50%, transparent);
  }
  .btn-danger:hover:not(:disabled) {
    background: color-mix(in oklab, oklch(0.68 0.16 25) 10%, transparent);
  }
</style>
