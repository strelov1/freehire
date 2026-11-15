<script lang="ts">
  import { Button, EmptyState, Skeleton } from 'freehire-design-system';
  import type { FreehireJob, JobMatch } from '../../lib/freehire';
  import AddJobOffer from './AddJobOffer.svelte';
  import MatchCard from './MatchCard.svelte';

  // The Match tab's verdict on the page: the match card, its loading and failed states,
  // or the offer to add a vacancy freehire does not carry yet.
  let {
    status,
    job,
    match,
    hasUploadField,
    error,
    onRetry,
    canAddJob,
    contributing,
    onAdd,
  }: {
    status: 'idle' | 'loading' | 'ready' | 'error' | 'empty';
    job: FreehireJob | null;
    match: JobMatch | null;
    hasUploadField: boolean;
    error: string;
    onRetry: () => void;
    canAddJob: boolean;
    contributing: boolean;
    onAdd: () => void;
  } = $props();
</script>

{#if status === 'ready' && job && match}
  <MatchCard {job} {match} {hasUploadField} />
{:else if status === 'loading'}
  <div class="match-skeleton">
    <Skeleton class="h-9 w-9 rounded-lg" />
    <div class="match-skeleton-lines">
      <Skeleton class="h-3 w-2/3 rounded" />
      <Skeleton class="h-3 w-1/3 rounded" />
    </div>
  </div>
{:else if status === 'error'}
  <EmptyState title="Match unavailable" description={error}>
    {#snippet action()}
      <Button variant="outline" size="sm" onclick={onRetry}>Retry</Button>
    {/snippet}
  </EmptyState>
{:else if status === 'empty' && canAddJob}
  <!-- Not an empty state but an offer: a vacancy freehire does not carry yet is
       the one moment the panel can ask for something useful, so it leads with
       the action and says what it buys — rather than reporting a miss. Any
       other page gets nothing here (see canAddJob). -->
  <AddJobOffer adding={contributing} {onAdd} />
{/if}

<style>
  .match-skeleton {
    display: flex;
    align-items: center;
    gap: 10px;
  }

  .match-skeleton-lines {
    flex: 1;
    display: flex;
    flex-direction: column;
    gap: 6px;
  }
</style>
