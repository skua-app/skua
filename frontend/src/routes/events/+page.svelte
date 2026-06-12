<script lang="ts">
  import { untrack } from 'svelte'
  import { fetchEvents } from '$lib/api'
  import type { EventItem, EventKind } from '$lib/api'
  import { camerasStore } from '$lib/stores/cameras.svelte'
  import { eventsStreamStore } from '$lib/stores/events-stream.svelte'
  import { groupsStore } from '$lib/stores/groups.svelte'
  import EventModal from '$lib/components/EventModal.svelte'
  import DesktopEvents from '$lib/screens/DesktopEvents.svelte'
  import MobileEvents from '$lib/screens/MobileEvents.svelte'
  import { groupEventsByDay } from '$lib/util/time'

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
  const kindOrder: EventKind[] = ['person', 'vehicle', 'animal']

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

  let width = $state(typeof window !== 'undefined' ? window.innerWidth : 0)
  const isDesktop = $derived(width >= 900)

  const eventDays = $derived(groupEventsByDay(items))
</script>

<svelte:window bind:innerWidth={width} />

{#if isDesktop}
  <DesktopEvents
    {eventDays}
    {loading}
    {loadError}
    groups={groupsStore.groups}
    {visibleCameras}
    {kindOrder}
    {activeGroupId}
    {activeCams}
    {activeKinds}
    onSelectGroup={(id) => (activeGroupId = id)}
    onToggleCam={toggleCam}
    onResetCams={resetCams}
    onToggleKind={toggleKind}
    onResetKinds={resetKinds}
    onOpen={(ev) => (modalEvent = ev)}
    onRetry={loadFirstPage}
    {loadMore}
  />
{:else}
  <MobileEvents
    {eventDays}
    {loading}
    {loadError}
    groups={groupsStore.groups}
    {visibleCameras}
    {kindOrder}
    {activeGroupId}
    {activeCams}
    {activeKinds}
    onSelectGroup={(id) => (activeGroupId = id)}
    onToggleCam={toggleCam}
    onResetCams={resetCams}
    onToggleKind={toggleKind}
    onResetKinds={resetKinds}
    onOpen={(ev) => (modalEvent = ev)}
    onRetry={loadFirstPage}
    {loadMore}
  />
{/if}

{#if modalEvent}
  <EventModal event={modalEvent} onClose={() => (modalEvent = null)} />
{/if}
