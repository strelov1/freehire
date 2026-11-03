<script lang="ts">
  import type { CvBaseItem } from '#lib/cv.js';
  import { messages } from './BasePicker.messages';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { format, t } from '#lib/i18n/t.js';

  // The choice of base to tailor from. It renders nothing for fewer than two bases: with one
  // there is nothing to choose, and showing a lone radio would ask a question that has no answer.

  let {
    bases,
    value = $bindable(''),
    name = 'base-cv',
    copyHint = false,
  }: { bases: CvBaseItem[]; value?: string; name?: string; copyHint?: boolean } = $props();

  const s = $derived(t(messages, locale()));
  const fmt = (iso: string) => new Date(iso).toLocaleDateString();
</script>

{#if bases.length >= 2}
  <fieldset class="flex flex-col gap-2">
    <legend class="mb-1 text-sm font-medium">{s.label}</legend>
    {#each bases as base (base.id)}
      <label
        class="flex cursor-pointer items-center gap-3 rounded-lg border border-border px-3 py-2 text-sm transition-colors has-[:checked]:border-brand has-[:checked]:bg-muted/40"
      >
        <input type="radio" {name} value={base.id} bind:group={value} class="accent-current" />
        <span class="min-w-0 flex-1">
          <span class="block truncate font-medium">{base.title}</span>
          <span class="block text-xs text-muted-foreground">{format(s.edited, { date: fmt(base.updated_at) })}</span>
        </span>
        {#if base.is_default}
          <span class="shrink-0 rounded bg-muted px-1.5 py-0.5 text-xs text-muted-foreground">{s.defaultBadge}</span>
        {/if}
      </label>
    {/each}
    {#if copyHint}<p class="text-xs text-muted-foreground">{s.copyHint}</p>{/if}
  </fieldset>
{/if}
