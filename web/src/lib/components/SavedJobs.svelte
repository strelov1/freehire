<script lang="ts">
  import { api } from '$lib/api';
  import { isAuthenticated } from '$lib/auth.svelte';
  import { Paginator } from '$lib/paginated.svelte';
  import JobRow from './JobRow.svelte';
  import LoadMore from './LoadMore.svelte';
  import ReminderChip from './ReminderChip.svelte';
  import States from './States.svelte';

  const page = new Paginator(async (limit, offset) => {
    const slice = await api.listMyJobs('saved', limit, offset);
    return slice;
  });

  $effect(() => {
    if (isAuthenticated()) void page.start();
  });
</script>

{#if page.status === 'loading'}
  <States state="loading" />
{:else if page.status === 'error'}
  <States state="error" message="Couldn't load your saved jobs." />
{:else if page.items.length === 0}
  <States state="empty" message="Nothing saved yet. Jobs you save will show up here." />
{:else}
  <ul class="flex flex-col gap-3">
    {#each page.items as item (item.job.public_slug)}
      <li class="flex flex-col gap-2">
        <JobRow job={item.job} dimViewed={false} />
        <ReminderChip slug={item.job.public_slug} fireAt={item.reminder_fire_at} />
      </li>
    {/each}
  </ul>
  {#if page.hasMore}
    <LoadMore loading={page.loadingMore} error={page.loadMoreError} onclick={() => page.loadMore()} />
  {/if}
{/if}
