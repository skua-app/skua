<script lang="ts">
  import { fly, fade } from 'svelte/transition'
  import { cubicOut } from 'svelte/easing'
  import type { EventItem } from '$lib/api'
  import { eventSnapshotURL, eventClipURL, fetchEventReview } from '$lib/api'
  import { camerasStore } from '$lib/stores/cameras.svelte'
  import { configStore } from '$lib/stores/config.svelte'
  import { eventKindLabels, ui } from '$lib/i18n/strings'
  import { formatDuration } from '$lib/util/time'
  import Mono from '$lib/components/Mono.svelte'
  import Icon from '$lib/components/Icon.svelte'

  type Props = {
    event: EventItem
    onClose: () => void
  }

  let { event, onClose }: Props = $props()

  // Honour reduced-motion: the card/backdrop transitions collapse to 0ms,
  // so the modal still appears but without the fly/fade.
  const reducedMotion =
    typeof window !== 'undefined' && window.matchMedia('(prefers-reduced-motion: reduce)').matches
  const cardDuration = reducedMotion ? 0 : 260
  const scrimDuration = reducedMotion ? 0 : 200

  const camName = $derived(
    camerasStore.cameras.find((c) => c.id === event.cam_id)?.name ?? event.cam_id
  )
  const kindLabel = $derived(eventKindLabels[event.kind] ?? event.kind)
  const absoluteTime = $derived(
    new Intl.DateTimeFormat([], {
      dateStyle: 'medium',
      timeStyle: 'medium'
    }).format(new Date(event.started_at))
  )
  const duration = $derived(formatDuration(event.duration_seconds))

  // Frigate UI deep-link.
  //
  // Events now resolve their containing review segment via
  // GET /api/events/{id}/review and deep-link to the review timeline
  // (/review?id=<review_id>) when one is found, falling back to
  // /explore?event_id=<id> when no review contains the event. The
  // button is usable immediately (Explore link) and upgrades to the
  // timeline link once the resolver responds, so a slow round-trip
  // never blocks the user.
  //
  // Verified against Frigate 0.17 on 2026-05-20: the UI is a SPA, so
  // /review?id=<id>, /events/<id>, /explore?event_id=<id>, and
  // /?camera=<cam> all return 200 unconditionally. 0.17 retired the
  // dedicated event-detail route used in 0.16; the review timeline
  // is the closest "scroll-to-this-activity" affordance, with
  // Explore as the safe fallback when no review covers the event.
  let reviewId = $state<string | null>(null)
  $effect(() => {
    const id = event.id
    reviewId = null
    void fetchEventReview(id).then((found) => {
      // Stale-assign guard: only commit if we're still on the same
      // event the request was made for.
      if (id === event.id) reviewId = found
    })
  })
  const deepLink = $derived(
    configStore.frigateUIURL
      ? reviewId
        ? `${configStore.frigateUIURL}/review?id=${encodeURIComponent(reviewId)}`
        : `${configStore.frigateUIURL}/explore?event_id=${encodeURIComponent(event.id)}&camera=${encodeURIComponent(event.cam_id)}`
      : ''
  )

  let closeBtn = $state<HTMLButtonElement | null>(null)
  let cardEl = $state<HTMLDivElement | null>(null)

  // Flips true if the <video> emits any `error` event — usually a decoder
  // failure on devices that can't handle the clip's native codec/resolution
  // (e.g. budget Android SoCs vs. 1440p HEVC). Don't inspect video.error.code:
  // browsers vary, and we treat every failure the same way (show the snapshot
  // poster + a pointer at Download / Open in Frigate, both already in the
  // action row). Reset when switching to another event id.
  let clipFailed = $state(false)
  $effect(() => {
    // Read event.id so this effect re-runs when the modal instance is reused
    // for another event; the `if` keeps eslint's no-unused-expressions happy
    // without changing behaviour (event.id is always a non-empty string).
    const id = event.id
    if (id) clipFailed = false
  })

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
    closeBtn?.focus()
  })

  function onBackdropClick(e: MouseEvent) {
    if (e.target === e.currentTarget) onClose()
  }
</script>

