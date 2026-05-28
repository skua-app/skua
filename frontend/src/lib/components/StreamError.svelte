<script lang="ts">
  import { streamErrorReasons, ui } from '$lib/i18n/strings'

  interface Props {
    reason: string
    onRetry: () => void
  }

  let { reason, onRetry }: Props = $props()

  const reasonText = $derived(streamErrorReasons[reason] ?? reason)
</script>

<div class="flex flex-col items-center justify-center gap-4 p-8 text-center">
  <svg
    xmlns="http://www.w3.org/2000/svg"
    class="h-12 w-12 text-yellow-400"
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

  <div>
    <p class="text-base font-semibold text-white">{ui.streamErrorHeading}</p>
    <p class="mt-1 text-sm text-white/60">{reasonText}</p>
  </div>

  <button
    onclick={onRetry}
    class="rounded-lg bg-white/10 px-5 py-2 text-sm font-medium text-white transition-colors hover:bg-white/20 active:bg-white/30"
  >
    {ui.retry}
  </button>
</div>
