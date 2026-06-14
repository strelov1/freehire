<script lang="ts">
  import { listMyJobs, trackJob, type MyJobsFilter } from '$lib/api';
  import { isAuthenticated } from '$lib/auth.svelte';
  import { Paginator } from '$lib/paginated.svelte';
  import type { MyJob, MyJobCounts } from '$lib/types';
  import { STAGES, humanizeStage } from '$lib/stages';
  import { Badge } from '$lib/ui';
  import { cn, timeAgo } from '$lib/utils';
  import JobRow from './JobRow.svelte';
  import LoadMore from './LoadMore.svelte';
  import States from './States.svelte';

  type ListFilter = Exclude<MyJobsFilter, 'board'>;
  const tabs: { value: ListFilter; label: string }[] = [
    { value: 'all', label: 'All' },
    { value: 'viewed', label: 'Viewed' },
    { value: 'saved', label: 'Saved' },
    { value: 'applied', label: 'Applied' },
  ];

  let filter = $state<ListFilter>('all');
  // Per-tab counts ride on every listing response, so the badges stay fresh
  // without extra requests.
  let counts = $state.raw<MyJobCounts | null>(null);

  const makePaginator = (f: ListFilter) =>
    new Paginator(async (limit, offset) => {
      const slice = await listMyJobs(f, limit, offset);
      counts = slice.counts;
      return slice;
    });

  let page = $state(makePaginator('all'));

  function selectTab(f: ListFilter) {
    if (f === filter) return;
    filter = f;
    page = makePaginator(f);
  }

  // Load whenever a paginator is (re)created — but only once the session is
  // confirmed, since the boot-time /me resolution may still be in flight when
  // the page is opened directly.
  $effect(() => {
    if (isAuthenticated()) void page.start();
  });

  const emptyMessages: Record<ListFilter, string> = {
    all: 'No activity yet. Jobs you open, save, or apply to will show up here.',
    viewed: 'Nothing here: every job you viewed is already saved or applied to.',
    saved: 'No saved jobs yet. Save a job to find it here.',
    applied: 'No applications yet. Confirm "Did you apply?" on a job to track it here.',
  };
  const emptyMessage = $derived(emptyMessages[filter]);

  // Optimistic per-job stage/notes overrides (keyed by slug) so an edit shows
  // immediately without refetching the paginator. oninput keeps the override in
  // sync as the user types (a controlled value otherwise reverts mid-keystroke).
  let edits = $state<Record<string, { stage?: string; notes?: string }>>({});

  const stageOf = (item: MyJob) => edits[item.job.public_slug]?.stage ?? item.stage ?? '';
  const notesOf = (item: MyJob) => edits[item.job.public_slug]?.notes ?? item.notes ?? '';

  function patch(slug: string, p: { stage?: string; notes?: string }) {
    edits[slug] = { ...edits[slug], ...p };
  }

  async function setStage(item: MyJob, stage: string) {
    if (!stage) return; // the "Set stage…" placeholder, not a real change
    patch(item.job.public_slug, { stage });
    try {
      await trackJob(item.job.public_slug, { stage });
    } catch {
      // Keep the optimistic value; a transient failure shouldn't drop the edit.
    }
  }

  async function saveNotes(item: MyJob, notes: string) {
    if (notes === (item.notes ?? '')) return; // unchanged since the server value
    try {
      await trackJob(item.job.public_slug, { notes });
    } catch {
      /* keep the optimistic value */
    }
  }
</script>

{#if !isAuthenticated()}
  <p class="py-12 text-center text-sm text-muted-foreground">
    Sign in to see the jobs you viewed, saved, and applied to.
  </p>
{:else}
  <div class="flex flex-col gap-4">
    <h1 class="text-2xl font-semibold tracking-tight">My jobs</h1>

    <div role="tablist" aria-label="Filter my jobs" class="flex items-center gap-1">
      {#each tabs as tab (tab.value)}
        <button
          type="button"
          role="tab"
          aria-selected={filter === tab.value}
          onclick={() => selectTab(tab.value)}
          class={cn(
            'flex items-center gap-1.5 rounded-md px-3 py-1.5 text-sm transition-colors',
            filter === tab.value
              ? 'bg-secondary font-medium text-secondary-foreground'
              : 'text-muted-foreground hover:bg-accent hover:text-accent-foreground',
          )}
        >
          {tab.label}
          {#if counts}
            <span class="text-xs tabular-nums text-muted-foreground">{counts[tab.value]}</span>
          {/if}
        </button>
      {/each}
    </div>

    {#if page.status === 'loading'}
      <States state="loading" />
    {:else if page.status === 'error'}
      <States state="error" message="Couldn't load your jobs." />
    {:else if page.items.length === 0}
      <States state="empty" message={emptyMessage} />
    {:else}
      <ul class="flex flex-col gap-3">
        {#each page.items as item (item.job.public_slug)}
          <li class="flex flex-col gap-1">
            <JobRow job={item.job} />
            <div class="flex flex-wrap items-center gap-2 px-1 text-xs text-muted-foreground">
              {#if item.applied_at}
                <Badge variant="secondary">Applied</Badge>
              {/if}
              {#if item.saved_at}
                <Badge variant="secondary">Saved</Badge>
              {/if}
              {#if stageOf(item)}
                <Badge variant="secondary">{humanizeStage(stageOf(item))}</Badge>
              {/if}
              <span>Viewed {timeAgo(item.viewed_at)}</span>
              <label class="ml-auto flex items-center gap-1">
                <span class="sr-only">Application stage</span>
                <select
                  value={stageOf(item)}
                  onchange={(e) => setStage(item, e.currentTarget.value)}
                  class="rounded-md border border-input bg-transparent px-1.5 py-0.5 text-xs"
                >
                  <option value="">Set stage…</option>
                  {#each STAGES as s (s.value)}
                    <option value={s.value}>{s.label}</option>
                  {/each}
                </select>
              </label>
            </div>
            <textarea
              value={notesOf(item)}
              oninput={(e) => patch(item.job.public_slug, { notes: e.currentTarget.value })}
              onblur={(e) => saveNotes(item, e.currentTarget.value)}
              placeholder="Notes…"
              rows="1"
              class="mx-1 resize-y rounded-md border border-input bg-transparent px-2 py-1 text-xs placeholder:text-muted-foreground focus-visible:border-ring focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50"
            ></textarea>
          </li>
        {/each}
      </ul>
      {#if page.hasMore}
        <LoadMore loading={page.loadingMore} error={page.loadMoreError} onclick={() => page.loadMore()} />
      {/if}
    {/if}
  </div>
{/if}
