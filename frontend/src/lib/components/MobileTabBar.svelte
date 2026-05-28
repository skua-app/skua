<script lang="ts">
  import { page } from '$app/state'
  import Icon from './Icon.svelte'
  import type { IconName } from '$lib/icons'
  import { ui } from '$lib/i18n/strings'

  type Tab = { icon: IconName; label: string; href: string }

  const tabs: Tab[] = [
    { icon: 'grid', label: ui.cameras, href: '/' },
    { icon: 'events', label: ui.events, href: '/events' },
    { icon: 'settings', label: ui.settings, href: '/settings' }
  ]

  function isActive(href: string): boolean {
    const path = page.url.pathname
    if (href === '/') return path === '/'
    return path === href || path.startsWith(href + '/')
  }
</script>

<nav class="tab-bar" aria-label={ui.primaryNavLabel}>
  {#each tabs as tab (tab.href)}
    <a
      href={tab.href}
      class="tab"
      class:active={isActive(tab.href)}
      aria-current={isActive(tab.href) ? 'page' : undefined}
    >
      <Icon name={tab.icon} size={20} />
      <span class="tab-label">{tab.label}</span>
    </a>
  {/each}
</nav>

<style>
  .tab-bar {
    position: fixed;
    left: 0;
    right: 0;
    bottom: 0;
    display: flex;
    justify-content: space-around;
    background: rgba(10, 11, 13, 0.85);
    backdrop-filter: blur(20px);
    border-top: 1px solid var(--border);
    padding-top: 10px;
    padding-bottom: max(env(safe-area-inset-bottom, 0px), 16px);
    z-index: 20;
  }
  .tab {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 3px;
    color: var(--text-3);
    text-decoration: none;
    transition: color 120ms;
  }
  .tab:hover {
    color: var(--text-2);
  }
  .tab.active {
    color: var(--text);
  }
  .tab-label {
    font-size: 10px;
    font-weight: 500;
    letter-spacing: 0.2px;
  }
</style>
