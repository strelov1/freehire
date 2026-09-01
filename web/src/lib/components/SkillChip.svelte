<script lang="ts">
  import { resolve } from '$app/paths';
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
      <!-- The button is the 24px target WCAG 2.5.8 asks for and the ring inside it is
           the 16px mark the chip can carry. They are separate elements because the
           affordance exists FOR touch: sized to the ring, it would sit a finger's width
           from the chip's own link and a near miss would navigate instead. -->
      <button
        type="button"
        class="ml-0.5 inline-flex size-6 items-center justify-center opacity-70 transition hover:opacity-100"
        aria-label="Show what {label} is"
      >
        <span
          class="inline-flex size-4 items-center justify-center rounded-full border border-current/30 text-xs leading-none"
        >
          ?
        </span>
      </button>
      {#snippet content()}
        <!-- The pending line is a placeholder rather than nothing: the reveal's box is
             already open by the time this renders, so "nothing" is a visibly empty
             popover. `text` is empty only when the chunk could not be fetched, and
             saying so beats the same empty box standing there for good. -->
        {#await description}
          <span class="block h-3 w-40 animate-pulse rounded bg-muted"></span>
        {:then text}
          {#if text}
            <span class="block text-left">{text}</span>
            <a
              href={resolve('/skills/[slug]', { slug })}
              class="mt-1 block text-left font-medium underline"
            >
              What is {label}? →
            </a>
          {:else}
            <span class="block text-left">Definition unavailable right now.</span>
          {/if}
        {/await}
      {/snippet}
    </Tooltip>
  {/if}
</span>
