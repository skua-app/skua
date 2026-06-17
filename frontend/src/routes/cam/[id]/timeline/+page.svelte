<!-- Phase 2b-i scaffold: this route exists only to mount HlsVideo against
     a fixed last-hour window so the recording VOD can be device-smoke-
     tested on iOS and Android. The real scrubber screen — driven start/end,
     gap rendering, markers — replaces this file in 2b-iii. Not advertised
     from the focus screen yet (that link lands in 2c). -->
<script lang="ts">
  import { page } from '$app/state'
  import { goto } from '$app/navigation'
  import { camerasStore } from '$lib/stores/cameras.svelte'
  import { timelineMasterURL } from '$lib/api'
  import HlsVideo from '$lib/components/HlsVideo.svelte'

  // page.params.id is typed string | undefined by SvelteKit's $types; the
  // route only matches when [id] is bound, so the empty fallback never
  // surfaces in practice — it just satisfies the string argument type
  // on timelineMasterURL.
  const camId = $derived(page.params.id ?? '')
  const camera = $derived(camerasStore.cameras.find((c) => c.id === camId) ?? null)

  // Temporary fixed window: last hour. 2b-iii will drive these from the
  // scrubber's selected range. End is wall-clock now; Frigate decides
  // whether the range actually has recording behind it.
  const end = Math.floor(Date.now() / 1000)
  const start = end - 3600
</script>

<div class="page">
  <header class="bar">
    <button type="button" class="back" onclick={() => goto(`/cam/${camId}`)}>← Live</button>
    <span class="title">{camera?.name ?? camId} · timeline (scaffold)</span>
  </header>

  <div class="frame">
    <HlsVideo src={timelineMasterURL(camId, start, end)} />
  </div>
</div>

<style>
  .page {
    display: flex;
    flex-direction: column;
    gap: 12px;
    padding: 12px;
    background: var(--bg);
    min-height: 100vh;
  }
  .bar {
    display: flex;
    align-items: center;
    gap: 12px;
    color: var(--text);
  }
  .back {
    appearance: none;
    background: var(--surface);
    color: var(--text);
    border: 1px solid var(--border);
    border-radius: var(--r-sm);
    height: var(--ctrl-h);
    padding: 0 12px;
    cursor: pointer;
  }
  .title {
    color: var(--text-2);
    font-size: 14px;
  }
  .frame {
    aspect-ratio: 16 / 9;
    width: 100%;
    border-radius: var(--r);
    overflow: hidden;
    background: var(--feed);
  }
</style>
