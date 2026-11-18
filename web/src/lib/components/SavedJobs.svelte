<script lang="ts">
  import { api } from '#lib/api.js';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { t } from '#lib/i18n/t.js';
  import { currentUser, isAuthenticated } from '#lib/auth.svelte.js';
  import { serialQueue } from '#lib/serialQueue.js';
  import { Paginator } from '#lib/paginated.svelte.js';
  import JobRow from './JobRow.svelte';
  import { LoadMore } from '#lib/ui/index.js';
  import { messages } from './activity.messages';
  import States from './States.svelte';
  import { must } from '#lib/utils.js';

  const s = $derived(t(messages, locale()).saved);
  const page = new Paginator((limit, offset) => api.listMyJobs('saved', limit, offset));
  const queue = serialQueue();

  $effect(() => {
    if (isAuthenticated()) void queue(() => page.start());
  });

  function unsave(slug: string): Promise<void> {
    const userID = currentUser()?.id;
    return queue(async () => {
      if (currentUser()?.id !== userID) throw new Error('Session ended.');
      await api.unsaveJob(slug);
      page.items = page.items.filter((item) => item.job?.public_slug !== slug);
      page.total = Math.max(0, page.total - 1);
      if (page.items.length === 0 && page.hasMore) await page.loadMore();
    });
  }

  function loadMore() {
    void queue(() => page.loadMore());
  }
</script>

{#if page.status === 'loading'}
  <States state="loading" />
{:else if page.status === 'error'}
  <States state="error" message={s.loadError} />
{:else if page.items.length === 0 && !page.hasMore}
  <States state="empty" message={s.empty} />
{:else}
  <ul class="flex flex-col gap-3">
    <!-- Saved jobs are always posting-backed: the bookmark lives on the posting and
         goes with it. The guard satisfies the type rather than a real case. -->
    {#each page.items.filter((i) => i.job) as item (item.id)}
      <li>
        <JobRow job={must(item.job, 'item.job')} dimViewed={false} {unsave} />
      </li>
    {/each}
  </ul>
  {#if page.hasMore}
    <LoadMore loading={page.loadingMore} error={page.loadMoreError} onclick={loadMore} />
  {/if}
{/if}
