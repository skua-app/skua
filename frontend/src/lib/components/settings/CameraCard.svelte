<script lang="ts">
  import { CameraNameApiError, StreamOverrideApiError, setCameraName } from '$lib/api'
  import type { Camera } from '$lib/api'
  import { camerasStore } from '$lib/stores/cameras.svelte'
  import { streamOverridesStore } from '$lib/stores/streamOverrides.svelte'
  import { ui } from '$lib/i18n/strings'

  type Props = {
    camera: Camera
    streams: string[]
    streamsLoaded: boolean
    streamsDisabled: boolean
  }
  let { camera, streams, streamsLoaded, streamsDisabled }: Props = $props()

  // Name draft + save (ported from current /settings, scoped to this card).
  let nameDraft = $state<string | null>(null)
  let nameBusy = $state(false)
  let nameError = $state<string | null>(null)
  const nameValue = $derived(nameDraft ?? camera.name)
  const nameDirty = $derived(nameDraft !== null && nameDraft.trim() !== camera.name)

  function onNameInput(e: Event) {
    nameDraft = (e.currentTarget as HTMLInputElement).value
    nameError = null
  }

  async function saveName() {
    if (!nameDirty) return
    nameBusy = true
    try {
      await setCameraName(camera.id, nameDraft ?? '')
      nameDraft = null
      nameError = null
      await camerasStore.refresh()
    } catch (err) {
      nameError = err instanceof CameraNameApiError ? err.message : ui.saveErrorGeneric
    } finally {
      nameBusy = false
    }
  }

  // Stream override auto-save state.
  let savingField = $state<'main' | 'sub' | null>(null)
  let streamError = $state<string | null>(null)
  let streamErrorTimer: ReturnType<typeof setTimeout> | null = null
  const currentOverride = $derived(streamOverridesStore.get(camera.id))

  function showStreamError(msg: string) {
    streamError = msg
    if (streamErrorTimer !== null) clearTimeout(streamErrorTimer)
    streamErrorTimer = setTimeout(() => {
      streamError = null
      streamErrorTimer = null
    }, 4000)
  }

  async function onStreamChange(field: 'main' | 'sub', e: Event) {
    const target = e.currentTarget as HTMLSelectElement
    const nextMain = field === 'main' ? target.value : currentOverride.main
    const nextSub = field === 'sub' ? target.value : currentOverride.sub
    savingField = field
    try {
      await streamOverridesStore.save(camera.id, nextMain, nextSub)
      streamError = null
    } catch (err) {
      const msg = err instanceof StreamOverrideApiError ? err.message : ui.streamOverrideSaveError
      showStreamError(msg)
    } finally {
      savingField = null
    }
  }

  // ResizeObserver-driven width so the card's label/control layout adapts
  // independent of whether the parent is single- or two-column.
  let cardEl = $state<HTMLDivElement | null>(null)
  let cardWidth = $state(0)
  $effect(() => {
    if (!cardEl) return
    const ro = new ResizeObserver((entries) => {
      const entry = entries[0]
      if (!entry) return
      cardWidth = Math.round(entry.contentRect.width)
    })
    ro.observe(cardEl)
    return () => ro.disconnect()
  })
  const wide = $derived(cardWidth >= 480)
</script>

