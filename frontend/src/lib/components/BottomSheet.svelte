<script lang="ts">
  import type { Snippet } from 'svelte'
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

  // Hand-rolled swipe-to-dismiss. Bound to the pinned header (handle +
  // title + close button), never to the scrolling content, so it can't
  // fight the list's vertical scroll. Pointer events handle touch + mouse
  // uniformly; touch-action: none on the header keeps iOS from stealing
  // the gesture for native scroll / pull-to-refresh.
  let dragOffset = $state(0)
  let dragging = $state(false)
  let dragStartY = 0
  let dragStartTime = 0
  const CLOSE_DISTANCE_PX = 80
  const FLICK_DISTANCE_PX = 24
  const FLICK_VELOCITY_PX_PER_MS = 0.6

  // Reset the drag state whenever the sheet is closed so the next open
  // starts from translateY(0).
  $effect(() => {
    if (!open) {
      dragOffset = 0
      dragging = false
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
    dragStartY = e.clientY
    dragStartTime = performance.now()
    dragOffset = 0
  }

  function onHeaderPointerMove(e: PointerEvent) {
    if (!dragging) return
    const delta = e.clientY - dragStartY
    dragOffset = delta > 0 ? delta : 0
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
      onClose()
    } else {
      dragOffset = 0
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
  >
    <div
      class="bs-sheet"
      class:dragging
      style:transform={dragOffset > 0 ? `translateY(${dragOffset}px)` : null}
      role="dialog"
      aria-modal="true"
      aria-label={title ?? ui.menuLabel}
      tabindex={-1}
      onclick={(e) => e.stopPropagation()}
      onkeydown={() => {}}
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
    animation: bs-fade 160ms ease;
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
    animation: bs-slide 200ms cubic-bezier(0.2, 0.8, 0.2, 1);
    transition: transform 200ms cubic-bezier(0.2, 0.8, 0.2, 1);
  }
  .bs-sheet.dragging {
    animation: none;
    transition: none;
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

  @keyframes bs-slide {
    from {
      transform: translateY(100%);
    }
    to {
      transform: translateY(0);
    }
  }
  @keyframes bs-fade {
    from {
      opacity: 0;
    }
    to {
      opacity: 1;
    }
  }
</style>
