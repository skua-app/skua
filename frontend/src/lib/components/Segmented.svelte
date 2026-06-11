<script lang="ts" generics="T extends string">
  type Option = { value: T; label: string; disabled?: boolean }
  type Props = {
    value: T
    options: Option[]
    onChange: (v: T) => void
    size?: 'sm' | 'md'
  }

  let { value, options, onChange, size = 'sm' }: Props = $props()

  let container: HTMLDivElement | undefined = $state()
  let thumb: HTMLDivElement | undefined = $state()
  let buttons: HTMLButtonElement[] = $state([])
  let measured = $state(false)

  function measure() {
    if (!container || !thumb) return
    const activeIdx = options.findIndex((o) => o.value === value)
    const btn = activeIdx >= 0 ? buttons[activeIdx] : undefined
    if (!btn) return
    const x = btn.offsetLeft
    const w = btn.offsetWidth
    if (w === 0) return
    if (!measured) {
      thumb.style.transition = 'none'
      thumb.style.transform = `translateX(${x}px)`
      thumb.style.width = `${w}px`
      // force layout flush so the next style change can transition
      void thumb.offsetWidth
      thumb.style.transition = ''
      measured = true
    } else {
      thumb.style.transform = `translateX(${x}px)`
      thumb.style.width = `${w}px`
    }
  }

  $effect(() => {
    void value
    void options
    void buttons.length
    measure()
  })

  $effect(() => {
    if (!container) return
    const ro = new ResizeObserver(() => measure())
    ro.observe(container)
    return () => ro.disconnect()
  })
</script>

<div class="segmented" class:md={size === 'md'} bind:this={container}>
  <div class="thumb" class:visible={measured} bind:this={thumb}></div>
  {#each options as opt, i (opt.value)}
    <button
      type="button"
      class="seg"
      class:active={opt.value === value}
      disabled={opt.disabled}
      bind:this={buttons[i]}
      onclick={() => {
        if (!opt.disabled) onChange(opt.value)
      }}
    >
      {opt.label}
    </button>
  {/each}
</div>

<style>
  .segmented {
    position: relative;
    display: inline-flex;
    padding: 2px;
    border: 1px solid var(--border);
    border-radius: 8px;
    background: rgba(255, 255, 255, 0.015);
  }
  .thumb {
    position: absolute;
    top: 2px;
    left: 0;
    height: calc(100% - 4px);
    border-radius: 6px;
    background: color-mix(in oklab, var(--accent) 14%, transparent);
    opacity: 0;
    pointer-events: none;
    z-index: 0;
    transition:
      transform 0.28s cubic-bezier(0.4, 0, 0.2, 1),
      width 0.28s cubic-bezier(0.4, 0, 0.2, 1);
  }
  .thumb.visible {
    opacity: 1;
  }
  .seg {
    position: relative;
    z-index: 1;
    padding: 5px 10px;
    font-size: 11px;
    font-weight: 500;
    letter-spacing: 0.3px;
    color: var(--text-2);
    background: transparent;
    border: none;
    border-radius: 6px;
    cursor: pointer;
    transition: color 120ms;
    font-family: inherit;
  }
  .md .seg {
    padding: 7px 14px;
    font-size: 12px;
  }
  .seg:not(:disabled):hover {
    color: var(--text);
  }
  .seg.active {
    color: var(--accent);
    font-weight: 600;
  }
  .seg:disabled {
    color: var(--text-3);
    cursor: not-allowed;
  }
  @media (prefers-reduced-motion: reduce) {
    .thumb {
      transition: none;
    }
  }
</style>
