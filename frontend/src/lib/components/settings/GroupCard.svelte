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

<article class="group-card">
  {#if editing}
    <div class="group-edit">
      <label class="group-edit-label" for="group-name-{group.id}">{ui.groupNameLabel}</label>
      <input
        id="group-name-{group.id}"
        class="text-input"
        type="text"
        bind:value={editingName}
        maxlength="60"
        disabled={busy}
      />
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
      <div class="group-edit-actions">
        <button type="button" class="btn" onclick={cancelEdit} disabled={busy}>{ui.cancel}</button>
        <button type="button" class="btn btn-primary" onclick={saveEdit} disabled={busy}>
          {ui.save}
        </button>
      </div>
    </div>
  {:else}
    <div class="group-card-head">
      <span class="group-name">{group.name}</span>
      <div class="group-card-actions">
        <button type="button" class="btn btn-sm" onclick={startEdit} disabled={busy}>
          {ui.edit}
        </button>
        <button
          type="button"
          class="btn btn-sm btn-danger"
          onclick={() => onDelete(group.id)}
          disabled={busy}
        >
          {ui.delete}
        </button>
      </div>
    </div>
    <div class="group-card-body">
      {#if summary === null}
        <span class="group-cams-empty">{ui.groupNoCameras}</span>
      {:else}
        <span class="group-cams-label">{ui.groupCameras}</span>
        <span class="group-cams">{summary}</span>
      {/if}
    </div>
  {/if}
</article>

<style>
  .group-card {
    border: 1px solid var(--border);
    border-radius: 10px;
    padding: 12px 14px;
    background: var(--surface);
  }
  .group-card-head {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 8px;
    margin-bottom: 6px;
  }
  .group-name {
    font-size: 14px;
    font-weight: 600;
    color: var(--text);
  }
  .group-card-actions {
    display: flex;
    gap: 6px;
  }
  .group-card-body {
    font-size: 12px;
    color: var(--text-2);
    line-height: 1.4;
  }
  .group-cams-label {
    color: var(--text-3);
    margin-right: 4px;
  }
  .group-cams-empty {
    color: var(--text-3);
    font-style: italic;
  }
  .group-edit {
    display: flex;
    flex-direction: column;
    gap: 10px;
  }
  .group-edit-label {
    font-size: 11px;
    color: var(--text-3);
    text-transform: uppercase;
    letter-spacing: 0.4px;
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
  .btn-danger {
    color: oklch(0.68 0.16 25);
    border-color: color-mix(in oklab, oklch(0.68 0.16 25) 50%, transparent);
  }
  .btn-danger:hover:not(:disabled) {
    background: color-mix(in oklab, oklch(0.68 0.16 25) 10%, transparent);
  }
</style>
