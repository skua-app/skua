<script lang="ts">
  import { untrack } from 'svelte'
  import { fetchEvents } from '$lib/api'
  import type { EventItem, EventKind } from '$lib/api'
  import { camerasStore } from '$lib/stores/cameras.svelte'
  import { eventsStreamStore } from '$lib/stores/events-stream.svelte'
  import { groupsStore } from '$lib/stores/groups.svelte'
  import { ui, eventKindLabels } from '$lib/i18n/strings'
  import EmptyState from '$lib/components/EmptyState.svelte'
  import EventCard from '$lib/components/EventCard.svelte'
  import EventModal from '$lib/components/EventModal.svelte'

  const PAGE_SIZE = 50

  // Frigate labels we want to filter by, grouped per kind. Sent to BFF as the
  // `label` query repeats; BFF passes them straight to Frigate as a comma
  // list. Keep this in sync with backend/internal/events.labelKind.
  const labelsByKind: Record<EventKind, string[]> = {
    person: ['person'],
    vehicle: ['car', 'truck', 'bus', 'motorcycle', 'bicycle'],
    animal: ['dog', 'cat', 'bird'],
    other: []
  }
  const kindOrder: EventKind[] = ['person', 'vehicle', 'animal', 'other']

  let activeCams = $state<Set<string>>(new Set())
  let activeKinds = $state<Set<EventKind>>(new Set())
  // Group filter is per-session (not persisted): null = no group filter.
  let activeGroupId = $state<string | null>(null)

  // Cameras visible after the group pre-filter is applied. The per-camera
  // chip row hides cameras outside the group, and the effective camera
  // filter is intersected with this set.
  const groupCameraIds = $derived.by<Set<string> | null>(() => {
    if (activeGroupId === null) return null
    const g = groupsStore.groups.find((x) => x.id === activeGroupId)
    if (!g) return new Set()
    return new Set(g.camera_ids)
  })
  const visibleCameras = $derived(
    groupCameraIds === null
      ? camerasStore.cameras
      : camerasStore.cameras.filter((c) => groupCameraIds.has(c.id))
  )

  // If the user-selected group is deleted (e.g. from another tab), drop it.
  $effect(() => {
    if (!groupsStore.loaded) return
    if (activeGroupId !== null && !groupsStore.groups.some((g) => g.id === activeGroupId)) {
      activeGroupId = null
    }
  })

  let items = $state<EventItem[]>([])
  let nextBefore = $state<string | null>(null)
  let loading = $state(false)
  let loadError = $state<string | null>(null)
  // Generation counter — bumps on every filter change to invalidate
  // in-flight pages from older filter states.
  let generation = $state(0)
  // Cursor for SSE merge: the started_at of the newest event we've already
  // applied from the events-stream store.
  let lastAppliedAt = $state('')

  let sentinel: HTMLDivElement | null = $state(null)
  let modalEvent = $state<EventItem | null>(null)

  function selectedKinds(): EventKind[] {
    return Array.from(activeKinds)
  }

  // Flat list of Frigate labels implied by the active kind set, used for the
  // upstream `label` filter. Empty kind set ⇒ no label filter.
  function activeLabels(): string[] {
    const kinds = selectedKinds()
    if (kinds.length === 0) return []
    const out: string[] = []
    for (const k of kinds) out.push(...labelsByKind[k])
    return out
  }

  // Effective set of cameras to send to the BFF: per-camera selection
  // intersected with group selection. Empty Set with a group active means
  // "all cameras in this group"; null means no filter at all.
  function effectiveCams(): string[] | undefined {
    const gIds = groupCameraIds
    if (gIds === null) {
      return activeCams.size > 0 ? Array.from(activeCams) : undefined
    }
    if (activeCams.size === 0) {
      return Array.from(gIds)
    }
    const inter: string[] = []
    for (const c of activeCams) {
      if (gIds.has(c)) inter.push(c)
    }
    return inter
  }

  function matchesFilters(ev: EventItem): boolean {
    const cams = effectiveCams()
    if (cams !== undefined && !cams.includes(ev.cam_id)) return false
    if (activeKinds.size > 0 && !activeKinds.has(ev.kind)) return false
    return true
  }

  async function loadFirstPage() {
    const gen = ++generation
    loading = true
    loadError = null
    items = []
    nextBefore = null
    try {
      const resp = await fetchEvents({
        cameras: effectiveCams(),
        labels: activeLabels().length > 0 ? activeLabels() : undefined,
        limit: PAGE_SIZE
      })
      if (gen !== generation) return
      items = resp.items
      nextBefore = resp.next_before
      lastAppliedAt = resp.items[0]?.started_at ?? new Date().toISOString()
    } catch (err) {
      if (gen !== generation) return
      loadError = err instanceof Error ? err.message : 'unknown'
    } finally {
      if (gen === generation) loading = false
    }
  }

  async function loadMore() {
    if (loading || nextBefore === null) return
    const gen = generation
    loading = true
    try {
      const resp = await fetchEvents({
        cameras: effectiveCams(),
        labels: activeLabels().length > 0 ? activeLabels() : undefined,
        before: nextBefore,
        limit: PAGE_SIZE
      })
      if (gen !== generation) return
      // Dedupe by id in case of overlapping cursors.
      const existing = new Set(items.map((e) => e.id))
      const fresh = resp.items.filter((e) => !existing.has(e.id))
      items = [...items, ...fresh]
      nextBefore = resp.next_before
    } catch (err) {
      if (gen !== generation) return
      loadError = err instanceof Error ? err.message : 'unknown'
    } finally {
      if (gen === generation) loading = false
    }
  }

  function toggleCam(id: string) {
    const next = new Set(activeCams)
    if (next.has(id)) next.delete(id)
    else next.add(id)
    activeCams = next
  }
  function resetCams() {
    activeCams = new Set()
  }
  function toggleKind(k: EventKind) {
    const next = new Set(activeKinds)
    if (next.has(k)) next.delete(k)
    else next.add(k)
    activeKinds = next
  }
  function resetKinds() {
    activeKinds = new Set()
  }

  // Re-fetch whenever filters change. The Array.from() reads register the
  // dependency on activeCams / activeKinds (and activeGroupId via the join)
  // while the fetch runs untracked.
  $effect(() => {
    const camKey = Array.from(activeCams).sort().join(',')
    const kindKey = Array.from(activeKinds).sort().join(',')
    const groupKey = activeGroupId ?? ''
    untrack(() => {
      void `${camKey}|${kindKey}|${groupKey}`
      loadFirstPage()
    })
  })

  // IntersectionObserver-driven infinite scroll. Re-creates when sentinel
  // re-binds; nextBefore is read inside the callback so it stays current.
  $effect(() => {
    if (!sentinel) return
    const el = sentinel
    const io = new IntersectionObserver(
      (entries) => {
        for (const entry of entries) {
          if (entry.isIntersecting) {
            loadMore()
          }
        }
      },
      { rootMargin: '320px' }
    )
    io.observe(el)
    return () => io.disconnect()
  })

  // SSE merge: when new events arrive in the stream and beat our newest,
  // prepend matching items to the page.
  $effect(() => {
    const stream = eventsStreamStore.latest
    if (stream.length === 0) return
    const newer = stream.filter((e) => e.started_at > lastAppliedAt && matchesFilters(e))
    if (newer.length === 0) return
    untrack(() => {
      const existing = new Set(items.map((e) => e.id))
      const toAdd = newer.filter((e) => !existing.has(e.id))
      const head = toAdd[0]
      if (!head) return
      // Stream is ordered newest-first; preserve that.
      items = [...toAdd, ...items]
      lastAppliedAt = head.started_at
    })
  })
