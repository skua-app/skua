<script lang="ts">
  import { ICONS, type IconName } from '$lib/icons'

  type Props = {
    name: IconName
    size?: number
    class?: string
  }

  let { name, size = 18, class: className = '' }: Props = $props()

  const def = $derived(ICONS[name])
  const pathList = $derived.by(() => {
    if (!def.paths) return [] as string[]
    return Array.isArray(def.paths) ? def.paths : [def.paths]
  })
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
  {#each def.rects ?? [] as r}
    <rect x={r.x} y={r.y} width={r.width} height={r.height} rx={r.rx ?? 0} />
  {/each}
  {#each def.lines ?? [] as l}
    <line x1={l.x1} y1={l.y1} x2={l.x2} y2={l.y2} />
  {/each}
  {#each def.circles ?? [] as c}
    <circle
      cx={c.cx}
      cy={c.cy}
      r={c.r}
      fill={c.fill ?? 'none'}
      stroke={c.stroke ?? 'currentColor'}
    />
  {/each}
</svg>
