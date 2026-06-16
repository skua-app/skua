<script lang="ts">
  import Segmented from '$lib/components/Segmented.svelte'
  import { prefsStore } from '$lib/stores/prefs.svelte'
  import { glanceStore } from '$lib/stores/glance.svelte'
  import { themeStore, type Theme } from '$lib/stores/theme.svelte'
  import { ui } from '$lib/i18n/strings'
  import { ACCENT_VALUES } from '$lib/api'
  import type { Accent, NameStyle, GlanceWindowHours, GlanceMaxMoments, GridFps } from '$lib/api'

  const themeOptions: { value: Theme; label: string }[] = [
    { value: 'auto', label: ui.themeAuto },
    { value: 'dark', label: ui.themeDark },
    { value: 'light', label: ui.themeLight }
  ]
  const nameStyleOptions: { value: NameStyle; label: string }[] = [
    { value: 'below', label: ui.nameStyleBelow },
    { value: 'overlay', label: ui.nameStyleOverlay }
  ]
  const showTimestampOptions = [
    { value: 'on' as const, label: ui.yes },
    { value: 'off' as const, label: ui.no }
  ]
  const gridFpsOptions = [
    { value: '1' as const, label: '1 Hz' },
    { value: '2' as const, label: '2 Hz' }
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

<div class="set-block">
  <div class="set-line">
    <div class="sl-label">{ui.themeLabel}</div>
    <div class="sl-control">
      <Segmented
        value={themeStore.theme}
        options={themeOptions}
        onChange={(v) => themeStore.setTheme(v)}
        tone="surface-2"
      />
    </div>
  </div>

  <div class="set-line">
    <div class="sl-label">{ui.nameStyleLabel}</div>
    <div class="sl-control">
      <Segmented
        value={prefsStore.nameStyle}
        options={nameStyleOptions}
        onChange={(v) => prefsStore.setNameStyle(v)}
        tone="surface-2"
      />
    </div>
  </div>

  <div class="set-line">
    <div class="sl-label">{ui.showTimestamp}</div>
    <div class="sl-control">
      <Segmented
        value={prefsStore.showTimestamp ? 'on' : 'off'}
        options={showTimestampOptions}
        onChange={(v) => prefsStore.setShowTimestamp(v === 'on')}
        tone="surface-2"
      />
    </div>
  </div>

  <div class="set-line">
    <div class="sl-label">{ui.gridFpsLabel}</div>
    <div class="sl-control">
      <Segmented
        value={String(prefsStore.gridFps) as '1' | '2'}
        options={gridFpsOptions}
        onChange={(v) => prefsStore.setGridFps(Number(v) as GridFps)}
        tone="surface-2"
      />
    </div>
  </div>

  <div class="set-line">
    <div class="sl-label">{ui.accentLabel}</div>
    <div class="sl-control">
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
    </div>
  </div>

  <div class="set-line stack">
    <div class="sl-label">{ui.glanceWindowLabel}</div>
    <div class="sl-control">
      <Segmented
        value={String(prefsStore.glanceWindowHours) as '6' | '12' | '24' | '48' | '72'}
        options={glanceWindowOptions}
        onChange={(v) => {
          void prefsStore.setGlanceWindowHours(Number(v) as GlanceWindowHours)
          void glanceStore.load()
        }}
        tone="surface-2"
      />
    </div>
  </div>

  <div class="set-line stack">
    <div class="sl-label">{ui.glanceMaxMomentsLabel}</div>
    <div class="sl-control">
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

<style>
  .set-block {
    display: flex;
    flex-direction: column;
  }
  .set-line {
    display: flex;
    align-items: center;
    gap: 14px;
    padding: 13px 4px;
    border-bottom: 1px solid var(--border);
  }
  .set-line:last-child {
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
  /* Stack-row segmented stretches full width with equal-flex options. */
  .set-line.stack .sl-control :global(.seg) {
    display: flex;
    width: 100%;
  }
  .set-line.stack .sl-control :global(.seg-opt) {
    flex: 1 1 0;
    justify-content: center;
  }

  /* Accent swatches — calm .acc-swatches / .acc-sw. */
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
