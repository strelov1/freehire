<script lang="ts">
  import { Terminal } from '@lucide/svelte';
  import CvSectionForm from '#lib/components/cv/CvSectionForm.svelte';
  import type { Document } from '#lib/generated/contracts.js';

  // The workspace's Editor tab: the section form, locked while an unattended run holds the
  // document, and the way into editing it from a terminal.
  let {
    doc = $bindable(),
    title = $bindable(),
    runActive,
    onOpenCli,
  }: { doc: Document; title: string; runActive: boolean; onOpenCli: () => void } = $props();
</script>

{#if runActive}
  <p class="mb-3 rounded-lg border border-border bg-muted/40 px-3 py-2 text-sm text-muted-foreground">
    The agent is editing this CV. Editing is paused until the run finishes.
  </p>
{/if}
<!-- A run rewrites this same document server-side while the form holds it in memory
     and saves on a debounce; letting both write means one side's work vanishes. -->
<fieldset disabled={runActive} class="contents">
  <CvSectionForm bind:doc bind:title />
</fieldset>
<!-- The terminal is another way to edit the same text, so it is offered where the
     text is edited rather than as an unlabelled glyph in the panel's header. -->
<button
  type="button"
  onclick={onOpenCli}
  class="mt-6 inline-flex items-center gap-1.5 text-sm text-muted-foreground transition-colors hover:text-foreground"
>
  <Terminal class="size-4" aria-hidden="true" /> Edit this CV from your terminal
</button>
