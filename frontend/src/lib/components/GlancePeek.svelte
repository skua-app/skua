<script lang="ts">
  import { fade } from 'svelte/transition'
  import { cubicOut } from 'svelte/easing'
  import { goto } from '$app/navigation'
  import type { GlanceMoment } from '$lib/api'
  import { eventThumbnailURL } from '$lib/api'
  import BottomSheet from '$lib/components/BottomSheet.svelte'
  import Icon from '$lib/components/Icon.svelte'
  import MomentModal from '$lib/components/MomentModal.svelte'
  import Mono from '$lib/components/Mono.svelte'
  import { glanceStore } from '$lib/stores/glance.svelte'
  import { camerasStore } from '$lib/stores/cameras.svelte'
  import { prefsStore } from '$lib/stores/prefs.svelte'
  import { eventKindLabels, ui } from '$lib/i18n/strings'
  import { isMomentLive } from '$lib/glance'
  import { relativeTime } from '$lib/util/time'

  // Local-only modal state: a row tap opens MomentModal on top of the
  // sheet so the user can review the cluster and switch between
  // detections inside it.
  let modalMoment = $state<GlanceMoment | null>(null)

  // One-minute tick so "5 min ago" doesn't drift while the surface is open.
  let now = $state(new Date())
  $effect(() => {
    if (!glanceStore.peekOpen) return
    const t = setInterval(() => {
      now = new Date()
    }, 60_000)
    return () => clearInterval(t)
  })

  function camName(camId: string): string {
    return camerasStore.cameras.find((c) => c.id === camId)?.name ?? camId
  }

  function kindsLine(moment: GlanceMoment): string {
    const parts = moment.kinds.map((k) => eventKindLabels[k] ?? k)
    return parts.length > 0 ? parts.join(' · ') : ui.eventsEmpty
  }

  function countChipText(moment: GlanceMoment): string {
    const tpl = moment.event_count === 1 ? ui.glanceEventOne : ui.glanceEventMany
    return tpl.replace('{n}', String(moment.event_count))
  }

  function newestTime(moment: GlanceMoment): string {
    const newest = moment.events[0]?.started_at ?? moment.started_at
    return relativeTime(newest, now)
  }

  function onRowClick(moment: GlanceMoment) {
    void glanceStore.markOneSeen(moment.representative_event_id)
    modalMoment = moment
  }

  function openLiveFor(moment: GlanceMoment | null): (() => void) | undefined {
    if (!moment) return undefined
    if (!isMomentLive(moment)) return undefined
    const cam = camerasStore.cameras.find((c) => c.id === moment.cam_id)
    if (!cam?.online) return undefined
    return () => {
      modalMoment = null
      glanceStore.closePeek()
      goto(`/cam/${encodeURIComponent(moment.cam_id)}`)
    }
  }

  function isDimmed(moment: GlanceMoment): boolean {
    return moment.seen
  }

  function onViewAll() {
    glanceStore.closePeek()
    goto('/events')
  }

  // Responsive surface: mobile is the BottomSheet, desktop is the calm
  // .dk-side right-docked panel. Same content rendered through {#snippet body()}.
  let width = $state(0)
  const isDesktop = $derived(width >= 900)

  // Honour reduced-motion: the slide-in/scrim animations collapse to 0ms.
  const reducedMotion = $derived(
    typeof window !== 'undefined' && window.matchMedia('(prefers-reduced-motion: reduce)').matches
  )
  const panelMs = $derived(reducedMotion ? 0 : 320)
  const scrimMs = $derived(reducedMotion ? 0 : 260)

  // Body scroll lock + Escape while the desktop panel is open.
  $effect(() => {
    if (!isDesktop || !glanceStore.peekOpen) return
    const prev = document.documentElement.style.overflow
    document.documentElement.style.overflow = 'hidden'
    function onKey(e: KeyboardEvent) {
      if (e.key === 'Escape') {
        e.preventDefault()
        glanceStore.closePeek()
      }
    }
    window.addEventListener('keydown', onKey)
    return () => {
      document.documentElement.style.overflow = prev
      window.removeEventListener('keydown', onKey)
    }
  })

  // Custom horizontal slide for the desktop panel — mirrors the prototype
  // .dk-side dkSideIn/dkSideOut keyframes.
  function slideInRight(_node: Element, { duration = 320 }: { duration?: number } = {}) {
    return {
      duration,
      easing: cubicOut,
      css: (t: number) => `transform: translateX(${(1 - t) * 100}%)`
    }
  }

  const windowChip = $derived(
    ui.glanceWindowChip.replace('{hours}', String(prefsStore.glanceWindowHours))
  )
</script>

<svelte:window bind:innerWidth={width} />

