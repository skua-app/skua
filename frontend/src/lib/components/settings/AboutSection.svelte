<script lang="ts">
  import Mono from '$lib/components/Mono.svelte'
  import { configStore } from '$lib/stores/config.svelte'
  import { ui } from '$lib/i18n/strings'

  // configStore is layout-inited; render the loaded value (defaults to "dev"
  // until the backend config response arrives).
  const version = $derived(configStore.version || 'dev')
</script>

<section id="about" class="settings-section">
  <header class="section-header">
    <h2 class="section-title">{ui.sectionAbout}</h2>
  </header>

  <div class="ab-card">
    <div class="ab-row">
      <span class="ab-key">{ui.aboutAppVersion}</span>
      <Mono color="var(--text)" size={12}>{version}</Mono>
    </div>
    <div class="ab-note">{ui.aboutUpstreamPending}</div>
  </div>

  <div class="ab-links">
    <a class="ab-link" href="https://github.com/skua-app/skua" target="_blank" rel="noopener">
      {ui.aboutLinkGithub}
    </a>
    <a
      class="ab-link"
      href="https://github.com/skua-app/skua#readme"
      target="_blank"
      rel="noopener"
    >
      {ui.aboutLinkDocs}
    </a>
    <a
      class="ab-link"
      href="https://github.com/skua-app/skua/blob/main/SECURITY.md"
      target="_blank"
      rel="noopener"
    >
      {ui.aboutLinkSecurity}
    </a>
    <a
      class="ab-link"
      href="https://github.com/skua-app/skua/blob/main/LICENSE"
      target="_blank"
      rel="noopener"
    >
      {ui.aboutLinkLicense}
    </a>
  </div>
</section>

<style>
  .settings-section {
    display: flex;
    flex-direction: column;
  }
  .section-header {
    margin-bottom: 10px;
  }
  .section-title {
    font-size: 15px;
    font-weight: 600;
    letter-spacing: -0.2px;
    margin: 0;
  }
  /* ab-card mirrors calm .set-card / .ab-id */
  .ab-card {
    border: 1px solid var(--border);
    border-radius: var(--r-sm);
    background: var(--surface);
    padding: 14px;
    display: flex;
    flex-direction: column;
    gap: 8px;
    margin-bottom: 12px;
  }
  .ab-row {
    display: flex;
    justify-content: space-between;
    align-items: baseline;
    font-size: 13px;
    color: var(--text-2);
    padding: 4px 0;
  }
  .ab-key {
    flex: 1 1 auto;
  }
  .ab-note {
    font-size: 12px;
    color: var(--text-3);
    line-height: 1.45;
    padding: 2px 4px 0;
  }
  /* footer-style link rows */
  .ab-links {
    border: 1px solid var(--border);
    border-radius: var(--r-sm);
    background: var(--surface);
    display: flex;
    flex-direction: column;
    overflow: hidden;
  }
  .ab-link {
    padding: 12px 14px;
    font-size: 13px;
    color: var(--accent);
    text-decoration: none;
    border-bottom: 1px solid var(--border);
    transition: background 0.14s ease;
  }
  .ab-link:last-child {
    border-bottom: none;
  }
  .ab-link:hover {
    background: var(--surface-2);
  }
</style>
