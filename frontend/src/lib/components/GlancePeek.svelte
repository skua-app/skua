<script lang="ts">
  import { goto } from '$app/navigation'
  import type { GlanceMoment } from '$lib/api'
  import { eventThumbnailURL } from '$lib/api'
  import BottomSheet from '$lib/components/BottomSheet.svelte'
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

  // kindsLine joins the moment's distinct kinds; the cluster count is
  // surfaced separately as an accent chip next to the camera name.
  function kindsLine(moment: GlanceMoment): string {
    const parts = moment.kinds.map((k) => eventKindLabels[k] ?? k)
    return parts.length > 0 ? parts.join(' · ') : ui.eventsEmpty
  }

  function countChipText(moment: GlanceMoment): string {
    const tpl = moment.event_count === 1 ? ui.glanceEventOne : ui.glanceEventMany
    return tpl.replace('{n}', String(moment.event_count))
  }

  // Row time reflects the NEWEST detection in the cluster, not the
  // earliest. moment.events is newest-first; fall back to started_at
  // when the cluster slice is unexpectedly empty.
  function newestTime(moment: GlanceMoment): string {
    const newest = moment.events[0]?.started_at ?? moment.started_at
    return relativeTime(newest, now)
  }

  function onRowClick(moment: GlanceMoment) {
    // Opening a moment marks just that one seen on the server. The
    // modal opens on top of the peek so the user can work through the
    // list and switch between the detections inside the cluster.
    void glanceStore.markOneSeen(moment.representative_event_id)
    modalMoment = moment
  }

  // openLiveFor produces the optional onOpenLive callback for the
  // modal. A non-null return wires the action button up; undefined
  // hides it entirely. Available only when the moment is still live
  // AND the camera is currently online — an offline live moment has
  // nothing playable behind the action.
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

  // A row is dimmed when the household has already marked its
  // representative event seen. Dimmed rows remain clickable so users can
  // re-watch what they've already looked at.
  function isDimmed(moment: GlanceMoment): boolean {
    return moment.seen
  }

  function onViewAll() {
    glanceStore.closePeek()
    goto('/events')
  }
</script>

<BottomSheet
  open={glanceStore.peekOpen}
  onClose={() => glanceStore.closePeek()}
  title={ui.glancePeekTitle}
>
  <div class="gp-window-chip">
    <Mono size={10} color="var(--text-3)" letterSpacing={0.3} uppercase>
      {ui.glanceWindowChip.replace('{hours}', String(prefsStore.glanceWindowHours))}
    </Mono>
  </div>

  {#if glanceStore.moments.length === 0}
    <div class="gp-empty">{ui.glancePeekEmpty}</div>
    <div class="gp-footer">
      <button type="button" class="gp-view-all" onclick={onViewAll}>{ui.glanceViewAll}</button>
    </div>
  {:else}
    {#if glanceStore.moments.length > 0}
      <div class="gp-actions">
        <button type="button" class="gp-clear" onclick={() => void glanceStore.clearGlance()}>
          {ui.glanceClear}
        </button>
      </div>
    {/if}
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
                <div class="gp-cam-wrap">
                  <span class="gp-cam">{camName(m.cam_id)}</span>
                  {#if m.event_count >= 1}
                    <span class="gp-count">
                      <Mono size={10} color="var(--accent)" weight={600} letterSpacing={0.3}>
                        {countChipText(m)}
                      </Mono>
                    </span>
                  {/if}
                </div>
                <Mono size={11} color="var(--text-3)">{newestTime(m)}</Mono>
              </div>
              <div class="gp-kinds">{kindsLine(m)}</div>
            </div>
          </button>
        </li>
      {/each}
    </ul>
    <div class="gp-footer">
      <button type="button" class="gp-view-all" onclick={onViewAll}>{ui.glanceViewAll}</button>
    </div>
  {/if}
</BottomSheet>

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
  .gp-empty {
    padding: 20px;
    text-align: center;
    color: var(--text-3);
    font-size: 13px;
  }
  .gp-window-chip {
    padding: 6px 20px 2px;
  }
  .gp-actions {
    display: flex;
    justify-content: flex-end;
    padding: 4px 16px 8px;
  }
  .gp-clear {
    background: transparent;
    border: none;
    padding: 4px 6px;
    color: var(--accent);
    font-family: inherit;
    font-size: 12px;
    font-weight: 500;
    cursor: pointer;
    border-radius: 4px;
    transition:
      color 120ms,
      background 120ms;
  }
  .gp-clear:hover {
    background: color-mix(in oklab, var(--accent) 12%, transparent);
  }
  .gp-footer {
    display: flex;
    justify-content: center;
    padding: 12px 20px 4px;
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
  .gp-cam-wrap {
    display: flex;
    align-items: center;
    gap: 8px;
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
  .gp-count {
    display: inline-flex;
    align-items: center;
    padding: 2px 7px;
    border-radius: 999px;
    background: color-mix(in oklab, var(--accent) 16%, transparent);
    text-transform: uppercase;
    flex-shrink: 0;
  }
  .gp-kinds {
    font-size: 12px;
    color: var(--text-2);
  }
</style>
