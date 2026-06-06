<script lang="ts">
  import Segmented from '$lib/components/Segmented.svelte'
  import { prefsStore } from '$lib/stores/prefs.svelte'
  import { ui } from '$lib/i18n/strings'
  import type {
    Accent,
    NameStyle,
    DesktopColumns,
    MobileColumns,
    GlanceWindowHours
  } from '$lib/api'

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
  const nameStyleOptions = [
    { value: 'below' as const, label: ui.nameStyleBelow },
    { value: 'overlay' as const, label: ui.nameStyleOverlay }
  ]
  const showTimestampOptions = [
    { value: 'true' as const, label: ui.yes },
    { value: 'false' as const, label: ui.no }
  ]
  const accentOptions = [
    { value: 'cyan' as const, label: ui.accentCyan },
    { value: 'sage' as const, label: ui.accentSage },
    { value: 'amber' as const, label: ui.accentAmber },
    { value: 'violet' as const, label: ui.accentViolet }
  ]
  const glanceWindowOptions = [
    { value: '6' as const, label: '6h' },
    { value: '12' as const, label: '12h' },
    { value: '24' as const, label: '24h' },
    { value: '48' as const, label: '48h' },
    { value: '72' as const, label: '72h' }
  ]
</script>

<section id="appearance" class="settings-section">
  <header class="section-header">
    <h2 class="section-title">{ui.sectionAppearance}</h2>
  </header>

  <div class="setting-row">
    <span class="setting-label">{ui.gridModeLabel}</span>
    <Segmented
      value={prefsStore.gridMode}
      options={gridModeOptions}
      onChange={(v) => prefsStore.setGridMode(v)}
    />
  </div>

  <div class="setting-row">
    <span class="setting-label">{ui.desktopColumnsLabel}</span>
    <Segmented
      value={String(prefsStore.desktopColumns) as '2' | '3' | '4' | '5'}
      options={desktopColumnsOptions}
      onChange={(v) => prefsStore.setDesktopColumns(Number(v) as DesktopColumns)}
    />
  </div>

  <div class="setting-row">
    <span class="setting-label">{ui.mobileColumnsLabel}</span>
    <Segmented
      value={String(prefsStore.mobileColumns) as '1' | '2'}
      options={mobileColumnsOptions}
      onChange={(v) => prefsStore.setMobileColumns(Number(v) as MobileColumns)}
    />
  </div>

  <div class="setting-row">
    <span class="setting-label">{ui.nameStyleLabel}</span>
    <Segmented
      value={prefsStore.nameStyle}
      options={nameStyleOptions}
      onChange={(v) => prefsStore.setNameStyle(v as NameStyle)}
    />
  </div>

  <div class="setting-row">
    <span class="setting-label">{ui.showTimestamp}</span>
    <Segmented
      value={prefsStore.showTimestamp ? 'true' : 'false'}
      options={showTimestampOptions}
      onChange={(v) => prefsStore.setShowTimestamp(v === 'true')}
    />
  </div>

  <div class="setting-row">
    <span class="setting-label">{ui.accentLabel}</span>
    <Segmented
      value={prefsStore.accent}
      options={accentOptions}
      onChange={(v) => prefsStore.setAccent(v as Accent)}
    />
  </div>

  <div class="setting-row">
    <span class="setting-label">{ui.glanceWindowLabel}</span>
    <Segmented
      value={String(prefsStore.glanceWindowHours) as '6' | '12' | '24' | '48' | '72'}
      options={glanceWindowOptions}
      onChange={(v) => prefsStore.setGlanceWindowHours(Number(v) as GlanceWindowHours)}
    />
  </div>
</section>

<style>
  .settings-section {
    display: block;
  }
  .section-header {
    margin-bottom: 6px;
  }
  .section-title {
    font-size: 15px;
    font-weight: 600;
    letter-spacing: -0.2px;
    margin-bottom: 6px;
  }
  .setting-row {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 16px;
    padding: 14px 0;
    border-bottom: 1px solid var(--border);
  }
  .setting-row:last-of-type {
    border-bottom: none;
  }
  .setting-label {
    font-size: 13px;
    color: var(--text-2);
  }
</style>
