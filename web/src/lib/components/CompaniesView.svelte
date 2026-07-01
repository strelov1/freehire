<script lang="ts">
  import { onMount, untrack } from 'svelte';
  import { browser } from '$app/environment';
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import { api, type Slice } from '$lib/api';
  import { Paginator } from '$lib/paginated.svelte';
  import { UrlSyncedState, syncOnNavigation } from '$lib/urlSynced.svelte';
  import { SvelteURLSearchParams } from 'svelte/reactivity';
  import type { CompanyListItem } from '$lib/types';
  import { Badge, Input } from '$lib/ui';
  import States from './States.svelte';
  import LoadMore from './LoadMore.svelte';
  import InfiniteScroll from './InfiniteScroll.svelte';
  import CompanyLogo from './CompanyLogo.svelte';

  // The first page is server-rendered (route `load`) for the current ?q, so the
  // rows are in the initial HTML.
  let { initial }: { initial: Slice<CompanyListItem> } = $props();

  // Search lives in the URL (?q=) so it survives reload, sharing, and
  // back/forward. The shared primitive owns the state<->URL transport and the
  // reload debounce; here it's a single string (no FilterStore, which models
  // job-only facets). `value` drives the input, `applied` drives the fetch.
  const search = new UrlSyncedState<string>(page.url.searchParams, {
    parse: (p) => p.get('q') ?? '',
    serialize: (q) => {
      const p = new SvelteURLSearchParams();
      if (q) p.set('q', q);
      return p;
    },
  });

  // Fetch against the debounced query so typing doesn't issue a request per key.
  const makePaginator = () =>
    new Paginator<CompanyListItem>((limit, offset) => api.listCompanies(search.applied, limit, offset));

  const seeded = makePaginator();
  seeded.seed(untrack(() => initial));
  let companies = $state.raw(seeded);
  let started = false;

  // `initial` was searched for page.url; if a shallow-routing back/forward left
  // page.url lagging the address bar, it's stale — reload on the first run instead.
  const initialStale = browser && page.url.search !== location.search;

  onMount(() => () => search.dispose());

  function reload() {
    companies = makePaginator();
    companies.start();
  }

  // Reload whenever the debounced query changes (typing settled, or back/forward
  // re-seeded it). Skip the first run: the SSR `initial` already rendered page one.
  $effect(() => {
    void search.applied; // track the debounced query
    untrack(() => {
      if (!started) {
        started = true;
        if (!initialStale) return;
      }
      reload();
    });
  });

  // Browser back/forward re-seeds the query from the URL.
  syncOnNavigation(search);
</script>

<div class="mb-4">
  <Input
    type="search"
    value={search.value}
    oninput={(e) => search.setSoon(e.currentTarget.value)}
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
  <States state="empty" message={search.value ? 'No matching companies.' : 'No companies yet.'} />
{:else}
  <p class="mb-3 text-sm text-muted-foreground" aria-live="polite">
    {companies.total.toLocaleString()} {companies.total === 1 ? 'company' : 'companies'}
  </p>
  <div class="flex flex-col gap-3">
    {#each companies.items as company (company.slug)}
      <a
        href={resolve('/companies/[slug]', { slug: company.slug })}
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
    <!-- Scroll-to-bottom auto-load; the button below stays as the accessible
         fallback (keyboard/screen-reader, and retry on a failed load). -->
    <InfiniteScroll
      onLoad={() => companies.loadMore()}
      enabled={!companies.loadingMore && !companies.loadMoreError}
    />
    <LoadMore
      loading={companies.loadingMore}
      error={companies.loadMoreError}
      onclick={() => companies.loadMore()}
    />
  {/if}
{/if}
