<script lang="ts">
  type Props = {
    title: string
    subtitle?: string
  }

  let { title, subtitle = '' }: Props = $props()
</script>

<div class="empty">
  <span class="empty-title">{title}</span>
  {#if subtitle}
    <span class="empty-sub">{subtitle}</span>
  {/if}
</div>

<style>
  .empty {
    min-height: 100dvh;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 8px;
    padding: 48px 24px;
    color: var(--text-2);
  }
  .empty-title {
    font-size: 16px;
    font-weight: 500;
    color: var(--text);
    letter-spacing: -0.1px;
  }
  .empty-sub {
    font-size: 13px;
    font-weight: 400;
    color: var(--text-3);
  }
</style>
