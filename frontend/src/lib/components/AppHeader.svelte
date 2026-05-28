<script lang="ts">
  import { page } from '$app/state'
  import BottomSheet from '$lib/components/BottomSheet.svelte'
  import Icon from '$lib/components/Icon.svelte'
  import Mono from '$lib/components/Mono.svelte'
  import OnlineDot from '$lib/components/OnlineDot.svelte'
  import Segmented from '$lib/components/Segmented.svelte'
  import { camerasStore } from '$lib/stores/cameras.svelte'
  import { groupsStore } from '$lib/stores/groups.svelte'
  import { prefsStore } from '$lib/stores/prefs.svelte'
  import { ui } from '$lib/i18n/strings'

  type Props = { isDesktop: boolean }
  let { isDesktop }: Props = $props()

  // Pages with their own sticky sub-bar (e.g. MobileGrid's HD/ECO row)
  // anchor with `top: var(--app-header-h)`. We publish the rendered height
  // to documentElement via ResizeObserver so the safe-area inset that lives
  // inside this header is counted too.
  let mobileHeader = $state<HTMLElement | null>(null)
  $effect(() => {
    if (isDesktop || !mobileHeader) return
    const ro = new ResizeObserver((entries) => {
      const entry = entries[0]
      if (!entry) return
      document.documentElement.style.setProperty(
        '--app-header-h',
        `${Math.round(entry.contentRect.height)}px`
      )
    })
    ro.observe(mobileHeader)
    return () => {
      ro.disconnect()
      document.documentElement.style.removeProperty('--app-header-h')
    }
  })

  const cameras = $derived(camerasStore.cameras)
  const onlineCount = $derived(cameras.filter((c) => c.online).length)
  const offlineCount = $derived(cameras.length - onlineCount)

  const navItems = [
    { label: ui.cameras, href: '/' },
    { label: ui.events, href: '/events' },
    { label: ui.settings, href: '/settings' }
  ]

  function isActiveNav(href: string): boolean {
    const id = page.route.id ?? ''
    if (href === '/') return id === '/'
    return id === href || id.startsWith(href + '/')
  }

  const titleByRoute: Record<string, string> = {
    '/': ui.cameras,
    '/events': ui.events,
    '/settings': ui.settings
  }
  const mobileTitle = $derived(titleByRoute[page.route.id ?? ''] ?? '')
  const showGridMode = $derived(page.route.id === '/')
  const showGridFilter = $derived(page.route.id === '/')
  const activeGroup = $derived(
    prefsStore.gridFilter ? groupsStore.groups.find((g) => g.id === prefsStore.gridFilter) : null
  )

  let filterSheetOpen = $state(false)

  function selectGroup(id: string | null) {
    prefsStore.setGridFilter(id)
    filterSheetOpen = false
  }

  function cameraCount(groupId: string): number {
    const g = groupsStore.byId(groupId)
    return g?.camera_ids.length ?? 0
  }

  // Local options because Segmented values are string-typed and the prefs
  // store wants the union; mapping is straightforward.
  const gridModeOptions = [
    { value: 'hd' as const, label: 'HD' },
    { value: 'eco' as const, label: 'ECO' }
  ]
</script>

