<script lang="ts">
  import type { EventItem } from '$lib/api'
  import { eventSnapshotURL } from '$lib/api'
  import { camerasStore } from '$lib/stores/cameras.svelte'
  import { eventKindLabels } from '$lib/i18n/strings'
  import { relativeTime, formatDuration } from '$lib/util/time'
  import Mono from '$lib/components/Mono.svelte'

  type Props = {
    event: EventItem
    onclick: () => void
  }

  let { event, onclick }: Props = $props()

  const camName = $derived(
    camerasStore.cameras.find((c) => c.id === event.cam_id)?.name ?? event.cam_id
  )
  const kindLabel = $derived(eventKindLabels[event.kind] ?? event.kind)
  // Tick once a minute so "5 min ago" doesn't drift.
  let now = $state(new Date())
  $effect(() => {
    const t = setInterval(() => {
      now = new Date()
    }, 60_000)
    return () => clearInterval(t)
  })
  const relative = $derived(relativeTime(event.started_at, now))
  const duration = $derived(formatDuration(event.duration_seconds))
</script>

<button type="button" class="ec" {onclick}>
  <div class="ec-thumb">
    <img
      src={eventSnapshotURL(event.id)}
      alt={`${camName} · ${kindLabel}`}
      loading="lazy"
      width="320"
      height="180"
    />
  </div>
  <div class="ec-body">
    <div class="ec-row">
      <span class="ec-cam">{camName}</span>
      <Mono size={11} color="var(--text-3)">{relative}</Mono>
    </div>
    <div class="ec-row">
      <span class="ec-kind">{kindLabel}</span>
      <Mono size={11} color="var(--text-3)">{duration}</Mono>
    </div>
    <div class="ec-row ec-row-score">
      <Mono size={10} color="var(--text-3)" letterSpacing={0.4} uppercase>score</Mono>
      <Mono size={12} color="var(--text-2)" weight={500}>
        {event.score !== null ? event.score.toFixed(2) : '—'}
      </Mono>
    </div>
  </div>
</button>

<style>
  .ec {
    display: grid;
    grid-template-columns: 160px 1fr;
    gap: 14px;
    align-items: stretch;
    padding: 10px;
    border: 1px solid var(--border);
    border-radius: 10px;
    background: var(--surface);
    color: inherit;
    text-align: left;
    cursor: pointer;
    font-family: inherit;
    transition:
      border-color 120ms,
      background 120ms;
  }
  .ec:hover {
    border-color: var(--border-strong);
    background: rgba(255, 255, 255, 0.04);
  }

  .ec-thumb {
    aspect-ratio: 16 / 9;
    border-radius: 6px;
    overflow: hidden;
    background: #0c0d0f;
  }
  .ec-thumb img {
    width: 100%;
    height: 100%;
    object-fit: cover;
    display: block;
  }

  .ec-body {
    display: flex;
    flex-direction: column;
    justify-content: space-between;
    gap: 4px;
    min-width: 0;
  }
  .ec-row {
    display: flex;
    justify-content: space-between;
    align-items: baseline;
    gap: 10px;
    min-width: 0;
  }
  .ec-row-score {
    margin-top: 2px;
  }
  .ec-cam {
    font-size: 13px;
    font-weight: 500;
    color: var(--text);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .ec-kind {
    font-size: 12px;
    color: var(--text-2);
  }

  @media (max-width: 520px) {
    .ec {
      grid-template-columns: 112px 1fr;
      gap: 10px;
    }
  }
</style>
