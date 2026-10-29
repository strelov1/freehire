<script lang="ts">
  import { Check, Circle } from '@lucide/svelte';
  import type { CompletenessSection } from '#lib/profileCompleteness.js';
  import { profileCompleteness } from '#lib/profileCompleteness.svelte.js';
  import { Badge } from '#lib/ui/index.js';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { t } from '#lib/i18n/t.js';
  import { messages } from './CompletenessBadge.messages';

  // A small filled/empty marker for one profile section. It reads the same binding the
  // summary does, so the badge and the "x/6" count can never disagree, and it depends on
  // nothing in the tab strip — any section route can mount it on its own.
  let { section }: { section: CompletenessSection } = $props();

  const s = $derived(t(messages, locale()));
  const complete = $derived(profileCompleteness());
  const filled = $derived(complete ? complete[section] : null);

  // Location is optional, so an empty one is neutral rather than a defect. Every other
  // section keeps the filled/missing pair.
  const optional = $derived(section === 'location');
  const variant = $derived(
    optional ? (filled ? 'brand' : 'outline') : filled ? 'brand' : 'missing',
  );
  const label = $derived(
    optional ? (filled ? s.optionalFilled : s.optionalEmpty) : filled ? s.filled : s.empty,
  );
</script>

{#if filled !== null}
  <Badge {variant} class="gap-1">
    {#if filled}
      <Check class="size-3.5" aria-hidden="true" />
    {:else}
      <Circle class="size-3.5" aria-hidden="true" />
    {/if}
    <span class="sr-only">{s.sections[section]}:</span>
    {label}
  </Badge>
{/if}
