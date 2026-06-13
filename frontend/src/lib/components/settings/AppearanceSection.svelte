<script lang="ts">
  import Segmented from '$lib/components/Segmented.svelte'
  import { prefsStore } from '$lib/stores/prefs.svelte'
  import { glanceStore } from '$lib/stores/glance.svelte'
  import { themeStore, type Theme } from '$lib/stores/theme.svelte'
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

  const themeOptions: { value: Theme; label: string }[] = [
    { value: 'auto', label: ui.themeAuto },
    { value: 'dark', label: ui.themeDark },
    { value: 'light', label: ui.themeLight }
  ]
  const gridModeOptions = [
    { value: 'hd' as const, label: 'HD' },
    { value: 'eco' as const, label: 'ECO' }
  ]
  const desktopColumnsOptions = [
    { value: '2' as const, label: '2' },
    { value: '3' as const, label: '3' },
    { value: '4' as const, label: '4' },
    { value: '5' as const, label: '5' }
  ]
  const mobileColumnsOptions = [
    { value: '1' as const, label: '1' },
    { value: '2' as const, label: '2' }
  ]
  const nameStyleOptions: { value: NameStyle; label: string }[] = [
    { value: 'below', label: ui.nameStyleBelow },
    { value: 'overlay', label: ui.nameStyleOverlay }
  ]
  const showTimestampOptions = [
    { value: 'on' as const, label: ui.yes },
    { value: 'off' as const, label: ui.no }
  ]
  const accentOptions: { value: Accent; label: string }[] = [
    { value: 'cyan', label: ui.accentCyan },
    { value: 'sage', label: ui.accentSage },
    { value: 'amber', label: ui.accentAmber },
    { value: 'violet', label: ui.accentViolet }
  ]
  const glanceWindowOptions = [
    { value: '6' as const, label: '6h' },
    { value: '12' as const, label: '12h' },
    { value: '24' as const, label: '24h' },
    { value: '48' as const, label: '48h' },
    { value: '72' as const, label: '72h' }
  ]
  const glanceMaxMomentsOptions = [
    { value: '10' as const, label: '10' },
    { value: '20' as const, label: '20' },
    { value: '30' as const, label: '30' },
    { value: '50' as const, label: '50' }
  ]
</script>

