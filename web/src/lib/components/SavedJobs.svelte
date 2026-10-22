<script lang="ts">
  import { api } from '#lib/api.js';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { t } from '#lib/i18n/t.js';
  import { isAuthenticated } from '#lib/auth.svelte.js';
  import { Paginator } from '#lib/paginated.svelte.js';
  import JobRow from './JobRow.svelte';
  import { LoadMore } from '#lib/ui/index.js';
  import { messages } from './activity.messages';
  import States from './States.svelte';
  import { must } from '#lib/utils.js';

  const s = $derived(t(messages, locale()).saved);
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
  <States state="error" message={s.loadError} />
{:else if page.items.length === 0}
  <States state="empty" message={s.empty} />
{:else}
  <ul class="flex flex-col gap-3">
    <!-- Saved jobs are always posting-backed: the bookmark lives on the posting and
         goes with it. The guard satisfies the type rather than a real case. -->
    {#each page.items.filter((i) => i.job) as item (item.id)}
      <li>
        <JobRow job={must(item.job, 'item.job')} dimViewed={false} />
      </li>
    {/each}
  </ul>
  {#if page.hasMore}
    <LoadMore loading={page.loadingMore} error={page.loadMoreError} onclick={() => page.loadMore()} />
  {/if}
{/if}