</script>

<div class="ev-page">
  <h1 class="ev-title">{ui.eventsTitle}</h1>

  <div class="ev-filters">
    {#if groupsStore.groups.length > 0}
      <div class="ev-chip-row" role="group" aria-label={ui.groupAria}>
        <button
          type="button"
          class="ev-chip"
          class:active={activeGroupId === null}
          onclick={() => (activeGroupId = null)}>{ui.filterAll}</button
        >
        {#each groupsStore.groups as g (g.id)}
          <button
            type="button"
            class="ev-chip"
            class:active={activeGroupId === g.id}
            onclick={() => (activeGroupId = g.id)}>{g.name}</button
          >
        {/each}
      </div>
    {/if}

    <div class="ev-chip-row" role="group" aria-label={ui.cameraAria}>
      <button type="button" class="ev-chip" class:active={activeCams.size === 0} onclick={resetCams}
        >{ui.filterAll}</button
      >
      {#each visibleCameras as cam (cam.id)}
        <button
          type="button"
          class="ev-chip"
          class:active={activeCams.has(cam.id)}
          onclick={() => toggleCam(cam.id)}>{cam.name}</button
        >
      {/each}
    </div>

    <div class="ev-chip-row" role="group" aria-label={ui.kindAria}>
      <button
        type="button"
        class="ev-chip"
        class:active={activeKinds.size === 0}
        onclick={resetKinds}>{ui.filterAll}</button
      >
      {#each kindOrder as k}
        <button
          type="button"
          class="ev-chip"
          class:active={activeKinds.has(k)}
          onclick={() => toggleKind(k)}>{eventKindLabels[k]}</button
        >
      {/each}
    </div>
  </div>

  {#if loadError && items.length === 0}
    <div class="ev-error">
      <span>{ui.eventsLoadError}</span>
      <button type="button" class="ev-retry" onclick={loadFirstPage}>{ui.retry}</button>
    </div>
  {:else if items.length === 0 && !loading}
    <EmptyState title={ui.eventsEmpty} />
  {:else}
    <div class="ev-list">
      {#each items as ev (ev.id)}
        <EventCard event={ev} onclick={() => (modalEvent = ev)} />
      {/each}
      {#if loading}
        <div class="ev-loading">{ui.eventsLoading}</div>
      {/if}
      <div bind:this={sentinel} class="ev-sentinel" aria-hidden="true"></div>
    </div>
  {/if}
</div>

{#if modalEvent}
  <EventModal event={modalEvent} onClose={() => (modalEvent = null)} />
{/if}

<style>
  .ev-page {
    max-width: 720px;
    margin: 0 auto;
    padding: 18px 16px calc(env(safe-area-inset-bottom, 0px) + 96px);
  }
  .ev-title {
    font-size: 19px;
    font-weight: 600;
    letter-spacing: -0.4px;
    margin: 6px 4px 14px;
  }
  /* AppHeader already shows the page title on mobile (.ah-m-title). The
     desktop header only has small nav links, so the h1 stays as the page's
     primary heading there. The breakpoint mirrors layout.svelte's
     isDesktop = width >= 900. */
  @media (max-width: 899px) {
    .ev-title {
      display: none;
    }
  }

  .ev-filters {
    display: flex;
    flex-direction: column;
    gap: 8px;
    margin-bottom: 16px;
  }
  .ev-chip-row {
    display: flex;
    gap: 6px;
    flex-wrap: wrap;
  }
  .ev-chip {
    padding: 5px 11px;
    font-size: 12px;
    color: var(--text-3);
    background: transparent;
    border: 1px solid transparent;
    border-radius: 6px;
    cursor: pointer;
    font-family: inherit;
    transition:
      color 120ms,
      border-color 120ms,
      background 120ms;
  }
  .ev-chip:hover {
    color: var(--text-2);
  }
  .ev-chip.active {
    color: var(--accent);
    border-color: color-mix(in oklab, var(--accent) 50%, transparent);
    background: color-mix(in oklab, var(--accent) 14%, transparent);
  }

  .ev-list {
    display: flex;
    flex-direction: column;
    gap: 10px;
  }
  .ev-loading {
    text-align: center;
    color: var(--text-3);
    font-size: 12px;
    padding: 14px 0;
  }
  .ev-sentinel {
    height: 1px;
  }

  .ev-error {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 10px;
    padding: 32px 12px;
    color: var(--text-2);
    font-size: 13px;
  }
  .ev-retry {
    padding: 6px 12px;
    font-size: 12px;
    color: var(--text);
    background: transparent;
    border: 1px solid var(--border-strong);
    border-radius: 6px;
    cursor: pointer;
    font-family: inherit;
  }
  .ev-retry:hover {
    border-color: var(--text-3);
  }
</style>
