<script lang="ts">
  import { onMount } from 'svelte'
  import { pushState } from '$app/navigation'
  import { page } from '$app/state'
  import AboutSection from '$lib/components/settings/AboutSection.svelte'
  import AppearanceSection from '$lib/components/settings/AppearanceSection.svelte'
  import CamerasSection from '$lib/components/settings/CamerasSection.svelte'
  import ConnectionSection from '$lib/components/settings/ConnectionSection.svelte'
  import GroupsSection from '$lib/components/settings/GroupsSection.svelte'
  import MobileAppearance from '$lib/components/settings/MobileAppearance.svelte'
  import Icon from '$lib/components/Icon.svelte'
  import { camerasStore } from '$lib/stores/cameras.svelte'
  import { configStore } from '$lib/stores/config.svelte'
  import { go2rtcStreamsStore } from '$lib/stores/go2rtcStreams.svelte'
  import { groupsStore } from '$lib/stores/groups.svelte'
  import { runtimeConfigStore } from '$lib/stores/runtimeConfig.svelte'
  import { streamOverridesStore } from '$lib/stores/streamOverrides.svelte'
  import { ui } from '$lib/i18n/strings'
  import type { IconName } from '$lib/icons'

  type SectionId = 'appearance' | 'cameras' | 'groups' | 'connection' | 'about'
  type SectionItem = { id: SectionId; label: string; icon: IconName }
  type NavGroup = { label: string; items: SectionItem[] }

  // Mirrors the prototype's SET_NAV grouping. Storage slots into the
  // System group once GET /api/storage exists; that row is intentionally
  // deferred until the endpoint lands.
  const navGroups: NavGroup[] = [
    {
      label: ui.settingsGroupPersonalize,
      items: [{ id: 'appearance', label: ui.sectionAppearance, icon: 'sliders' }]
    },
    {
      label: ui.settingsGroupManage,
      items: [
        { id: 'cameras', label: ui.sectionCameras, icon: 'cams' },
        { id: 'groups', label: ui.sectionGroups, icon: 'grid' },
        { id: 'connection', label: ui.sectionConnection, icon: 'link' }
      ]
    },
    {
      label: ui.settingsGroupSystem,
      items: [{ id: 'about', label: ui.sectionAbout, icon: 'info' }]
    }
  ]

  let width = $state(0)
  const isDesktop = $derived(width >= 900)

  // Desktop: which pane is rendered. Default to Appearance.
  let activeSection = $state<SectionId>('appearance')

  // Mobile drillable sections — Appearance is rendered inline on the mobile
  // index, never as a drill-in. The remaining sections still drill from the
  // grouped nav rows below the inline Appearance block.
  type MobileDrillId = Exclude<SectionId, 'appearance'>
  type MobileView = 'index' | MobileDrillId

  // Mobile: 'index' shows the inline Appearance block plus the grouped row
  // list; otherwise we're drilled into a section. The current drill is held
  // in SvelteKit shallow-routing state so the iOS left-edge swipe (and
  // browser back) pops the drill before leaving the /settings route. On a
  // fresh load page.state is empty, so we resolve to 'index' — phones still
  // always land on the index.
  const mobileView = $derived<MobileView>(page.state.settingsSection ?? 'index')

  // Mobile nav rows: same shape as desktop, minus the Appearance row (and
  // therefore minus the now-empty Personalize group).
  const mobileNavGroups = navGroups
    .map((g) => ({ ...g, items: g.items.filter((it) => it.id !== 'appearance') }))
    .filter((g) => g.items.length > 0)

  function sectionLabel(id: SectionId): string {
    for (const g of navGroups) for (const it of g.items) if (it.id === id) return it.label
    return ''
  }

  // Mobile-only trailing value per section (prototype .set-row .chev). Desktop
  // nav stays text-only.
  function sectionValue(id: SectionId): string | null {
    if (id === 'cameras') return String(camerasStore.cameras.length)
    if (id === 'groups') return String(groupsStore.groups.length)
    if (id === 'about') return `v${configStore.version || 'dev'}`
    return null
  }

  // /settings-only stores are lazy-inited here so non-settings sessions don't
  // pay the shell-mount cost. groupsStore + camerasStore stay layout-inited.
  onMount(() => {
    streamOverridesStore.init()
    go2rtcStreamsStore.init()
    runtimeConfigStore.init()
  })
