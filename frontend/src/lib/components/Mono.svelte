<script lang="ts">
  import type { Snippet } from 'svelte'

  type Props = {
    size?: number
    weight?: 400 | 500 | 600
    color?: string
    letterSpacing?: number
    uppercase?: boolean
    class?: string
    children: Snippet
  }

  let {
    size = 11,
    weight = 400,
    color = 'var(--text-3)',
    letterSpacing = 0,
    uppercase = false,
    class: className = '',
    children
  }: Props = $props()
</script>

<span
  class="mono {className}"
  class:uppercase
  style:--mono-size="{size}px"
  style:--mono-weight={weight}
  style:--mono-color={color}
  style:--mono-ls="{letterSpacing}px"
>
  {@render children()}
</span>

<style>
  .mono {
    font-family: 'JetBrains Mono', ui-monospace, monospace;
    font-variant-numeric: tabular-nums;
    white-space: nowrap;
    font-size: var(--mono-size);
    font-weight: var(--mono-weight);
    color: var(--mono-color);
    letter-spacing: var(--mono-ls);
  }
  .uppercase {
    text-transform: uppercase;
  }
</style>
