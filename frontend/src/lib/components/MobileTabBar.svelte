<script lang="ts">
  import { page } from '$app/state'
  import Icon from './Icon.svelte'
  import type { IconName } from '$lib/icons'
  import { ui } from '$lib/i18n/strings'

  type Tab = { icon: IconName; label: string; href: string }

  const tabs: Tab[] = [
    { icon: 'cams', label: ui.cameras, href: '/' },
    { icon: 'events', label: ui.events, href: '/events' },
    { icon: 'settings', label: ui.settings, href: '/settings' }
  ]

  function isActive(href: string): boolean {
    const path = page.url.pathname
    if (href === '/') return path === '/'
    return path === href || path.startsWith(href + '/')
  }
</script>

<nav class="tabbar" aria-label={ui.primaryNavLabel}>
  {#each tabs as tab (tab.href)}
    <a
      href={tab.href}
      class="ti"
      class:on={isActive(tab.href)}
      aria-current={isActive(tab.href) ? 'page' : undefined}
    >
      <Icon name={tab.icon} size={25} />
      <span class="lbl">{tab.label}</span>
    </a>
  {/each}
</nav>

<style>
  .tabbar {
    position: fixed;
    left: 0;
    right: 0;
    bottom: 0;
    display: flex;
    background: var(--bg);
    border-top: 1px solid var(--border);
    padding: 9px 0 max(env(safe-area-inset-bottom, 0px), 14px);
    z-index: 20;
  }
  .ti {
    flex: 1;
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 5px;
    font-size: 11px;
    font-weight: 500;
    color: var(--text-3);
    text-decoration: none;
    cursor: pointer;
    transition: color 120ms;
  }
  .ti:hover {
    color: var(--text-2);
  }
  .ti.on {
    color: var(--accent-ink);
  }
  .lbl {
    letter-spacing: 0.1px;
  }
</style>
