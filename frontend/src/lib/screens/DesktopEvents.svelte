<script lang="ts">
  import type { Camera, EventItem, EventKind, Group } from '$lib/api'
  import EmptyState from '$lib/components/EmptyState.svelte'
  import EventCardWide from '$lib/components/EventCardWide.svelte'
  import Mono from '$lib/components/Mono.svelte'
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

  // Tick once per minute so "Today" / "Yesterday" stay correct when a day
  // rolls over while the page is open.
  let now = $state(new Date())
  $effect(() => {
    const t = setInterval(() => {
      now = new Date()
    }, 60_000)
    return () => clearInterval(t)
  })

  const dateFmt = new Intl.DateTimeFormat([], {
    weekday: 'short',
    month: 'short',
    day: 'numeric'
  })

  function dayLabel(date: Date): string {
    const today = new Date(now.getFullYear(), now.getMonth(), now.getDate())
    const diffDays = Math.round((today.getTime() - date.getTime()) / 86_400_000)
    if (diffDays === 0) return ui.today
    if (diffDays === 1) return ui.yesterday
    return dateFmt.format(date)
  }

  const totalItems = $derived(eventDays.reduce((sum, d) => sum + d.items.length, 0))

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

<div class="dk-events">
  <h1 class="dk-title">{ui.eventsTitle}</h1>

  <div class="dk-filterbar">
    {#if groups.length > 0}
      <div class="dk-filterrow">
        <span class="dk-filterrow-label">
          <Mono size={10} color="var(--text-3)" letterSpacing={0.8} uppercase
            >{ui.filterLabelGroup}</Mono
          >
        </span>
        <div class="dk-chips" role="group" aria-label={ui.groupAria}>
          <button
            type="button"
            class="dk-chip"
            class:active={activeGroupId === null}
            onclick={() => onSelectGroup(null)}>{ui.filterAll}</button
          >
          {#each groups as g (g.id)}
            <button
              type="button"
              class="dk-chip"
              class:active={activeGroupId === g.id}
              onclick={() => onSelectGroup(g.id)}>{g.name}</button
            >
          {/each}
        </div>
      </div>
    {/if}

    <div class="dk-filterrow">
      <span class="dk-filterrow-label">
        <Mono size={10} color="var(--text-3)" letterSpacing={0.8} uppercase
          >{ui.filterLabelCamera}</Mono
        >
      </span>
      <div class="dk-chips" role="group" aria-label={ui.cameraAria}>
        <button
          type="button"
          class="dk-chip"
          class:active={activeCams.size === 0}
          onclick={onResetCams}>{ui.filterAll}</button
        >
        {#each visibleCameras as cam (cam.id)}
          <button
            type="button"
            class="dk-chip"
            class:active={activeCams.has(cam.id)}
            onclick={() => onToggleCam(cam.id)}>{cam.name}</button
          >
        {/each}
      </div>
    </div>

    <div class="dk-filterrow">
      <span class="dk-filterrow-label">
        <Mono size={10} color="var(--text-3)" letterSpacing={0.8} uppercase
          >{ui.filterLabelType}</Mono
        >
      </span>
      <div class="dk-chips" role="group" aria-label={ui.kindAria}>
        <button
          type="button"
          class="dk-chip"
          class:active={activeKinds.size === 0}
          onclick={onResetKinds}>{ui.filterAll}</button
        >
        {#each kindOrder as k}
          <button
            type="button"
            class="dk-chip"
            class:active={activeKinds.has(k)}
            onclick={() => onToggleKind(k)}>{eventKindLabels[k]}</button
          >
        {/each}
      </div>
    </div>
  </div>

  {#if loadError && totalItems === 0}
    <div class="dk-error">
      <span>{ui.eventsLoadError}</span>
      <button type="button" class="dk-retry" onclick={onRetry}>{ui.retry}</button>
    </div>
  {:else if totalItems === 0 && !loading}
    <EmptyState title={ui.eventsEmpty} />
  {:else}
    <div class="dk-evgrid">
      {#each eventDays as day (day.key)}
        <div class="dk-daydiv">
          <Mono size={11} color="var(--text-3)" letterSpacing={1.2} uppercase
            >{dayLabel(day.date)}</Mono
          >
          <span class="dk-daydiv-rule" aria-hidden="true"></span>
        </div>
        {#each day.items as ev (ev.id)}
          <EventCardWide event={ev} onclick={() => onOpen(ev)} />
        {/each}
      {/each}
    </div>
    {#if loading}
      <div class="dk-loading">{ui.eventsLoading}</div>
    {/if}
    <div bind:this={sentinel} class="dk-sentinel" aria-hidden="true"></div>
  {/if}
</div>

<style>
  .dk-events {
    padding: 24px 28px calc(env(safe-area-inset-bottom, 0px) + 48px);
  }
  .dk-title {
    font-size: 28px;
    font-weight: 600;
    letter-spacing: -0.6px;
    margin: 0 0 18px;
  }

  .dk-filterbar {
    display: flex;
    flex-direction: column;
    gap: 10px;
    margin-bottom: 22px;
  }
  .dk-filterrow {
    display: flex;
    align-items: center;
    gap: 14px;
  }
  .dk-filterrow-label {
    width: 84px;
    flex: none;
    display: inline-flex;
    align-items: center;
  }
  .dk-chips {
    display: flex;
    gap: 6px;
    flex-wrap: wrap;
  }
  .dk-chip {
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
  .dk-chip:hover {
    color: var(--text-2);
  }
  .dk-chip.active {
    color: var(--accent);
    border-color: color-mix(in oklab, var(--accent) 50%, transparent);
    background: color-mix(in oklab, var(--accent) 14%, transparent);
  }

  .dk-evgrid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(236px, 1fr));
    gap: 14px;
  }
  .dk-daydiv {
    grid-column: 1 / -1;
    display: flex;
    align-items: center;
    gap: 12px;
    margin-top: 6px;
  }
  .dk-daydiv:first-child {
    margin-top: 0;
  }
  .dk-daydiv-rule {
    flex: 1;
    height: 1px;
    background: var(--border);
  }

  .dk-loading {
    text-align: center;
    color: var(--text-3);
    font-size: 12px;
    padding: 18px 0;
  }
  .dk-sentinel {
    height: 1px;
  }
  .dk-error {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 10px;
    padding: 32px 12px;
    color: var(--text-2);
    font-size: 13px;
  }
  .dk-retry {
    padding: 6px 12px;
    font-size: 12px;
    color: var(--text);
    background: transparent;
    border: 1px solid var(--border-strong);
    border-radius: 6px;
    cursor: pointer;
    font-family: inherit;
  }
  .dk-retry:hover {
    border-color: var(--text-3);
  }
</style>
