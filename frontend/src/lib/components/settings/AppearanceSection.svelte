<script lang="ts">
  import { prefsStore } from '$lib/stores/prefs.svelte'
  import { glanceStore } from '$lib/stores/glance.svelte'
  import { ui } from '$lib/i18n/strings'
  import { ACCENT_VALUES } from '$lib/api'
  import type {
    Accent,
    NameStyle,
    DesktopColumns,
    MobileColumns,
    GlanceWindowHours,
    GlanceMaxMoments
  } from '$lib/api'

  const gridModeOptions: { value: 'hd' | 'eco'; label: string }[] = [
    { value: 'hd', label: 'HD' },
    { value: 'eco', label: 'ECO' }
  ]
  const desktopColumnsOptions: { value: DesktopColumns; label: string }[] = [
    { value: 2, label: '2' },
    { value: 3, label: '3' },
    { value: 4, label: '4' },
    { value: 5, label: '5' }
  ]
  const mobileColumnsOptions: { value: MobileColumns; label: string }[] = [
    { value: 1, label: '1' },
    { value: 2, label: '2' }
  ]
  const nameStyleOptions: { value: NameStyle; label: string }[] = [
    { value: 'below', label: ui.nameStyleBelow },
    { value: 'overlay', label: ui.nameStyleOverlay }
  ]
  const showTimestampOptions: { value: boolean; label: string }[] = [
    { value: true, label: ui.yes },
    { value: false, label: ui.no }
  ]
  const accentOptions: { value: Accent; label: string }[] = [
    { value: 'cyan', label: ui.accentCyan },
    { value: 'sage', label: ui.accentSage },
    { value: 'amber', label: ui.accentAmber },
    { value: 'violet', label: ui.accentViolet }
  ]
  const glanceWindowOptions: { value: GlanceWindowHours; label: string }[] = [
    { value: 6, label: '6h' },
    { value: 12, label: '12h' },
    { value: 24, label: '24h' },
    { value: 48, label: '48h' },
    { value: 72, label: '72h' }
  ]
  const glanceMaxMomentsOptions: { value: GlanceMaxMoments; label: string }[] = [
    { value: 10, label: '10' },
    { value: 20, label: '20' },
    { value: 30, label: '30' },
    { value: 50, label: '50' }
  ]
</script>

