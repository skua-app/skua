<script lang="ts">
  import { fade } from 'svelte/transition'
  import { cubicOut } from 'svelte/easing'
  import { goto } from '$app/navigation'
  import type { GlanceMoment } from '$lib/api'
  import { eventThumbnailURL } from '$lib/api'
  import Icon from '$lib/components/Icon.svelte'
  import MomentModal from '$lib/components/MomentModal.svelte'
  import { glanceStore } from '$lib/stores/glance.svelte'
  import { camerasStore } from '$lib/stores/cameras.svelte'
  import { prefsStore } from '$lib/stores/prefs.svelte'
  import { eventKindLabels, ui } from '$lib/i18n/strings'
  import { isMomentLive } from '$lib/glance'
  import { relativeTime } from '$lib/util/time'

  // Local moment-filter state — prototype `MSTATE.filter`.
  let filter = $state<'all' | 'unseen'>('all')

  // Sliding-thumb refs for the All/Unseen segmented switch. The subBar
  // snippet renders in exactly one branch at a time (mobile sheet OR desktop
  // panel), so component-scoped refs target the live instance.
  let mfEl: HTMLDivElement | undefined = $state()
  let mfThumb: HTMLSpanElement | undefined = $state()
  let mfButtons: HTMLButtonElement[] = $state([])
  let mfMeasured = $state(false)

  function placeMfThumb() {
    if (!mfEl || !mfThumb) return
    const idx = filter === 'all' ? 0 : 1
    const btn = mfButtons[idx]
    if (!btn || btn.offsetWidth === 0) return
    if (!mfMeasured) {
      mfThumb.style.transition = 'none'
      mfThumb.style.transform = `translate(${btn.offsetLeft}px, ${btn.offsetTop}px)`
      mfThumb.style.width = `${btn.offsetWidth}px`
      mfThumb.style.height = `${btn.offsetHeight}px`
      void mfThumb.offsetWidth
      mfThumb.style.transition = ''
      mfMeasured = true
    } else {
      mfThumb.style.transform = `translate(${btn.offsetLeft}px, ${btn.offsetTop}px)`
      mfThumb.style.width = `${btn.offsetWidth}px`
      mfThumb.style.height = `${btn.offsetHeight}px`
    }
  }
  $effect(() => {
    void filter
    void mfButtons.length
    void glanceStore.unseenCount
    placeMfThumb()
  })
  $effect(() => {
    if (!mfEl) return
    const ro = new ResizeObserver(() => placeMfThumb())
    ro.observe(mfEl)
    return () => ro.disconnect()
  })

  // Local-only modal: a row tap opens MomentModal on top of the surface so
  // the user can review the cluster and switch between detections inside it.
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

  // Filtered moment view per the prototype: All shows every moment (seen or
  // not — dimmed when seen); Unseen drops already-seen rows entirely.
  const visibleMoments = $derived(
    filter === 'unseen' ? glanceStore.moments.filter((m) => !m.seen) : glanceStore.moments
  )
  const caption = $derived(
    ui.glanceWindowCaption.replace('{hours}', String(prefsStore.glanceWindowHours))
  )

  // Responsive surface: mobile = detented bottom sheet; desktop = right panel.
  let width = $state(0)
  const isDesktop = $derived(width >= 900)

  // Reduced motion: collapse durations to 0ms and skip transitions imperatively.
  const reducedMotion =
    typeof window !== 'undefined' && window.matchMedia('(prefers-reduced-motion: reduce)').matches

  /* ===================== DESKTOP PANEL ===================== */
  const panelMs = reducedMotion ? 0 : 320
  const scrimMs = reducedMotion ? 0 : 260

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

  function slideInRight(_node: Element, { duration = 320 }: { duration?: number } = {}) {
    return {
      duration,
      easing: cubicOut,
      css: (t: number) => `transform: translateX(${(1 - t) * 100}%)`
    }
  }

  /* ===================== MOBILE DETENTED SHEET ===================== */
  // Imperative iOS-style detent controller. Mirrors calm/app.js: detents are
  // translateY offsets (0 = full, height - 0.56*screenH = half, height - 280
  // = peek). Open animates from translateY(100%) to the peek detent; dragging
  // the head snaps to the nearest detent; dragging below peek + 70 dismisses.
  let mobileMounted = $state(false)
  let sheetEl: HTMLDivElement | undefined = $state()
  let scrimEl: HTMLDivElement | undefined = $state()
  let detents = $state<[number, number, number]>([0, 0, 0])
  let detentIdx = $state(2)
  let sheetHeight = $state(800)
  let dragging = false
  let dragStartY = 0
  let dragStartT = 0
  let curT = 0

  function computeDetents() {
    const screenH = window.visualViewport?.height ?? window.innerHeight
    const full = Math.round(screenH * 0.92)
    const peekVisible = 280
    const halfVisible = Math.round(screenH * 0.56)
    sheetHeight = full
    detents = [0, full - halfVisible, full - peekVisible]
  }
  function setScrimForTranslate(t: number) {
    if (!scrimEl) return
    const frac = 1 - t / sheetHeight
    scrimEl.style.opacity = (0.1 + frac * 0.28).toFixed(3)
  }
  function applyDetent(i: number, animate: boolean) {
    if (!sheetEl) return
    detentIdx = Math.max(0, Math.min(detents.length - 1, i))
    const t = detents[detentIdx]!
    sheetEl.style.transition = animate && !reducedMotion ? '' : 'none'
    sheetEl.style.transform = `translateY(${t}px)`
    sheetEl.dataset.detent = ['full', 'half', 'peek'][detentIdx]!
    setScrimForTranslate(t)
  }

  // Mount/unmount the mobile sheet in response to glanceStore.peekOpen.
  $effect(() => {
    if (isDesktop) return
    if (glanceStore.peekOpen && !mobileMounted) {
      mobileMounted = true
    } else if (!glanceStore.peekOpen && mobileMounted) {
      animateClose()
    }
  })

  // Initial pre-paint position + scheduled snap-in (per the prototype's
  // setTimeout(20) pattern, which lets the first paint commit the off-screen
  // transform before we transition to the peek detent).
  $effect(() => {
    if (!mobileMounted || !sheetEl) return
    const sheet = sheetEl
    const scrim = scrimEl
    computeDetents()
    sheet.style.height = sheetHeight + 'px'
    sheet.style.transition = 'none'
    sheet.style.transform = 'translateY(100%)'
    if (scrim) scrim.style.opacity = '0'
    // Force reflow so the next style change can transition.
    void sheet.getBoundingClientRect()
    const id = setTimeout(() => {
      detentIdx = 2 // open to PEEK
      applyDetent(2, true)
    }, 20)
    return () => clearTimeout(id)
  })

  function animateClose() {
    if (!sheetEl) {
      mobileMounted = false
      return
    }
    sheetEl.style.transition = reducedMotion ? 'none' : 'transform .32s cubic-bezier(.4, 0, 1, 1)'
    sheetEl.style.transform = `translateY(${sheetHeight}px)`
    if (scrimEl) {
      scrimEl.style.transition = reducedMotion ? 'none' : 'opacity .26s ease'
      scrimEl.style.opacity = '0'
    }
    setTimeout(
      () => {
        mobileMounted = false
      },
      reducedMotion ? 0 : 320
    )
  }

  // Body scroll lock + Escape while the mobile sheet is mounted.
  $effect(() => {
    if (!mobileMounted) return
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

  function onHeadPointerDown(e: PointerEvent) {
    if (!sheetEl) return
    // Let the close button's own click pass through without starting a drag.
    if ((e.target as HTMLElement | null)?.closest('.sm-close')) return
    if (e.pointerType === 'mouse' && e.button !== 0) return
    const head = e.currentTarget as HTMLElement
    head.setPointerCapture(e.pointerId)
    dragging = true
    dragStartY = e.clientY
    dragStartT = detents[detentIdx]!
    curT = dragStartT
    sheetEl.style.transition = 'none'
    e.preventDefault()
  }
  function onHeadPointerMove(e: PointerEvent) {
    if (!dragging || !sheetEl) return
    const dy = e.clientY - dragStartY
    const peekT = detents[detents.length - 1]!
    const max = peekT + 140
    curT = Math.max(0, Math.min(max, dragStartT + dy))
    sheetEl.style.transform = `translateY(${curT}px)`
    setScrimForTranslate(curT)
  }
  function onHeadPointerUp(e: PointerEvent) {
    if (!dragging) return
    dragging = false
    const head = e.currentTarget as HTMLElement
    if (head.hasPointerCapture(e.pointerId)) head.releasePointerCapture(e.pointerId)
    const peekT = detents[detents.length - 1]!
    if (curT > peekT + 70) {
      glanceStore.closePeek()
      return
    }
    // Snap to nearest detent.
    let best = 0
    let bd = Infinity
    detents.forEach((t, i) => {
      const d = Math.abs(t - curT)
      if (d < bd) {
        bd = d
        best = i
      }
    })
    applyDetent(best, true)
  }
  function onHeadPointerCancel() {
    if (!dragging) return
    dragging = false
    applyDetent(detentIdx, true)
  }
</script>

<svelte:window bind:innerWidth={width} />

{#snippet subBar()}
  <div class="moment-filter" role="group" aria-label={ui.glanceTitle} bind:this={mfEl}>
    <span class="seg-thumb" class:visible={mfMeasured} bind:this={mfThumb}></span>
    <button
      type="button"
      class="mf-seg"
      class:on={filter === 'all'}
      bind:this={mfButtons[0]}
      onclick={() => (filter = 'all')}
    >
      {ui.glanceFilterAll}
    </button>
    <button
      type="button"
      class="mf-seg"
      class:on={filter === 'unseen'}
      bind:this={mfButtons[1]}
      onclick={() => (filter = 'unseen')}
    >
      {ui.glanceFilterUnseen}
      {#if glanceStore.unseenCount > 0}
        <span class="mf-num">{glanceStore.unseenCount}</span>
      {/if}
    </button>
  </div>
  <button
    type="button"
    class="mark-seen"
    disabled={glanceStore.unseenCount === 0}
    onclick={() => void glanceStore.markAllSeen()}
  >
    {ui.glanceMarkAllSeen}
  </button>
{/snippet}

{#snippet rows()}
  {#if visibleMoments.length === 0}
    <div class="sheet-empty">{ui.glanceAllCaughtUp}</div>
  {:else}
    {#each visibleMoments as m (m.cam_id + m.started_at)}
      <button type="button" class="moment" class:seen={m.seen} onclick={() => onRowClick(m)}>
        <span class="m-dotcol" aria-hidden="true">
          {#if !m.seen}<span class="m-unseen"></span>{/if}
        </span>
        <div class="m-thumb">
          <img
            src={eventThumbnailURL(m.representative_event_id)}
            alt={camName(m.cam_id)}
            loading="lazy"
            width="112"
            height="63"
          />
        </div>
        <div class="m-meta">
          <div class="m-l1">
            <span class="m-name">{camName(m.cam_id)}</span>
            <span class="m-ago">{newestTime(m)}</span>
          </div>
          <div class="m-l2">
            <span class="m-count">{countChipText(m)}</span>
            <span class="m-sep"> · {kindsLine(m)}</span>
          </div>
        </div>
        <span class="m-chev" aria-hidden="true"><Icon name="chevRight" size={20} /></span>
      </button>
    {/each}
  {/if}
{/snippet}

{#if isDesktop && glanceStore.peekOpen}
  <div
    class="dk-scrim"
    role="presentation"
    aria-hidden="true"
    onclick={() => glanceStore.closePeek()}
    transition:fade={{ duration: scrimMs }}
  ></div>
  <div
    class="dk-side"
    role="dialog"
    aria-modal="true"
    aria-label={ui.glanceTitle}
    transition:slideInRight={{ duration: panelMs }}
  >
    <div class="dk-side-head">
      <div class="dk-side-titlebar">
        <div class="dk-side-title-block">
          <div class="dk-side-title">{ui.glanceTitle}</div>
          <div class="dk-side-cap">{caption}</div>
        </div>
        <button
          type="button"
          class="dk-mclose"
          aria-label={ui.close}
          onclick={() => glanceStore.closePeek()}
        >
          <Icon name="close" size={18} />
        </button>
      </div>
      <div class="dk-side-sub">
        {@render subBar()}
      </div>
    </div>
    <div class="dk-side-list">
      {@render rows()}
    </div>
  </div>
{:else if mobileMounted}
  <div
    class="scrim"
    role="presentation"
    aria-hidden="true"
    bind:this={scrimEl}
    onclick={() => glanceStore.closePeek()}
  ></div>
  <div
    class="sheet"
    role="dialog"
    aria-modal="true"
    aria-label={ui.glanceTitle}
    bind:this={sheetEl}
  >
    <div
      class="sheet-head"
      role="presentation"
      onpointerdown={onHeadPointerDown}
      onpointermove={onHeadPointerMove}
      onpointerup={onHeadPointerUp}
      onpointercancel={onHeadPointerCancel}
    >
      <div class="grab" aria-hidden="true"></div>
      <div class="sheet-titlebar">
        <div class="sheet-title-block">
          <span class="sheet-title">{ui.glanceTitle}</span>
          <span class="sheet-caption">{caption}</span>
        </div>
        <button
          type="button"
          class="sm-close"
          aria-label={ui.close}
          onclick={() => glanceStore.closePeek()}
        >
          <Icon name="close" size={18} />
        </button>
      </div>
    </div>
    <div class="sheet-sub">
      {@render subBar()}
    </div>
    <div class="sheet-list">
      {@render rows()}
    </div>
  </div>
{/if}

{#if modalMoment}
  {#key modalMoment}
    <MomentModal
      moment={modalMoment}
      onClose={() => {
        modalMoment = null
      }}
      onOpenLive={openLiveFor(modalMoment)}
    />
  {/key}
{/if}

<style>
  /* ============================================================
     DESKTOP — calm .dk-side / .dk-scrim
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
  .dk-side-title-block {
    display: flex;
    flex-direction: column;
    gap: 4px;
    min-width: 0;
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
  }
  .dk-mclose {
    width: 38px;
    height: 38px;
    flex: 0 0 auto;
    margin-top: -2px;
    display: grid;
    place-items: center;
    border-radius: 10px;
    background: var(--surface-2);
    border: none;
    color: var(--text);
    cursor: pointer;
    font-family: inherit;
    transition: background 0.15s ease;
  }
  .dk-mclose:hover {
    background: var(--border-strong);
  }
  .dk-side-sub {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
    margin-top: 16px;
  }
  .dk-side-list {
    flex: 1 1 auto;
    overflow-y: auto;
    padding: 8px 14px 22px;
  }

  /* ============================================================
     MOBILE — calm .scrim / .sheet (iOS-style detents)
     ============================================================ */
  .scrim {
    position: fixed;
    inset: 0;
    z-index: 100;
    background: var(--scrim);
    opacity: 0;
    transition: opacity 0.3s ease;
  }
  .sheet {
    position: fixed;
    left: 0;
    right: 0;
    bottom: 0;
    z-index: 101;
    background: var(--surface);
    border-top: 1px solid var(--border-strong);
    border-top-left-radius: 28px;
    border-top-right-radius: 28px;
    box-shadow: 0 -16px 40px rgba(0, 0, 0, 0.4);
    display: flex;
    flex-direction: column;
    transform: translateY(100%);
    transition: transform 0.4s cubic-bezier(0.32, 0.72, 0, 1);
    overflow: hidden;
    will-change: transform;
  }
  .sheet-head {
    flex: 0 0 auto;
    padding: 11px 18px 14px;
    cursor: grab;
    touch-action: none;
    user-select: none;
  }
  .sheet-head:active {
    cursor: grabbing;
  }
  .grab {
    width: 40px;
    height: 5px;
    border-radius: 3px;
    background: var(--border-strong);
    margin: 0 auto 16px;
  }
  .sheet-titlebar {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 14px;
  }
  .sheet-title-block {
    display: flex;
    flex-direction: column;
    gap: 3px;
    min-width: 0;
  }
  .sheet-title {
    font-size: 22px;
    font-weight: 600;
    letter-spacing: -0.3px;
    line-height: 1.1;
    color: var(--text);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .sheet-caption {
    font-size: 13px;
    color: var(--text-2);
    white-space: nowrap;
  }
  .sm-close {
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
    transition: background 0.15s ease;
  }
  .sm-close:active {
    transform: scale(0.92);
    background: var(--border-strong);
  }
  .sheet-sub {
    flex: 0 0 auto;
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
    padding: 0 18px 13px;
    border-bottom: 1px solid var(--border);
    white-space: nowrap;
  }
  .sheet-list {
    flex: 1 1 auto;
    overflow-y: auto;
    overflow-x: hidden;
    scrollbar-width: none;
    padding: 4px 20px 20px;
  }
  .sheet-list::-webkit-scrollbar {
    width: 0;
  }

  /* ============================================================
     SHARED — sub-bar, rows, empty
     ============================================================ */
  .moment-filter {
    position: relative;
    display: inline-flex;
    background: var(--surface-2);
    border: 1px solid var(--border);
    border-radius: 999px;
    padding: 3px;
    gap: 2px;
  }
  .seg-thumb {
    position: absolute;
    left: 0;
    top: 0;
    z-index: 0;
    border-radius: 999px;
    background: var(--accent-soft);
    opacity: 0;
    pointer-events: none;
    transition:
      transform 0.3s cubic-bezier(0.4, 0, 0.2, 1),
      width 0.3s cubic-bezier(0.4, 0, 0.2, 1);
  }
  .seg-thumb.visible {
    opacity: 1;
  }
  @media (prefers-reduced-motion: reduce) {
    .seg-thumb {
      transition: none;
    }
  }
  .mf-seg {
    position: relative;
    z-index: 1;
    -webkit-appearance: none;
    appearance: none;
    background: transparent;
    border: none;
    color: var(--text-2);
    font-family: inherit;
    font-size: 13.5px;
    font-weight: 500;
    padding: 7px 15px;
    border-radius: 999px;
    display: inline-flex;
    align-items: center;
    gap: 6px;
    cursor: pointer;
    transition: color 0.18s ease;
  }
  .mf-seg.on {
    color: var(--accent-ink);
  }
  .mf-seg:active {
    transform: translateY(1px);
  }
  .mf-num {
    font-family: 'JetBrains Mono', ui-monospace, monospace;
    font-size: 11px;
    font-weight: 600;
    line-height: 1;
    padding: 3px 6px;
    border-radius: 999px;
    background: var(--surface);
    color: var(--text-3);
  }
  .mf-seg.on .mf-num {
    background: var(--accent-soft);
    color: var(--accent-ink);
  }
  .mark-seen {
    -webkit-appearance: none;
    appearance: none;
    font-family: inherit;
    background: transparent;
    border: none;
    color: var(--accent-ink);
    font-size: 14px;
    font-weight: 500;
    padding: 8px 4px;
    min-height: 38px;
    white-space: nowrap;
    cursor: pointer;
  }
  .mark-seen:hover:not(:disabled) {
    text-decoration: underline;
  }
  .mark-seen:active:not(:disabled) {
    opacity: 0.55;
  }
  .mark-seen:disabled {
    color: var(--text-3);
    cursor: not-allowed;
  }

  .sheet-empty {
    text-align: center;
    color: var(--text-2);
    font-size: 14px;
    padding: 44px 0;
  }

  .moment {
    display: flex;
    align-items: center;
    gap: 12px;
    width: 100%;
    padding: 13px 0;
    background: transparent;
    border: none;
    border-bottom: 1px solid var(--border);
    border-radius: 0;
    color: inherit;
    text-align: left;
    cursor: pointer;
    font-family: inherit;
    transition: background 0.14s ease;
  }
  .dk-side-list .moment {
    padding: 12px 8px;
    border-bottom: none;
    border-radius: 12px;
  }
  .dk-side-list .moment:hover {
    background: var(--surface-2);
  }
  .moment:active {
    opacity: 0.65;
  }
  .moment.seen .m-thumb,
  .moment.seen .m-meta {
    opacity: 0.6;
  }
  .m-dotcol {
    flex: 0 0 8px;
    width: 8px;
    display: flex;
    align-items: center;
    justify-content: center;
  }
  .m-unseen {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: var(--accent);
  }
  .m-thumb {
    width: 84px;
    height: 58px;
    flex: 0 0 84px;
    border-radius: var(--r-sm);
    overflow: hidden;
    background: var(--feed);
  }
  .dk-side-list .m-thumb {
    width: 88px;
    height: 56px;
    flex: 0 0 88px;
  }
  .m-thumb img {
    width: 100%;
    height: 100%;
    /* Still thumbnail: cover (NEVER fill). */
    object-fit: cover;
    display: block;
  }
  .m-meta {
    flex: 1 1 auto;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 3px;
  }
  .m-l1 {
    display: flex;
    align-items: baseline;
    gap: 9px;
  }
  .m-name {
    font-size: 17px;
    font-weight: 600;
    color: var(--text);
    line-height: 1.2;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    flex: 0 1 auto;
    min-width: 0;
  }
  .dk-side-list .m-name {
    font-size: 16px;
  }
  .m-ago {
    font-size: 13px;
    color: var(--text-2);
    line-height: 1.2;
    white-space: nowrap;
    flex: 0 0 auto;
  }
  .dk-side-list .m-ago {
    font-size: 12.5px;
  }
  .m-l2 {
    font-size: 13px;
    color: var(--text-2);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    min-width: 0;
  }
  .dk-side-list .m-l2 {
    font-size: 12.5px;
  }
  .m-count {
    color: var(--text-2);
    font-weight: 500;
  }
  .m-chev {
    flex: 0 0 auto;
    color: var(--text-3);
    align-self: center;
    display: inline-flex;
  }
</style>
