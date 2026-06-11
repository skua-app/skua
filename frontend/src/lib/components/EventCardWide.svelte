<script lang="ts">
  import type { EventItem } from '$lib/api'
  import { eventSnapshotURL } from '$lib/api'
  import { camerasStore } from '$lib/stores/cameras.svelte'
  import { eventKindLabels } from '$lib/i18n/strings'
  import { relativeTime } from '$lib/util/time'
  import Icon from './Icon.svelte'
  import Mono from './Mono.svelte'

  type Props = {
    event: EventItem
    onclick: () => void
  }

  let { event, onclick }: Props = $props()

  const camName = $derived(
    camerasStore.cameras.find((c) => c.id === event.cam_id)?.name ?? event.cam_id
  )
  const kindLabel = $derived(eventKindLabels[event.kind] ?? event.kind)

  let now = $state(new Date())
  $effect(() => {
    const t = setInterval(() => {
      now = new Date()
    }, 60_000)
    return () => clearInterval(t)
  })
  const relative = $derived(relativeTime(event.started_at, now))
</script>

<button type="button" class="ecw" {onclick}>
  <div class="ecw-thumb">
    <img
      src={eventSnapshotURL(event.id)}
      alt={`${camName} · ${kindLabel}`}
      loading="lazy"
      width="320"
      height="180"
    />
    <span class="ecw-kind">
      <Mono size={9} color="rgba(255,255,255,0.92)" letterSpacing={0.8} weight={600} uppercase
        >{kindLabel}</Mono
      >
    </span>
    {#if event.score !== null}
      <span class="ecw-score">
        <Mono size={9} color="rgba(255,255,255,0.92)" weight={500}>{event.score.toFixed(2)}</Mono>
      </span>
    {/if}
    <span class="ecw-play" aria-hidden="true">
      <Icon name="play" size={16} />
    </span>
  </div>
  <div class="ecw-body">
    <span class="ecw-cam">{camName}</span>
    <Mono size={10} color="var(--text-3)">{relative}</Mono>
  </div>
</button>

<style>
  .ecw {
    display: flex;
    flex-direction: column;
    gap: 0;
    padding: 0;
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: 10px;
    text-align: left;
    cursor: pointer;
    color: inherit;
    font-family: inherit;
    overflow: hidden;
    transition:
      border-color 160ms,
      transform 160ms,
      box-shadow 160ms;
  }
  .ecw-thumb {
    position: relative;
    aspect-ratio: 16 / 9;
    overflow: hidden;
    background: #0c0d0f;
  }
  .ecw-thumb img {
    width: 100%;
    height: 100%;
    object-fit: cover;
    display: block;
  }
  .ecw-kind {
    position: absolute;
    top: 8px;
    left: 8px;
    padding: 3px 7px;
    border-radius: 4px;
    background: rgba(0, 0, 0, 0.55);
    backdrop-filter: blur(8px);
  }
  .ecw-score {
    position: absolute;
    top: 8px;
    right: 8px;
    padding: 3px 7px;
    border-radius: 4px;
    background: rgba(0, 0, 0, 0.55);
    backdrop-filter: blur(8px);
  }
  .ecw-play {
    position: absolute;
    top: 50%;
    left: 50%;
    transform: translate(-50%, -50%);
    width: 36px;
    height: 36px;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    border-radius: 999px;
    background: rgba(0, 0, 0, 0.55);
    backdrop-filter: blur(8px);
    color: #fff;
    opacity: 0;
    pointer-events: none;
    transition: opacity 160ms;
  }
  .ecw-body {
    display: flex;
    justify-content: space-between;
    align-items: baseline;
    gap: 10px;
    padding: 10px 12px 12px;
    min-width: 0;
  }
  .ecw-cam {
    font-size: 13px;
    font-weight: 500;
    color: var(--text);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  @media (hover: hover) and (pointer: fine) {
    .ecw:hover {
      border-color: var(--border-strong);
      transform: translateY(-2px);
      box-shadow: 0 8px 24px rgba(0, 0, 0, 0.35);
    }
    .ecw:hover .ecw-play {
      opacity: 1;
    }
  }
</style>
