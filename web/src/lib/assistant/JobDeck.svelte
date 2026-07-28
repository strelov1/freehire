<script lang="ts">
  import JobCardSkeleton from './JobCardSkeleton.svelte';
  import JobDeckCard from './JobDeckCard.svelte';
  import type { DeckSlot } from './deck';

  // One `present_jobs` call, rendered. The cards are a single spaced group under
  // one optional heading, which is the whole point of routing recommendations
  // through a tool: prose can no longer land between them.
  let { slot }: { slot: DeckSlot } = $props();
</script>

<div class="my-3">
  {#if slot.status === 'pending'}
    <!-- The call is in flight. Nothing is drawn from its arguments yet: the
         backend has not said which slugs exist, and a deck built from unvalidated
         slugs would be replaced the moment the model corrected itself. -->
    <div class="space-y-2">
      {#each [0, 1] as i (i)}
        <JobCardSkeleton />
      {/each}
    </div>
  {:else}
    {#if slot.deck.heading}
      <h3 class="mb-2 px-1 text-sm font-semibold tracking-tight text-foreground">
        {slot.deck.heading}
      </h3>
    {/if}
    <div class="space-y-2">
      {#each slot.deck.entries as entry (entry.slug)}
        <JobDeckCard {entry} />
      {/each}
    </div>
  {/if}
</div>
