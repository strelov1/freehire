<script lang="ts">
  import type { CollectionRow } from '#lib/server/collectionSections.js';
  import CollectionMark from './CollectionMark.svelte';
  import CompanyLogoStack from './CompanyLogoStack.svelte';
  import CountryFlagStack from './CountryFlagStack.svelte';

  // What leads a collection row: its company logo stack, its country flag stack, or
  // — when it has neither — its single mark.
  let { row }: { row: Pick<CollectionRow, 'companies' | 'flags' | 'mark'> } = $props();
</script>

{#if row.companies.length > 0}
  <CompanyLogoStack names={row.companies} />
{:else if row.flags.length > 1}
  <CountryFlagStack codes={row.flags} max={4} class="shrink-0 text-3xl" />
{:else}
  <CollectionMark mark={row.mark} />
{/if}
