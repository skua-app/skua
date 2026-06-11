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
  let thumb: HTMLSpanElement | undefined = $state()
  let buttons: HTMLButtonElement[] = $state([])
  let measured = $state(false)

  function measure() {
    if (!container || !thumb) return
    const activeIdx = options.findIndex((o) => o.value === value)
    const btn = activeIdx >= 0 ? buttons[activeIdx] : undefined
    if (!btn) return
    const x = btn.offsetLeft
    const w = btn.offsetWidth
    const h = btn.offsetHeight
    if (w === 0) return
    if (!measured) {
      thumb.style.transition = 'none'
      thumb.style.transform = `translate(${x}px, ${btn.offsetTop}px)`
      thumb.style.width = `${w}px`
      thumb.style.height = `${h}px`
      void thumb.offsetWidth
      thumb.style.transition = ''
      measured = true
    } else {
      thumb.style.transform = `translate(${x}px, ${btn.offsetTop}px)`
      thumb.style.width = `${w}px`
      thumb.style.height = `${h}px`
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

<div class="seg ctrl" class:md={size === 'md'} bind:this={container}>
  <span class="seg-thumb" class:visible={measured} bind:this={thumb}></span>
  {#each options as opt, i (opt.value)}
    <button
      type="button"
      class="seg-opt"
      class:on={opt.value === value}
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
  .seg {
    position: relative;
    display: inline-flex;
    overflow: hidden;
    padding: 3px;
    gap: 2px;
    border-radius: 999px;
    height: var(--ctrl-h);
    box-sizing: border-box;
    background: var(--surface);
    border: 1px solid var(--border);
  }
  .seg-thumb {
    position: absolute;
    left: 0;
    top: 0;
    z-index: 0;
    border-radius: 999px;
    background: var(--accent-soft);
    opacity: 0;
    pointer-events: none;
    transition:
      transform 0.3s cubic-bezier(0.4, 0, 0.2, 1),
      width 0.3s cubic-bezier(0.4, 0, 0.2, 1);
  }
  .seg-thumb.visible {
    opacity: 1;
  }
  .seg-opt {
    position: relative;
    z-index: 1;
    -webkit-appearance: none;
    appearance: none;
    display: flex;
    align-items: center;
    padding: 0 14px;
    font-size: 13px;
    font-weight: 600;
    color: var(--text-2);
    background: transparent;
    border: none;
    border-radius: 999px;
    cursor: pointer;
    font-family: inherit;
    transition: color 0.2s ease;
  }
  .seg-opt.on {
    color: var(--accent);
  }
  .seg-opt:not(:disabled):active {
    transform: translateY(1px);
  }
  .seg-opt:disabled {
    color: var(--text-3);
    cursor: not-allowed;
  }
  .md .seg-opt {
    font-size: 14px;
    padding: 0 16px;
  }
  @media (prefers-reduced-motion: reduce) {
    .seg-thumb {
      transition: none;
    }
  }
</style>
