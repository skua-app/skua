<script lang="ts">
  import GroupCard from '$lib/components/settings/GroupCard.svelte'
  import { GroupApiError } from '$lib/api'
  import { camerasStore } from '$lib/stores/cameras.svelte'
  import { groupsStore } from '$lib/stores/groups.svelte'
  import { ui } from '$lib/i18n/strings'

  let busy = $state(false)

  let creating = $state(false)
  let newName = $state('')
  let createError = $state<string | null>(null)

  let confirmDeleteId = $state<string | null>(null)

  function startCreate() {
    creating = true
    newName = ''
    createError = null
  }
  function cancelCreate() {
    creating = false
    newName = ''
    createError = null
  }
  async function saveCreate() {
    busy = true
    createError = null
    try {
      await groupsStore.create(newName.trim())
      creating = false
      newName = ''
    } catch (err) {
      createError = err instanceof GroupApiError ? err.message : ui.createErrorGeneric
    } finally {
      busy = false
    }
  }

  function requestDelete(id: string) {
    confirmDeleteId = id
  }
  async function doConfirmDelete() {
    if (!confirmDeleteId) return
    busy = true
    try {
      await groupsStore.remove(confirmDeleteId)
      confirmDeleteId = null
    } finally {
      busy = false
    }
  }

  function setBusy(v: boolean) {
    busy = v
  }

  const pendingDeleteGroup = $derived(
    confirmDeleteId ? groupsStore.byId(confirmDeleteId) : undefined
  )
  const confirmText = $derived(
    pendingDeleteGroup ? ui.confirmDeleteGroup.replace('{name}', pendingDeleteGroup.name) : ''
  )
</script>

<section id="groups" class="settings-section dk-set-section">
  <header class="section-header">
    <h2 class="dk-set-h2">{ui.sectionGroups}</h2>
  </header>

  <div class="group-cards">
    {#each groupsStore.groups as g (g.id)}
      <GroupCard
        group={g}
        cameras={camerasStore.cameras}
        {busy}
        onBusyChange={setBusy}
        onDelete={requestDelete}
      />
    {/each}
  </div>

  {#if creating}
    <div class="set-card">
      <input
        class="set-input"
        type="text"
        placeholder={ui.newGroupPlaceholder}
        bind:value={newName}
        maxlength="60"
        disabled={busy}
      />
      {#if createError}
        <div class="form-error">{createError}</div>
      {/if}
      <div class="set-actions">
        <button type="button" class="set-btn" onclick={cancelCreate} disabled={busy}>
          {ui.cancel}
        </button>
        <button
          type="button"
          class="set-btn primary"
          onclick={saveCreate}
          disabled={busy || newName.trim().length === 0}
        >
          {ui.create}
        </button>
      </div>
    </div>
  {:else}
    <button type="button" class="set-btn add" onclick={startCreate} disabled={busy}>
      {ui.createGroup}
    </button>
  {/if}
</section>

{#if confirmDeleteId}
  <div
    class="confirm-backdrop"
    role="presentation"
    onclick={() => (confirmDeleteId = null)}
    onkeydown={() => {}}
  >
    <div
      class="confirm-dialog"
      role="dialog"
      aria-modal="true"
      tabindex={-1}
      onclick={(e) => e.stopPropagation()}
      onkeydown={() => {}}
    >
      <p class="confirm-text">{confirmText}</p>
      <div class="set-actions">
        <button
          type="button"
          class="set-btn"
          onclick={() => (confirmDeleteId = null)}
          disabled={busy}
        >
          {ui.cancel}
        </button>
        <button type="button" class="set-btn danger" onclick={doConfirmDelete} disabled={busy}>
          {ui.delete}
        </button>
      </div>
    </div>
  </div>
{/if}

<style>
  .settings-section {
    display: block;
  }
  .dk-set-section {
    max-width: 720px;
  }
  .section-header {
    margin-bottom: 12px;
  }
  .dk-set-h2 {
    font-size: 20px;
    font-weight: 600;
    letter-spacing: -0.3px;
    margin: 0 0 4px;
    color: var(--text);
  }
  .group-cards {
    display: flex;
    flex-direction: column;
    gap: 10px;
    margin-bottom: 12px;
  }

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

  /* prototype .set-input */
  .set-input {
    width: 100%;
    padding: 9px 11px;
    font-size: 13px;
    color: var(--text);
    background: rgba(0, 0, 0, 0.22);
    border: 1px solid var(--border);
    border-radius: var(--r-xs);
    font-family: inherit;
  }
  .set-input:focus {
    outline: none;
    border-color: var(--border-strong);
  }

  .form-error {
    font-size: 12px;
    color: var(--warn);
  }

  /* prototype .set-actions */
  .set-actions {
    display: flex;
    gap: 8px;
    justify-content: flex-end;
    flex-wrap: wrap;
    margin-top: 2px;
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
    cursor: pointer;
    font-family: inherit;
    white-space: nowrap;
    transition: background 0.15s ease;
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
  .set-btn.primary {
    background: var(--accent);
    color: var(--on-accent);
    border-color: var(--accent);
  }
  .set-btn.primary:hover:not(:disabled) {
    background: var(--accent);
    filter: brightness(1.05);
  }
  .set-btn.danger {
    color: var(--warn);
    border-color: color-mix(in oklab, var(--warn) 45%, transparent);
  }
  .set-btn.danger:hover:not(:disabled) {
    background: color-mix(in oklab, var(--warn) 10%, transparent);
  }
  .set-btn.add {
    width: 100%;
    padding: 11px;
    border-style: dashed;
    border-color: var(--border);
    color: var(--text-2);
  }
  .set-btn.add:hover:not(:disabled) {
    color: var(--text);
    border-color: var(--border-strong);
  }

  /* confirm-delete dialog — restyled with tokens */
  .confirm-backdrop {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.55);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 200;
    padding: 16px;
  }
  .confirm-dialog {
    background: var(--elev);
    border: 1px solid var(--border);
    border-radius: 12px;
    padding: 18px;
    max-width: 360px;
    width: 100%;
  }
  .confirm-text {
    font-size: 13px;
    color: var(--text);
    margin-bottom: 16px;
    line-height: 1.45;
  }
</style>