{#if isDesktop}
  <header class="ah-desktop">
    <div class="ah-d-left">
      <div class="logo-mark" aria-hidden="true">
        <span class="bracket tl"></span>
        <span class="bracket tr"></span>
        <span class="bracket bl"></span>
        <span class="bracket br"></span>
      </div>
      <span class="ah-brand">skua</span>
      <Mono color="var(--text-3)" size={11}>example.com</Mono>
      <nav class="ah-nav" aria-label={ui.primaryNavLabel}>
        {#each navItems as item (item.href)}
          <a
            href={item.href}
            class="ah-nav-item"
            class:active={isActiveNav(item.href)}
            aria-current={isActiveNav(item.href) ? 'page' : undefined}
          >
            {item.label}
          </a>
        {/each}
      </nav>
    </div>

    <div class="ah-d-right">
      <span class="status-item">
        <OnlineDot online={true} size={5} />
        <Mono size={10} color="var(--text-2)" letterSpacing={0.5} uppercase>
          {onlineCount}/{cameras.length}
          {ui.online}
        </Mono>
      </span>
    </div>
  </header>
{:else}
  <header class="ah-mobile" bind:this={mobileHeader}>
    <div class="ah-m-title-row">
      <div class="ah-m-title-left">
        {#if showGridFilter && groupsStore.groups.length > 0}
          <button
            type="button"
            class="ah-m-filter-btn"
            class:active={activeGroup !== null}
            aria-label={ui.groupFilterLabel}
            aria-expanded={filterSheetOpen}
            onclick={() => (filterSheetOpen = true)}
          >
            <Icon name="filter" size={14} />
            {#if activeGroup}
              <span class="ah-m-filter-label">{activeGroup.name}</span>
            {/if}
          </button>
        {/if}
        <span class="ah-m-title">{mobileTitle}</span>
      </div>
      {#if showGridMode}
        <Segmented
          value={prefsStore.gridMode}
          options={gridModeOptions}
          onChange={(v) => prefsStore.setGridMode(v)}
        />
      {/if}
    </div>
    <div class="ah-m-status-row">
      <div class="ah-m-status-left">
        <span class="status-item">
          <OnlineDot online={true} size={5} />
          <Mono color="var(--text-2)" size={10} letterSpacing={0.6} uppercase>
            {onlineCount}
            {ui.online}
          </Mono>
        </span>
        {#if offlineCount > 0}
          <span class="status-item">
            <OnlineDot online={false} size={5} />
            <Mono color="var(--text-3)" size={10} letterSpacing={0.6} uppercase>
              {offlineCount}
              {ui.offline}
            </Mono>
          </span>
        {/if}
      </div>
      <Mono color="var(--text-3)" size={10}>skua.example.com</Mono>
    </div>
  </header>

  <BottomSheet
    open={filterSheetOpen}
    onClose={() => (filterSheetOpen = false)}
    title={ui.cameraGroupTitle}
  >
    <ul class="ah-filter-list" role="listbox">
      <li>
        <button
          type="button"
          class="ah-filter-row"
          class:selected={prefsStore.gridFilter === null}
          role="option"
          aria-selected={prefsStore.gridFilter === null}
          onclick={() => selectGroup(null)}
        >
          <span class="ah-filter-name">
            {ui.filterAll}
            {#if prefsStore.gridFilter === null}
              <span class="ah-filter-check" aria-hidden="true">✓</span>
            {/if}
          </span>
          <span class="ah-filter-count">{camerasStore.cameras.length} {ui.camerasCountWord}</span>
        </button>
      </li>
      {#each groupsStore.groups as g (g.id)}
        <li>
          <button
            type="button"
            class="ah-filter-row"
            class:selected={prefsStore.gridFilter === g.id}
            role="option"
            aria-selected={prefsStore.gridFilter === g.id}
            onclick={() => selectGroup(g.id)}
          >
            <span class="ah-filter-name">
              {g.name}
              {#if prefsStore.gridFilter === g.id}
                <span class="ah-filter-check" aria-hidden="true">✓</span>
              {/if}
            </span>
            <span class="ah-filter-count">{cameraCount(g.id)} {ui.camerasCountWord}</span>
          </button>
        </li>
      {/each}
    </ul>
  </BottomSheet>
{/if}

<style>
  /* Mobile header — sticky so it stays pinned while the grid scrolls.
     Carries its own safe-area-inset-top so `top: 0` lands below the iOS
     notch in PWA standalone (body no longer pads for it). Translucent +
     blur match MobileTabBar's style. */
  .ah-mobile {
    position: sticky;
    top: 0;
    z-index: 30;
    padding-top: env(safe-area-inset-top);
    background: rgba(10, 11, 13, 0.85);
    backdrop-filter: blur(20px);
  }
  .ah-m-title-row {
    padding: 6px 18px 14px;
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
    min-height: 32px;
  }
  .ah-m-title {
    font-size: 19px;
    font-weight: 600;
    letter-spacing: -0.4px;
  }
  .ah-m-title-left {
    display: inline-flex;
    align-items: center;
    gap: 10px;
    min-width: 0;
  }
  .ah-m-filter-btn {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 4px 8px;
    font-size: 12px;
    font-family: inherit;
    color: var(--text-2);
    background: transparent;
    border: 1px solid var(--border);
    border-radius: 6px;
    cursor: pointer;
    transition:
      color 120ms,
      border-color 120ms,
      background 120ms;
  }
  .ah-m-filter-btn:hover {
    color: var(--text);
    border-color: var(--border-strong);
    background: rgba(255, 255, 255, 0.04);
  }
  .ah-m-filter-btn.active {
    color: var(--accent);
    border-color: color-mix(in oklab, var(--accent) 50%, transparent);
    background: color-mix(in oklab, var(--accent) 14%, transparent);
  }
  .ah-m-filter-label {
    max-width: 96px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .ah-filter-list {
    list-style: none;
    padding: 0;
    margin: 0;
    display: flex;
    flex-direction: column;
  }
  .ah-filter-row {
    display: flex;
    justify-content: space-between;
    align-items: center;
    width: 100%;
    padding: 14px 20px;
    background: transparent;
    border: none;
    border-bottom: 1px solid var(--border);
    font-family: inherit;
    font-size: 14px;
    color: var(--text);
    cursor: pointer;
    text-align: left;
  }
  .ah-filter-row:hover {
    background: rgba(255, 255, 255, 0.03);
  }
  .ah-filter-row.selected {
    color: var(--accent);
    background: color-mix(in oklab, var(--accent) 14%, transparent);
  }
  .ah-filter-name {
    display: inline-flex;
    align-items: center;
    gap: 8px;
    font-weight: 500;
  }
  .ah-filter-check {
    font-size: 13px;
    line-height: 1;
    color: var(--accent);
  }
  .ah-filter-count {
    font-size: 11px;
    color: var(--text-3);
  }
  .ah-m-status-row {
    padding: 0 18px 12px;
    display: flex;
    align-items: center;
    justify-content: space-between;
    border-bottom: 1px solid var(--border);
  }
  .ah-m-status-left {
    display: flex;
    align-items: center;
    gap: 14px;
  }

  /* Desktop header */
  .ah-desktop {
    padding: 14px 28px;
    display: flex;
    align-items: center;
    justify-content: space-between;
    border-bottom: 1px solid var(--border);
    background: var(--bg);
  }
  .ah-d-left {
    display: flex;
    align-items: center;
    gap: 32px;
  }
  .ah-d-left > :global(*) {
    display: inline-flex;
    align-items: center;
  }
  .ah-d-left .ah-brand,
  .ah-d-left .logo-mark {
    align-items: baseline;
  }
  .ah-d-right {
    display: flex;
    align-items: center;
    gap: 14px;
  }

  .logo-mark {
    width: 14px;
    height: 14px;
    position: relative;
    display: inline-block;
  }
  .bracket {
    position: absolute;
    width: 5px;
    height: 5px;
  }
  .tl {
    top: 0;
    left: 0;
    border-top: 1.5px solid var(--accent);
    border-left: 1.5px solid var(--accent);
  }
  .tr {
    top: 0;
    right: 0;
    border-top: 1.5px solid var(--accent);
    border-right: 1.5px solid var(--accent);
  }
  .bl {
    bottom: 0;
    left: 0;
    border-bottom: 1.5px solid var(--accent);
    border-left: 1.5px solid var(--accent);
  }
  .br {
    bottom: 0;
    right: 0;
    border-bottom: 1.5px solid var(--accent);
    border-right: 1.5px solid var(--accent);
  }

  .ah-brand {
    font-size: 15px;
    font-weight: 600;
    letter-spacing: -0.2px;
    margin-left: 10px;
  }
  .ah-nav {
    display: flex;
    gap: 22px;
    margin-left: 22px;
  }
  .ah-nav-item {
    font-size: 13px;
    font-weight: 500;
    color: var(--text-2);
    text-decoration: none;
    padding-bottom: 4px;
    border-bottom: 1px solid transparent;
    transition:
      color 120ms,
      border-color 120ms;
  }
  .ah-nav-item:hover {
    color: var(--text);
  }
  .ah-nav-item.active {
    color: var(--text);
    border-bottom-width: 2px;
    border-bottom-color: var(--accent);
  }

  .status-item {
    display: inline-flex;
    align-items: center;
    gap: 7px;
  }
</style>
