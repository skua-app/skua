<script lang="ts">
  import { goto } from '$app/navigation'
  import type { EventItem, Moment } from '$lib/api'
  import { eventThumbnailURL } from '$lib/api'
  import BottomSheet from '$lib/components/BottomSheet.svelte'
  import EventModal from '$lib/components/EventModal.svelte'
  import Mono from '$lib/components/Mono.svelte'
  import { glanceStore } from '$lib/stores/glance.svelte'
  import { camerasStore } from '$lib/stores/cameras.svelte'
  import { eventKindLabels, ui } from '$lib/i18n/strings'
  import { momentRouteTarget, momentToEventItem } from '$lib/glance'
  import { relativeTime } from '$lib/util/time'

  // Local-only modal state so a row tap can open EventModal on top of
  // the sheet (mirrors routes/events/+page.svelte's modalEvent pattern).
  let modalEvent = $state<EventItem | null>(null)

  // One-minute tick so "5 min ago" doesn't drift while the sheet is open.
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

  function kindsLine(moment: Moment): string {
    const parts = moment.kinds.map((k) => eventKindLabels[k] ?? k)
    const base = parts.length > 0 ? parts.join(' · ') : ui.eventsEmpty
    return moment.event_count > 1 ? `${base} (${moment.event_count})` : base
  }

  function onRowClick(moment: Moment) {
    // Mark the row as viewed in this session so it dims after the user
    // navigates back to the peek. Household seen-state is only advanced
    // when the user explicitly dismisses the peek (BottomSheet onClose),
    // never on a row tap — that lets the user open several clips in a
    // row from the same sheet without losing the "unseen" badge.
    glanceStore.markViewed(moment.representative_event_id)
    if (momentRouteTarget(moment) === 'focus') {
      glanceStore.closePeek()
      goto(`/cam/${encodeURIComponent(moment.cam_id)}`)
    } else {
      modalEvent = momentToEventItem(moment)
    }
  }

  // A row is dimmed when it has been opened in this session OR when the
  // household last_seen has already advanced past its started_at (the
  // peek was reopened from the bell after a previous dismissal). Dimmed
  // rows remain clickable so users can re-watch what they've already
  // looked at.
  function isDimmed(moment: Moment): boolean {
    if (glanceStore.viewed.has(moment.representative_event_id)) return true
    const seen = glanceStore.lastSeen
    if (seen !== null && moment.started_at <= seen) return true
    return false
  }
</script>

<BottomSheet
  open={glanceStore.peekOpen}
  onClose={() => glanceStore.dismiss()}
  title={ui.glancePeekTitle}
>
  {#if glanceStore.moments.length === 0}
    <div class="gp-empty">{ui.glancePeekEmpty}</div>
  {:else}
    <ul class="gp-list" role="list">
      {#each glanceStore.moments as m (m.cam_id + m.started_at)}
        <li>
          <button
            type="button"
            class="gp-row"
            class:dimmed={isDimmed(m)}
            onclick={() => onRowClick(m)}
          >
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
              <div class="gp-row-line">
                <span class="gp-cam">{camName(m.cam_id)}</span>
                <Mono size={11} color="var(--text-3)">{relativeTime(m.started_at, now)}</Mono>
              </div>
              <div class="gp-kinds">{kindsLine(m)}</div>
            </div>
          </button>
        </li>
      {/each}
    </ul>
  {/if}
</BottomSheet>

{#if modalEvent}
  <EventModal event={modalEvent} onClose={() => (modalEvent = null)} />
{/if}

<style>
  .gp-empty {
    padding: 20px;
    text-align: center;
    color: var(--text-3);
    font-size: 13px;
  }
  .gp-list {
    list-style: none;
    padding: 0;
    margin: 0;
    display: flex;
    flex-direction: column;
  }
  .gp-row {
    display: grid;
    grid-template-columns: 112px 1fr;
    gap: 12px;
    width: 100%;
    align-items: center;
    padding: 10px 20px;
    background: transparent;
    border: none;
    border-bottom: 1px solid var(--border);
    color: inherit;
    text-align: left;
    cursor: pointer;
    font-family: inherit;
    transition: background 120ms;
  }
  .gp-row:hover {
    background: rgba(255, 255, 255, 0.03);
  }
  .gp-row.dimmed {
    opacity: 0.45;
  }
  .gp-row.dimmed:hover {
    opacity: 0.7;
  }
  .gp-thumb {
    aspect-ratio: 16 / 9;
    border-radius: 6px;
    overflow: hidden;
    background: #0c0d0f;
  }
  .gp-thumb img {
    width: 100%;
    height: 100%;
    object-fit: cover;
    display: block;
  }
  .gp-body {
    display: flex;
    flex-direction: column;
    gap: 4px;
    min-width: 0;
  }
  .gp-row-line {
    display: flex;
    justify-content: space-between;
    align-items: baseline;
    gap: 10px;
    min-width: 0;
  }
  .gp-cam {
    font-size: 13px;
    font-weight: 500;
    color: var(--text);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .gp-kinds {
    font-size: 12px;
    color: var(--text-2);
  }
</style>
