<script lang="ts">
  import { Tooltip, badgeVariants } from '$lib/ui';
  import { filterHref } from '$lib/enrichment';
  import { skillLabel } from '$lib/facets';
  import { hasSkillDescription, skillDescription } from '$lib/skillDescriptions';
  import SkillIcon from './SkillIcon.svelte';

  // A skill chip that can say what the skill IS.
  //
  // The chip stays a link to the postings filtered to that skill — that is what someone
  // clicking "Go" on a posting wants, and it is the behaviour this replaces. The
  // definition gets its own small target beside the label rather than sharing the chip's:
  // a tap on a link navigates, so a touch reader given only the chip would reach the
  // filter and never the glossary.
  //
  // This file has no component test: web/ runs vitest in plain Node with no DOM. Its two
  // decisions live in tested pure modules — skillDescriptions.test.ts for what is
  // described and what the text is, tooltip.test.ts for how the reveal behaves.

  let { slug }: { slug: string } = $props();

  const label = $derived(skillLabel(slug));

  // Synchronous, from the eagerly loaded slug list. An affordance that popped in once a
  // lazy chunk arrived would be worse than none, and one drawn on a skill that turns out
  // to have no definition would be a lie.
  const described = $derived(hasSkillDescription(slug));

  // Read only from inside the reveal's content, which mounts when the reveal opens — so
  // a $derived that is never read is a chunk never fetched. That is the whole point of
  // the split, and it is why this is not simply awaited up front.
  const description = $derived(skillDescription(slug));
</script>

<span class={badgeVariants({ variant: 'brand' })}>
  <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- internal /jobs filter link from filterHref; query-only, no route to resolve -->
  <a href={filterHref('skills', slug)} class="inline-flex items-center gap-1 transition hover:opacity-80">
    <SkillIcon {slug} />
    {label}
  </a>
  {#if described}
    <!-- The tooltip's own max-w-xs is the width here; a narrower override would be an
         arbitrary value where a token exists. -->
    <Tooltip side="top">
      <button
        type="button"
        class="ml-1 inline-flex size-4 items-center justify-center rounded-full border border-current/30 text-xs leading-none opacity-70 transition hover:opacity-100"
        aria-label="What is {label}?"
      >
        ?
      </button>
      {#snippet content()}
        <!-- Nothing renders while the chunk is in flight: the reveal is already open by
             then, and a spinner inside a tooltip is more motion than a sentence is
             worth. `text` is empty when the fetch failed, and an empty reveal is the
             right answer to that too. -->
        {#await description then text}
          {#if text}
            <span class="block text-left">{text}</span>
            <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- static glossary path, one segment from a dictionary slug -->
            <a href="/skills/{slug}" class="mt-1 block text-left font-medium underline">
              What is {label}? →
            </a>
          {/if}
        {/await}
      {/snippet}
    </Tooltip>
  {/if}
</span>
