<script lang="ts">
  import { tick, untrack } from 'svelte'
  import { fade, fly, slide } from 'svelte/transition'
  import { cubicOut } from 'svelte/easing'
  import type { EventItem, GlanceMoment } from '$lib/api'
  import { eventSnapshotURL, eventClipURL } from '$lib/api'
  import { camerasStore } from '$lib/stores/cameras.svelte'
  import { configStore } from '$lib/stores/config.svelte'
  import { eventKindLabels, ui } from '$lib/i18n/strings'
  import { formatDuration, eventTimestamp } from '$lib/util/time'
  import Mono from '$lib/components/Mono.svelte'
  import ZoomPane from '$lib/components/ZoomPane.svelte'

  type Props = {
    moment: GlanceMoment
    onClose: () => void
    onOpenLive?: () => void
  }

  let { moment, onClose, onOpenLive }: Props = $props()

  // Honour reduced-motion: collapse the fly/fade open animation to 0ms.
  const reducedMotion =
    typeof window !== 'undefined' && window.matchMedia('(prefers-reduced-motion: reduce)').matches
  const cardDuration = reducedMotion ? 0 : 260
  const scrimDuration = reducedMotion ? 0 : 200

  function pickInitial(m: GlanceMoment): EventItem {
    const rep = m.events.find((e) => e.id === m.representative_event_id)
    return rep ?? m.events[0]!
  }

  // Initial selection is captured once at modal construction; the
  // component is destroyed and recreated by the {#if modalMoment} guard
  // in GlancePeek when the user moves to another moment, so we don't
  // need this value to track moment reactively.
  let selectedEvent = $state<EventItem>(untrack(() => pickInitial(moment)))
  let clipFailed = $state(false)
  // Default the "Events (N)" section expanded so the cluster context is
  // immediately visible. A single-event moment has nothing to browse so
  // hide the section entirely (see {#if showEventsSection}).
  let eventsExpanded = $state(true)

  const showEventsSection = $derived(moment.events.length > 1)
  const camName = $derived(
    camerasStore.cameras.find((c) => c.id === selectedEvent.cam_id)?.name ?? selectedEvent.cam_id
  )
  const kindLabel = $derived(eventKindLabels[selectedEvent.kind] ?? selectedEvent.kind)
  const absoluteTime = $derived(
    new Intl.DateTimeFormat([], {
      dateStyle: 'medium',
      timeStyle: 'medium'
    }).format(new Date(selectedEvent.started_at))
  )
  const duration = $derived(formatDuration(selectedEvent.duration_seconds))
  const deepLink = $derived(
    configStore.frigateUIURL
      ? `${configStore.frigateUIURL}/explore?event_id=${encodeURIComponent(selectedEvent.id)}&camera=${encodeURIComponent(selectedEvent.cam_id)}`
      : ''
  )
  const titleText = $derived(`${ui.glanceMomentTitle} · ${camName}`)

  // Reset the clip-failed flag whenever the user switches detections —
  // same pattern as EventModal: don't carry one event's decode failure
  // over to the next.
  $effect(() => {
    const id = selectedEvent.id
    if (id) clipFailed = false
  })

  let closeBtn = $state<HTMLButtonElement | null>(null)
  let cardEl = $state<HTMLDivElement | null>(null)
  let videoEl = $state<HTMLVideoElement | null>(null)

  function focusableIn(root: HTMLElement): HTMLElement[] {
    const sel = 'button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])'
    return Array.from(root.querySelectorAll<HTMLElement>(sel)).filter(
      (el) => !el.hasAttribute('disabled') && el.offsetParent !== null
    )
  }

  function onKey(e: KeyboardEvent) {
    if (e.key === 'Escape') {
      onClose()
      return
    }
    if (e.key !== 'Tab' || !cardEl) return
    const items = focusableIn(cardEl)
    if (items.length === 0) {
      e.preventDefault()
      return
    }
    const first = items[0]!
    const last = items[items.length - 1]!
    const active = document.activeElement as HTMLElement | null
    if (e.shiftKey) {
      if (active === first || !cardEl.contains(active)) {
        e.preventDefault()
        last.focus()
      }
    } else {
      if (active === last || !cardEl.contains(active)) {
        e.preventDefault()
        first.focus()
      }
    }
  }

  $effect(() => {
    const prev = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    window.addEventListener('keydown', onKey)
    return () => {
      document.body.style.overflow = prev
      window.removeEventListener('keydown', onKey)
    }
  })

  $effect(() => {
    // preventScroll keeps the card from scrolling to the focused button
    // on open — belt-and-suspenders now that the card itself no longer
    // scrolls, but it also stops the focused element from being pushed
    // into view on any future layout change.
    closeBtn?.focus({ preventScroll: true })
  })

  function onBackdropClick(e: MouseEvent) {
    if (e.target === e.currentTarget) onClose()
  }

  async function selectEvent(ev: EventItem) {
    // Re-tapping the already-playing row must not restart playback.
    if (ev.id === selectedEvent.id) return
    selectedEvent = ev
    // The snapshot <img> renders for clipless events, so there's nothing
    // to drive — leave the existing clipFailed-reset effect to handle
    // the meta refresh.
    if (!ev.has_clip) return
    // After tick() the reactive src binding (and the clipFailed reset
    // effect) have applied, so videoEl points at the <video> with the
    // new source. iOS Safari does not restart the media pipeline on a
    // bare src swap; an explicit load() + play() is required. The
    // user-gesture activation from the tap survives the tick()
    // microtask, so play() is permitted. Any rejection (autoplay
    // policy, decoder error) is swallowed — the native controls remain
    // usable.
    await tick()
    if (!videoEl) return
    videoEl.load()
    void videoEl.play().catch(() => {})
  }

  // On initial open we scroll the active (representative) detection
  // into view inside the list so users can see which row is playing
  // even when it sits below the visible cluster window. Subsequent
  // taps on other detections must NOT trigger this — the user already
  // sees the row they tapped — so a one-shot guard locks the behaviour
  // to the first measurement.
  let eventsListEl = $state<HTMLUListElement | null>(null)
  let initialScrollDone = $state(false)

  function scrollSelectedIntoView() {
    if (initialScrollDone || !eventsListEl) return
    const active = eventsListEl.querySelector<HTMLElement>('.mm-events-row.active')
    if (!active) return
    // block:'center' centers the active row inside the nearest scroller
    // (.mm-events-list — the card is overflow:hidden and the backdrop is
    // position:fixed, so the page never moves). 'nearest' aligned to the
    // bottom edge and partly clipped the row; centering shows context
    // both above and below the playing detection.
    active.scrollIntoView({ block: 'center' })
    initialScrollDone = true
  }

  // Fallback for when transition:slide does not emit `introend` on the
  // initial mount (Svelte's local transitions are suppressed when an
  // {#if} block opens already true). After tick() + rAF the list's
  // geometry is laid out, so we can scroll the active row into view.
  // The introend handler on the <ul> covers the case where the user
  // collapses then re-expands the section before the initial scroll
  // ever runs; the initialScrollDone guard keeps both paths idempotent.
  $effect(() => {
    if (!showEventsSection || !eventsExpanded) return
    if (untrack(() => initialScrollDone)) return
    void tick().then(() => {
      requestAnimationFrame(() => {
        scrollSelectedIntoView()
      })
    })
  })
