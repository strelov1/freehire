<script lang="ts">
  import type { Snippet } from 'svelte';
  import { Button, EmptyState, Skeleton } from '#lib/ui/index.js';

  // Shared rendering for the three async states every data view goes through.
  // `loading` shows placeholder rows; `empty`/`error` show a centered message.
  //
  // `row` replaces the generic bar with a placeholder shaped like the view's real row, so
  // the list holds its height when the rows arrive. `onRetry` gives the error state a way
  // out: without it a failed load is a dead end that only a page reload undoes.
  let {
    state,
    message,
    rows = 5,
    row,
    onRetry,
  }: {
    state: 'loading' | 'empty' | 'error';
    message?: string;
    rows?: number;
    row?: Snippet;
    onRetry?: () => void;
  } = $props();

  const fallback = $derived(
    message ?? (state === 'error' ? 'Something went wrong.' : 'Nothing here yet.'),
  );
</script>

{#if state === 'loading'}
  <div class="flex flex-col gap-3" aria-busy="true">
    {#each Array(rows) as _, i (i)}
      {#if row}
        {@render row()}
      {:else}
        <Skeleton class="h-16 w-full" />
      {/if}
    {/each}
  </div>
{:else}
  <EmptyState
    title={fallback}
    variant={state === 'error' ? 'destructive' : 'muted'}
    action={state === 'error' && onRetry ? retry : undefined}
  />
{/if}

{#snippet retry()}
  <Button variant="outline" size="sm" onclick={onRetry}>Retry</Button>
{/snippet}
