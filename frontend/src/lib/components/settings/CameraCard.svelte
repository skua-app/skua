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

<div class="set-card" class:wide bind:this={cardEl}>
  <header class="sc-head">
    <span class="sc-id">{camera.id}</span>
    <span class="sc-name">{camera.name}</span>
  </header>

  <div class="set-field row">
    <label class="f-label" for="name-{camera.id}">{ui.nameLabel}</label>
    <div class="f-inline row-control">
      <input
        id="name-{camera.id}"
        class="set-input"
        type="text"
        value={nameValue}
        oninput={onNameInput}
        maxlength="60"
        disabled={nameBusy}
        aria-label="{ui.cameraNameAria} {camera.id}"
      />
      <button
        type="button"
        class="set-btn sm primary"
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

  <div class="set-field row">
    <label class="f-label" for="main-{camera.id}">{ui.streamMain}</label>
    <div class="row-control">
      <select
        id="main-{camera.id}"
        class="set-select"
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

  <div class="set-field row">
    <label class="f-label" for="sub-{camera.id}">{ui.streamSub}</label>
    <div class="row-control">
      <select
        id="sub-{camera.id}"
        class="set-select"
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
  /* prototype .set-card */
  .set-card {
    border: 1px solid var(--border);
    border-radius: var(--r-sm);
    background: var(--surface);
    padding: 14px;
    display: flex;
    flex-direction: column;
    gap: 11px;
    margin-bottom: 10px;
  }
  .sc-head {
    display: flex;
    justify-content: space-between;
    align-items: baseline;
    gap: 12px;
  }
  .sc-id {
    font-family: 'JetBrains Mono', ui-monospace, monospace;
    font-size: 11px;
    color: var(--text-3);
    letter-spacing: 0.3px;
  }
  .sc-name {
    font-size: 14px;
    font-weight: 600;
    color: var(--text);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    text-align: right;
  }

  /* prototype .set-field (narrow) — label above */
  .set-field {
    display: flex;
    flex-direction: column;
    gap: 5px;
  }
  .f-label {
    font-size: 12px;
    color: var(--text-3);
  }
  .f-inline {
    display: flex;
    gap: 8px;
    align-items: center;
  }
  .f-inline .set-input {
    flex: 1 1 auto;
  }
  .row-control {
    display: flex;
    align-items: center;
    gap: 8px;
    min-width: 0;
  }

  /* wide variant — label-left / control-right (.dk-setline-style) */
  .wide .set-field.row {
    flex-direction: row;
    align-items: center;
    gap: 18px;
  }
  .wide .f-label {
    flex: 0 0 110px;
    font-size: 13px;
    color: var(--text-2);
  }
  .wide .row-control {
    flex: 1 1 auto;
  }

  /* prototype .set-input / .set-select */
  .set-input,
  .set-select {
    width: 100%;
    padding: 9px 11px;
    font-size: 13px;
    color: var(--text);
    background: rgba(0, 0, 0, 0.22);
    border: 1px solid var(--border);
    border-radius: var(--r-xs);
    font-family: inherit;
    min-width: 0;
  }
  .set-input:focus,
  .set-select:focus {
    outline: none;
    border-color: var(--border-strong);
  }
  .set-select {
    flex: 1;
    -webkit-appearance: none;
    appearance: none;
    background-image: url("data:image/svg+xml;utf8,<svg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 12 7' width='12' height='7'><path fill='%23888' d='M0 0l6 7 6-7z'/></svg>");
    background-repeat: no-repeat;
    background-position: right 11px center;
    padding-right: 28px;
  }
  .set-select:disabled {
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
    color: var(--warn);
    line-height: 1.4;
    margin-top: -4px;
  }

  /* prototype .set-btn */
  .set-btn {
    padding: 9px 14px;
    font-size: 13px;
    font-weight: 600;
    border-radius: 8px;
    border: 1px solid var(--border-strong);
    background: transparent;
    color: var(--text);
    font-family: inherit;
    cursor: pointer;
    white-space: nowrap;
    transition: background 0.15s ease;
    flex-shrink: 0;
  }
  .set-btn:active {
    transform: translateY(1px);
  }
  .set-btn:hover:not(:disabled) {
    background: rgba(125, 125, 125, 0.07);
  }
  .set-btn:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }
  .set-btn.sm {
    padding: 5px 11px;
    font-size: 12px;
  }
  .set-btn.primary {
    background: var(--accent);
    color: var(--on-accent);
    border-color: var(--accent);
  }
  .set-btn.primary:hover:not(:disabled) {
    background: var(--accent);
    filter: brightness(1.05);
  }
</style>
