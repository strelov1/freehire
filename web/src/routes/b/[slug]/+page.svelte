<script lang="ts">
  import { api } from '$lib/api';
  import { Paginator } from '$lib/paginated.svelte';
  import type { Job } from '$lib/types';
  import JobRow from '$lib/components/JobRow.svelte';
  import LoadMore from '$lib/components/LoadMore.svelte';
  import States from '$lib/components/States.svelte';
  import type { PageData } from './$types';

  let { data }: { data: PageData } = $props();
  const board = $derived(data.board);

  // A public board is read-only: page the board's stored query (kept fixed here — there is
  // no filter panel to change it). The paginator is rebuilt when the loaded board changes
  // (e.g. client-side navigation between two boards) and seeded with that load's SSR first
  // page, so rows are in the initial HTML; "load more" fetches client-side. The query can
  // carry repeated keys (multi-value facets), passed through URLSearchParams verbatim.
  const jobs = $derived.by(() => {
    const p = new Paginator<Job>((limit, offset) =>
      api.searchJobs(new URLSearchParams(data.board.query), limit, offset),
    );
    p.seed(data.initial);
    return p;
  });
</script>

<svelte:head>
  <title>{board.name} — freehire</title>
  <!-- Share-by-link, not meant for search discovery. -->
  <meta name="robots" content="noindex" />
</svelte:head>

<div class="mx-auto w-full max-w-4xl px-4 py-6">
  <div class="mb-4 flex flex-col gap-1">
    <h1 class="text-2xl font-semibold tracking-tight">{board.name}</h1>
    <p class="text-sm text-muted-foreground">
      {jobs.total.toLocaleString()}
      {jobs.total === 1 ? 'job' : 'jobs'}{#if board.author_label} · shared by {board.author_label}{/if}
    </p>
  </div>

  {#if jobs.items.length === 0}
    <States state="empty" message="No jobs match this board right now." />
  {:else}
    <ul class="flex flex-col divide-y divide-border rounded-lg border border-border">
      {#each jobs.items as job (job.public_slug)}
        <JobRow {job} />
      {/each}
    </ul>
    {#if jobs.hasMore}
      <LoadMore
        loading={jobs.loadingMore}
        error={jobs.loadMoreError}
        onclick={() => jobs.loadMore()}
      />
    {/if}
  {/if}
</div>
