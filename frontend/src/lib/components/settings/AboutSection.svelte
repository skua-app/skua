<script lang="ts">
  import { configStore } from '$lib/stores/config.svelte'
  import { ui } from '$lib/i18n/strings'

  // configStore is layout-inited; render the loaded value (defaults to "dev"
  // until the backend config response arrives).
  const version = $derived(configStore.version || 'dev')
</script>

<section id="about" class="settings-section dk-set-section">
  <h2 class="dk-set-h2">{ui.sectionAbout}</h2>
  <p class="dk-set-desc">{ui.aboutDescription}</p>

  <div class="set-card ab-id">
    <span class="ab-mark" aria-hidden="true">
      <span class="logo-mark">
        <span class="bracket tl"></span>
        <span class="bracket tr"></span>
        <span class="bracket bl"></span>
        <span class="bracket br"></span>
      </span>
    </span>
    <div class="ab-name">Skua</div>
    <div class="ab-tag">{ui.aboutTagline}</div>
    <div class="ab-ver mono">v{version}</div>
  </div>

  <div class="set-card">
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
    display: block;
  }
  .dk-set-section {
    max-width: 720px;
  }
  .dk-set-h2 {
    font-size: 20px;
    font-weight: 600;
    letter-spacing: -0.3px;
    margin: 0 0 4px;
    color: var(--text);
  }
  .dk-set-desc {
    font-size: 13.5px;
    color: var(--text-2);
    margin: 0 0 20px;
    line-height: 1.5;
  }

  /* prototype .set-card + .set-card.ab-id */
  .set-card {
    border: 1px solid var(--border);
    border-radius: var(--r-sm);
    background: var(--surface);
    padding: 14px;
    display: flex;
    flex-direction: column;
    gap: 11px;
    margin-bottom: 10px;
  }
  .ab-id {
    align-items: center;
    text-align: center;
    gap: 7px;
    padding: 24px 14px;
  }
  .ab-mark {
    width: 46px;
    height: 46px;
    border-radius: 13px;
    background: var(--accent-soft);
    color: var(--accent);
    display: grid;
    place-items: center;
    margin-bottom: 6px;
  }
  .ab-name {
    font-size: 21px;
    font-weight: 600;
    letter-spacing: -0.3px;
    color: var(--text);
  }
  .ab-tag {
    font-size: 13px;
    line-height: 1.4;
    color: var(--text-2);
  }
  .ab-ver {
    font-family: 'JetBrains Mono', ui-monospace, monospace;
    font-size: 11px;
    color: var(--text-3);
    margin-top: 4px;
    line-height: 1.4;
  }

  /* four-bracket logo, scaled up from AppHeader's 14px to 22px */
  .logo-mark {
    width: 22px;
    height: 22px;
    position: relative;
    display: inline-block;
  }
  .bracket {
    position: absolute;
    width: 7px;
    height: 7px;
  }
  .logo-mark .tl {
    top: 0;
    left: 0;
    border-top: 1.8px solid var(--accent);
    border-left: 1.8px solid var(--accent);
  }
  .logo-mark .tr {
    top: 0;
    right: 0;
    border-top: 1.8px solid var(--accent);
    border-right: 1.8px solid var(--accent);
  }
  .logo-mark .bl {
    bottom: 0;
    left: 0;
    border-bottom: 1.8px solid var(--accent);
    border-left: 1.8px solid var(--accent);
  }
  .logo-mark .br {
    bottom: 0;
    right: 0;
    border-bottom: 1.8px solid var(--accent);
    border-right: 1.8px solid var(--accent);
  }

  .ab-note {
    font-size: 12px;
    color: var(--text-3);
    line-height: 1.45;
  }

  /* link rows */
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
