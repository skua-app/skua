<script lang="ts">
  import type { Camera, EventItem, EventKind, Group } from '$lib/api'
  import EmptyState from '$lib/components/EmptyState.svelte'
  import EventCardWide from '$lib/components/EventCardWide.svelte'
  import Icon from '$lib/components/Icon.svelte'
  import OnlineDot from '$lib/components/OnlineDot.svelte'
  import { kindIcon } from '$lib/icons'
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

  // Tick once a minute so day labels stay correct around midnight.
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

<div class="dk-page wide">
  <div class="dk-pagehead">
    <div class="dk-titlewrap">
      <div class="dk-h1">{ui.eventsTitle}</div>
    </div>
  </div>

  <div class="dk-filterbar">
    {#if groups.length > 0}
      <div class="dk-filterrow">
        <span class="lbl">{ui.filterLabelGroup}</span>
        <div class="dk-chips" role="group" aria-label={ui.groupAria}>
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

    <div class="dk-filterrow">
      <span class="lbl">{ui.filterLabelCamera}</span>
      <div class="dk-chips" role="group" aria-label={ui.cameraAria}>
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

    <div class="dk-filterrow">
      <span class="lbl">{ui.filterLabelType}</span>
      <div class="dk-chips" role="group" aria-label={ui.kindAria}>
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
            aria-label={eventKindLabels[k]}
            title={eventKindLabels[k]}
            onclick={() => onToggleKind(k)}
          >
            <Icon name={kindIcon[k]} size={15} />{eventKindLabels[k]}</button
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
          <span class="dk-daydiv-text">{dayLabel(day.date)}</span>
          <span class="ln" aria-hidden="true"></span>
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
  .dk-page {
    padding: 24px 28px calc(env(safe-area-inset-bottom, 0px) + 48px);
  }

  /* dk-pagehead */
  .dk-pagehead {
    display: flex;
    align-items: flex-end;
    justify-content: space-between;
    gap: 18px;
    margin-bottom: 20px;
  }
  .dk-titlewrap {
    display: flex;
    align-items: baseline;
    gap: 12px;
    min-width: 0;
  }
  .dk-h1 {
    font-size: 28px;
    font-weight: 600;
    letter-spacing: -0.6px;
    line-height: 1.05;
    color: var(--text);
  }

  /* dk-filterbar */
  .dk-filterbar {
    display: flex;
    flex-direction: column;
    gap: 8px;
    /* tune-on-device: a touch more space so the TYPE row and the first day
       divider don't crowd now that both are Geist. */
    margin-bottom: 20px;
  }
  .dk-filterrow {
    display: flex;
    align-items: center;
    gap: 10px;
  }
  .dk-filterrow .lbl {
    flex: 0 0 64px;
    /* Geist eyebrow label. tune-on-device: 11px / 600 / 0.06em. */
    font-family: inherit;
    font-size: 11px;
    font-weight: 600;
    letter-spacing: 0.06em;
    text-transform: uppercase;
    color: var(--text-2);
  }
  .dk-chips {
    display: flex;
    gap: 8px;
    flex-wrap: wrap;
    flex: 1 1 auto;
    min-width: 0;
  }

  /* calm .pill — shared with mobile but here within dk-chips */
  .pill {
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
  .pill:hover {
    border-color: var(--border-strong);
    color: var(--text);
  }
  .pill.active {
    background: var(--accent-soft);
    border-color: transparent;
    color: var(--accent-ink);
  }
  .pill:active {
    transform: translateY(1px);
  }

  /* dk-evgrid card grid */
  .dk-evgrid {
    display: grid;
    gap: 14px;
    grid-template-columns: repeat(auto-fill, minmax(236px, 1fr));
  }

  /* dk-daydiv — full-width Geist uppercase divider with trailing line.
     tune-on-device: 12px / 600 / 0.05em. */
  .dk-daydiv {
    grid-column: 1 / -1;
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 18px 2px 0;
    font-family: inherit;
    font-size: 12px;
    font-weight: 600;
    letter-spacing: 0.05em;
    text-transform: uppercase;
    color: var(--text-2);
  }
  .dk-daydiv:first-child {
    padding-top: 0;
  }
  .dk-daydiv-text {
    flex: 0 0 auto;
  }
  .dk-daydiv .ln {
    flex: 1 1 auto;
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
