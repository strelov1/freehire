<script lang="ts">
  import { onMount, untrack } from 'svelte';
  import { page } from '$app/state';
  import { replaceState } from '$app/navigation';
  import { api, type Slice } from '$lib/api';
  import { Paginator } from '$lib/paginated.svelte';
  import type { CompanyListItem } from '$lib/types';
  import { Badge, Input } from '$lib/ui';
  import States from './States.svelte';
  import LoadMore from './LoadMore.svelte';
  import CompanyLogo from './CompanyLogo.svelte';

  // The first page is server-rendered (route `load`) for the current ?q, so the
  // rows are in the initial HTML.
  let { initial }: { initial: Slice<CompanyListItem> } = $props();

  // Search lives in the URL (?q=) so it survives reload, sharing, and
  // back/forward — the jobs-list pattern scaled down to a single field (no
  // FilterStore, which models job-only facets). Seeded from the current URL.
  let q = $state(page.url.searchParams.get('q') ?? '');

  const makePaginator = () =>
    new Paginator<CompanyListItem>((limit, offset) => api.listCompanies(q, limit, offset));

  const seeded = makePaginator();
  seeded.seed(untrack(() => initial));
  let companies = $state.raw(seeded);

  let timer: ReturnType<typeof setTimeout>;

  // A debounce timer left running after unmount would start a fetch for a
  // component that no longer exists.
  onMount(() => () => clearTimeout(timer));

  function reload() {
    companies = makePaginator();
    companies.start();
  }

  // Typing: update q and mirror it to the URL *synchronously* in this handler,
  // then re-query debounced. Writing the URL here rather than in an effect keeps
  // q and the URL in lockstep within the same tick, so the back/forward effect
  // below never runs mid-keystroke against a stale URL and reverts the input.
  function search(value: string) {
    q = value;
    const params = new URLSearchParams();
    if (q) params.set('q', q);
    const qs = params.toString();
    replaceState(page.url.pathname + (qs ? `?${qs}` : ''), {});
    clearTimeout(timer);
    timer = setTimeout(reload, 300);
  }

  // Browser back/forward changes the URL externally — pull it into q and
  // re-query. Track *only* the URL: reading q reactively here would also fire the
  // effect on our own search()/replaceState write, against a page.url that hasn't
  // updated yet (it propagates a tick later), reverting the just-typed value. So
  // read q under untrack — the effect runs only on real URL changes, when page.url
  // is current, and no-ops on initial mount (q is seeded from the same URL).
  $effect(() => {
    page.url.search; // track
    untrack(() => {
      const urlQ = page.url.searchParams.get('q') ?? '';
      if (urlQ !== q) {
        q = urlQ;
        clearTimeout(timer);
        reload();
      }
    });
  });
</script>

<div class="mb-4">
  <Input
    type="search"
    value={q}
    oninput={(e) => search(e.currentTarget.value)}
    placeholder="Search companies…"
    aria-label="Search companies"
    class="w-full"
  />
</div>

{#if companies.status === 'loading'}
  <States state="loading" />
{:else if companies.status === 'error'}
  <States state="error" message="Failed to load companies." />
{:else if companies.items.length === 0}
  <States state="empty" message={q ? 'No matching companies.' : 'No companies yet.'} />
{:else}
  <p class="mb-3 text-sm text-muted-foreground" aria-live="polite">
    {companies.total.toLocaleString()} {companies.total === 1 ? 'company' : 'companies'}
  </p>
  <div class="flex flex-col gap-3">
    {#each companies.items as company (company.slug)}
      <a
        href={`/companies/${company.slug}`}
        class="flex items-center justify-between rounded-lg border border-border px-4 py-3 transition-colors hover:bg-accent"
      >
        <span class="flex min-w-0 items-center gap-2.5">
          <CompanyLogo name={company.name} size="size-6" />
          <span class="truncate font-medium">{company.name}</span>
        </span>
        <Badge variant="outline">{company.job_count} jobs</Badge>
      </a>
    {/each}
  </div>

  {#if companies.hasMore}
    <LoadMore
      loading={companies.loadingMore}
      error={companies.loadMoreError}
      onclick={() => companies.loadMore()}
    />
  {/if}
{/if}
