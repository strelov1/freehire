<script lang="ts">
  import type { Component } from 'svelte';
  import type { StageKey } from '#lib/pipelineStages.js';
  import DedupCard from './DedupCard.svelte';
  import DeriveCard from './DeriveCard.svelte';
  import EnrichCard from './EnrichCard.svelte';
  import IngestCard from './IngestCard.svelte';
  import ReindexCard from './ReindexCard.svelte';

  // Exhaustive at compile time, same as ghost/SignalDiagram.svelte's registry — a stage
  // added to pipelineStages.ts without a matching entry here fails the build rather than
  // rendering nothing.
  const REGISTRY: Record<StageKey, Component> = {
    ingest: IngestCard,
    derive: DeriveCard,
    enrich: EnrichCard,
    deduplicate: DedupCard,
    reindex: ReindexCard,
  };

  let { stage }: { stage: StageKey } = $props();
  const Illustration = $derived(REGISTRY[stage]);
</script>

<Illustration />
