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

<section id="groups" class="settings-section">
  <header class="section-header">
    <h2 class="section-title">{ui.sectionGroups}</h2>
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
    <div class="group-create">
      <input
        class="text-input"
        type="text"
        placeholder={ui.newGroupPlaceholder}
        bind:value={newName}
        maxlength="60"
        disabled={busy}
      />
      {#if createError}
        <div class="form-error">{createError}</div>
      {/if}
      <div class="group-edit-actions">
        <button type="button" class="btn" onclick={cancelCreate} disabled={busy}>
          {ui.cancel}
        </button>
        <button
          type="button"
          class="btn btn-primary"
          onclick={saveCreate}
          disabled={busy || newName.trim().length === 0}
        >
          {ui.create}
        </button>
      </div>
    </div>
  {:else}
    <button type="button" class="btn btn-add" onclick={startCreate} disabled={busy}>
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
      <div class="confirm-actions">
        <button type="button" class="btn" onclick={() => (confirmDeleteId = null)} disabled={busy}>
          {ui.cancel}
        </button>
        <button type="button" class="btn btn-danger" onclick={doConfirmDelete} disabled={busy}>
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
  .section-header {
    margin-bottom: 12px;
  }
  .section-title {
    font-size: 15px;
    font-weight: 600;
    letter-spacing: -0.2px;
    margin-bottom: 6px;
  }
  .group-cards {
    display: flex;
    flex-direction: column;
    gap: 10px;
    margin-bottom: 12px;
  }
  .group-create {
    display: flex;
    flex-direction: column;
    gap: 10px;
    border: 1px dashed var(--border);
    border-radius: 10px;
    padding: 12px 14px;
  }
  .text-input {
    padding: 8px 10px;
    font-size: 13px;
    color: var(--text);
    background: rgba(0, 0, 0, 0.25);
    border: 1px solid var(--border);
    border-radius: 8px;
    font-family: inherit;
  }
  .text-input:focus {
    outline: none;
    border-color: var(--border-strong);
  }
  .group-edit-actions {
    display: flex;
    justify-content: flex-end;
    gap: 8px;
    margin-top: 4px;
  }
  .form-error {
    font-size: 12px;
    color: oklch(0.68 0.16 25);
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
  .btn-add {
    width: 100%;
    padding: 10px;
    color: var(--text-2);
    border-style: dashed;
    border-color: var(--border);
  }
  .btn-add:hover:not(:disabled) {
    color: var(--text);
    border-color: var(--border-strong);
  }

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
    background: #15171a;
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
  .confirm-actions {
    display: flex;
    justify-content: flex-end;
    gap: 8px;
  }
</style>
