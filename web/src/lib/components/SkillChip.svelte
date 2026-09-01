<script lang="ts">
  import { resolve } from '$app/paths';
  import { Tooltip, badgeVariants } from '$lib/ui';
  import { filterHref } from '$lib/enrichment';
  import { skillLabel } from '$lib/facets';
  import { skillDescription } from '$lib/skillDescriptions';
  import SkillIcon from './SkillIcon.svelte';

  // A skill chip that says what the skill IS.
  //
  // Hover or focus anywhere on the chip opens the definition, which is what the reveal
  // is for. Touch cannot share that: the chip is a link, and a tap on a link navigates,
  // so the "?" beside the label is the tap target — without it a touch reader would
  // reach the filter and never the glossary.
  //
  // Unconditional, because every canonical carries an entry. While the waves were
  // landing this asked first and drew nothing for a skill with no definition behind it;
  // that question now has one answer and asking it was a cost of the programme rather
  // than of the vocabulary.
  //
  // `linked` exists because two callers show a posting's skills inside something that is
  // already a surface of its own (the preview card, the drawer). Turning their chips into
  // links would navigate out of the thing being previewed; they want the definition and
  // the dictionary's spelling, not a new destination.
  //
  // This file has no component test: web/ runs vitest in plain Node with no DOM. Its
  // decisions live in tested pure modules — skillDescriptions.test.ts for the text,
  // tooltip.test.ts for how the reveal behaves.

  let { slug, linked = true }: { slug: string; linked?: boolean } = $props();

  const label = $derived(skillLabel(slug));

  // Read only from inside the reveal's content, which mounts when the reveal opens — so
  // a $derived that is never read is a chunk never fetched. That is the whole point of
  // the split, and it is why this is not simply awaited up front.
  const description = $derived(skillDescription(slug));

  // A tap on the LABEL must not also toggle the reveal. The tooltip's toggle listens on
  // its wrapper, which encloses the whole chip so that hovering anywhere on it works —
  // and that wrapper is an ancestor of this link. Stopping the touch pointerdown here
  // keeps the two gestures apart: tap the label to filter, tap the "?" to read.
  function keepTapOnTheLink(e: PointerEvent) {
    if (e.pointerType === 'touch') e.stopPropagation();
  }
</script>

<!-- The tooltip's own max-w-xs is the width here; a narrower override would be an
     arbitrary value where a token exists. It encloses the WHOLE chip so hovering the
     label — the obvious gesture — opens the definition, not only the "?". -->
<Tooltip side="top">
  <span class={badgeVariants({ variant: 'brand' })}>
    {#if linked}
      <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- internal /jobs filter link from filterHref; query-only, no route to resolve -->
      <a href={filterHref('skills', slug)} class="inline-flex items-center gap-1 transition hover:opacity-80" onpointerdown={keepTapOnTheLink}>
        <SkillIcon {slug} />
        {label}
      </a>
    {:else}
      <span class="inline-flex items-center gap-1">
        <SkillIcon {slug} />
        {label}
      </span>
    {/if}
    <!-- The button is the 24px target WCAG 2.5.8 asks for and the ring inside it is the
         16px mark the chip can carry. They are separate elements because the affordance
         exists FOR touch: sized to the ring, it would sit a finger's width from the
         chip's own link and a near miss would navigate instead. -->
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
  </span>
  {#snippet content()}
    <!-- The pending line is a placeholder rather than nothing: the reveal's box is
         already open by the time this renders, so "nothing" is a visibly empty popover.
         `text` is empty only when the chunk could not be fetched, and saying so beats
         the same empty box standing there for good. -->
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
