<script lang="ts">
  import { Eye } from '@lucide/svelte';
  import { api } from '$lib/api';
  import { locale } from '$lib/i18n/currentLocale.svelte';
  import { t } from '$lib/i18n/t';
  import { isAuthenticated } from '$lib/auth.svelte';
  import { markUndismissed } from '$lib/dismissedJobs.svelte';
  import { Paginator } from '$lib/paginated.svelte';
  import JobRow from './JobRow.svelte';
  import { LoadMore } from '$lib/ui';
  import { messages } from './activity.messages';
  import States from './States.svelte';
  import { must } from '$lib/utils';

  const s = $derived(t(messages, locale()).hidden);
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
      page.items = page.items.filter((it) => it.job?.public_slug !== slug);
    } finally {
      unhiding = '';
    }
  }
</script>

{#if page.status === 'loading'}
  <States state="loading" />
{:else if page.status === 'error'}
  <States state="error" message={s.loadError} />
{:else if page.items.length === 0}
  <States state="empty" message={s.empty} />
{:else}
  <ul class="flex flex-col gap-3">
    <!-- Hidden jobs are always posting-backed: hiding is a mark on a posting. -->
    {#each page.items.filter((i) => i.job) as item (item.id)}
      {@const job = must(item.job, 'item.job')}
      <li>
        <!-- Un-hide lives in the card's footer row (a divided sibling of the link),
             not an overlay, so it never sits on top of the title or blurb on a narrow
             screen. -->
        <JobRow {job} dimViewed={false}>
          {#snippet footer()}
            <div class="flex justify-end">
              <button
                type="button"
                onclick={() => unhide(job.public_slug)}
                disabled={unhiding === job.public_slug}
                title={s.unhideTitle}
                class="inline-flex items-center gap-1.5 rounded-lg px-2 py-1 text-xs font-medium text-muted-foreground transition hover:text-brand disabled:pointer-events-none disabled:opacity-50"
              >
                <Eye class="size-4" aria-hidden="true" />
                {s.unhide}
              </button>
            </div>
          {/snippet}
        </JobRow>
      </li>
    {/each}
  </ul>
  {#if page.hasMore}
    <LoadMore loading={page.loadingMore} error={page.loadMoreError} onclick={() => page.loadMore()} />
  {/if}
{/if}
