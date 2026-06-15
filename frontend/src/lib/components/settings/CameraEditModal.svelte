<script lang="ts">
  import { fly, fade } from 'svelte/transition'
  import { cubicOut } from 'svelte/easing'
  import { CameraNameApiError, StreamOverrideApiError, setCameraName } from '$lib/api'
  import type { Camera } from '$lib/api'
  import { camerasStore } from '$lib/stores/cameras.svelte'
  import { streamOverridesStore } from '$lib/stores/streamOverrides.svelte'
  import { ui } from '$lib/i18n/strings'
  import Mono from '$lib/components/Mono.svelte'

  type Props = {
    camera: Camera
    streams: string[]
    streamsLoaded: boolean
    streamsDisabled: boolean
    onClose: () => void
  }

  let { camera, streams, streamsLoaded, streamsDisabled, onClose }: Props = $props()

  const reducedMotion =
    typeof window !== 'undefined' && window.matchMedia('(prefers-reduced-motion: reduce)').matches
  const cardDuration = reducedMotion ? 0 : 260
  const scrimDuration = reducedMotion ? 0 : 200

  // Name draft + save. Mirrors the previous CameraCard behaviour: the
  // input is dirty until the trimmed draft matches the live camera.name,
  // and the explicit Save button is the only commit path (so a typo never
  // round-trips through the server unintentionally).
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

  // Stream override auto-save state, kept identical to the in-card version
  // — selecting an option commits straight to the server; a transient
  // error message replaces the row hint for 4 s.
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

  let closeBtn = $state<HTMLButtonElement | null>(null)
  let cardEl = $state<HTMLDivElement | null>(null)

  function focusableIn(root: HTMLElement): HTMLElement[] {
    const sel = 'button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])'
    return Array.from(root.querySelectorAll<HTMLElement>(sel)).filter(
      (el) => !el.hasAttribute('disabled') && el.offsetParent !== null
    )
  }

  function onKey(e: KeyboardEvent) {
    if (e.key === 'Escape') {
      onClose()
      return
    }
    if (e.key !== 'Tab' || !cardEl) return
    const items = focusableIn(cardEl)
    if (items.length === 0) {
      e.preventDefault()
      return
    }
    const first = items[0]!
    const last = items[items.length - 1]!
    const active = document.activeElement as HTMLElement | null
    if (e.shiftKey) {
      if (active === first || !cardEl.contains(active)) {
        e.preventDefault()
        last.focus()
      }
    } else {
      if (active === last || !cardEl.contains(active)) {
        e.preventDefault()
        first.focus()
      }
    }
  }

  $effect(() => {
    const prev = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    window.addEventListener('keydown', onKey)
    return () => {
      document.body.style.overflow = prev
      window.removeEventListener('keydown', onKey)
      if (streamErrorTimer !== null) clearTimeout(streamErrorTimer)
    }
  })

  $effect(() => {
    closeBtn?.focus({ preventScroll: true })
  })

  function onBackdropClick(e: MouseEvent) {
    if (e.target === e.currentTarget) onClose()
  }
</script>

<!-- svelte-ignore a11y_click_events_have_key_events -->
<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="cm-backdrop" onclick={onBackdropClick} transition:fade={{ duration: scrimDuration }}>
  <div
    class="cm-card"
    role="dialog"
    aria-modal="true"
    aria-label={ui.editCameraTitle}
    bind:this={cardEl}
    transition:fly={{ y: 16, duration: cardDuration, easing: cubicOut }}
  >
    <header class="cm-head">
      <div class="cm-head-text">
        <span class="cm-title">{ui.editCameraTitle}</span>
        <span class="cm-name">{camera.name}</span>
        <Mono size={11} color="var(--text-3)">{camera.id}</Mono>
      </div>
    </header>

    <div class="cm-body">
      <div class="set-field row">
        <label class="f-label" for="cm-name-{camera.id}">{ui.nameLabel}</label>
        <div class="f-inline row-control">
          <input
            id="cm-name-{camera.id}"
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
        <label class="f-label" for="cm-main-{camera.id}">{ui.streamMain}</label>
        <div class="row-control">
          <select
            id="cm-main-{camera.id}"
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
        <label class="f-label" for="cm-sub-{camera.id}">{ui.streamSub}</label>
        <div class="row-control">
          <select
            id="cm-sub-{camera.id}"
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

    <footer class="cm-foot">
      <button type="button" class="set-btn" onclick={onClose} bind:this={closeBtn}>
        {ui.close}
      </button>
    </footer>
  </div>
</div>

<style>
  .cm-backdrop {
    position: fixed;
    inset: 0;
    background: var(--scrim);
    backdrop-filter: blur(6px);
    -webkit-backdrop-filter: blur(6px);
    display: flex;
    align-items: center;
    justify-content: center;
    padding-top: max(20px, env(safe-area-inset-top));
    padding-right: max(20px, env(safe-area-inset-right));
    padding-bottom: max(20px, env(safe-area-inset-bottom));
    padding-left: max(20px, env(safe-area-inset-left));
    z-index: 110;
  }
  .cm-card {
    width: min(520px, 100%);
    max-height: 100%;
    background: var(--surface);
    border: 1px solid var(--border-strong);
    border-radius: var(--r);
    box-shadow: var(--shadow);
    display: flex;
    flex-direction: column;
    overflow: hidden;
  }
  .cm-head {
    padding: 16px 18px 12px;
    border-bottom: 1px solid var(--border);
    flex-shrink: 0;
  }
  .cm-head-text {
    display: flex;
    flex-direction: column;
    gap: 2px;
  }
  .cm-title {
    font-size: 12px;
    font-weight: 500;
    text-transform: uppercase;
    letter-spacing: 0.4px;
    color: var(--text-3);
  }
  .cm-name {
    font-size: 17px;
    font-weight: 600;
    color: var(--text);
    letter-spacing: -0.2px;
  }
  .cm-body {
    padding: 16px 18px;
    display: flex;
    flex-direction: column;
    gap: 14px;
    overflow-y: auto;
    flex: 1 1 auto;
    min-height: 0;
  }
  .cm-foot {
    padding: 12px 18px;
    border-top: 1px solid var(--border);
    display: flex;
    justify-content: flex-end;
    flex-shrink: 0;
  }

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
  .set-input,
  .set-select {
    width: 100%;
    padding: 9px 11px;
    font-size: 13px;
    color: var(--text);
    background: var(--input-bg);
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
