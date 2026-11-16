<script lang="ts">
  import { Check } from '@lucide/svelte';

  // JobRow's metadata line: the role's facets, the touch-only posted date and the pay, as one
  // line of quiet text joined by dots.
  let {
    metaFacts,
    salary,
    touchPosted,
    hideCompany,
    isViewed,
    metaLeadDot,
  }: {
    metaFacts: string[];
    salary: string | null;
    touchPosted: string | null;
    hideCompany: boolean;
    isViewed: boolean;
    metaLeadDot: boolean;
  } = $props();
</script>

{#if metaFacts.length || salary || touchPosted || (hideCompany && isViewed)}
  <!-- Inline flow, not a flex row: it wraps between facts the way a sentence does. Each
       fact is an inline-block carrying the dot that follows it, so a wrapped line ends
       on "·" and the next one starts on a fact — a flex row left the dots as items of
       their own, free to open a line. The gap is a margin, not a space: Svelte trims
       whitespace at an {#each} boundary, which is how "views ·Remote" ran together.
       The opening dot is a flex item of its own, so when the whole line wraps under
       the badges it stays behind at the end of theirs; when the touch-only date is all
       the line holds, it is touch-only too, or a desktop card ends on "New ·". -->
  {#if metaLeadDot}<span
      class={[
        'text-xs text-muted-foreground',
        !(metaFacts.length || salary || (hideCompany && isViewed)) && 'hidden pointer-coarse:inline',
      ]}
      aria-hidden="true">·</span
    >{/if}
  <span class="min-w-0 text-xs font-medium text-muted-foreground">
    <!-- The rail's viewed mark, for the employer's page where the rail is not drawn. -->
    {#if hideCompany && isViewed}<Check
        class="mr-1 inline size-3.5 align-text-bottom"
        aria-label="You have viewed this"
      />{/if}
    {#if touchPosted}
      <span class="mr-1 hidden whitespace-nowrap tabular-nums pointer-coarse:inline-block"
        >{touchPosted}{#if metaFacts.length || salary}<span aria-hidden="true">&nbsp;·</span>{/if}</span
      >
    {/if}
    {#each metaFacts as fact, i (i)}
      <span class="mr-1 inline-block whitespace-nowrap"
        >{fact}{#if i < metaFacts.length - 1 || salary}<span aria-hidden="true">&nbsp;·</span>{/if}</span
      >
    {/each}
    <!-- The pay leads into the same line, in ink, rather than standing alone in bold at
       the card's foot, where cards with and without it ended at different heights. -->
    {#if salary}
      <span class="inline-block whitespace-nowrap font-semibold tabular-nums text-foreground">{salary}</span>
    {/if}
  </span>
{/if}
