<script lang="ts">
  import { listMyJobs } from '$lib/api';
  import { isAuthenticated } from '$lib/auth.svelte';
  import { Paginator } from '$lib/paginated.svelte';
  import JobRow from './JobRow.svelte';
  import LoadMore from './LoadMore.svelte';
  import States from './States.svelte';

  const page = new Paginator(async (limit, offset) => {
    const slice = await listMyJobs('viewed', limit, offset);
    return slice;
  });

  $effect(() => {
    if (isAuthenticated()) void page.start();
  });
</script>

{#if page.status === 'loading'}
  <States state="loading" />
{:else if page.status === 'error'}
  <States state="error" message="Couldn't load your history." />
{:else if page.items.length === 0}
  <States state="empty" message="Nothing viewed yet. Jobs you open will show up here." />
{:else}
  <ul class="flex flex-col gap-3">
    {#each page.items as item (item.job.public_slug)}
      <li><JobRow job={item.job} dimViewed={false} /></li>
    {/each}
  </ul>
  {#if page.hasMore}
    <LoadMore loading={page.loadingMore} error={page.loadMoreError} onclick={() => page.loadMore()} />
  {/if}
{/if}