<section id="appearance" class="settings-section dk-set-section">
  <header class="section-header">
    <h2 class="dk-set-h2">{ui.sectionAppearance}</h2>
    <p class="dk-set-desc">{ui.appearanceDescription}</p>
  </header>

  <div class="set-line">
    <span class="sl-label">{ui.gridModeLabel}</span>
    <span class="sl-control">
      <span class="setseg">
        {#each gridModeOptions as opt (opt.value)}
          <button
            type="button"
            class:on={prefsStore.gridMode === opt.value}
            onclick={() => prefsStore.setGridMode(opt.value)}
          >
            {opt.label}
          </button>
        {/each}
      </span>
    </span>
  </div>

  <div class="set-line stack">
    <span class="sl-label">{ui.desktopColumnsLabel}</span>
    <span class="sl-control">
      <span class="setseg">
        {#each desktopColumnsOptions as opt (opt.value)}
          <button
            type="button"
            class:on={prefsStore.desktopColumns === opt.value}
            onclick={() => prefsStore.setDesktopColumns(opt.value)}
          >
            {opt.label}
          </button>
        {/each}
      </span>
    </span>
  </div>

  <div class="set-line">
    <span class="sl-label">{ui.mobileColumnsLabel}</span>
    <span class="sl-control">
      <span class="setseg">
        {#each mobileColumnsOptions as opt (opt.value)}
          <button
            type="button"
            class:on={prefsStore.mobileColumns === opt.value}
            onclick={() => prefsStore.setMobileColumns(opt.value)}
          >
            {opt.label}
          </button>
        {/each}
      </span>
    </span>
  </div>

  <div class="set-line">
    <span class="sl-label">{ui.nameStyleLabel}</span>
    <span class="sl-control">
      <span class="setseg">
        {#each nameStyleOptions as opt (opt.value)}
          <button
            type="button"
            class:on={prefsStore.nameStyle === opt.value}
            onclick={() => prefsStore.setNameStyle(opt.value)}
          >
            {opt.label}
          </button>
        {/each}
      </span>
    </span>
  </div>

  <div class="set-line">
    <span class="sl-label">{ui.showTimestamp}</span>
    <span class="sl-control">
      <span class="setseg">
        {#each showTimestampOptions as opt (String(opt.value))}
          <button
            type="button"
            class:on={prefsStore.showTimestamp === opt.value}
            onclick={() => prefsStore.setShowTimestamp(opt.value)}
          >
            {opt.label}
          </button>
        {/each}
      </span>
    </span>
  </div>

  <div class="set-line">
    <span class="sl-label">{ui.accentLabel}</span>
    <span class="sl-control">
      <span class="acc-swatches">
        {#each accentOptions as opt (opt.value)}
          <button
            type="button"
            class="acc-sw"
            class:on={prefsStore.accent === opt.value}
            style:background={ACCENT_VALUES[opt.value]}
            aria-label={opt.label}
            onclick={() => prefsStore.setAccent(opt.value)}
          ></button>
        {/each}
      </span>
    </span>
  </div>

  <div class="set-line stack">
    <span class="sl-label">{ui.glanceWindowLabel}</span>
    <span class="sl-control">
      <span class="setseg">
        {#each glanceWindowOptions as opt (opt.value)}
          <button
            type="button"
            class:on={prefsStore.glanceWindowHours === opt.value}
            onclick={() => {
              // prefs setters assign local state synchronously before awaiting
              // the PUT, so the immediately-following refetch reads the new
              // value from prefsStore. Refetch so the peek's window chip,
              // moment list, and badge count match the new setting on the
              // next open.
              void prefsStore.setGlanceWindowHours(opt.value)
              void glanceStore.load()
            }}
          >
            {opt.label}
          </button>
        {/each}
      </span>
    </span>
  </div>

  <div class="set-line">
    <span class="sl-label">{ui.glanceMaxMomentsLabel}</span>
    <span class="sl-control">
      <span class="setseg">
        {#each glanceMaxMomentsOptions as opt (opt.value)}
          <button
            type="button"
            class:on={prefsStore.glanceMaxMoments === opt.value}
            onclick={() => {
              void prefsStore.setGlanceMaxMoments(opt.value)
              void glanceStore.load()
            }}
          >
            {opt.label}
          </button>
        {/each}
      </span>
    </span>
  </div>
</section>

<style>
  .settings-section {
    display: block;
  }
  .dk-set-section {
    max-width: 720px;
  }
  .section-header {
    margin-bottom: 12px;
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

  /* prototype .set-line — label-left / control-right */
  .set-line {
    display: flex;
    align-items: center;
    gap: 14px;
    padding: 13px 4px;
    border-bottom: 1px solid var(--border);
  }
  .set-line:last-of-type {
    border-bottom: none;
  }
  .set-line .sl-label {
    font-size: 15px;
    color: var(--text);
  }
  .set-line .sl-control {
    margin-left: auto;
    flex: 0 0 auto;
  }
  .set-line.stack {
    flex-direction: column;
    align-items: stretch;
    gap: 10px;
  }
  .set-line.stack .sl-control {
    margin-left: 0;
  }

  /* prototype .setseg — compact static segmented (no sliding thumb) */
  .setseg {
    display: inline-flex;
    padding: 3px;
    gap: 2px;
    border-radius: 999px;
    background: var(--surface-2);
    border: 1px solid var(--border);
  }
  .set-line.stack .setseg {
    display: flex;
  }
  .setseg button {
    flex: 1 1 auto;
    -webkit-appearance: none;
    appearance: none;
    border: none;
    background: transparent;
    font-family: inherit;
    font-size: 13px;
    font-weight: 600;
    color: var(--text-2);
    padding: 6px 13px;
    border-radius: 999px;
    cursor: pointer;
    white-space: nowrap;
    transition:
      color 0.18s ease,
      background 0.18s ease;
  }
  .setseg button.on {
    background: var(--accent-soft);
    color: var(--accent);
  }
  .setseg button:active {
    transform: translateY(1px);
  }

  /* prototype .acc-swatches — color circles */
  .acc-swatches {
    display: flex;
    gap: 11px;
  }
  .acc-sw {
    width: 26px;
    height: 26px;
    border-radius: 999px;
    padding: 0;
    border: none;
    cursor: pointer;
    box-shadow:
      0 0 0 2px var(--bg),
      0 0 0 2px var(--border) inset;
    transition: transform 0.15s ease;
  }
  .acc-sw.on {
    box-shadow:
      0 0 0 2px var(--bg),
      0 0 0 2px var(--text);
  }
  .acc-sw:active {
    transform: scale(0.92);
  }
</style>
