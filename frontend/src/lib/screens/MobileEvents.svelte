<script lang="ts">
  import type { Camera, EventItem, EventKind, Group } from '$lib/api'
  import { eventSnapshotURL } from '$lib/api'
  import EmptyState from '$lib/components/EmptyState.svelte'
  import Icon from '$lib/components/Icon.svelte'
  import Mono from '$lib/components/Mono.svelte'
  import OnlineDot from '$lib/components/OnlineDot.svelte'
  import { kindIcon } from '$lib/icons'
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
        <span class="chip-label">{ui.filterLabelGroup}</span>
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
      <span class="chip-label">{ui.filterLabelCamera}</span>
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
            onclick={() => onToggleCam(cam.id)}
          >
            <OnlineDot online={cam.online} size={6} />{cam.name}</button
          >
        {/each}
      </div>
    </div>

    <div class="chip-group">
      <span class="chip-label">{ui.filterLabelType}</span>
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
            class="pill icon-only"
            class:active={activeKinds.has(k)}
            aria-label={eventKindLabels[k]}
            title={eventKindLabels[k]}
            onclick={() => onToggleKind(k)}
          >
            <Icon name={kindIcon[k]} size={15} /></button
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
        <div class="day-div">{dayLabel(day.date)}</div>
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
                <Mono size={13} color="var(--text-2)">{relativeTime(ev.started_at, now)}</Mono>
              </div>
              <div class="l2">
                <span class="ev-kindwrap">
                  <Icon name={kindIcon[ev.kind]} size={14} />
                  <span class="ev-kind">{eventKindLabels[ev.kind] ?? ev.kind}</span>
                </span>
                <Mono size={12} color="var(--accent-ink)" weight={500}
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
    padding: 14px 18px calc(env(safe-area-inset-bottom, 0px) + 96px);
  }
  .chip-groups {
    display: flex;
    flex-direction: column;
    gap: 8px;
    /* tune-on-device: a hairline + extra bottom space separate the filter
       block from the day-grouped list so the TYPE label and the first TODAY
       divider stop crowding. */
    padding-bottom: 14px;
    margin-bottom: 10px;
    border-bottom: 1px solid var(--border);
  }
  .chip-group {
    display: flex;
    flex-direction: row;
    align-items: center;
    gap: 10px;
    min-width: 0;
  }
  .chip-label {
    /* 54px (was 46px) so the longest sub-label ("CAMERA") still fits on one
       line at the raised 11px size. */
    flex: 0 0 54px;
    display: inline-flex;
    align-items: center;
    /* Geist eyebrow label — far more legible than mono at small uppercase.
       tune-on-device: 11px / 600 / 0.06em (mono's 1.5px tracking was too loose
       for sans). */
    font-family: inherit;
    font-size: 11px;
    font-weight: 600;
    letter-spacing: 0.06em;
    text-transform: uppercase;
    color: var(--text-2);
  }
  /* calm .chips — horizontally-scrolling pill row, scrollbar hidden */
  .chips {
    display: flex;
    gap: 8px;
    flex-wrap: nowrap;
    overflow-x: auto;
    padding-bottom: 2px;
    scrollbar-width: none;
    -webkit-overflow-scrolling: touch;
    flex: 1 1 auto;
    min-width: 0;
  }
  .chips::-webkit-scrollbar {
    height: 0;
  }
  /* calm .pill */
  .pill {
    flex: 0 0 auto;
    display: inline-flex;
    align-items: center;
    gap: 7px;
    padding: 6px 12px;
    font-size: 13px;
    font-weight: 500;
    color: var(--text-2);
    background: var(--surface);
    border: 1px solid var(--border-strong);
    border-radius: 999px;
    cursor: pointer;
    user-select: none;
    white-space: nowrap;
    font-family: inherit;
    transition:
      color 120ms,
      border-color 120ms,
      background 120ms;
  }
  .pill:active {
    transform: translateY(1px);
  }
  .pill.active {
    background: var(--accent-soft);
    border-color: transparent;
    color: var(--accent-ink);
  }
  /* icon-only TYPE pill: even padding for a roughly square chip, icon centered,
     no label gap to collapse since the text child is gone. */
  .pill.icon-only {
    gap: 0;
    padding: 6px 7px;
    justify-content: center;
  }

  .ev-list {
    display: flex;
    flex-direction: column;
  }
  /* calm .day-div */
  .day-div {
    padding-top: 6px;
    padding-bottom: 4px;
    /* Geist day divider. tune-on-device: 12px / 600 / 0.05em. */
    font-family: inherit;
    font-size: 12px;
    font-weight: 600;
    letter-spacing: 0.05em;
    text-transform: uppercase;
    color: var(--text-2);
  }
  /* calm .ev */
  .ev {
    display: flex;
    gap: 13px;
    align-items: center;
    width: 100%;
    padding: 11px 0;
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
    flex: 0 0 76px;
    border-radius: var(--r-sm);
    overflow: hidden;
    background: var(--feed);
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
    gap: 2px;
    flex: 1 1 auto;
    min-width: 0;
  }
  .l1 {
    display: flex;
    justify-content: space-between;
    align-items: baseline;
    gap: 8px;
    min-width: 0;
    font-size: 16px;
    font-weight: 600;
    color: var(--text);
  }
  .l2 {
    display: flex;
    justify-content: space-between;
    align-items: baseline;
    gap: 8px;
    min-width: 0;
    margin-top: 2px;
    font-size: 13px;
    color: var(--text-2);
  }
  .ev-cam {
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    min-width: 0;
  }
  /* tune-on-device: ~14px kind icon + label, small gap. */
  .ev-kindwrap {
    display: inline-flex;
    align-items: center;
    gap: 5px;
    min-width: 0;
  }
  .ev-kind {
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    min-width: 0;
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