<!-- svelte-ignore a11y_click_events_have_key_events -->
<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="em-backdrop" onclick={onBackdropClick} transition:fade={{ duration: scrimDuration }}>
  <div
    class="em-card"
    role="dialog"
    aria-modal="true"
    aria-label={kindLabel}
    bind:this={cardEl}
    transition:fly={{ y: 16, duration: cardDuration, easing: cubicOut }}
  >
    <div class="em-snap">
      {#if event.has_clip && !clipFailed}
        <!-- iOS Safari: `playsinline` keeps playback inside the modal instead
             of triggering fullscreen. The BFF buffers the upstream clip and
             serves it via http.ServeContent, so HTTP Range works end-to-end
             — required for iOS Safari to play <video> inline at all, and
             enables seek across browsers. See backend/internal/events.Client.ServeClip. -->
        <!-- svelte-ignore a11y_media_has_caption -->
        <video
          src={eventClipURL(event.id)}
          poster={eventSnapshotURL(event.id)}
          controls
          playsinline
          preload="metadata"
          onerror={() => (clipFailed = true)}
        ></video>
      {:else}
        <img src={eventSnapshotURL(event.id)} alt={`${camName} · ${kindLabel}`} loading="lazy" />
      {/if}
    </div>

    {#if event.has_clip && clipFailed}
      <div class="em-clip-fallback" role="status">
        <div class="em-clip-fallback-heading">{ui.clipUnplayableHeading}</div>
        <div class="em-clip-fallback-hint">{ui.clipUnplayableHint}</div>
      </div>
    {/if}

    <div class="em-meta">
      <div class="em-meta-row">
        <span class="em-cam">{camName}</span>
        <Mono size={11} color="var(--text-3)">{event.cam_id}</Mono>
      </div>
      <div class="em-meta-row">
        <span class="em-kind">{kindLabel} · {event.label}</span>
        <Mono size={11} color="var(--text-2)" weight={500}>
          {event.score !== null ? event.score.toFixed(2) : '—'}
        </Mono>
      </div>
      <div class="em-meta-row em-meta-row-faint">
        <Mono size={11} color="var(--text-3)">{absoluteTime}</Mono>
        <Mono size={11} color="var(--text-3)">{duration}</Mono>
      </div>
    </div>

    <div class="em-actions">
      <button type="button" class="em-btn em-btn-secondary" onclick={onClose} bind:this={closeBtn}>
        {ui.close}
      </button>
      {#if event.has_clip}
        <a
          class="em-btn em-btn-secondary em-btn-icon"
          href={eventClipURL(event.id, true)}
          download
          aria-label={ui.downloadVideo}
          title={ui.downloadVideo}
        >
          <Icon name="download" size={20} />
        </a>
      {/if}
      {#if deepLink}
        <a class="em-btn em-btn-primary" href={deepLink} target="_blank" rel="noopener noreferrer">
          <Icon name="link" size={16} />
          {ui.openInFrigate}
        </a>
      {/if}
    </div>
  </div>
</div>

<style>
  .em-backdrop {
    position: fixed;
    inset: 0;
    background: var(--scrim);
    backdrop-filter: blur(6px);
    -webkit-backdrop-filter: blur(6px);
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 20px;
    z-index: 100;
  }
  .em-card {
    width: min(560px, 100%);
    max-height: calc(100dvh - 40px);
    overflow: auto;
    background: var(--surface);
    border: 1px solid var(--border-strong);
    border-radius: var(--r);
    box-shadow: var(--shadow);
    display: flex;
    flex-direction: column;
  }

  .em-snap {
    aspect-ratio: 16 / 9;
    background: var(--feed);
    overflow: hidden;
    border-top-left-radius: var(--r);
    border-top-right-radius: var(--r);
  }
  .em-snap img,
  .em-snap video {
    width: 100%;
    height: 100%;
    /* Review media: contain (NEVER fill). */
    object-fit: contain;
    display: block;
    background: var(--feed);
  }

  .em-clip-fallback {
    padding: 12px 18px 0;
    display: flex;
    flex-direction: column;
    gap: 4px;
  }
  .em-clip-fallback-heading {
    font-size: 13px;
    font-weight: 500;
    color: var(--text);
  }
  .em-clip-fallback-hint {
    font-size: 12px;
    color: var(--text-2);
    line-height: 1.4;
  }

  .em-meta {
    padding: 16px 18px 8px;
    display: flex;
    flex-direction: column;
    gap: 8px;
    border-bottom: 1px solid var(--border);
  }
  .em-meta-row {
    display: flex;
    justify-content: space-between;
    align-items: baseline;
    gap: 10px;
  }
  .em-meta-row-faint {
    margin-top: 2px;
  }
  .em-cam {
    font-size: 15px;
    font-weight: 600;
    color: var(--text);
    letter-spacing: -0.1px;
  }
  .em-kind {
    font-size: 13px;
    color: var(--text-2);
  }

  .em-actions {
    padding: 14px 18px 18px;
    display: flex;
    justify-content: flex-end;
    gap: 10px;
    border-top: 1px solid var(--border);
  }
  .em-btn {
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
  .em-btn:active {
    transform: translateY(1px);
  }
  .em-btn-secondary {
    color: var(--text);
    background: var(--surface-2);
  }
  .em-btn-secondary:hover {
    background: var(--border-strong);
  }
  .em-btn-icon {
    width: 44px;
    height: 44px;
    padding: 0;
    flex: 0 0 auto;
    color: var(--text-2);
  }
  .em-btn-icon:hover {
    color: var(--text);
  }
  .em-btn-primary {
    color: var(--on-accent);
    background: var(--accent);
    flex: 1 1 auto;
    min-width: 0;
  }
  .em-btn-primary:hover {
    filter: brightness(1.05);
  }
</style>
