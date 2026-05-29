<script lang="ts" generics="T extends string">
  type Option = { value: T; label: string; disabled?: boolean }
  type Props = {
    value: T
    options: Option[]
    onChange: (v: T) => void
    size?: 'sm' | 'md'
  }

  let { value, options, onChange, size = 'sm' }: Props = $props()
</script>

<div class="segmented" class:md={size === 'md'}>
  {#each options as opt (opt.value)}
    <button
      type="button"
      class="seg"
      class:active={opt.value === value}
      disabled={opt.disabled}
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
    display: inline-flex;
    padding: 2px;
    border: 1px solid var(--border);
    border-radius: 8px;
    background: rgba(255, 255, 255, 0.015);
  }
  .seg {
    padding: 5px 10px;
    font-size: 11px;
    font-weight: 500;
    letter-spacing: 0.3px;
    color: var(--text-2);
    background: transparent;
    border: none;
    border-radius: 6px;
    cursor: pointer;
    transition:
      background 120ms,
      color 120ms;
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
    color: var(--bg);
    background: var(--accent);
  }
  .seg:disabled {
    color: var(--text-3);
    cursor: not-allowed;
  }
</style>
