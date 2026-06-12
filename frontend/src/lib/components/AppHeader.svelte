<script lang="ts">
  import { page } from '$app/state'
  import Icon from '$lib/components/Icon.svelte'
  import Mono from '$lib/components/Mono.svelte'
  import Segmented from '$lib/components/Segmented.svelte'
  import { camerasStore } from '$lib/stores/cameras.svelte'
  import { glanceStore } from '$lib/stores/glance.svelte'
  import { groupsStore } from '$lib/stores/groups.svelte'
  import { prefsStore } from '$lib/stores/prefs.svelte'
  import { ui } from '$lib/i18n/strings'
  import type { MobileColumns } from '$lib/api'
  import type { IconName } from '$lib/icons'

  type Props = { isDesktop: boolean }
  let { isDesktop }: Props = $props()

  // Pages with their own sticky sub-bar anchor with `top: var(--app-header-h)`.
  // Publish the rendered mobile-header height (incl. the safe-area inset that
  // lives inside this header) to documentElement via ResizeObserver.
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

  let host = $state('')
  $effect(() => {
    host = window.location.host
  })

  const cameras = $derived(camerasStore.cameras)
  const onlineCount = $derived(cameras.filter((c) => c.online).length)
  const offlineCount = $derived(cameras.length - onlineCount)

  const navItems: { label: string; href: string; icon: IconName }[] = [
    { label: ui.cameras, href: '/', icon: 'cams' },
    { label: ui.events, href: '/events', icon: 'events' },
    { label: ui.settings, href: '/settings', icon: 'settings' }
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
  const showControlBar = $derived(page.route.id === '/')
  const activeGroup = $derived(
    prefsStore.gridFilter ? groupsStore.groups.find((g) => g.id === prefsStore.gridFilter) : null
  )

  let filterOpen = $state(false)
  function selectGroup(id: string | null) {
    prefsStore.setGridFilter(id)
    filterOpen = false
  }
  function cameraCount(groupId: string): number {
    return groupsStore.byId(groupId)?.camera_ids.length ?? 0
  }
  $effect(() => {
    if (!filterOpen) return
    function onKey(e: KeyboardEvent) {
      if (e.key === 'Escape') filterOpen = false
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  })

  const gridModeOptions = [
    { value: 'hd' as const, label: 'HD' },
    { value: 'eco' as const, label: 'ECO' }
  ]

  // gridtog: density toggle with the calm slide-thumb. Same logic family as
  // Segmented's measure(), but the buttons hold icons (not text) and the
  // thumb size tracks the active button's offsetWidth/Height.
  let gridtogEl: HTMLDivElement | undefined = $state()
  let gridtogThumb: HTMLSpanElement | undefined = $state()
  let gtButtons: HTMLButtonElement[] = $state([])
  let gtMeasured = $state(false)

  function placeGridtog() {
    if (!gridtogEl || !gridtogThumb) return
    const idx = prefsStore.mobileColumns === 1 ? 0 : 1
    const btn = gtButtons[idx]
    if (!btn || btn.offsetWidth === 0) return
    if (!gtMeasured) {
      gridtogThumb.style.transition = 'none'
      gridtogThumb.style.transform = `translate(${btn.offsetLeft}px, ${btn.offsetTop}px)`
      gridtogThumb.style.width = `${btn.offsetWidth}px`
      gridtogThumb.style.height = `${btn.offsetHeight}px`
      void gridtogThumb.offsetWidth
      gridtogThumb.style.transition = ''
      gtMeasured = true
    } else {
      gridtogThumb.style.transform = `translate(${btn.offsetLeft}px, ${btn.offsetTop}px)`
      gridtogThumb.style.width = `${btn.offsetWidth}px`
      gridtogThumb.style.height = `${btn.offsetHeight}px`
    }
  }
  $effect(() => {
    void prefsStore.mobileColumns
    void gtButtons.length
    placeGridtog()
  })
  $effect(() => {
    if (!gridtogEl) return
    const ro = new ResizeObserver(() => placeGridtog())
    ro.observe(gridtogEl)
    return () => ro.disconnect()
  })
</script>

{#if isDesktop}
  <header class="dk-top">
    <div class="dk-brand">
      <span class="dk-mark" aria-hidden="true">
        <span class="logo-mark">
          <span class="bracket tl"></span>
          <span class="bracket tr"></span>
          <span class="bracket bl"></span>
          <span class="bracket br"></span>
        </span>
      </span>
      <span class="dk-wordmark">Skua</span>
    </div>
    <nav class="dk-tabs" aria-label={ui.primaryNavLabel}>
      {#each navItems as item (item.href)}
        <a
          href={item.href}
          class="dk-tab"
          class:on={isActiveNav(item.href)}
          aria-current={isActiveNav(item.href) ? 'page' : undefined}
        >
          <Icon name={item.icon} size={18} />
          <span class="tl">{item.label}</span>
        </a>
      {/each}
    </nav>
    <span class="spacer" aria-hidden="true"></span>
    <div class="dk-actions">
      {#if glanceStore.loaded && (glanceStore.unseenCount > 0 || glanceStore.moments.length > 0)}
        <button
          type="button"
          class="dk-iconbtn"
          class:quiet={glanceStore.unseenCount === 0}
          aria-label={ui.glanceBellLabel}
          onclick={() => glanceStore.openPeek()}
        >
          <Icon name="bell" size={20} />
          {#if glanceStore.unseenCount > 0}
            <span class="badge">{glanceStore.unseenCount}</span>
          {/if}
        </button>
      {/if}
    </div>
  </header>
{:else}
  <header class="ah-mobile sticky-head" bind:this={mobileHeader}>
    <div class="head-row">
      <div class="head-title">
        <span class="title-xl">{mobileTitle}</span>
        <span class="head-caption">
          <span class="gd" aria-hidden="true"></span>
          {onlineCount}
          {ui.online} · {offlineCount}
          {ui.offline}{host ? ` · ${host}` : ''}
        </span>
      </div>
      {#if glanceStore.loaded && (glanceStore.unseenCount > 0 || glanceStore.moments.length > 0)}
        <button
          type="button"
          class="bell"
          class:quiet={glanceStore.unseenCount === 0}
          aria-label={ui.glanceBellLabel}
          onclick={() => glanceStore.openPeek()}
        >
          <Icon name="bell" size={22} />
          {#if glanceStore.unseenCount > 0}
            <span class="badge">{glanceStore.unseenCount}</span>
          {/if}
        </button>
      {/if}
    </div>

    {#if showControlBar}
      <div class="control-bar">
        <div class="grpfilter">
          <button
            type="button"
            class="ctrl filter-btn"
            class:open={filterOpen}
            aria-label={ui.groupFilterLabel}
            aria-haspopup="listbox"
            aria-expanded={filterOpen}
            onclick={() => (filterOpen = !filterOpen)}
          >
            <span class="fb-label">{activeGroup ? activeGroup.name : ui.filterAllCams}</span>
            <span class="fb-chev" aria-hidden="true"><Icon name="chevDown" size={16} /></span>
          </button>
          {#if filterOpen}
            <button
              type="button"
              class="grp-catch"
              tabindex={-1}
              aria-label={ui.close}
              onclick={() => (filterOpen = false)}
            ></button>
            <ul class="grp-menu" role="listbox">
              <li>
                <button
                  type="button"
                  class="grp-opt"
                  class:on={prefsStore.gridFilter === null}
                  role="option"
                  aria-selected={prefsStore.gridFilter === null}
                  onclick={() => selectGroup(null)}
                >
                  <span class="check" aria-hidden="true">
                    {#if prefsStore.gridFilter === null}<Icon name="check" size={15} />{/if}
                  </span>
                  <span class="g-name">{ui.filterAllCams}</span>
                  <Mono size={11} color="inherit" class="g-count">{cameras.length}</Mono>
                </button>
              </li>
              {#each groupsStore.groups as g (g.id)}
                <li>
                  <button
                    type="button"
                    class="grp-opt"
                    class:on={prefsStore.gridFilter === g.id}
                    role="option"
                    aria-selected={prefsStore.gridFilter === g.id}
                    onclick={() => selectGroup(g.id)}
                  >
                    <span class="check" aria-hidden="true">
                      {#if prefsStore.gridFilter === g.id}<Icon name="check" size={15} />{/if}
                    </span>
                    <span class="g-name">{g.name}</span>
                    <Mono size={11} color="inherit" class="g-count">{cameraCount(g.id)}</Mono>
                  </button>
                </li>
              {/each}
            </ul>
          {/if}
        </div>
        <div class="control-right">
          <Segmented
            value={prefsStore.gridMode}
            options={gridModeOptions}
            onChange={(v) => prefsStore.setGridMode(v)}
          />
          <div
            class="ctrl gridtog"
            bind:this={gridtogEl}
            role="group"
            aria-label={ui.gridDensityLabel}
          >
            <span class="seg-thumb" class:visible={gtMeasured} bind:this={gridtogThumb}></span>
            <button
              type="button"
              class="gt-btn"
              class:on={prefsStore.mobileColumns === 1}
              aria-pressed={prefsStore.mobileColumns === 1}
              aria-label="1"
              bind:this={gtButtons[0]}
              onclick={() => prefsStore.setMobileColumns(1 as MobileColumns)}
            >
              <Icon name="cols1" size={19} />
            </button>
            <button
              type="button"
              class="gt-btn"
              class:on={prefsStore.mobileColumns === 2}
              aria-pressed={prefsStore.mobileColumns === 2}
              aria-label="2"
              bind:this={gtButtons[1]}
              onclick={() => prefsStore.setMobileColumns(2 as MobileColumns)}
            >
              <Icon name="cols2" size={19} />
            </button>
          </div>
        </div>
      </div>
    {/if}
  </header>
{/if}

<style>
  /* Mobile sticky header — mirrors calm .sticky-head, plus translucent blur
     for the iOS-PWA feel and a safe-area inset so top:0 lands below the notch. */
  .ah-mobile {
    position: sticky;
    top: 0;
    z-index: 30;
    padding: 6px 18px 13px;
    padding-top: calc(env(safe-area-inset-top) + 6px);
    background: color-mix(in oklab, var(--bg) 86%, transparent);
    backdrop-filter: saturate(1.2) blur(16px);
    -webkit-backdrop-filter: saturate(1.2) blur(16px);
    border-bottom: 1px solid var(--border);
  }
  .head-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 10px;
  }
  .head-title {
    display: flex;
    flex-direction: column;
    gap: 3px;
    min-width: 0;
  }
  .title-xl {
    font-size: 22px;
    font-weight: 600;
    letter-spacing: -0.4px;
    color: var(--text);
  }
  .head-caption {
    font-size: 13px;
    color: var(--text-2);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .head-caption .gd {
    display: inline-block;
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: var(--online);
    margin-right: 6px;
    vertical-align: 1px;
  }

  .control-bar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 10px;
    margin-top: 14px;
  }
  .control-right {
    display: flex;
    align-items: center;
    gap: 8px;
    flex: 0 0 auto;
  }

  /* shared .ctrl frame for pill controls */
  .ctrl {
    height: var(--ctrl-h);
    border-radius: 999px;
    background: var(--surface);
    border: 1px solid var(--border);
    box-sizing: border-box;
  }

  /* group filter button + popover */
  .grpfilter {
    position: relative;
    flex: 0 1 auto;
    min-width: 0;
  }
  .filter-btn {
    display: inline-flex;
    align-items: center;
    gap: 7px;
    max-width: 100%;
    padding: 0 15px;
    font-size: 14px;
    font-weight: 500;
    color: var(--text);
    background: var(--surface);
    cursor: pointer;
    user-select: none;
    font-family: inherit;
  }
  .filter-btn .fb-label {
    min-width: 0;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .filter-btn .fb-chev {
    color: var(--text-2);
    display: inline-flex;
    transition: transform 0.2s ease;
    flex: 0 0 auto;
  }
  .filter-btn.open {
    background: var(--accent-soft);
    border-color: transparent;
    color: var(--accent);
  }
  .filter-btn.open .fb-chev {
    color: var(--accent);
    transform: rotate(180deg);
  }
  .filter-btn:active {
    transform: translateY(1px);
  }

  .grp-catch {
    position: fixed;
    inset: 0;
    z-index: 7;
    background: transparent;
    border: none;
    padding: 0;
    cursor: default;
  }
  .grp-menu {
    list-style: none;
    margin: 0;
    position: absolute;
    top: calc(100% + 8px);
    left: 0;
    z-index: 8;
    width: 244px;
    max-height: 280px;
    overflow-y: auto;
    background: var(--elev);
    border: 1px solid var(--border-strong);
    border-radius: 16px;
    box-shadow: var(--shadow);
    padding: 6px;
    animation: pop 0.16s ease;
  }
  .grp-menu::-webkit-scrollbar {
    width: 0;
  }
  @keyframes pop {
    from {
      transform: translateY(-6px) scale(0.98);
      opacity: 0;
    }
    to {
      transform: none;
      opacity: 1;
    }
  }
  .grp-opt {
    display: flex;
    align-items: center;
    gap: 10px;
    width: 100%;
    padding: 11px;
    border: none;
    background: transparent;
    border-radius: 11px;
    font-size: 15px;
    color: var(--text);
    font-family: inherit;
    text-align: left;
    cursor: pointer;
  }
  .grp-opt:active {
    background: var(--surface-2);
  }
  .grp-opt.on {
    background: var(--accent-soft);
  }
  .grp-opt .check {
    width: 16px;
    flex: 0 0 16px;
    color: var(--accent);
    display: inline-flex;
  }
  .grp-opt .g-name {
    flex: 1 1 auto;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .grp-opt.on .g-name {
    color: var(--accent);
    font-weight: 500;
  }
  .grp-opt :global(.g-count) {
    flex: 0 0 auto;
    color: var(--text-2);
    background: var(--surface-2);
    border-radius: 999px;
    padding: 2px 9px;
  }
  .grp-opt.on :global(.g-count) {
    color: var(--accent);
    background: transparent;
    box-shadow: inset 0 0 0 1px var(--accent-soft);
  }

  /* density toggle (gridtog) */
  .gridtog {
    position: relative;
    display: flex;
    gap: 2px;
    padding: 4px;
  }
  .seg-thumb {
    position: absolute;
    left: 0;
    top: 0;
    z-index: 0;
    border-radius: 999px;
    background: var(--accent-soft);
    opacity: 0;
    pointer-events: none;
    transition:
      transform 0.3s cubic-bezier(0.4, 0, 0.2, 1),
      width 0.3s cubic-bezier(0.4, 0, 0.2, 1);
  }
  .seg-thumb.visible {
    opacity: 1;
  }
  .gt-btn {
    position: relative;
    z-index: 1;
    -webkit-appearance: none;
    appearance: none;
    width: 34px;
    display: flex;
    align-items: center;
    justify-content: center;
    background: transparent;
    border: none;
    border-radius: 999px;
    color: var(--text-3);
    cursor: pointer;
    font-family: inherit;
    transition: color 0.2s ease;
  }
  .gt-btn.on {
    color: var(--accent);
  }
  .gt-btn:active {
    transform: translateY(1px);
  }
  @media (prefers-reduced-motion: reduce) {
    .seg-thumb {
      transition: none;
    }
  }

  /* bell / moments */
  .bell {
    position: relative;
    width: var(--ctrl-h);
    height: var(--ctrl-h);
    display: grid;
    place-items: center;
    flex: 0 0 auto;
    background: transparent;
    border: none;
    color: var(--text);
    cursor: pointer;
    font-family: inherit;
  }
  .bell:active {
    transform: translateY(1px);
  }
  .bell.quiet {
    color: var(--text-3);
  }
  .bell .badge {
    position: absolute;
    top: -2px;
    right: -2px;
    min-width: 18px;
    height: 18px;
    padding: 0 5px;
    background: var(--accent);
    color: var(--on-accent);
    border: 2px solid var(--bg);
    border-radius: 999px;
    font-family: 'JetBrains Mono', ui-monospace, monospace;
    font-size: 10px;
    font-weight: 600;
    display: grid;
    place-items: center;
  }

  /* ============================================================
     DESKTOP TOP BAR (calm .dk-top)
     ============================================================ */
  .dk-top {
    position: sticky;
    top: 0;
    z-index: 30;
    height: 64px;
    flex: 0 0 64px;
    display: flex;
    align-items: center;
    gap: 28px;
    padding: 0 26px;
    background: color-mix(in oklab, var(--bg) 86%, transparent);
    backdrop-filter: saturate(1.2) blur(16px);
    -webkit-backdrop-filter: saturate(1.2) blur(16px);
    border-bottom: 1px solid var(--border);
  }
  .dk-brand {
    display: flex;
    align-items: center;
    gap: 11px;
    flex: 0 0 auto;
    user-select: none;
  }
  .dk-mark {
    width: 34px;
    height: 34px;
    border-radius: 10px;
    background: var(--accent-soft);
    color: var(--accent);
    display: grid;
    place-items: center;
  }
  .dk-wordmark {
    font-size: 19px;
    font-weight: 600;
    letter-spacing: -0.4px;
    color: var(--text);
  }
  /* The four-bracket logo sits inside the rounded .dk-mark square. */
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
  .logo-mark .tl {
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

  /* tabs */
  .dk-tabs {
    display: flex;
    align-items: center;
    gap: 2px;
    flex: 0 0 auto;
  }
  .dk-tab {
    display: inline-flex;
    align-items: center;
    gap: 9px;
    height: 38px;
    padding: 0 16px;
    border-radius: 999px;
    font-size: 14.5px;
    font-weight: 500;
    color: var(--text-2);
    text-decoration: none;
    cursor: pointer;
    user-select: none;
    transition:
      color 0.16s ease,
      background 0.16s ease;
  }
  .dk-tab:hover {
    color: var(--text);
    background: var(--surface);
  }
  .dk-tab.on {
    color: var(--accent);
    background: var(--accent-soft);
  }

  .spacer {
    flex: 1 1 auto;
  }

  /* actions */
  .dk-actions {
    display: flex;
    align-items: center;
    gap: 8px;
    flex: 0 0 auto;
  }
  .dk-iconbtn {
    position: relative;
    width: 40px;
    height: 40px;
    border-radius: 11px;
    border: 1px solid var(--border);
    background: var(--surface);
    color: var(--text-2);
    display: grid;
    place-items: center;
    cursor: pointer;
    font-family: inherit;
    transition:
      color 0.15s ease,
      background 0.15s ease,
      border-color 0.15s ease;
  }
  .dk-iconbtn:hover {
    color: var(--text);
    border-color: var(--border-strong);
  }
  .dk-iconbtn.quiet {
    color: var(--text-3);
  }
  .dk-iconbtn .badge {
    position: absolute;
    top: -5px;
    right: -5px;
    min-width: 19px;
    height: 19px;
    padding: 0 5px;
    background: var(--accent);
    color: var(--on-accent);
    border: 2px solid var(--bg);
    border-radius: 999px;
    font-family: 'JetBrains Mono', ui-monospace, monospace;
    font-size: 10px;
    font-weight: 600;
    display: grid;
    place-items: center;
  }
</style>
