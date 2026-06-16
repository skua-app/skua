<script lang="ts">
  import { untrack } from 'svelte'
  import { dragHandle, dragHandleZone, SHADOW_ITEM_MARKER_PROPERTY_NAME } from 'svelte-dnd-action'
  import type { Camera } from '$lib/api'
  import { RefreshApiError, refreshCameras, setCameraOrder } from '$lib/api'
  import { camerasStore } from '$lib/stores/cameras.svelte'
  import { groupsStore } from '$lib/stores/groups.svelte'
  import { go2rtcStreamsStore } from '$lib/stores/go2rtcStreams.svelte'
  import { ui } from '$lib/i18n/strings'
  import Icon from '$lib/components/Icon.svelte'
  import CameraEditModal from '$lib/components/settings/CameraEditModal.svelte'

  type RowItem = Camera & { [SHADOW_ITEM_MARKER_PROPERTY_NAME]?: unknown }

  let refreshing = $state(false)
  let refreshStatus = $state<string | null>(null)
  let refreshStatusKind = $state<'success' | 'error' | null>(null)
  let refreshStatusTimer: ReturnType<typeof setTimeout> | null = null

  // dnd-action's `items` array MUST be a stable reference between drags
  // so the lib can identify rows by their `id` field. We mirror the
  // store on every cameras update and overwrite on drop.
  let items = $state<RowItem[]>([])
  $effect(() => {
    // Track the cameras array; untrack the assignment so this effect
    // does NOT re-fire on its own write.
    const next = camerasStore.cameras
    untrack(() => {
      items = next.map((c) => ({ ...c }))
    })
  })

  let orderError = $state<string | null>(null)
  let orderErrorTimer: ReturnType<typeof setTimeout> | null = null
  function showOrderError() {
    orderError = ui.reorderSaveError
    if (orderErrorTimer !== null) clearTimeout(orderErrorTimer)
    orderErrorTimer = setTimeout(() => {
      orderError = null
      orderErrorTimer = null
    }, 4000)
  }

  // Editing one camera at a time: store the cam id so the modal mounts
  // afresh per camera (Svelte's {#if editing} block tears the modal
  // down on close, so the next open starts with clean draft state).
  let editingId = $state<string | null>(null)
  const editingCamera = $derived(
    editingId === null ? null : (camerasStore.cameras.find((c) => c.id === editingId) ?? null)
  )

  // Honour reduced-motion the way svelte-dnd-action's recipes recommend:
  // disable the lib's flip animation when the user has the OS-level
  // preference enabled. Read once at module load — switching mid-session
  // is rare and not worth a media-query listener.
  const reducedMotion =
    typeof window !== 'undefined' && window.matchMedia('(prefers-reduced-motion: reduce)').matches
  const flipDurationMs = reducedMotion ? 0 : 180

  type DndDetail = { items: RowItem[] }
  type DndEvent = CustomEvent<DndDetail>

  // Cache-busting tick for the camera thumbnails. /api/cameras/{id}/tile.jpg
  // is StaleWhileRevalidate'd by the SW with the query stripped from the
  // cache key — appending ?t={tileTick} doesn't bust the SW cache directly,
  // but it forces the browser to re-request, which triggers the SW's
  // background revalidation. Repeated ticks converge the thumbnail to the
  // current frame. The dragging gate freezes the tick while a row is
  // being moved (svelte-dnd-action owns the DOM during the drag); on
  // drop we bump immediately and let the interval resume so the moved
  // row re-requests right away and then settles.
  let tileTick = $state(Date.now())
  let dragging = $state(false)
  $effect(() => {
    const t = setInterval(() => {
      if (!dragging) tileTick = Date.now()
    }, 2000)
    return () => clearInterval(t)
  })

  function onConsider(e: DndEvent) {
    dragging = true
    items = e.detail.items
  }

  async function onFinalize(e: DndEvent) {
    items = e.detail.items
    const orderedIds = items.map((c) => c.id)
    dragging = false
    // Re-request the moved row's thumbnail right after the drop so it
    // does not linger on the stale frame the SW served while paused.
    tileTick = Date.now()
    // Optimistic: reflect the new order across the app immediately so the
    // grid and any other consumer of `camerasStore.cameras` follow the drop
    // without waiting on the round-trip.
    camerasStore.applyLocalOrder(orderedIds)
    try {
      await setCameraOrder(orderedIds)
    } catch {
      showOrderError()
      // Revert: pull authoritative order from the server.
      await camerasStore.refresh()
    }
  }

  function clearRefreshStatusSoon() {
    if (refreshStatusTimer !== null) clearTimeout(refreshStatusTimer)
    refreshStatusTimer = setTimeout(() => {
      refreshStatus = null
      refreshStatusKind = null
      refreshStatusTimer = null
    }, 5000)
  }

  function formatDiff(added: string[], removed: string[]): string {
    if (added.length === 0 && removed.length === 0) return ui.refreshNoChanges
    const parts: string[] = []
    if (added.length > 0) parts.push(`${ui.refreshDiffAdded}: ${added.join(', ')}.`)
    if (removed.length > 0) parts.push(`${ui.refreshDiffRemoved}: ${removed.join(', ')}.`)
    return parts.join(' ')
  }

  async function doRefreshCameras() {
    refreshing = true
    if (refreshStatusTimer !== null) {
      clearTimeout(refreshStatusTimer)
      refreshStatusTimer = null
    }
    refreshStatus = null
    refreshStatusKind = null
    try {
      const diff = await refreshCameras()
      refreshStatus = formatDiff(diff.added, diff.removed)
      refreshStatusKind = 'success'
      camerasStore.refresh().catch(() => {})
      groupsStore.refresh().catch(() => {})
    } catch (err) {
      refreshStatus =
        err instanceof RefreshApiError && err.code === 'frigate_unreachable'
          ? ui.refreshFrigateDown
          : ui.refreshGenericError
      refreshStatusKind = 'error'
    } finally {
      refreshing = false
      clearRefreshStatusSoon()
    }
  }