</script>

<svelte:window bind:innerWidth={width} />

{#snippet pane(id: SectionId)}
  {#if id === 'appearance'}
    <AppearanceSection />
  {:else if id === 'cameras'}
    <CamerasSection />
  {:else if id === 'groups'}
    <GroupsSection />
  {:else if id === 'connection'}
    <ConnectionSection />
  {:else if id === 'about'}
    <AboutSection />
  {/if}
{/snippet}

{#if isDesktop}
  <div class="dk-page wide">
    <div class="dk-pagehead">
      <div class="dk-h1">{ui.settings}</div>
    </div>
    <div class="dk-settings">
      <nav class="dk-setnav" aria-label={ui.settings}>
        {#each navGroups as g (g.label)}
          <div class="sn-group">{g.label}</div>
          {#each g.items as it (it.id)}
            {@const v = sectionValue(it.id)}
            <button
              type="button"
              class="dk-setitem"
              class:on={activeSection === it.id}
              aria-current={activeSection === it.id ? 'page' : undefined}
              onclick={() => (activeSection = it.id)}
            >
              <Icon name={it.icon} size={19} />
              <span class="dk-setitem-label">{it.label}</span>
              {#if v !== null}<span class="val">{v}</span>{/if}
            </button>
          {/each}
        {/each}
      </nav>
      <div class="dk-setpane">
        {@render pane(activeSection)}
      </div>
    </div>
  </div>
{:else if mobileView === 'index'}
  <div class="m-index">
    <div class="set-grouplabel first">{ui.sectionAppearance}</div>
    <MobileAppearance />
    {#each mobileNavGroups as g (g.label)}
      <div class="set-grouplabel">{g.label}</div>
      <div class="set-block">
        {#each g.items as it (it.id)}
          {@const v = sectionValue(it.id)}
          <button
            type="button"
            class="set-row"
            onclick={() => pushState('', { settingsSection: it.id as MobileDrillId })}
          >
            <span class="ico" aria-hidden="true"><Icon name={it.icon} size={22} /></span>
            <span class="set-row-label">{it.label}</span>
            <span class="chev">
              {#if v !== null}<span class="chev-val">{v}</span>{/if}
              <Icon name="chevDown" size={16} />
            </span>
          </button>
        {/each}
      </div>
    {/each}
  </div>
{:else}
  <div class="m-drill">
    <div class="drill-head">
      <button
        type="button"
        class="cbtn"
        onclick={() => history.back()}
        aria-label={ui.settingsBack}
      >
        <Icon name="back" size={21} />
      </button>
      <div class="crumb">
        <div class="title-lg">{sectionLabel(mobileView)}</div>
      </div>
      <span class="cbtn-spacer" aria-hidden="true"></span>
    </div>
    <div class="m-pane">
      {@render pane(mobileView)}
    </div>
  </div>
{/if}

<style>
  /* ============================================================
     DESKTOP — master / detail (dk-settings)
     ============================================================ */
  .dk-page {
    padding: 24px 28px calc(env(safe-area-inset-bottom, 0px) + 48px);
  }
  .dk-pagehead {
    display: flex;
    align-items: flex-end;
    justify-content: space-between;
    gap: 18px;
    margin-bottom: 20px;
  }
  .dk-h1 {
    font-size: 28px;
    font-weight: 600;
    letter-spacing: -0.6px;
    line-height: 1.05;
    color: var(--text);
  }
  .dk-settings {
    display: grid;
    grid-template-columns: 248px minmax(0, 1fr);
    gap: 28px;
    align-items: start;
  }
  .dk-setnav {
    position: sticky;
    top: calc(var(--app-header-h, 64px) + 16px);
    display: flex;
    flex-direction: column;
    gap: 3px;
  }
  .sn-group {
    padding: 16px 12px 6px;
    font-family: 'JetBrains Mono', ui-monospace, monospace;
    font-size: 10px;
    font-weight: 500;
    letter-spacing: 1.4px;
    text-transform: uppercase;
    color: var(--text-3);
  }
  .sn-group:first-child {
    padding-top: 0;
  }
  .dk-setitem {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 11px 12px;
    border: none;
    background: transparent;
    border-radius: 11px;
    font-family: inherit;
    font-size: 14.5px;
    color: var(--text-2);
    cursor: pointer;
    text-align: left;
    transition:
      color 0.14s ease,
      background 0.14s ease;
  }
  .dk-setitem-label {
    flex: 1 1 auto;
    min-width: 0;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .dk-setitem .val {
    margin-left: auto;
    font-family: 'JetBrains Mono', ui-monospace, monospace;
    font-size: 11px;
    color: var(--text-3);
    white-space: nowrap;
    flex: 0 0 auto;
  }
  .dk-setitem:hover {
    color: var(--text);
    background: var(--surface);
  }
  .dk-setitem.on {
    color: var(--accent-ink);
    background: var(--accent-soft);
  }
  .dk-setitem.on .val {
    color: var(--accent-ink);
  }
  .dk-setpane {
    min-width: 0;
    max-width: 860px;
  }

  /* ============================================================
     MOBILE — grouped index (calm .set-grouplabel + .set-row)
     ============================================================ */
  .m-index {
    max-width: 720px;
    margin: 0 auto;
    padding: 6px 18px calc(env(safe-area-inset-bottom, 0px) + 96px);
  }
  .set-grouplabel {
    padding: 20px 4px 6px;
    font-family: 'JetBrains Mono', ui-monospace, monospace;
    font-size: 10px;
    font-weight: 500;
    letter-spacing: 1.5px;
    text-transform: uppercase;
    color: var(--text-3);
  }
  /* First grouplabel sits below the sticky AppHeader; trim its top padding
     a little so it doesn't feel detached now that the in-page title is gone. */
  .set-grouplabel.first {
    padding-top: 12px;
  }
  .set-block {
    display: flex;
    flex-direction: column;
  }
  .set-row {
    display: flex;
    align-items: center;
    gap: 14px;
    padding: 16px 4px;
    background: transparent;
    border: none;
    border-bottom: 1px solid var(--border);
    color: var(--text);
    font-family: inherit;
    font-size: 16px;
    cursor: pointer;
    text-align: left;
    width: 100%;
  }
  .set-row:active {
    background: var(--surface);
  }
  .set-row .ico {
    width: 24px;
    flex: 0 0 24px;
    color: var(--text-2);
    display: inline-flex;
    justify-content: center;
  }
  .set-row-label {
    flex: 1 1 auto;
    min-width: 0;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .set-row .chev {
    margin-left: auto;
    color: var(--text-3);
    display: inline-flex;
    align-items: center;
    gap: 8px;
    font-family: 'JetBrains Mono', ui-monospace, monospace;
    font-size: 12px;
    white-space: nowrap;
    flex: 0 0 auto;
  }
  .set-row .chev :global(svg) {
    transform: rotate(-90deg);
  }
  .chev-val {
    color: var(--text-2);
  }

  /* ============================================================
     MOBILE — drilled-in pane (calm sub-screen head + section component)
     ============================================================ */
  .m-drill {
    max-width: 720px;
    margin: 0 auto;
    padding: 6px 18px calc(env(safe-area-inset-bottom, 0px) + 96px);
  }
  .drill-head {
    display: flex;
    align-items: center;
    gap: 12px;
    margin-bottom: 14px;
  }
  .cbtn {
    width: 42px;
    height: 42px;
    flex: 0 0 auto;
    display: grid;
    place-items: center;
    border-radius: var(--r-sm);
    border: 1px solid var(--border);
    background: var(--surface);
    color: var(--text);
    cursor: pointer;
    font-family: inherit;
  }
  .cbtn:active {
    transform: translateY(1px);
  }
  .cbtn-spacer {
    width: 42px;
    flex: 0 0 auto;
  }
  .crumb {
    flex: 1 1 auto;
    text-align: center;
    min-width: 0;
  }
  .title-lg {
    font-size: 18px;
    font-weight: 600;
    letter-spacing: -0.2px;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .m-pane {
    min-width: 0;
  }
</style>
