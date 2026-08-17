<script lang="ts">
  import Mono from '$lib/components/Mono.svelte'
  import { streamErrorHints, streamErrorReasons, ui } from '$lib/i18n/strings'

  const SETUP_DOCS_URL = 'https://github.com/skua-app/skua/blob/main/docs/setup/frigate-config.md'
  const ICE_REASONS = ['ice_no_candidates', 'ice_unreachable']

  interface Props {
    reason: string
    candidates?: string[]
    hint?: string
    onRetry: () => void
  }

  let { reason, candidates = [], hint, onRetry }: Props = $props()

  const reasonText = $derived(streamErrorReasons[reason] ?? reason)
  const hintText = $derived(hint ?? streamErrorHints[reason] ?? null)
  const showDocsLink = $derived(ICE_REASONS.includes(reason))
</script>

<div class="card flex h-full flex-col items-center gap-4 px-6 py-5 text-center">
  <svg
    xmlns="http://www.w3.org/2000/svg"
    class="h-12 w-12 shrink-0 text-yellow-400"
    viewBox="0 0 24 24"
    fill="currentColor"
    aria-hidden="true"
  >
    <path
      fill-rule="evenodd"
      d="M9.401 3.003c1.155-2 4.043-2 5.197 0l7.355 12.748c1.154 2-.29 4.5-2.599 4.5H4.645c-2.309 0-3.752-2.5-2.598-4.5L9.4 3.003zM12 8.25a.75.75 0 0 1 .75.75v3.75a.75.75 0 0 1-1.5 0V9a.75.75 0 0 1 .75-.75zm0 8.25a.75.75 0 1 0 0-1.5.75.75 0 0 0 0 1.5z"
      clip-rule="evenodd"
    />
  </svg>

  <div class="detail">
    <p class="text-base font-semibold text-white">{ui.streamErrorHeading}</p>
    <p class="mt-1 text-sm text-white/60">{reasonText}</p>

    {#if hintText}
      <p class="hint">{hintText}</p>
    {/if}

    {#if candidates.length > 0}
      <p class="candidates-label">{ui.streamErrorCandidatesLabel}</p>
      <ul class="candidates">
        {#each candidates as candidate (candidate)}
          <li><Mono size={11} color="rgba(255,255,255,0.7)" wrap>{candidate}</Mono></li>
        {/each}
      </ul>
    {/if}

    {#if showDocsLink}
      <a class="docs-link" href={SETUP_DOCS_URL} target="_blank" rel="noopener noreferrer">
        {ui.streamErrorSetupDocs}
      </a>
    {/if}
  </div>

  <button
    onclick={onRetry}
    class="shrink-0 rounded-lg bg-white/10 px-5 py-2 text-sm font-medium text-white transition-colors hover:bg-white/20 active:bg-white/30"
  >
    {ui.retry}
  </button>
</div>

<style>
  /* The diagnosis can outgrow a 16:9 feed frame on a phone, so the card
     scrolls. Auto margins rather than justify-content: center — a centred
     flex child that overflows its container cannot be scrolled back to,
     which would strand the heading and hint above the scroll origin. */
  .card {
    overflow-y: auto;
  }
  .card > :global(:first-child) {
    margin-top: auto;
  }
  .card > :global(:last-child) {
    margin-bottom: auto;
  }
  .detail {
    max-width: 34rem;
  }
  .hint {
    margin-top: 0.5rem;
    font-size: 0.75rem;
    line-height: 1.45;
    color: rgba(255, 255, 255, 0.5);
  }
  .candidates-label {
    margin-top: 0.75rem;
    font-size: 0.6875rem;
    text-transform: uppercase;
    letter-spacing: 0.06em;
    color: rgba(255, 255, 255, 0.4);
  }
  .candidates {
    margin-top: 0.25rem;
    display: flex;
    flex-direction: column;
    gap: 0.125rem;
    overflow-wrap: anywhere;
  }
  .docs-link {
    display: inline-block;
    margin-top: 0.75rem;
    font-size: 0.75rem;
    color: var(--accent);
    text-decoration: underline;
    text-underline-offset: 2px;
  }
</style>
