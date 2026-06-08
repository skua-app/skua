<script lang="ts">
  import type { Snippet } from 'svelte'
  import { fade } from 'svelte/transition'
  import { cubicOut } from 'svelte/easing'
  import { ui } from '$lib/i18n/strings'

  type Props = {
    open: boolean
    onClose: () => void
    title?: string
    children: Snippet
  }

  let { open, onClose, title, children }: Props = $props()

  // Body scroll lock while the sheet is open. We toggle inline overflow on
  // <html> rather than swapping a class so the lock is self-contained.
  $effect(() => {
    if (typeof document === 'undefined') return
    if (open) {
      const prev = document.documentElement.style.overflow
      document.documentElement.style.overflow = 'hidden'
      return () => {
        document.documentElement.style.overflow = prev
      }
    }
  })

  function onKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape' && open) {
      e.preventDefault()
      onClose()
    }
  }

  function onBackdrop(e: MouseEvent) {
    if (e.target === e.currentTarget) onClose()
  }

  // Symmetric intro/outro: percentage-based slide of the full sheet
  // height. Svelte's built-in fly takes pixels only; this custom
  // transition lets us slide from translateY(100%) regardless of how
  // tall the sheet ends up.
  function slideUp(_node: Element, { duration = 200 }: { duration?: number } = {}) {
    return {
      duration,
      easing: cubicOut,
      css: (t: number) => `transform: translateY(${(1 - t) * 100}%)`
    }
  }

  // Hand-rolled swipe-to-dismiss. Bound to the pinned header (handle +
  // title + close button), never to the scrolling content, so it can't
  // fight the list's vertical scroll. Pointer events handle touch + mouse
  // uniformly; touch-action: none on the header keeps iOS from stealing
  // the gesture for native scroll / pull-to-refresh.
  let dragOffset = $state(0)
  let dragging = $state(false)
  let snapping = $state(false)
  let dragStartY = 0
  let dragStartTime = 0
  const CLOSE_DISTANCE_PX = 80
  const FLICK_DISTANCE_PX = 24
  const FLICK_VELOCITY_PX_PER_MS = 0.6

  // Asymptotic damping on upward drag: the sheet "gives" a little and
  // never crosses the rubber-band ceiling, so a flick up always
  // springs back to 0 on release.
  const RUBBER_BAND_MAX_PX = 50
  function rubberBand(absDelta: number): number {
    return RUBBER_BAND_MAX_PX * (1 - 1 / (1 + absDelta / RUBBER_BAND_MAX_PX))
  }

  // Reset the drag state whenever the sheet is closed so the next open
  // starts from translateY(0).
  $effect(() => {
    if (!open) {
      dragOffset = 0
      dragging = false
      snapping = false
    }
  })

  function onHeaderPointerDown(e: PointerEvent) {
    // Let the close button handle its own click without starting a drag.
    if ((e.target as HTMLElement | null)?.closest('.bs-close')) return
    // Primary input only.
    if (e.pointerType === 'mouse' && e.button !== 0) return
    const target = e.currentTarget as HTMLElement
    target.setPointerCapture(e.pointerId)
    dragging = true
    snapping = false
    dragStartY = e.clientY
    dragStartTime = performance.now()
    dragOffset = 0
  }

  function onHeaderPointerMove(e: PointerEvent) {
    if (!dragging) return
    const delta = e.clientY - dragStartY
    dragOffset = delta > 0 ? delta : -rubberBand(-delta)
  }

  function onHeaderPointerUp(e: PointerEvent) {
    if (!dragging) return
    const target = e.currentTarget as HTMLElement
    if (target.hasPointerCapture(e.pointerId)) {
      target.releasePointerCapture(e.pointerId)
    }
    const elapsed = Math.max(performance.now() - dragStartTime, 1)
    const velocity = dragOffset / elapsed
    const shouldClose =
      dragOffset >= CLOSE_DISTANCE_PX ||
      (dragOffset >= FLICK_DISTANCE_PX && velocity >= FLICK_VELOCITY_PX_PER_MS)
    dragging = false
    if (shouldClose) {
      // Hand the transform back to the out-transition. Clearing the
      // inline drag transform first stops it fighting slideUp during
      // unmount.
      dragOffset = 0
      onClose()
    } else {
      snapping = true
      dragOffset = 0
      setTimeout(() => {
        snapping = false
      }, 220)
    }
  }

  function onHeaderPointerCancel() {
    if (!dragging) return
    dragging = false
    dragOffset = 0
  }
