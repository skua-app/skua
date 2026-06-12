<script lang="ts">
  import type { EventItem } from '$lib/api'
  import { eventSnapshotURL } from '$lib/api'
  import { camerasStore } from '$lib/stores/cameras.svelte'
  import { eventKindLabels } from '$lib/i18n/strings'
  import { relativeTime } from '$lib/util/time'
  import Icon from './Icon.svelte'

  type Props = {
    event: EventItem
    onclick: () => void
  }

  let { event, onclick }: Props = $props()

  const camName = $derived(
    camerasStore.cameras.find((c) => c.id === event.cam_id)?.name ?? event.cam_id
  )
  const kindLabel = $derived(eventKindLabels[event.kind] ?? event.kind)

  // Tick once a minute so `5 min ago` stays current while the page is open.
  let now = $state(new Date())
  $effect(() => {
    const t = setInterval(() => {
      now = new Date()
    }, 60_000)
    return () => clearInterval(t)
  })
  const relative = $derived(relativeTime(event.started_at, now))
</script>

<button type="button" class="dk-evcard" {onclick}>
  <div class="shot">
    <img
      src={eventSnapshotURL(event.id)}
      alt={`${camName} · ${kindLabel}`}
      loading="lazy"
      width="320"
      height="180"
    />
    <span class="ktag">{kindLabel}</span>
    {#if event.score !== null}
      <span class="sc">{event.score.toFixed(2)}</span>
    {/if}
    <span class="play" aria-hidden="true">
      <Icon name="play" size={19} />
    </span>
  </div>
  <div class="info">
    <div class="info-l">
      <div class="nm">{camName}</div>
      <div class="rm">{kindLabel}</div>
    </div>
    <span class="when">{relative}</span>
  </div>
</button>

<style>
  /* calm .dk-evcard */
  .dk-evcard {
    display: block;
    width: 100%;
    padding: 0;
    border: 1px solid var(--border);
    border-radius: var(--r);
    background: var(--surface);
    color: inherit;
    text-align: left;
    cursor: pointer;
    font-family: inherit;
    overflow: hidden;
    transition:
      transform 0.16s ease,
      box-shadow 0.2s ease,
      border-color 0.16s ease;
  }
  .shot {
    position: relative;
    aspect-ratio: 16 / 9;
    background: var(--feed);
    overflow: hidden;
  }
  .shot img {
    width: 100%;
    height: 100%;
    /* Still snapshot: cover (NEVER fill). */
    object-fit: cover;
    display: block;
  }
  .ktag {
    position: absolute;
    left: 11px;
    top: 11px;
    padding: 4px 9px;
    border-radius: 7px;
    background: rgba(8, 10, 12, 0.55);
    border: 1px solid rgba(255, 255, 255, 0.18);
    color: #fff;
    font-family: 'JetBrains Mono', ui-monospace, monospace;
    font-size: 10px;
    font-weight: 700;
    letter-spacing: 0.8px;
    text-transform: uppercase;
    backdrop-filter: blur(6px);
    -webkit-backdrop-filter: blur(6px);
  }
  .sc {
    position: absolute;
    right: 11px;
    top: 11px;
    padding: 4px 8px;
    border-radius: 7px;
    background: rgba(8, 10, 12, 0.55);
    border: 1px solid rgba(255, 255, 255, 0.18);
    color: #fff;
    font-family: 'JetBrains Mono', ui-monospace, monospace;
    font-size: 11px;
    font-weight: 600;
    backdrop-filter: blur(6px);
    -webkit-backdrop-filter: blur(6px);
  }
  .play {
    position: absolute;
    left: 50%;
    top: 50%;
    transform: translate(-50%, -50%);
    width: 46px;
    height: 46px;
    border-radius: 50%;
    background: rgba(10, 12, 14, 0.5);
    border: 1px solid rgba(255, 255, 255, 0.24);
    display: grid;
    place-items: center;
    opacity: 0;
    pointer-events: none;
    backdrop-filter: blur(4px);
    -webkit-backdrop-filter: blur(4px);
    transition: opacity 0.16s ease;
    color: #fff;
  }
  .play :global(svg) {
    fill: #fff;
    stroke: none;
    margin-left: 2px;
  }
  .info {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 10px;
    padding: 10px 13px 12px;
  }
  .info-l {
    min-width: 0;
  }
  .nm {
    font-size: 14px;
    font-weight: 600;
    color: var(--text);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .rm {
    font-size: 12px;
    color: var(--text-2);
    margin-top: 2px;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .when {
    font-family: 'JetBrains Mono', ui-monospace, monospace;
    font-size: 12px;
    color: var(--text-2);
    white-space: nowrap;
    flex: 0 0 auto;
  }

  /* Hover affordance gated to pointer-fine; touchscreens get nothing. */
  @media (hover: hover) and (pointer: fine) {
    .dk-evcard:hover {
      transform: translateY(-2px);
      border-color: var(--border-strong);
      box-shadow: 0 16px 34px -22px rgba(0, 0, 0, 0.7);
    }
    .dk-evcard:hover .play {
      opacity: 1;
    }
  }
</style>
