<script lang="ts">
  // Draws a SaveStatus: "Saving…", "Saved" for a moment (with an Undo when the write can be
  // reversed), or the failure with a Retry.
  // Sits on the label row of the control it reports on. Nothing is drawn while idle, so
  // the row looks the same as before until something is saved. The words are announced
  // by SaveStatus itself, so this markup carries no live region of its own.
  import { Check } from '@lucide/svelte';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { t } from '#lib/i18n/t.js';
  import type { SaveStatus } from '#lib/saveStatus.svelte.js';
  import { messages } from './SaveStatus.messages';

  let { status }: { status: SaveStatus } = $props();

  const s = $derived(t(messages, locale()));
</script>

{#if status.state === 'saving'}
  <span class="text-xs text-muted-foreground">{s.saving}</span>
{:else if status.state === 'saved'}
  <span class="flex items-center gap-1 text-xs text-brand-strong">
    <Check class="size-3.5" aria-hidden="true" />{s.saved}
    {#if status.canUndo}
      <span aria-hidden="true">·</span>
      <button type="button" class="font-medium underline underline-offset-2" onclick={() => void status.undo()}>
        {s.undo}
      </button>
    {/if}
  </span>
{:else if status.state === 'error'}
  <span class="text-xs text-destructive">
    <span>{status.failure ?? s.failed}</span>
    <button type="button" class="font-medium underline underline-offset-2" onclick={() => void status.retry()}>
      {s.retry}
    </button>
  </span>
{/if}
