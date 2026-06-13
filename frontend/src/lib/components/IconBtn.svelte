<script lang="ts">
  import Icon from './Icon.svelte'
  import type { IconName } from '$lib/icons'

  type Props = {
    icon: IconName
    label: string
    size?: number
    active?: boolean
    accent?: boolean
    disabled?: boolean
    onclick?: (e: MouseEvent) => void
  }

  let {
    icon,
    label,
    size = 36,
    active = false,
    accent = false,
    disabled = false,
    onclick
  }: Props = $props()

  const iconSize = $derived(size > 32 ? 17 : 16)
</script>

<button
  type="button"
  {onclick}
  {disabled}
  aria-label={label}
  class="icon-btn"
  class:accent
  class:active
  style:--icon-btn-size="{size}px"
>
  <Icon name={icon} size={iconSize} />
</button>

<style>
  .icon-btn {
    width: var(--icon-btn-size);
    height: var(--icon-btn-size);
    display: inline-flex;
    align-items: center;
    justify-content: center;
    border-radius: 10px;
    border: 1px solid var(--border);
    background: transparent;
    color: var(--text-2);
    cursor: pointer;
    transition:
      background 120ms,
      color 120ms,
      border-color 120ms;
    flex-shrink: 0;
  }
  .icon-btn:hover:not(:disabled) {
    background: rgba(125, 125, 125, 0.1);
    color: var(--text);
  }
  .icon-btn.active {
    border-color: color-mix(in oklab, var(--accent) 50%, transparent);
    background: color-mix(in oklab, var(--accent) 14%, transparent);
    color: var(--accent);
  }
  .icon-btn.accent {
    border-color: color-mix(in oklab, var(--accent) 50%, transparent);
    background: color-mix(in oklab, var(--accent) 14%, transparent);
    color: var(--accent);
  }
  .icon-btn:disabled {
    opacity: 0.4;
    cursor: not-allowed;
  }
</style>
