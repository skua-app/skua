<script lang="ts">
  import { fade, fly } from 'svelte/transition'
  import { cubicOut } from 'svelte/easing'
  import type { GlanceMoment } from '$lib/api'
  import { eventSnapshotURL, momentClipURL } from '$lib/api'
  import { camerasStore } from '$lib/stores/cameras.svelte'
  import { configStore } from '$lib/stores/config.svelte'
  import { eventKindLabels, ui } from '$lib/i18n/strings'
  import Mono from '$lib/components/Mono.svelte'
  import Icon from '$lib/components/Icon.svelte'
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

  // A single failure flag for the moment-wide clip. Falls back to
  // the thumb-event snapshot when present; otherwise the placeholder
  // background carries the moment.
  let clipFailed = $state(false)

  const camName = $derived(
    camerasStore.cameras.find((c) => c.id === moment.cam_id)?.name ?? moment.cam_id
  )
  const kindsLine = $derived(moment.kinds.map((k) => eventKindLabels[k] ?? k).join(' · '))
  const absoluteTime = $derived(
    new Intl.DateTimeFormat([], {
      dateStyle: 'medium',
      timeStyle: 'medium'
    }).format(new Date(moment.started_at))
  )
  // Moments deep-link straight to the Frigate review timeline at this
  // segment: moment.id IS the Frigate review id, so /review?id=<id>
  // scrolls to (and selects) the matching review on Frigate's
  // history view. No /explore fallback is needed — the BFF only
  // surfaces moments that came from /api/review, so the id is
  // guaranteed to be valid for the timeline.
  const deepLink = $derived(
    configStore.frigateUIURL
      ? `${configStore.frigateUIURL}/review?id=${encodeURIComponent(moment.id)}`
      : ''
  )
  const titleText = $derived(`${ui.glanceMomentTitle} · ${camName}`)

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
      <ZoomPane resetKey={moment.id}>
        {#if moment.thumb_event_id && !clipFailed}
          <!-- svelte-ignore a11y_media_has_caption -->
          <video
            src={momentClipURL(moment.id)}
            poster={eventSnapshotURL(moment.thumb_event_id)}
            controls
            playsinline
            preload="metadata"
            bind:this={videoEl}
            onerror={() => (clipFailed = true)}
          ></video>
        {:else if moment.thumb_event_id}
          <img src={eventSnapshotURL(moment.thumb_event_id)} alt={camName} loading="lazy" />
        {:else}
          <div class="mm-placeholder" aria-hidden="true"></div>
        {/if}
      </ZoomPane>
    </div>

    {#if moment.thumb_event_id && clipFailed}
      <div class="mm-clip-fallback" role="status">
        <div class="mm-clip-fallback-heading">{ui.clipUnplayableHeading}</div>
        <div class="mm-clip-fallback-hint">{ui.clipUnplayableHint}</div>
      </div>
    {/if}

    <div class="mm-meta">
      <div class="mm-meta-row">
        <span class="mm-cam">{camName}</span>
        <Mono size={11} color="var(--text-3)">{moment.cam_id}</Mono>
      </div>
      {#if kindsLine}
        <div class="mm-meta-row">
          <span class="mm-kind">{kindsLine}</span>
          <span class="mm-severity">{moment.severity}</span>
        </div>
      {:else}
        <div class="mm-meta-row">
          <span class="mm-kind"></span>
          <span class="mm-severity">{moment.severity}</span>
        </div>
      {/if}
      <div class="mm-meta-row mm-meta-row-faint">
        <Mono size={11} color="var(--text-3)">{absoluteTime}</Mono>
      </div>
    </div>

    <div class="mm-actions">
      <button type="button" class="mm-btn mm-btn-secondary" onclick={onClose} bind:this={closeBtn}>
        {ui.close}
      </button>
      {#if onOpenLive}
        <button
          type="button"
          class="mm-btn mm-btn-secondary mm-btn-icon"
          onclick={onOpenLive}
          aria-label={ui.openLive}
          title={ui.openLive}
        >
          <Icon name="cams" size={20} />
        </button>
      {/if}
      {#if deepLink}
        <a class="mm-btn mm-btn-primary" href={deepLink} target="_blank" rel="noopener noreferrer">
          <Icon name="link" size={16} />
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
       the 16:9 box eat the entire modal. */
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
  .mm-placeholder {
    width: 100%;
    height: 100%;
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
    padding: 16px 18px 14px;
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
  .mm-severity {
    font-size: 10px;
    text-transform: uppercase;
    letter-spacing: 0.4px;
    color: var(--text-3);
    padding: 2px 6px;
    border: 1px solid var(--border);
    border-radius: 4px;
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
  .mm-btn-icon {
    width: 44px;
    height: 44px;
    padding: 0;
    flex: 0 0 auto;
    color: var(--text-2);
  }
  .mm-btn-icon:hover {
    color: var(--text);
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