</script>

<svelte:window onkeydown={onKeydown} />

{#if open}
  <div
    class="bs-backdrop"
    role="presentation"
    onclick={onBackdrop}
    onkeydown={() => {}}
    aria-hidden="true"
    transition:fade={{ duration: 160 }}
  >
    <div
      class="bs-sheet"
      class:snapping
      style:transform={dragOffset !== 0 ? `translateY(${dragOffset}px)` : null}
      role="dialog"
      aria-modal="true"
      aria-label={title ?? ui.menuLabel}
      tabindex={-1}
      onclick={(e) => e.stopPropagation()}
      onkeydown={() => {}}
      transition:slideUp={{ duration: 200 }}
    >
      <div
        class="bs-header"
        role="presentation"
        onpointerdown={onHeaderPointerDown}
        onpointermove={onHeaderPointerMove}
        onpointerup={onHeaderPointerUp}
        onpointercancel={onHeaderPointerCancel}
      >
        <div class="bs-handle" aria-hidden="true"></div>
        <div class="bs-header-row" class:no-title={!title}>
          {#if title}
            <div class="bs-title">{title}</div>
          {/if}
          <button type="button" class="bs-close" aria-label={ui.close} onclick={onClose}>
            <span aria-hidden="true">×</span>
          </button>
        </div>
      </div>
      <div class="bs-content">
        {@render children()}
      </div>
    </div>
  </div>
{/if}

<style>
  .bs-backdrop {
    position: fixed;
    inset: 0;
    z-index: 100;
    background: rgba(0, 0, 0, 0.55);
    display: flex;
    flex-direction: column;
    justify-content: flex-end;
  }
  .bs-sheet {
    background: #15171a;
    border-top-left-radius: 16px;
    border-top-right-radius: 16px;
    border-top: 1px solid var(--border);
    padding: 6px 0 calc(env(safe-area-inset-bottom, 0px) + 12px);
    max-height: 60vh;
    display: flex;
    flex-direction: column;
    overflow: hidden;
  }
  /* Snap-back transition is applied only when releasing the drag and
     letting it spring to 0 — never during the drag itself, and never
     during the slideUp intro/outro (which writes transform via the
     transition system and would stutter against a CSS transition). */
  .bs-sheet.snapping {
    transition: transform 200ms cubic-bezier(0.2, 0.8, 0.2, 1);
  }
  .bs-header {
    flex: 0 0 auto;
    touch-action: none;
    cursor: grab;
    user-select: none;
  }
  .bs-header:active {
    cursor: grabbing;
  }
  .bs-handle {
    width: 36px;
    height: 4px;
    border-radius: 2px;
    background: var(--border-strong);
    margin: 6px auto 10px;
  }
  .bs-header-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
    padding: 0 10px 10px 20px;
    border-bottom: 1px solid var(--border);
    min-height: 28px;
  }
  .bs-header-row.no-title {
    justify-content: flex-end;
    border-bottom: none;
    padding-bottom: 0;
  }
  .bs-title {
    font-size: 13px;
    font-weight: 600;
    color: var(--text);
  }
  .bs-close {
    width: 28px;
    height: 28px;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    padding: 0;
    color: var(--text-2);
    background: transparent;
    border: none;
    border-radius: 6px;
    cursor: pointer;
    font-family: inherit;
    font-size: 20px;
    line-height: 1;
    transition:
      color 120ms,
      background 120ms;
  }
  .bs-close:hover {
    color: var(--text);
    background: rgba(255, 255, 255, 0.06);
  }
  .bs-content {
    padding: 8px 0;
    flex: 1 1 auto;
    overflow-y: auto;
    /* min-height: 0 unlocks the flex item so its scrollable child can
       actually overflow rather than expanding the parent past max-height. */
    min-height: 0;
  }

  /* Desktop: the full-bleed bottom sheet looks wrong on wide viewports,
     so above the grid breakpoint (matches +layout.svelte's isDesktop ≥
     900px) we centre the sheet horizontally and cap its width, turning
     it into a floating card docked just above the bottom edge. The
     slide-up + swipe-to-dismiss behaviour is preserved because
     translateY-based animation and the pointer-drag logic operate on
     the sheet's own bounds. */
  @media (min-width: 900px) {
    .bs-backdrop {
      align-items: center;
    }
    .bs-sheet {
      width: 100%;
      max-width: 480px;
      margin-bottom: 16px;
      border: 1px solid var(--border);
      border-radius: 16px;
    }
  }
</style>
