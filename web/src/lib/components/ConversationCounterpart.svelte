<script lang="ts">
  import { counterpartName } from '#lib/messaging.js';
  import type { Counterpart } from '#lib/types.js';
  import BuddyIdentity from './BuddyIdentity.svelte';

  // The other side of a conversation in one line, drawn by the persona they speak as: one
  // snippet per persona. A counterpart who is no longer reachable is only a name in text
  // (`nameClass` styles it for the host), since the core sends nothing else about them.
  //
  // `link` makes a reachable counterpart the way to their page; a host that is already a
  // button (a conversation row, a dock header) leaves it false.
  let {
    counterpart,
    link = false,
    nameClass = '',
  }: { counterpart: Counterpart; link?: boolean; nameClass?: string } = $props();
</script>

{#snippet buddy(detail: { handle: string; card_summary: { category: string; seniority: string; country: string } })}
  <BuddyIdentity handle={detail.handle} summary={detail.card_summary} size="sm" {link} />
{/snippet}

{#if !counterpart.reachable}
  <span class={nameClass}>{counterpartName(counterpart)}</span>
{:else if counterpart.persona === 'buddy'}
  {@render buddy(counterpart.detail)}
{/if}
