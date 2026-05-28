<script lang="ts">
  import type { EventItem } from '$lib/api'
  import { eventSnapshotURL, eventClipURL } from '$lib/api'
  import { camerasStore } from '$lib/stores/cameras.svelte'
  import { configStore } from '$lib/stores/config.svelte'
  import { eventKindLabels, ui } from '$lib/i18n/strings'
  import { formatDuration } from '$lib/util/time'
  import Mono from '$lib/components/Mono.svelte'

  type Props = {
    event: EventItem
    onClose: () => void
  }

  let { event, onClose }: Props = $props()

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
  // Verified against Frigate 0.17 on 2026-05-20:
  // The UI is a SPA, so /events/<id>, /explore?event_id=<id>, and /?camera=<cam>
  // all return 200 unconditionally. 0.17 retired the dedicated event-detail
  // route used in 0.16, so there is no reliable "scroll-to-event" deep-link.
  // We use /explore?event_id=<id>: Explore is the tab that lists per-event
  // snapshots, so even if the query param is ignored the user lands in the
  // right context to find the event. The cam_id query is appended so that,
  // if Explore filters by camera, only the relevant camera's events show.
  const deepLink = $derived(
    configStore.frigateUIURL
      ? `${configStore.frigateUIURL}/explore?event_id=${encodeURIComponent(event.id)}&camera=${encodeURIComponent(event.cam_id)}`
      : ''
  )

  let closeBtn = $state<HTMLButtonElement | null>(null)

  function onKey(e: KeyboardEvent) {
    if (e.key === 'Escape') onClose()
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
<div class="em-backdrop" onclick={onBackdropClick}>
  <div class="em-card" role="dialog" aria-modal="true" aria-label={kindLabel}>
    <div class="em-snap">
      {#if event.has_clip}
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
        ></video>
      {:else}
        <img src={eventSnapshotURL(event.id)} alt={`${camName} · ${kindLabel}`} loading="lazy" />
      {/if}
    </div>

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
        <a class="em-btn em-btn-secondary" href={eventClipURL(event.id, true)} download>
          {ui.downloadVideo}
        </a>
      {/if}
      {#if deepLink}
        <a class="em-btn em-btn-primary" href={deepLink} target="_blank" rel="noopener noreferrer">
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
    background: rgba(0, 0, 0, 0.65);
    backdrop-filter: blur(6px);
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
    background: #15171a;
    border: 1px solid var(--border-strong);
    border-radius: 12px;
    display: flex;
    flex-direction: column;
  }

  .em-snap {
    aspect-ratio: 16 / 9;
    background: #0c0d0f;
    overflow: hidden;
    border-top-left-radius: 12px;
    border-top-right-radius: 12px;
  }
  .em-snap img,
  .em-snap video {
    width: 100%;
    height: 100%;
    object-fit: contain;
    display: block;
    background: #000;
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
  }
  .em-btn {
    padding: 8px 14px;
    font-size: 13px;
    font-weight: 500;
    border-radius: 8px;
    border: 1px solid transparent;
    cursor: pointer;
    text-decoration: none;
    font-family: inherit;
    transition:
      background 120ms,
      border-color 120ms,
      color 120ms;
  }
  .em-btn-secondary {
    color: var(--text-2);
    background: transparent;
    border-color: var(--border-strong);
  }
  .em-btn-secondary:hover {
    color: var(--text);
    border-color: var(--text-3);
  }
  .em-btn-primary {
    color: #08121c;
    background: var(--accent);
    border-color: var(--accent);
  }
  .em-btn-primary:hover {
    filter: brightness(1.1);
  }
</style>
