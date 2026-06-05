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
      role="dialog"
      aria-modal="true"
      aria-label={title ?? ui.menuLabel}
      tabindex={-1}
      onclick={(e) => e.stopPropagation()}
      onkeydown={() => {}}
    >
      <div class="bs-handle" aria-hidden="true"></div>
      {#if title}
        <div class="bs-title">{title}</div>
      {/if}
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
  }
  .bs-handle {
    width: 36px;
    height: 4px;
    border-radius: 2px;
    background: var(--border-strong);
    margin: 6px auto 10px;
    flex: 0 0 auto;
  }
  .bs-title {
    padding: 0 20px 10px;
    font-size: 13px;
    font-weight: 600;
    color: var(--text);
    border-bottom: 1px solid var(--border);
    flex: 0 0 auto;
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