<section id="appearance" class="settings-section dk-set-section">
  <h2 class="dk-set-h2">{ui.sectionAppearance}</h2>
  <p class="dk-set-desc">{ui.appearanceDescription}</p>

  <div class="dk-card">
    <div class="dk-setline">
      <div class="sl-l">
        <div class="t">{ui.themeLabel}</div>
        <div class="d">{ui.themeDesc}</div>
      </div>
      <div class="sl-c">
        <Segmented
          value={themeStore.theme}
          options={themeOptions}
          onChange={(v) => themeStore.setTheme(v)}
          tone="surface-2"
        />
      </div>
    </div>
  </div>

  <div class="dk-card">
    <div class="dk-setline">
      <div class="sl-l">
        <div class="t">{ui.gridModeLabel}</div>
        <div class="d">{ui.gridModeDesc}</div>
      </div>
      <div class="sl-c">
        <Segmented
          value={prefsStore.gridMode}
          options={gridModeOptions}
          onChange={(v) => prefsStore.setGridMode(v)}
          tone="surface-2"
        />
      </div>
    </div>

    <div class="dk-setline">
      <div class="sl-l">
        <div class="t">{ui.desktopColumnsLabel}</div>
        <div class="d">{ui.desktopColumnsDesc}</div>
      </div>
      <div class="sl-c">
        <Segmented
          value={String(prefsStore.desktopColumns) as '2' | '3' | '4' | '5'}
          options={desktopColumnsOptions}
          onChange={(v) => prefsStore.setDesktopColumns(Number(v) as DesktopColumns)}
          tone="surface-2"
        />
      </div>
    </div>

    <div class="dk-setline">
      <div class="sl-l">
        <div class="t">{ui.mobileColumnsLabel}</div>
        <div class="d">{ui.mobileColumnsDesc}</div>
      </div>
      <div class="sl-c">
        <Segmented
          value={String(prefsStore.mobileColumns) as '1' | '2'}
          options={mobileColumnsOptions}
          onChange={(v) => prefsStore.setMobileColumns(Number(v) as MobileColumns)}
          tone="surface-2"
        />
      </div>
    </div>
  </div>

  <div class="dk-card">
    <div class="dk-setline">
      <div class="sl-l">
        <div class="t">{ui.nameStyleLabel}</div>
        <div class="d">{ui.nameStyleDesc}</div>
      </div>
      <div class="sl-c">
        <Segmented
          value={prefsStore.nameStyle}
          options={nameStyleOptions}
          onChange={(v) => prefsStore.setNameStyle(v)}
          tone="surface-2"
        />
      </div>
    </div>

    <div class="dk-setline">
      <div class="sl-l">
        <div class="t">{ui.showTimestamp}</div>
        <div class="d">{ui.showTimestampDesc}</div>
      </div>
      <div class="sl-c">
        <Segmented
          value={prefsStore.showTimestamp ? 'on' : 'off'}
          options={showTimestampOptions}
          onChange={(v) => prefsStore.setShowTimestamp(v === 'on')}
          tone="surface-2"
        />
      </div>
    </div>

    <div class="dk-setline">
      <div class="sl-l">
        <div class="t">{ui.accentLabel}</div>
        <div class="d">{ui.accentDesc}</div>
      </div>
      <div class="sl-c">
        <span class="dk-acc-row">
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
      </div>
    </div>
  </div>

  <div class="dk-card">
    <div class="dk-setline">
      <div class="sl-l">
        <div class="t">{ui.glanceWindowLabel}</div>
        <div class="d">{ui.glanceWindowDesc}</div>
      </div>
      <div class="sl-c">
        <Segmented
          value={String(prefsStore.glanceWindowHours) as '6' | '12' | '24' | '48' | '72'}
          options={glanceWindowOptions}
          onChange={(v) => {
            // prefs setters assign local state synchronously before awaiting
            // the PUT, so the immediately-following refetch reads the new
            // value from prefsStore. Refetch so the peek's window chip,
            // moment list, and badge count match the new setting on the
            // next open.
            void prefsStore.setGlanceWindowHours(Number(v) as GlanceWindowHours)
            void glanceStore.load()
          }}
          tone="surface-2"
        />
      </div>
    </div>

    <div class="dk-setline">
      <div class="sl-l">
        <div class="t">{ui.glanceMaxMomentsLabel}</div>
        <div class="d">{ui.glanceMaxMomentsDesc}</div>
      </div>
      <div class="sl-c">
        <Segmented
          value={String(prefsStore.glanceMaxMoments) as '10' | '20' | '30' | '50'}
          options={glanceMaxMomentsOptions}
          onChange={(v) => {
            void prefsStore.setGlanceMaxMoments(Number(v) as GlanceMaxMoments)
            void glanceStore.load()
          }}
          tone="surface-2"
        />
      </div>
    </div>
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

  /* prototype .dk-card — bordered surface card */
  .dk-card {
    border: 1px solid var(--border);
    border-radius: var(--r);
    background: var(--surface);
    padding: 6px 18px;
    margin-bottom: 16px;
  }

  /* prototype .dk-setline — row with title/description left, control right */
  .dk-setline {
    display: flex;
    align-items: center;
    gap: 18px;
    padding: 16px 0;
    border-bottom: 1px solid var(--border);
  }
  .dk-setline:last-child {
    border-bottom: none;
  }
  .dk-setline .sl-l {
    flex: 1 1 auto;
    min-width: 0;
  }
  .dk-setline .sl-l .t {
    font-size: 15px;
    font-weight: 500;
    color: var(--text);
  }
  .dk-setline .sl-l .d {
    font-size: 12.5px;
    color: var(--text-2);
    margin-top: 3px;
    line-height: 1.4;
  }
  .dk-setline .sl-c {
    flex: 0 0 auto;
  }

  /* Narrow viewports — stack like prototype .set-line.stack so multi-option
     segments (window, columns) don't overflow next to their label. */
  @media (max-width: 640px) {
    .dk-setline {
      flex-direction: column;
      align-items: stretch;
      gap: 10px;
    }
    .dk-setline .sl-c {
      width: 100%;
    }
    .dk-setline .sl-c :global(.seg) {
      display: flex;
      width: 100%;
    }
    .dk-setline .sl-c :global(.seg-opt) {
      flex: 1 1 0;
      justify-content: center;
    }
  }

  /* prototype .dk-acc-row — accent swatches in a row */
  .dk-acc-row {
    display: flex;
    gap: 12px;
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