{#snippet body()}
  <div class="gp-header">
    <span class="gp-window-chip">
      <Mono size={10} color="var(--text-3)" letterSpacing={0.3} uppercase>{windowChip}</Mono>
    </span>
    {#if glanceStore.moments.length > 0}
      <button
        type="button"
        class="gp-clear"
        onclick={() => void glanceStore.markAllSeen()}
        disabled={glanceStore.unseenCount === 0}
      >
        {ui.glanceMarkAllSeen}
      </button>
    {/if}
  </div>

  {#if glanceStore.moments.length === 0}
    <div class="gp-empty">{ui.glancePeekEmpty}</div>
    <div class="gp-footer">
      <button type="button" class="gp-view-all" onclick={onViewAll}>{ui.glanceViewAll}</button>
    </div>
  {:else}
    <ul class="gp-list" role="list">
      {#each glanceStore.moments as m (m.cam_id + m.started_at)}
        <li>
          <button
            type="button"
            class="gp-row"
            class:seen={isDimmed(m)}
            onclick={() => onRowClick(m)}
          >
            <span class="dotcol" aria-hidden="true">
              {#if !m.seen}<span class="unseen"></span>{/if}
            </span>
            <div class="gp-thumb">
              <img
                src={eventThumbnailURL(m.representative_event_id)}
                alt={camName(m.cam_id)}
                loading="lazy"
                width="112"
                height="63"
              />
            </div>
            <div class="gp-body">
              <div class="gp-l1">
                <span class="gp-cam">{camName(m.cam_id)}</span>
                {#if m.event_count >= 1}
                  <span class="gp-count">
                    <Mono size={10} color="var(--accent)" weight={600} letterSpacing={0.3}>
                      {countChipText(m)}
                    </Mono>
                  </span>
                {/if}
              </div>
              <div class="gp-l2">
                <span class="gp-kinds">{kindsLine(m)}</span>
                <Mono size={11} color="var(--text-3)">{newestTime(m)}</Mono>
              </div>
            </div>
            <span class="gp-chev" aria-hidden="true"><Icon name="chevDown" size={20} /></span>
          </button>
        </li>
      {/each}
    </ul>
    <div class="gp-footer">
      <button type="button" class="gp-view-all" onclick={onViewAll}>{ui.glanceViewAll}</button>
    </div>
  {/if}
{/snippet}

{#if !isDesktop}
  <BottomSheet
    open={glanceStore.peekOpen}
    onClose={() => glanceStore.closePeek()}
    title={ui.glancePeekTitle}
  >
    {@render body()}
  </BottomSheet>
{:else if glanceStore.peekOpen}
  <div
    class="dk-scrim"
    role="presentation"
    onclick={() => glanceStore.closePeek()}
    onkeydown={() => {}}
    aria-hidden="true"
    transition:fade={{ duration: scrimMs }}
  ></div>
  <div
    class="dk-side"
    role="dialog"
    aria-modal="true"
    aria-label={ui.glancePeekTitle}
    transition:slideInRight={{ duration: panelMs }}
  >
    <div class="dk-side-head">
      <div class="dk-side-titlebar">
        <div>
          <div class="dk-side-title">{ui.glancePeekTitle}</div>
          <div class="dk-side-cap">{ui.glanceWindowLabel}</div>
        </div>
        <button
          type="button"
          class="dk-close"
          aria-label={ui.close}
          onclick={() => glanceStore.closePeek()}
        >
          <span aria-hidden="true">×</span>
        </button>
      </div>
    </div>
    {@render body()}
  </div>
{/if}

{#if modalMoment}
  <MomentModal
    moment={modalMoment}
    onClose={() => {
      modalMoment = null
    }}
    onOpenLive={openLiveFor(modalMoment)}
  />
{/if}

<style>
  /* ============================================================
     DESKTOP — right-docked panel (calm .dk-side / .dk-scrim)
     ============================================================ */
  .dk-scrim {
    position: fixed;
    inset: 0;
    z-index: 40;
    background: var(--scrim);
  }
  .dk-side {
    position: fixed;
    top: 0;
    right: 0;
    bottom: 0;
    z-index: 41;
    width: 408px;
    max-width: 92vw;
    background: var(--surface);
    border-left: 1px solid var(--border-strong);
    box-shadow: -24px 0 60px -30px rgba(0, 0, 0, 0.6);
    display: flex;
    flex-direction: column;
    padding-top: env(safe-area-inset-top, 0px);
    padding-bottom: env(safe-area-inset-bottom, 0px);
  }
  .dk-side-head {
    flex: 0 0 auto;
    padding: 22px 22px 16px;
    border-bottom: 1px solid var(--border);
  }
  .dk-side-titlebar {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 14px;
  }
  .dk-side-title {
    font-size: 21px;
    font-weight: 600;
    letter-spacing: -0.3px;
    line-height: 1.1;
    color: var(--text);
  }
  .dk-side-cap {
    font-size: 13px;
    color: var(--text-2);
    margin-top: 4px;
  }
  .dk-close {
    width: 40px;
    height: 40px;
    flex: 0 0 auto;
    margin-top: -2px;
    display: grid;
    place-items: center;
    border-radius: 50%;
    background: var(--surface-2);
    border: none;
    color: var(--text);
    cursor: pointer;
    font-family: inherit;
    font-size: 22px;
    line-height: 1;
    transition: background 0.15s ease;
  }
  .dk-close:hover {
    background: var(--border-strong);
  }
  .dk-close:active {
    transform: scale(0.92);
  }

  /* ============================================================
     SHARED content — header chip + Mark all + rows + footer
     ============================================================ */
  .gp-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 12px;
    padding: 12px 20px;
    flex: 0 0 auto;
  }
  .gp-window-chip {
    display: inline-flex;
    align-items: center;
    padding: 4px 8px;
    border-radius: 999px;
    background: var(--surface-2);
    border: 1px solid var(--border);
  }
  .gp-clear {
    background: transparent;
    border: none;
    padding: 4px 6px;
    color: var(--accent);
    font-family: inherit;
    font-size: 13px;
    font-weight: 500;
    cursor: pointer;
    border-radius: 4px;
    transition:
      color 120ms,
      background 120ms;
  }
  .gp-clear:hover:not(:disabled) {
    background: color-mix(in oklab, var(--accent) 12%, transparent);
  }
  .gp-clear:disabled {
    color: var(--text-3);
    cursor: not-allowed;
  }
  .gp-empty {
    text-align: center;
    color: var(--text-2);
    font-size: 14px;
    padding: 44px 0;
  }
  .gp-footer {
    display: flex;
    justify-content: center;
    padding: 12px 20px 6px;
    flex: 0 0 auto;
  }
  .gp-view-all {
    background: transparent;
    border: none;
    padding: 6px 10px;
    color: var(--accent);
    font-family: inherit;
    font-size: 13px;
    font-weight: 500;
    cursor: pointer;
    border-radius: 6px;
    transition:
      color 120ms,
      background 120ms;
  }
  .gp-view-all:hover {
    background: color-mix(in oklab, var(--accent) 12%, transparent);
  }
  .gp-list {
    list-style: none;
    padding: 4px 14px 22px;
    margin: 0;
    display: flex;
    flex-direction: column;
    flex: 1 1 auto;
    overflow-y: auto;
  }
  .gp-list::-webkit-scrollbar {
    width: 0;
  }
  .gp-row {
    display: flex;
    align-items: center;
    gap: 13px;
    width: 100%;
    padding: 12px 8px;
    background: transparent;
    border: none;
    border-radius: 12px;
    color: inherit;
    text-align: left;
    cursor: pointer;
    font-family: inherit;
    transition: background 0.14s ease;
  }
  .gp-row:hover {
    background: var(--surface-2);
  }
  .gp-row:active {
    opacity: 0.65;
  }
  .gp-row.seen .gp-thumb,
  .gp-row.seen .gp-body {
    opacity: 0.56;
  }
  .dotcol {
    flex: 0 0 8px;
    width: 8px;
    display: flex;
    align-items: center;
    justify-content: center;
  }
  .unseen {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: var(--accent);
  }
  .gp-thumb {
    width: 88px;
    height: 56px;
    flex: 0 0 88px;
    border-radius: var(--r-sm);
    overflow: hidden;
    background: var(--feed);
  }
  .gp-thumb img {
    width: 100%;
    height: 100%;
    /* Still image: cover (NEVER fill). */
    object-fit: cover;
    display: block;
  }
  .gp-body {
    flex: 1 1 auto;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 3px;
  }
  .gp-l1 {
    display: flex;
    align-items: center;
    gap: 9px;
    min-width: 0;
  }
  .gp-l2 {
    display: flex;
    justify-content: space-between;
    align-items: baseline;
    gap: 10px;
    min-width: 0;
    font-size: 12.5px;
    color: var(--text-2);
  }
  .gp-cam {
    font-size: 16px;
    font-weight: 600;
    color: var(--text);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    flex: 0 1 auto;
    min-width: 0;
  }
  .gp-count {
    display: inline-flex;
    align-items: center;
    padding: 2px 7px;
    border-radius: 999px;
    background: var(--accent-soft);
    text-transform: uppercase;
    flex-shrink: 0;
  }
  .gp-kinds {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .gp-chev {
    flex: 0 0 auto;
    color: var(--text-3);
    align-self: center;
    display: inline-flex;
  }
</style>
