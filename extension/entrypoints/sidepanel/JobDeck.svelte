<script lang="ts">
  import { Skeleton } from 'freehire-design-system';
  import JobDeckCard from './JobDeckCard.svelte';
  import type { DeckSlot } from '../../lib/assistant/deck';

  // One `present_jobs` call, rendered. The cards are a single spaced group under
  // one optional heading, which is the whole point of routing recommendations
  // through a tool: prose can no longer land between them.
  let { slot }: { slot: DeckSlot } = $props();
</script>

<div class="deck">
  {#if slot.status === 'pending'}
    <!-- The call is in flight. Nothing is drawn from its arguments yet: the backend
         has not said which slugs exist, and a deck built from unvalidated slugs
         would be replaced the moment the model corrected itself. -->
    {#each { length: slot.count }, i (i)}
      <Skeleton class="skeleton" />
    {/each}
  {:else}
    {#if slot.deck.heading}
      <h3>{slot.deck.heading}</h3>
    {/if}
    {#each slot.deck.entries as entry (entry.slug)}
      <JobDeckCard {entry} />
    {/each}
  {/if}
</div>

<style>
  .deck {
    display: flex;
    flex-direction: column;
    gap: 8px;
    margin: 10px 0;
  }

  h3 {
    margin: 0;
    font-size: 12px;
    font-weight: 700;
    color: var(--foreground);
  }

  :global(.skeleton) {
    height: 62px;
    border-radius: 10px;
  }
</style>
