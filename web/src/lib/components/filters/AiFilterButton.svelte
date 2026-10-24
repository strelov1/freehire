<script lang="ts">
  import { MessageSquareText } from '@lucide/svelte';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { t } from '#lib/i18n/t.js';
  import type { FilterStore } from '#lib/filters.js';
  import AiFilterDialog from './AiFilterDialog.svelte';
  import { messages } from './AiFilterButton.messages';

  // Rendered for everyone and opens for everyone, signed in or not. The interpreter and
  // the CV reader both admit guests (bounded per IP server-side), so a signed-out visitor
  // gets the whole flow; the sign-in invitation comes only when that guest budget is
  // spent, from inside the dialog, and as the "Create free account" beside the results.
  //
  // `onApplied` fires once the dialog has written its filters. It exists for the host
  // that DEFERS its own edits — the filter modal — where the dialog's write goes
  // straight to the live store and would then be overwritten by whatever the modal had
  // staged. There the host closes itself, which is honest: describing the search in
  // words replaces the filters, so there is nothing left to stage.
  let { store, onApplied }: { store: FilterStore; onApplied?: () => void } = $props();

  const s = $derived(t(messages, locale()));
  let open = $state(false);
</script>

<button
  type="button"
  class="flex w-full items-center justify-center gap-2 rounded-xl border border-primary/30 bg-primary/5 px-3 py-2 text-sm font-medium text-primary transition-colors hover:bg-primary/10"
  onclick={() => (open = true)}
>
  <MessageSquareText class="size-4" />
  {s.describeWithAi}
  <span class="rounded bg-primary/15 px-1 text-xs font-semibold uppercase tracking-wide">{s.beta}</span>
</button>

{#if open}
  <AiFilterDialog {store} {onApplied} onclose={() => (open = false)} />
{/if}
