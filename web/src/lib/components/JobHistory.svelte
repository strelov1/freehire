<script lang="ts">
  import { api } from '$lib/api';
  import { locale } from '$lib/i18n/currentLocale.svelte';
  import { t } from '$lib/i18n/t';
  import { isAuthenticated } from '$lib/auth.svelte';
  import { Paginator } from '$lib/paginated.svelte';
  import JobRow from './JobRow.svelte';
  import { LoadMore } from '$lib/ui';
  import { messages } from './activity.messages';
  import States from './States.svelte';
  import { must } from '$lib/utils';

  const s = $derived(t(messages, locale()).history);
  const page = new Paginator(async (limit, offset) => {
    const slice = await api.listMyJobs('viewed', limit, offset);
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
    <!-- Viewed history is always posting-backed: a row with no posting is an
         application, and an application was never a view. The guard is the type's,
         not a real case. -->
    {#each page.items.filter((i) => i.job) as item (item.id)}
      <li><JobRow job={must(item.job, 'item.job')} dimViewed={false} /></li>
    {/each}
  </ul>
  {#if page.hasMore}
    <LoadMore loading={page.loadingMore} error={page.loadMoreError} onclick={() => page.loadMore()} />
  {/if}
{/if}
