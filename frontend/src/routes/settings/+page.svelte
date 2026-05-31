<script lang="ts">
  import { onMount } from 'svelte'
  import AppearanceSection from '$lib/components/settings/AppearanceSection.svelte'
  import CamerasSection from '$lib/components/settings/CamerasSection.svelte'
  import GroupsSection from '$lib/components/settings/GroupsSection.svelte'
  import ConnectionSection from '$lib/components/settings/ConnectionSection.svelte'
  import { streamOverridesStore } from '$lib/stores/streamOverrides.svelte'
  import { go2rtcStreamsStore } from '$lib/stores/go2rtcStreams.svelte'
  import { runtimeConfigStore } from '$lib/stores/runtimeConfig.svelte'
  import { ui } from '$lib/i18n/strings'

  let width = $state(0)
  const isDesktop = $derived(width >= 900)

  type SectionId = 'appearance' | 'cameras' | 'groups' | 'connection'
  const railItems: { id: SectionId; label: string }[] = [
    { id: 'appearance', label: ui.sectionAppearance },
    { id: 'cameras', label: ui.sectionCameras },
    { id: 'groups', label: ui.sectionGroups },
    { id: 'connection', label: ui.sectionConnection }
  ]
  let activeSection = $state<SectionId>('appearance')

  // /settings-only stores are lazy-inited here so non-settings sessions don't
  // pay the shell-mount cost. groupsStore + camerasStore stay layout-inited.
  onMount(() => {
    streamOverridesStore.init()
    go2rtcStreamsStore.init()
    runtimeConfigStore.init()
  })

  // Desktop scroll-spy: pick the section with the highest intersection ratio
  // within a band around the top third of the viewport.
  $effect(() => {
    if (!isDesktop || typeof IntersectionObserver === 'undefined') return
    const targets = railItems
      .map((item) => document.getElementById(item.id))
      .filter((el): el is HTMLElement => el !== null)
    if (targets.length === 0) return

    const observer = new IntersectionObserver(
      (entries) => {
        let best: { id: SectionId; ratio: number } | null = null
        for (const entry of entries) {
          if (!entry.isIntersecting) continue
          const id = entry.target.id as SectionId
          if (!best || entry.intersectionRatio > best.ratio) {
            best = { id, ratio: entry.intersectionRatio }
          }
        }
        if (best) activeSection = best.id
      },
      { rootMargin: '-30% 0px -60% 0px', threshold: [0, 0.25, 0.5, 0.75, 1] }
    )
    for (const t of targets) observer.observe(t)
    return () => observer.disconnect()
  })
</script>

<svelte:window bind:innerWidth={width} />

{#if isDesktop}
  <div class="settings-desktop">
    <aside class="settings-rail" aria-label={ui.settings}>
      {#each railItems as item (item.id)}
        <a
          href={`#${item.id}`}
          class="rail-link"
          class:active={item.id === activeSection}
          aria-current={item.id === activeSection ? 'true' : undefined}
        >
          {item.label}
        </a>
      {/each}
    </aside>
    <main class="settings-main">
      <AppearanceSection />
      <CamerasSection />
      <GroupsSection />
      <ConnectionSection />
    </main>
  </div>
{:else}
  <div class="settings-mobile">
    <AppearanceSection />
    <CamerasSection />
    <GroupsSection />
    <ConnectionSection />
  </div>
{/if}

<style>
  .settings-desktop {
    display: grid;
    grid-template-columns: 180px 1fr;
    gap: 32px;
    max-width: 940px;
    margin: 0 auto;
    padding: 24px 28px 96px;
  }
  .settings-rail {
    position: sticky;
    top: 24px;
    align-self: start;
    display: flex;
    flex-direction: column;
    gap: 4px;
  }
  .rail-link {
    padding: 8px 12px;
    font-size: 13px;
    color: var(--text-2);
    text-decoration: none;
    border-radius: 6px;
    border-left: 2px solid transparent;
    transition:
      color 120ms,
      background 120ms,
      border-color 120ms;
  }
  .rail-link:hover {
    color: var(--text);
    background: rgba(255, 255, 255, 0.03);
  }
  .rail-link.active {
    color: var(--text);
    border-left-color: var(--accent);
  }
  .settings-main {
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 36px;
  }
  .settings-mobile {
    max-width: none;
    padding: 18px 18px 96px;
    display: flex;
    flex-direction: column;
    gap: 32px;
  }
</style>
