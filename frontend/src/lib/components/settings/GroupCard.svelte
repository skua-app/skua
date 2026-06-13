<script lang="ts">
  import { GroupApiError } from '$lib/api'
  import type { Camera, Group } from '$lib/api'
  import { groupsStore } from '$lib/stores/groups.svelte'
  import { ui } from '$lib/i18n/strings'

  type Props = {
    group: Group
    cameras: Camera[]
    busy: boolean
    onBusyChange: (busy: boolean) => void
    onDelete: (id: string) => void
  }
  let { group, cameras, busy, onBusyChange, onDelete }: Props = $props()

  let editing = $state(false)
  let editingName = $state('')
  let editingCams = $state<Set<string>>(new Set())
  let editError = $state<string | null>(null)

  function startEdit() {
    editing = true
    editingName = group.name
    editingCams = new Set(group.camera_ids)
    editError = null
  }
  function cancelEdit() {
    editing = false
    editingName = ''
    editingCams = new Set()
    editError = null
  }
  function toggleCam(id: string) {
    const next = new Set(editingCams)
    if (next.has(id)) next.delete(id)
    else next.add(id)
    editingCams = next
  }
  async function saveEdit() {
    editError = null
    onBusyChange(true)
    try {
      await groupsStore.update(group.id, {
        name: editingName.trim(),
        camera_ids: Array.from(editingCams)
      })
      editing = false
    } catch (err) {
      editError = err instanceof GroupApiError ? err.message : ui.saveErrorGeneric
    } finally {
      onBusyChange(false)
    }
  }

  function cameraNameOf(id: string): string {
    return cameras.find((c) => c.id === id)?.name ?? id
  }

  const summary = $derived(
    group.camera_ids.length === 0 ? null : group.camera_ids.map((id) => cameraNameOf(id)).join(', ')
  )
</script>

<article class="set-card group">
  {#if editing}
    <div class="set-field">
      <label class="f-label" for="group-name-{group.id}">{ui.groupNameLabel}</label>
      <input
        id="group-name-{group.id}"
        class="set-input"
        type="text"
        bind:value={editingName}
        maxlength="60"
        disabled={busy}
      />
    </div>
    <div class="group-edit-cams">
      {#each cameras as cam (cam.id)}
        <label class="cam-row">
          <input
            type="checkbox"
            checked={editingCams.has(cam.id)}
            onchange={() => toggleCam(cam.id)}
            disabled={busy}
          />
          <span class="cam-name">{cam.name}</span>
        </label>
      {/each}
    </div>
    {#if editError}
      <div class="form-error">{editError}</div>
    {/if}
    <div class="set-actions">
      <button type="button" class="set-btn" onclick={cancelEdit} disabled={busy}>
        {ui.cancel}
      </button>
      <button type="button" class="set-btn primary" onclick={saveEdit} disabled={busy}>
        {ui.save}
      </button>
    </div>
  {:else}
    <div class="g-head">
      <span class="g-name">{group.name}</span>
      <div class="g-acts">
        <button type="button" class="set-btn sm" onclick={startEdit} disabled={busy}>
          {ui.edit}
        </button>
        <button
          type="button"
          class="set-btn sm danger"
          onclick={() => onDelete(group.id)}
          disabled={busy}
        >
          {ui.delete}
        </button>
      </div>
    </div>
    <div class="g-body">
      {#if summary === null}
        <span class="g-empty">{ui.groupNoCameras}</span>
      {:else}
        <span class="g-camlabel">{ui.groupCameras}</span>
        <span class="g-cams">{summary}</span>
      {/if}
    </div>
  {/if}
</article>

<style>
  /* prototype .set-card.group */
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
  .set-card.group {
    gap: 7px;
  }
  .g-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
  }
  .g-name {
    font-size: 14px;
    font-weight: 600;
    color: var(--text);
  }
  .g-acts {
    display: flex;
    gap: 6px;
  }
  .g-body {
    font-size: 12px;
    color: var(--text-2);
    line-height: 1.4;
  }
  .g-camlabel {
    color: var(--text-3);
    margin-right: 4px;
  }
  .g-empty {
    color: var(--text-3);
    font-style: italic;
  }

  /* edit-mode */
  .set-field {
    display: flex;
    flex-direction: column;
    gap: 5px;
  }
  .f-label {
    font-size: 12px;
    color: var(--text-3);
  }
  .set-input {
    width: 100%;
    padding: 9px 11px;
    font-size: 13px;
    color: var(--text);
    background: var(--input-bg);
    border: 1px solid var(--border);
    border-radius: var(--r-xs);
    font-family: inherit;
  }
  .set-input:focus {
    outline: none;
    border-color: var(--border-strong);
  }
  .group-edit-cams {
    display: flex;
    flex-direction: column;
    gap: 4px;
    margin-top: 4px;
  }
  .cam-row {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 6px 4px;
    font-size: 13px;
    color: var(--text);
    cursor: pointer;
  }
  .cam-row input[type='checkbox'] {
    width: 16px;
    height: 16px;
    accent-color: var(--accent);
    cursor: pointer;
  }
  .form-error {
    font-size: 12px;
    color: var(--warn);
  }

  /* prototype .set-actions + .set-btn */
  .set-actions {
    display: flex;
    gap: 8px;
    justify-content: flex-end;
    flex-wrap: wrap;
    margin-top: 2px;
  }
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
  .set-btn.danger {
    color: var(--warn);
    border-color: color-mix(in oklab, var(--warn) 45%, transparent);
  }
  .set-btn.danger:hover:not(:disabled) {
    background: color-mix(in oklab, var(--warn) 10%, transparent);
  }
</style>
