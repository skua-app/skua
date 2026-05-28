<script lang="ts">
  import { ICONS, type IconName } from '$lib/icons'

  type Props = {
    name: IconName
    size?: number
    class?: string
  }

  let { name, size = 18, class: className = '' }: Props = $props()

  const def = $derived(ICONS[name])
  const pathList = $derived(Array.isArray(def.paths) ? def.paths : [def.paths])
</script>

<svg
  width={size}
  height={size}
  viewBox="0 0 24 24"
  fill={def.fill ?? 'none'}
  stroke={def.stroke ?? 'currentColor'}
  stroke-width={def.strokeWidth ?? 1.5}
  stroke-linecap="round"
  stroke-linejoin="round"
  class={className}
  aria-hidden="true"
>
  {#each pathList as d}
    <path {d} />
  {/each}
</svg>