</script>

<!-- svelte-ignore a11y_click_events_have_key_events -->
<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="mm-backdrop" onclick={onBackdropClick} transition:fade={{ duration: scrimDuration }}>
  <div
    class="mm-card"
    role="dialog"
    aria-modal="true"
    aria-label={titleText}
    bind:this={cardEl}
    transition:fly={{ y: 16, duration: cardDuration, easing: cubicOut }}
  >
    <div class="mm-title">{titleText}</div>

    <div class="mm-snap">
      <ZoomPane resetKey={selectedEvent.id}>
        {#if selectedEvent.has_clip && !clipFailed}
          <!-- svelte-ignore a11y_media_has_caption -->
          <video
            src={eventClipURL(selectedEvent.id)}
            poster={eventSnapshotURL(selectedEvent.id)}
            controls
            playsinline
            preload="metadata"
            bind:this={videoEl}
            onerror={() => (clipFailed = true)}
          ></video>
        {:else}
          <img
            src={eventSnapshotURL(selectedEvent.id)}
            alt={`${camName} · ${kindLabel}`}
            loading="lazy"
          />
        {/if}
      </ZoomPane>
    </div>

    {#if selectedEvent.has_clip && clipFailed}
      <div class="mm-clip-fallback" role="status">
        <div class="mm-clip-fallback-heading">{ui.clipUnplayableHeading}</div>
        <div class="mm-clip-fallback-hint">{ui.clipUnplayableHint}</div>
      </div>
    {/if}

    <div class="mm-meta">
      <div class="mm-meta-row">
        <span class="mm-cam">{camName}</span>
        <Mono size={11} color="var(--text-3)">{selectedEvent.cam_id}</Mono>
      </div>
      <div class="mm-meta-row">
        <span class="mm-kind">{kindLabel} · {selectedEvent.label}</span>
        <Mono size={11} color="var(--text-2)" weight={500}>
          {selectedEvent.score !== null ? selectedEvent.score.toFixed(2) : '—'}
        </Mono>
      </div>
      <div class="mm-meta-row mm-meta-row-faint">
        <Mono size={11} color="var(--text-3)">{absoluteTime}</Mono>
        <Mono size={11} color="var(--text-3)">{duration}</Mono>
      </div>
    </div>

    {#if showEventsSection}
      <div class="mm-events">
        <button
          type="button"
          class="mm-events-toggle"
          aria-expanded={eventsExpanded}
          onclick={() => (eventsExpanded = !eventsExpanded)}
        >
          <span class="mm-events-toggle-text">
            {ui.glanceEventsSection} ({moment.event_count})
          </span>
          <span class="mm-events-toggle-chevron" class:open={eventsExpanded}>›</span>
        </button>
        {#if eventsExpanded}
          <ul
            class="mm-events-list"
            role="list"
            bind:this={eventsListEl}
            transition:slide={{ duration: 160 }}
            onintroend={scrollSelectedIntoView}
          >
            {#each moment.events as ev (ev.id)}
              <li>
                <button
                  type="button"
                  class="mm-events-row"
                  class:active={ev.id === selectedEvent.id}
                  onclick={() => selectEvent(ev)}
                >
                  <span class="mm-events-time">
                    <Mono size={11} color="var(--text-3)">{eventTimestamp(ev.started_at)}</Mono>
                  </span>
                  <span class="mm-events-kind">{eventKindLabels[ev.kind] ?? ev.kind}</span>
                  <Mono size={11} color="var(--text-2)" weight={500}>
                    {ev.score !== null ? ev.score.toFixed(2) : '—'}
                  </Mono>
                  <span class="mm-events-affordance" class:has-clip={ev.has_clip}>
                    {ev.has_clip ? 'clip' : 'snapshot'}
                  </span>
                </button>
              </li>
            {/each}
          </ul>
        {/if}
      </div>
    {/if}

    <div class="mm-actions">
      <button type="button" class="mm-btn mm-btn-secondary" onclick={onClose} bind:this={closeBtn}>
        {ui.close}
      </button>
      {#if selectedEvent.has_clip}
        <a class="mm-btn mm-btn-secondary" href={eventClipURL(selectedEvent.id, true)} download>
          {ui.downloadVideo}
        </a>
      {/if}
      {#if onOpenLive}
        <button type="button" class="mm-btn mm-btn-secondary" onclick={onOpenLive}>
          {ui.openLive}
        </button>
      {/if}
      {#if deepLink}
        <a class="mm-btn mm-btn-primary" href={deepLink} target="_blank" rel="noopener noreferrer">
          {ui.openInFrigate}
        </a>
      {/if}
    </div>
  </div>
</div>

<style>
  .mm-backdrop {
    position: fixed;
    inset: 0;
    background: var(--scrim);
    backdrop-filter: blur(6px);
    -webkit-backdrop-filter: blur(6px);
    display: flex;
    align-items: center;
    justify-content: center;
    /* iOS PWA: status-bar-style=black-translucent + viewport-fit=cover
       means the layout extends under the notch and home indicator.
       Pad by the larger of the 20px visual gap and the device inset
       per side so the card never collides with system chrome. Matches
       the .ah-mobile { padding-top: env(safe-area-inset-top) }
       convention in AppHeader. */
    padding-top: max(20px, env(safe-area-inset-top));
    padding-right: max(20px, env(safe-area-inset-right));
    padding-bottom: max(20px, env(safe-area-inset-bottom));
    padding-left: max(20px, env(safe-area-inset-left));
    z-index: 110;
  }
  .mm-card {
    width: min(560px, 100%);
    /* The backdrop is position:fixed inset:0 with safe-area-aware
       padding; bounding the card to 100% of that padded content box
       keeps it clear of both the notch and the home indicator without
       hard-coding any inset assumption. */
    max-height: 100%;
    /* Card itself is NOT the scroller — its only flexible child (the
       detection list) scrolls. This keeps the player, meta, and actions
       pinned even when the detection list overflows. */
    overflow: hidden;
    background: var(--surface);
    border: 1px solid var(--border-strong);
    border-radius: var(--r);
    box-shadow: var(--shadow);
    display: flex;
    flex-direction: column;
  }

  .mm-title {
    padding: 14px 18px 6px;
    font-size: 13px;
    font-weight: 600;
    color: var(--text-2);
    letter-spacing: -0.1px;
    flex-shrink: 0;
  }

  .mm-snap {
    aspect-ratio: 16 / 9;
    /* Cap on short viewports so a tall iPhone in landscape doesn't let
       the 16:9 box eat the entire modal — the detection list still
       needs room below. */
    max-height: 45dvh;
    background: var(--feed);
    overflow: hidden;
    flex-shrink: 0;
  }
  .mm-snap img,
  .mm-snap video {
    width: 100%;
    height: 100%;
    /* Review media: contain (NEVER fill). */
    object-fit: contain;
    display: block;
    background: var(--feed);
  }

  .mm-clip-fallback {
    padding: 12px 18px 0;
    display: flex;
    flex-direction: column;
    gap: 4px;
    flex-shrink: 0;
  }
  .mm-clip-fallback-heading {
    font-size: 13px;
    font-weight: 500;
    color: var(--text);
  }
  .mm-clip-fallback-hint {
    font-size: 12px;
    color: var(--text-2);
    line-height: 1.4;
  }

  .mm-meta {
    padding: 16px 18px 8px;
    display: flex;
    flex-direction: column;
    gap: 8px;
    border-bottom: 1px solid var(--border);
    flex-shrink: 0;
  }
  .mm-meta-row {
    display: flex;
    justify-content: space-between;
    align-items: baseline;
    gap: 10px;
  }
  .mm-meta-row-faint {
    margin-top: 2px;
  }
  .mm-cam {
    font-size: 15px;
    font-weight: 600;
    color: var(--text);
    letter-spacing: -0.1px;
  }
  .mm-kind {
    font-size: 13px;
    color: var(--text-2);
  }

  .mm-events {
    /* The events block is the only flexible column child: it takes
       whatever space remains and its list scrolls. min-height: 0 lets
       the flex item shrink below its content's intrinsic height so
       overflow:auto on the list actually engages. */
    flex: 0 1 auto;
    min-height: 0;
    display: flex;
    flex-direction: column;
    border-bottom: 1px solid var(--border);
  }
  .mm-events-toggle {
    display: flex;
    align-items: center;
    justify-content: space-between;
    width: 100%;
    padding: 12px 18px;
    background: transparent;
    border: none;
    color: var(--text-2);
    font-family: inherit;
    font-size: 13px;
    font-weight: 500;
    text-align: left;
    cursor: pointer;
    flex-shrink: 0;
  }
  .mm-events-toggle:hover {
    color: var(--text);
  }
  .mm-events-toggle-text {
    flex: 1;
  }
  .mm-events-toggle-chevron {
    display: inline-block;
    font-size: 14px;
    color: var(--text-3);
    transform: rotate(90deg);
    transition: transform 140ms;
  }
  .mm-events-toggle-chevron.open {
    transform: rotate(270deg);
  }
  .mm-events-list {
    list-style: none;
    padding: 0;
    margin: 0;
    display: flex;
    flex-direction: column;
    min-height: 0;
    overflow-y: auto;
  }
  .mm-events-row {
    display: grid;
    grid-template-columns: auto 1fr auto auto;
    align-items: center;
    gap: 12px;
    width: 100%;
    padding: 10px 18px;
    background: transparent;
    border: none;
    border-top: 1px solid var(--border);
    color: inherit;
    font-family: inherit;
    text-align: left;
    cursor: pointer;
    transition: background 120ms;
  }
  .mm-events-row:hover {
    background: var(--surface-2);
  }
  .mm-events-row.active {
    background: var(--accent-soft);
  }
  .mm-events-time {
    min-width: 60px;
  }
  .mm-events-kind {
    font-size: 12px;
    color: var(--text);
  }
  .mm-events-affordance {
    font-size: 10px;
    text-transform: uppercase;
    letter-spacing: 0.4px;
    color: var(--text-3);
    padding: 2px 6px;
    border: 1px solid var(--border);
    border-radius: 4px;
  }
  .mm-events-affordance.has-clip {
    color: var(--accent-ink);
    border-color: color-mix(in oklab, var(--accent) 40%, var(--border));
  }

  .mm-actions {
    padding: 14px 18px;
    display: flex;
    justify-content: flex-end;
    gap: 10px;
    flex-wrap: wrap;
    flex-shrink: 0;
    border-top: 1px solid var(--border);
  }
  .mm-btn {
    padding: 12px 18px;
    font-size: 14.5px;
    font-weight: 600;
    border-radius: 11px;
    border: 1px solid transparent;
    cursor: pointer;
    text-decoration: none;
    font-family: inherit;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    gap: 7px;
    transition:
      background 0.15s ease,
      color 0.15s ease,
      filter 0.15s ease;
  }
  .mm-btn:active {
    transform: translateY(1px);
  }
  .mm-btn-secondary {
    color: var(--text);
    background: var(--surface-2);
  }
  .mm-btn-secondary:hover {
    background: var(--border-strong);
  }
  .mm-btn-primary {
    color: var(--on-accent);
    background: var(--accent);
    flex: 1 1 auto;
    min-width: 0;
  }
  .mm-btn-primary:hover {
    filter: brightness(1.05);
  }
</style>
