<script lang="ts">
  import { onMount, untrack } from 'svelte';
  import { browser } from '$app/environment';
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import { api, type Slice } from '$lib/api';
  import { Paginator } from '$lib/paginated.svelte';
  import { syncOnNavigation } from '$lib/urlSynced.svelte';
  import { CompanyFilterStore, companyFiltersToParams } from '$lib/companyFilters';
  import type { CompanyListItem } from '$lib/types';
  import { Badge, Input } from '$lib/ui';
  import States from './States.svelte';
  import LoadMore from './LoadMore.svelte';
  import InfiniteScroll from './InfiniteScroll.svelte';
  import CompanyLogo from './CompanyLogo.svelte';
  import CompanyFiltersPanel from './CompanyFiltersPanel.svelte';

  // The first page is server-rendered (route `load`) for the current filters, so
  // the rows are in the initial HTML.
  let { initial }: { initial: Slice<CompanyListItem> } = $props();

  // The search query and sidebar facets live in the URL so a filtered view survives
  // reload, sharing, and back/forward. The store owns the state<->URL transport and
  // the reload debounce; `value` drives inputs, `applied` (debounced) drives the
  // fetch.
  const filters = new CompanyFilterStore(page.url.searchParams);

  // Fetch against the debounced filters so typing/toggling doesn't issue a request
  // per change. `q` lives inside the serialized params, so it's passed as ''.
  const makePaginator = () =>
    new Paginator<CompanyListItem>((limit, offset) =>
      api.listCompanies('', limit, offset, companyFiltersToParams(filters.applied)),
    );

  const seeded = makePaginator();
  seeded.seed(untrack(() => initial));
  let companies = $state.raw(seeded);
  let started = false;
  let drawerOpen = $state(false);

  // `initial` was fetched for page.url; if a shallow-routing back/forward left
  // page.url lagging the address bar, it's stale — reload on the first run instead.
  const initialStale = browser && page.url.search !== location.search;

  onMount(() => () => filters.dispose());

  function reload() {
    companies = makePaginator();
    companies.start();
  }

  // Reload whenever the debounced filters change (typing settled, a facet toggled,
  // or back/forward re-seeded them). Skip the first run: the SSR `initial` already
  // rendered page one.
  $effect(() => {
    void filters.applied; // track the debounced filters
    untrack(() => {
      if (!started) {
        started = true;
        if (!initialStale) return;
      }
      reload();
    });
  });

  // Browser back/forward re-seeds the filters from the URL.
  syncOnNavigation(filters);
</script>

<div class="flex gap-6">
  <aside class="hidden w-72 shrink-0 md:block">
    <div class="sticky top-6 max-h-[calc(100vh-5rem)] overflow-y-auto rounded-xl border border-border bg-card p-4">
      <CompanyFiltersPanel store={filters} />
    </div>
  </aside>

  <div class="min-w-0 flex-1">
    <div class="mb-4 flex items-center gap-2">
      <Input
        type="search"
        value={filters.value.q}
        oninput={(e) => filters.setQuery(e.currentTarget.value)}
        placeholder="Search companies…"
        aria-label="Search companies"
        class="min-w-0 flex-1"
      />
      <button
        type="button"
        class="h-9 shrink-0 rounded-lg border border-border bg-secondary px-3 text-sm font-medium text-secondary-foreground transition-colors hover:bg-accent md:hidden"
        onclick={() => (drawerOpen = true)}
      >
        Filters{#if filters.active > 0}&nbsp;({filters.active}){/if}
      </button>
    </div>

    {#if companies.status === 'loading'}
      <States state="loading" />
    {:else if companies.status === 'error'}
      <States state="error" message="Failed to load companies." />
    {:else if companies.items.length === 0}
      <States
        state="empty"
        message={filters.value.q || filters.active > 0 ? 'No matching companies.' : 'No companies yet.'}
      />
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
  </div>
</div>

{#if drawerOpen}
  <div class="fixed inset-0 z-40 md:hidden">
    <button class="absolute inset-0 bg-black/40" aria-label="Close filters" onclick={() => (drawerOpen = false)}></button>
    <div class="absolute left-0 top-0 flex h-full w-80 max-w-[85%] flex-col overflow-y-auto bg-background p-4 shadow-xl">
      <div class="mb-3 flex items-center justify-between">
        <span class="text-sm font-semibold tracking-tight">Filters</span>
        <button type="button" class="text-sm text-muted-foreground hover:text-foreground" onclick={() => (drawerOpen = false)}>Done</button>
      </div>
      <CompanyFiltersPanel store={filters} />
    </div>
  </div>
{/if}
