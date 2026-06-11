<script lang="ts">
  import type { Camera, EventItem, EventKind, Group } from '$lib/api'
  import { eventSnapshotURL } from '$lib/api'
  import EmptyState from '$lib/components/EmptyState.svelte'
  import Mono from '$lib/components/Mono.svelte'
  import { camerasStore } from '$lib/stores/cameras.svelte'
  import { ui, eventKindLabels } from '$lib/i18n/strings'
  import { relativeTime, formatDuration } from '$lib/util/time'
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

  // Tick once per minute so day labels + relative times stay correct.
  let now = $state(new Date())
  $effect(() => {
    const t = setInterval(() => (now = new Date()), 60_000)
    return () => clearInterval(t)
  })

  const dateFmt = new Intl.DateTimeFormat([], {
    weekday: 'short',
    month: 'short',
    day: 'numeric'
  })
  function dayLabel(date: Date): string {
    const today = new Date(now.getFullYear(), now.getMonth(), now.getDate())
    const diff = Math.round((today.getTime() - date.getTime()) / 86_400_000)
    if (diff === 0) return ui.today
    if (diff === 1) return ui.yesterday
    return dateFmt.format(date)
  }
  function camName(id: string): string {
    return camerasStore.cameras.find((c) => c.id === id)?.name ?? id
  }

  const totalItems = $derived(eventDays.reduce((sum, d) => sum + d.items.length, 0))

  let sentinel: HTMLDivElement | null = $state(null)
  $effect(() => {
    if (!sentinel) return
    const el = sentinel
    const io = new IntersectionObserver(
      (entries) => {
        for (const e of entries) if (e.isIntersecting) loadMore()
      },
      { rootMargin: '320px' }
    )
    io.observe(el)
    return () => io.disconnect()
  })
</script>