<div class="card" class:wide bind:this={cardEl}>
  <header class="card-head">
    <span class="card-id">{camera.id}</span>
    <span class="card-name">{camera.name}</span>
  </header>

  <div class="row">
    <label class="row-label" for="name-{camera.id}">{ui.nameLabel}</label>
    <div class="row-control name-control">
      <input
        id="name-{camera.id}"
        class="text-input"
        type="text"
        value={nameValue}
        oninput={onNameInput}
        maxlength="60"
        disabled={nameBusy}
        aria-label="{ui.cameraNameAria} {camera.id}"
      />
      <button
        type="button"
        class="btn btn-primary btn-sm"
        disabled={!nameDirty || nameBusy}
        onclick={saveName}
      >
        {ui.save}
      </button>
    </div>
  </div>
  {#if nameError}
    <div class="row-error">{nameError}</div>
  {/if}

  <div class="row">
    <label class="row-label" for="main-{camera.id}">{ui.streamMain}</label>
    <div class="row-control">
      <select
        id="main-{camera.id}"
        class="select"
        value={currentOverride.main}
        onchange={(e) => onStreamChange('main', e)}
        disabled={streamsDisabled || !streamsLoaded}
      >
        <option value="">{streamsLoaded ? ui.streamDefault : ui.streamsLoading}</option>
        {#each streams as name (name)}
          <option value={name}>{name}</option>
        {/each}
      </select>
      {#if savingField === 'main'}
        <span class="save-hint">{ui.streamOverrideSaving}</span>
      {/if}
    </div>
  </div>

  <div class="row">
    <label class="row-label" for="sub-{camera.id}">{ui.streamSub}</label>
    <div class="row-control">
      <select
        id="sub-{camera.id}"
        class="select"
        value={currentOverride.sub}
        onchange={(e) => onStreamChange('sub', e)}
        disabled={streamsDisabled || !streamsLoaded}
      >
        <option value="">{streamsLoaded ? ui.streamDefault : ui.streamsLoading}</option>
        {#each streams as name (name)}
          <option value={name}>{name}</option>
        {/each}
      </select>
      {#if savingField === 'sub'}
        <span class="save-hint">{ui.streamOverrideSaving}</span>
      {/if}
    </div>
  </div>
  {#if streamError}
    <div class="row-error">{streamError}</div>
  {/if}
</div>

<style>
  .card {
    border: 1px solid var(--border);
    border-radius: 10px;
    padding: 14px 16px;
    background: var(--surface);
    display: flex;
    flex-direction: column;
    gap: 8px;
  }
  .card-head {
    display: flex;
    justify-content: space-between;
    align-items: baseline;
    gap: 12px;
    margin-bottom: 4px;
  }
  .card-id {
    font-family: 'JetBrains Mono', ui-monospace, monospace;
    font-size: 11px;
    color: var(--text-3);
    letter-spacing: 0.3px;
  }
  .card-name {
    font-size: 14px;
    font-weight: 600;
    color: var(--text);
    text-align: right;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .row {
    display: flex;
    flex-direction: column;
    gap: 4px;
  }
  .wide .row {
    flex-direction: row;
    align-items: center;
    gap: 12px;
  }
  .row-label {
    font-size: 12px;
    color: var(--text-3);
  }
  .wide .row-label {
    width: 100px;
    flex: 0 0 100px;
  }
  .row-control {
    display: flex;
    align-items: center;
    gap: 8px;
    min-width: 0;
    flex: 1;
  }
  .name-control {
    display: flex;
    gap: 8px;
  }
  .name-control .text-input {
    flex: 1;
    min-width: 0;
  }

  .text-input,
  .select {
    padding: 8px 10px;
    font-size: 13px;
    color: var(--text);
    background: rgba(0, 0, 0, 0.25);
    border: 1px solid var(--border);
    border-radius: 8px;
    font-family: inherit;
    min-width: 0;
  }
  .text-input:focus,
  .select:focus {
    outline: none;
    border-color: var(--border-strong);
  }
  .select {
    flex: 1;
    appearance: none;
    background-image: url("data:image/svg+xml;utf8,<svg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 12 7' width='12' height='7'><path fill='rgba(245,246,247,0.58)' d='M0 0l6 7 6-7z'/></svg>");
    background-repeat: no-repeat;
    background-position: right 10px center;
    padding-right: 28px;
  }
  .select:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }

  .save-hint {
    font-size: 11px;
    color: var(--text-3);
    flex-shrink: 0;
  }
  .row-error {
    font-size: 12px;
    color: oklch(0.68 0.16 25);
    line-height: 1.4;
    margin-top: -2px;
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
  .btn-sm {
    padding: 4px 10px;
    font-size: 11px;
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
