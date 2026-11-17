<script lang="ts">
  import { ConfirmDialog } from '#lib/ui/index.js';
  import type { Placeholder } from './readiness';

  // The download's stop when the CV still carries template slots: they would print verbatim.
  let {
    open = $bindable(),
    placeholders,
    pdfUrl,
    onFix,
  }: { open: boolean; placeholders: Placeholder[]; pdfUrl: string; onFix: () => void } = $props();

  const tokens = $derived([...new Set(placeholders.map((p) => p.token))]);
</script>

<ConfirmDialog
  bind:open
  title="Your CV still has gaps to fill"
  description="These bracketed slots will print in the PDF exactly as written."
  confirmLabel="Download anyway"
  cancelLabel="Keep editing"
  onConfirm={() => {
    window.open(pdfUrl, '_blank', 'noopener');
  }}
>
  <ul class="flex flex-wrap gap-1.5">
    {#each tokens as token (token)}
      <li><code class="rounded bg-muted px-1.5 py-0.5 text-xs">{token}</code></li>
    {/each}
  </ul>
  <button
    type="button"
    class="mt-3 text-sm font-medium text-brand-strong underline-offset-4 hover:underline"
    onclick={onFix}
  >
    Fix them in the editor
  </button>
</ConfirmDialog>
