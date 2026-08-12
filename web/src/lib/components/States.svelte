<script lang="ts">
  import { EmptyState, Skeleton } from '$lib/ui';

  // Shared rendering for the three async states every data view goes through.
  // `loading` shows placeholder rows; `empty`/`error` show a centered message.
  let {
    state,
    message,
    rows = 5,
  }: {
    state: 'loading' | 'empty' | 'error';
    message?: string;
    rows?: number;
  } = $props();

  const fallback = $derived(
    message ?? (state === 'error' ? 'Something went wrong.' : 'Nothing here yet.'),
  );
</script>

{#if state === 'loading'}
  <div class="flex flex-col gap-3">
    {#each Array(rows) as _, i (i)}
      <Skeleton class="h-16 w-full" />
    {/each}
  </div>
{:else}
  <EmptyState title={fallback} variant={state === 'error' ? 'destructive' : 'muted'} />
{/if}