</script>

<section id="cameras" class="settings-section dk-set-section">
  <header class="section-header">
    <h2 class="dk-set-h2">{ui.sectionCameras}</h2>
    <p class="dk-set-desc">{ui.refreshHint}</p>
    <p class="set-hint">{ui.reorderHint}</p>
    <p class="set-hint">{ui.streamsHint}</p>
  </header>

  <div class="set-rowbtn">
    <button type="button" class="set-btn primary" disabled={refreshing} onclick={doRefreshCameras}>
      {refreshing ? ui.refreshingCameras : ui.refreshCameras}
    </button>
    {#if refreshStatus}
      <div
        class="set-msg"
        class:set-msg-error={refreshStatusKind === 'error'}
        role="status"
        aria-live="polite"
      >
        {refreshStatus}
      </div>
    {/if}
  </div>

  {#if orderError}
    <div class="set-msg set-msg-error" role="status" aria-live="polite">{orderError}</div>
  {/if}

  {#if go2rtcStreamsStore.error}
    <div class="streams-error" role="alert">{ui.streamsLoadError}</div>
  {/if}

  <ul
    class="cam-list"
    use:dragHandleZone={{
      items,
      flipDurationMs,
      dropTargetStyle: {},
      type: 'cameras'
    }}
    onconsider={onConsider}
    onfinalize={onFinalize}
  >
    {#each items as cam (cam.id)}
      <li class="cam-row" class:dragging={cam[SHADOW_ITEM_MARKER_PROPERTY_NAME]}>
        <span
          class="cam-handle"
          use:dragHandle
          aria-label={ui.dragHandleAria}
          role="button"
          tabindex="0"
        >
          <Icon name="grip" size={18} />
        </span>
        <img
          class="cam-thumb"
          src="/api/cameras/{cam.id}/tile.jpg?t={tileTick}"
          alt=""
          loading="lazy"
        />
        <span class="cam-text">
          <span class="cam-name">{cam.name}</span>
          <span class="cam-id">{cam.id}</span>
        </span>
        <button
          type="button"
          class="cam-edit"
          aria-label="{ui.editCameraAria} {cam.name}"
          title={ui.editCameraAria}
          onclick={() => (editingId = cam.id)}
        >
          <Icon name="edit" size={16} />
        </button>
      </li>
    {/each}
  </ul>
</section>

{#if editingCamera}
  <CameraEditModal
    camera={editingCamera}
    streams={go2rtcStreamsStore.streams}
    streamsLoaded={go2rtcStreamsStore.loaded && go2rtcStreamsStore.error === null}
    streamsDisabled={go2rtcStreamsStore.error !== null}
    onClose={() => (editingId = null)}
  />
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
  .dk-set-desc {
    font-size: 13.5px;
    color: var(--text-2);
    margin: 0 0 8px;
    line-height: 1.5;
  }
  @media (max-width: 899.98px) {
    .dk-set-h2 {
      display: none;
    }
  }
  .set-hint {
    font-size: 12px;
    color: var(--text-3);
    line-height: 1.45;
    margin: 0 0 4px;
  }
  .set-hint:last-of-type {
    margin-bottom: 12px;
  }
  .set-rowbtn {
    display: flex;
    gap: 8px;
    flex-wrap: wrap;
    align-items: center;
    margin-bottom: 10px;
  }
  .set-msg {
    font-size: 12px;
    color: var(--text-2);
    padding: 4px;
    line-height: 1.4;
  }
  .set-msg-error {
    color: var(--warn);
  }
  .streams-error {
    margin-bottom: 12px;
    padding: 10px 12px;
    border: 1px solid color-mix(in oklab, var(--warn) 45%, transparent);
    border-radius: 8px;
    font-size: 12px;
    color: var(--warn);
    background: color-mix(in oklab, var(--warn) 10%, transparent);
    line-height: 1.4;
  }

  .cam-list {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 8px;
  }
  .cam-row {
    display: grid;
    grid-template-columns: auto 76px 1fr auto;
    align-items: center;
    gap: 12px;
    padding: 10px 12px 10px 8px;
    border: 1px solid var(--border);
    border-radius: var(--r-sm);
    background: var(--surface);
    transition:
      border-color 0.15s ease,
      box-shadow 0.15s ease;
  }
  .cam-row.dragging {
    border-color: var(--border-strong);
    box-shadow: var(--shadow);
  }
  .cam-handle {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 28px;
    height: 36px;
    color: var(--text-3);
    cursor: grab;
    border-radius: 6px;
    touch-action: none;
  }
  .cam-handle:hover,
  .cam-handle:focus-visible {
    color: var(--text);
    background: rgba(125, 125, 125, 0.07);
    outline: none;
  }
  .cam-handle:active {
    cursor: grabbing;
  }
  .cam-thumb {
    width: 76px;
    height: 44px;
    object-fit: cover;
    background: var(--feed);
    border-radius: 6px;
    display: block;
    border: 1px solid var(--border);
  }
  .cam-text {
    display: flex;
    flex-direction: column;
    gap: 2px;
    min-width: 0;
  }
  .cam-name {
    font-size: 14px;
    font-weight: 600;
    color: var(--text);
    letter-spacing: -0.1px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .cam-id {
    font-family: 'JetBrains Mono', ui-monospace, monospace;
    font-size: 11px;
    color: var(--text-3);
    letter-spacing: 0.3px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .cam-edit {
    width: 36px;
    height: 36px;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    background: transparent;
    border: 1px solid var(--border);
    border-radius: 8px;
    color: var(--text-2);
    cursor: pointer;
    transition:
      color 0.15s ease,
      background 0.15s ease,
      border-color 0.15s ease;
  }
  .cam-edit:hover {
    color: var(--text);
    background: rgba(125, 125, 125, 0.07);
    border-color: var(--border-strong);
  }
  .cam-edit:active {
    transform: translateY(1px);
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
