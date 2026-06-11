<script lang="ts">
  import type { Camera, EventItem, EventKind, Group } from '$lib/api'
  import EmptyState from '$lib/components/EmptyState.svelte'
  import EventCard from '$lib/components/EventCard.svelte'
  import { ui, eventKindLabels } from '$lib/i18n/strings'
  import type { EventDay } from '$lib/util/time'

  type Props = {
    eventDays: EventDay[]
    loading: boolean
    loadError: string | null
    groups: Group[]
    visibleCameras: Camera[]
    kindOrder: EventKind[]
    activeGroupId: string | null
    activeCams: Set<string>
    activeKinds: Set<EventKind>
    onSelectGroup: (id: string | null) => void
    onToggleCam: (id: string) => void
    onResetCams: () => void
    onToggleKind: (k: EventKind) => void
    onResetKinds: () => void
    onOpen: (ev: EventItem) => void
    onRetry: () => void
    loadMore: () => void
  }

  let {
    eventDays,
    loading,
    loadError,
    groups,
    visibleCameras,
    kindOrder,
    activeGroupId,
    activeCams,
    activeKinds,
    onSelectGroup,
    onToggleCam,
    onResetCams,
    onToggleKind,
    onResetKinds,
    onOpen,
    onRetry,
    loadMore
  }: Props = $props()

  // Mobile shows a flat newest-first list (no day dividers) — flatten the
  // route-provided buckets back to a single sequence.
  const items = $derived(eventDays.flatMap((d) => d.items))

  let sentinel: HTMLDivElement | null = $state(null)
  $effect(() => {
    if (!sentinel) return
    const el = sentinel
    const io = new IntersectionObserver(
      (entries) => {
        for (const entry of entries) {
          if (entry.isIntersecting) loadMore()
        }
      },
      { rootMargin: '320px' }
    )
    io.observe(el)
    return () => io.disconnect()
  })
</script>

<div class="ev-page">
  <div class="ev-filters">
    {#if groups.length > 0}
      <div class="ev-chip-row" role="group" aria-label={ui.groupAria}>
        <button
          type="button"
          class="ev-chip"
          class:active={activeGroupId === null}
          onclick={() => onSelectGroup(null)}>{ui.filterAll}</button
        >
        {#each groups as g (g.id)}
          <button
            type="button"
            class="ev-chip"
            class:active={activeGroupId === g.id}
            onclick={() => onSelectGroup(g.id)}>{g.name}</button
          >
        {/each}
      </div>
    {/if}

    <div class="ev-chip-row" role="group" aria-label={ui.cameraAria}>
      <button
        type="button"
        class="ev-chip"
        class:active={activeCams.size === 0}
        onclick={onResetCams}>{ui.filterAll}</button
      >
      {#each visibleCameras as cam (cam.id)}
        <button
          type="button"
          class="ev-chip"
          class:active={activeCams.has(cam.id)}
          onclick={() => onToggleCam(cam.id)}>{cam.name}</button
        >
      {/each}
    </div>

    <div class="ev-chip-row" role="group" aria-label={ui.kindAria}>
      <button
        type="button"
        class="ev-chip"
        class:active={activeKinds.size === 0}
        onclick={onResetKinds}>{ui.filterAll}</button
      >
      {#each kindOrder as k}
        <button
          type="button"
          class="ev-chip"
          class:active={activeKinds.has(k)}
          onclick={() => onToggleKind(k)}>{eventKindLabels[k]}</button
        >
      {/each}
    </div>
  </div>

  {#if loadError && items.length === 0}
    <div class="ev-error">
      <span>{ui.eventsLoadError}</span>
      <button type="button" class="ev-retry" onclick={onRetry}>{ui.retry}</button>
    </div>
  {:else if items.length === 0 && !loading}
    <EmptyState title={ui.eventsEmpty} />
  {:else}
    <div class="ev-list">
      {#each items as ev (ev.id)}
        <EventCard event={ev} onclick={() => onOpen(ev)} />
      {/each}
      {#if loading}
        <div class="ev-loading">{ui.eventsLoading}</div>
      {/if}
      <div bind:this={sentinel} class="ev-sentinel" aria-hidden="true"></div>
    </div>
  {/if}
</div>

<style>
  .ev-page {
    max-width: 720px;
    margin: 0 auto;
    padding: 18px 16px calc(env(safe-area-inset-bottom, 0px) + 96px);
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
