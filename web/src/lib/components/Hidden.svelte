<script lang="ts">
  import { Eye } from '@lucide/svelte';
  import { api } from '$lib/api';
  import { isAuthenticated } from '$lib/auth.svelte';
  import { markUndismissed } from '$lib/dismissedJobs.svelte';
  import { Paginator } from '$lib/paginated.svelte';
  import JobRow from './JobRow.svelte';
  import LoadMore from './LoadMore.svelte';
  import States from './States.svelte';

  // The jobs the signed-in user has hidden from the feed, newest-hidden first.
  // Mirrors SavedJobs, with a per-row un-hide action: this is the durable way to
  // reverse a hide once the feed's undo toast is gone.
  const page = new Paginator(async (limit, offset) => api.listMyJobs('dismissed', limit, offset));

  $effect(() => {
    if (isAuthenticated()) void page.start();
  });

  // Guards against a double-click firing two un-hide requests for the same job.
  let unhiding = $state('');

  // Un-hide: clear the dismissed mark, drop the row from this list, and update the
  // shared set so the job reappears in the feed. Optimistic — a failed request still
  // leaves the job usable (the mark simply stays until a later retry).
  async function unhide(slug: string) {
    if (unhiding) return;
    unhiding = slug;
    try {
      await api.undismissJob(slug);
      markUndismissed(slug);
      page.items = page.items.filter((it) => it.job.public_slug !== slug);
    } finally {
      unhiding = '';
    }
  }
</script>

{#if page.status === 'loading'}
  <States state="loading" />
{:else if page.status === 'error'}
  <States state="error" message="Couldn't load your hidden jobs." />
{:else if page.items.length === 0}
  <States state="empty" message="Nothing hidden. Jobs you hide from the feed show up here." />
{:else}
  <ul class="flex flex-col gap-3">
    {#each page.items as item (item.job.public_slug)}
      <li class="relative">
        <JobRow job={item.job} dimViewed={false} />
        <button
          type="button"
          onclick={() => unhide(item.job.public_slug)}
          disabled={unhiding === item.job.public_slug}
          title="Un-hide — show this job in the feed again"
          class="absolute bottom-2.5 right-2.5 inline-flex items-center gap-1.5 rounded-lg bg-accent px-2.5 py-1.5 text-xs font-medium text-foreground shadow-sm ring-1 ring-border transition hover:text-brand disabled:pointer-events-none disabled:opacity-50"
        >
          <Eye class="size-4" aria-hidden="true" />
          Un-hide
        </button>
      </li>
    {/each}
  </ul>
  {#if page.hasMore}
    <LoadMore loading={page.loadingMore} error={page.loadMoreError} onclick={() => page.loadMore()} />
  {/if}
{/if}