<div class="ev-page">
  <div class="chip-groups">
    {#if groups.length > 0}
      <div class="chip-group">
        <span class="chip-label">
          <Mono size={10} color="var(--text-3)" letterSpacing={1.5} uppercase
            >{ui.filterLabelGroup}</Mono
          >
        </span>
        <div class="chips" role="group" aria-label={ui.groupAria}>
          <button
            type="button"
            class="pill"
            class:active={activeGroupId === null}
            onclick={() => onSelectGroup(null)}>{ui.filterAll}</button
          >
          {#each groups as g (g.id)}
            <button
              type="button"
              class="pill"
              class:active={activeGroupId === g.id}
              onclick={() => onSelectGroup(g.id)}>{g.name}</button
            >
          {/each}
        </div>
      </div>
    {/if}

    <div class="chip-group">
      <span class="chip-label">
        <Mono size={10} color="var(--text-3)" letterSpacing={1.5} uppercase
          >{ui.filterLabelCamera}</Mono
        >
      </span>
      <div class="chips" role="group" aria-label={ui.cameraAria}>
        <button
          type="button"
          class="pill"
          class:active={activeCams.size === 0}
          onclick={onResetCams}>{ui.filterAll}</button
        >
        {#each visibleCameras as cam (cam.id)}
          <button
            type="button"
            class="pill"
            class:active={activeCams.has(cam.id)}
            onclick={() => onToggleCam(cam.id)}>{cam.name}</button
          >
        {/each}
      </div>
    </div>

    <div class="chip-group">
      <span class="chip-label">
        <Mono size={10} color="var(--text-3)" letterSpacing={1.5} uppercase
          >{ui.filterLabelType}</Mono
        >
      </span>
      <div class="chips" role="group" aria-label={ui.kindAria}>
        <button
          type="button"
          class="pill"
          class:active={activeKinds.size === 0}
          onclick={onResetKinds}>{ui.filterAll}</button
        >
        {#each kindOrder as k}
          <button
            type="button"
            class="pill"
            class:active={activeKinds.has(k)}
            onclick={() => onToggleKind(k)}>{eventKindLabels[k]}</button
          >
        {/each}
      </div>
    </div>
  </div>

  {#if loadError && totalItems === 0}
    <div class="ev-error">
      <span>{ui.eventsLoadError}</span>
      <button type="button" class="ev-retry" onclick={onRetry}>{ui.retry}</button>
    </div>
  {:else if totalItems === 0 && !loading}
    <EmptyState title={ui.eventsEmpty} />
  {:else}
    <div class="ev-list">
      {#each eventDays as day (day.key)}
        <div class="day-div">
          <Mono size={11} color="var(--text-3)" letterSpacing={1} uppercase
            >{dayLabel(day.date)}</Mono
          >
        </div>
        {#each day.items as ev (ev.id)}
          <button type="button" class="ev" onclick={() => onOpen(ev)}>
            <div class="ev-thumb">
              <img
                src={eventSnapshotURL(ev.id)}
                alt={`${camName(ev.cam_id)} · ${eventKindLabels[ev.kind] ?? ev.kind}`}
                loading="lazy"
                width="76"
                height="48"
              />
            </div>
            <div class="ev-meta">
              <div class="l1">
                <span class="ev-cam">{camName(ev.cam_id)}</span>
                <Mono size={11} color="var(--text-3)">{relativeTime(ev.started_at, now)}</Mono>
              </div>
              <div class="l2">
                <span class="ev-kind">{eventKindLabels[ev.kind] ?? ev.kind}</span>
                <Mono size={11} color="var(--accent)" weight={500}
                  >{ev.score !== null
                    ? ev.score.toFixed(2)
                    : formatDuration(ev.duration_seconds)}</Mono
                >
              </div>
            </div>
          </button>
        {/each}
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
    padding: 14px 16px calc(env(safe-area-inset-bottom, 0px) + 96px);
  }
  .chip-groups {
    display: flex;
    flex-direction: column;
    gap: 10px;
    margin-bottom: 14px;
  }
  .chip-group {
    display: flex;
    flex-direction: column;
    gap: 6px;
    min-width: 0;
  }
  .chip-label {
    padding: 0 2px;
    display: inline-flex;
  }
  .chips {
    display: flex;
    gap: 6px;
    flex-wrap: nowrap;
    overflow-x: auto;
    scrollbar-width: none;
    -webkit-overflow-scrolling: touch;
    margin: 0 -16px;
    padding: 0 16px;
  }
  .chips::-webkit-scrollbar {
    display: none;
  }
  .pill {
    flex: none;
    padding: 6px 12px;
    font-size: 12px;
    color: var(--text-3);
    background: transparent;
    border: 1px solid var(--border);
    border-radius: 999px;
    cursor: pointer;
    font-family: inherit;
    white-space: nowrap;
    transition:
      color 120ms,
      border-color 120ms,
      background 120ms;
  }
  .pill:hover {
    color: var(--text-2);
  }
  .pill.active {
    color: var(--accent);
    border-color: color-mix(in oklab, var(--accent) 50%, transparent);
    background: color-mix(in oklab, var(--accent) 14%, transparent);
  }
  .ev-list {
    display: flex;
    flex-direction: column;
  }
  .day-div {
    padding: 14px 0 8px;
  }
  .ev {
    display: flex;
    gap: 12px;
    align-items: center;
    width: 100%;
    padding: 10px 0;
    border: none;
    border-bottom: 1px solid var(--border);
    background: transparent;
    color: inherit;
    text-align: left;
    cursor: pointer;
    font-family: inherit;
  }
  .ev:last-child {
    border-bottom: none;
  }
  .ev-thumb {
    width: 76px;
    height: 48px;
    flex: none;
    border-radius: 10px;
    overflow: hidden;
    background: #0c0d0f;
  }
  .ev-thumb img {
    width: 100%;
    height: 100%;
    object-fit: cover;
    display: block;
  }
  .ev-meta {
    display: flex;
    flex-direction: column;
    gap: 3px;
    flex: 1;
    min-width: 0;
  }
  .l1,
  .l2 {
    display: flex;
    justify-content: space-between;
    align-items: baseline;
    gap: 10px;
    min-width: 0;
  }
  .ev-cam {
    font-size: 13px;
    font-weight: 600;
    color: var(--text);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .ev-kind {
    font-size: 12px;
    color: var(--text-2);
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
</style>
