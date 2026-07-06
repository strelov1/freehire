<script lang="ts">
  import { resolve } from '$app/paths';
  import { api } from '$lib/api';
  import { isAuthenticated } from '$lib/auth.svelte';
  import { Paginator } from '$lib/paginated.svelte';
  import JobRow from './JobRow.svelte';
  import LoadMore from './LoadMore.svelte';
  import States from './States.svelte';

  // Open jobs ranked by semantic similarity to the caller's uploaded CV. An empty
  // page means no usable CV vector yet (no CV uploaded, or the embedder was
  // superseded), so the empty state points the user at their profile to add a CV
  // rather than showing a dead end.
  const page = new Paginator((limit, offset) => api.recommendations(limit, offset));

  $effect(() => {
    if (isAuthenticated()) void page.start();
  });
</script>

{#if page.status === 'loading'}
  <States state="loading" />
{:else if page.status === 'error'}
  <States state="error" message="Couldn't load your recommendations." />
{:else if page.items.length === 0}
  <div class="rounded-xl border border-border bg-card p-6 text-center">
    <p class="text-sm font-medium text-foreground">No recommendations yet</p>
    <p class="mt-1 text-sm text-muted-foreground">
      Add or update your CV on your
      <a
        href={resolve('/my/profile')}
        class="font-medium text-foreground underline underline-offset-4 hover:no-underline">profile</a
      >
      to get jobs matched to your experience.
    </p>
  </div>
{:else}
  <ul class="flex flex-col gap-3">
    {#each page.items as job (job.public_slug)}
      <li><JobRow {job} /></li>
    {/each}
  </ul>
  {#if page.hasMore}
    <LoadMore loading={page.loadingMore} error={page.loadMoreError} onclick={() => page.loadMore()} />
  {/if}
{/if}
