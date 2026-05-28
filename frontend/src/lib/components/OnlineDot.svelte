<script lang="ts">
  import { ui } from '$lib/i18n/strings'

  type Props = {
    online: boolean | null
    size?: number
  }

  let { online, size = 5 }: Props = $props()

  const label = $derived(online === null ? ui.statusUnknown : online ? ui.online : ui.offline)
</script>

<span
  role="img"
  aria-label={label}
  class="dot"
  class:online={online === true}
  class:offline={online === false}
  class:unknown={online === null}
  style:--dot-size="{size}px"
></span>

<style>
  .dot {
    display: inline-block;
    width: var(--dot-size);
    height: var(--dot-size);
    border-radius: 999px;
    flex-shrink: 0;
  }
  .online {
    background: var(--online);
    box-shadow: 0 0 0 3px color-mix(in oklab, var(--online) 18%, transparent);
  }
  .offline {
    background: var(--offline);
  }
  .unknown {
    background: transparent;
    box-shadow: inset 0 0 0 1px var(--text-3);
  }
</style>
