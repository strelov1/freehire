<script lang="ts">
  import { onMount, untrack } from 'svelte';
  import { browser } from '$app/environment';
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import { api, type Slice } from '$lib/api';
  import { Paginator } from '$lib/paginated.svelte';
  import { syncOnNavigation } from '$lib/urlSynced.svelte';
  import { CompanyFilterStore, companyFiltersToParams } from '$lib/companyFilters';
  import { setListSearchTarget } from '$lib/listSearch.svelte';
  import type { CompanyListItem } from '$lib/types';
  import { Badge } from '$lib/ui';
  import { countryLabel } from '$lib/facets';
  import States from './States.svelte';
  import LoadMore from './LoadMore.svelte';
  import InfiniteScroll from './InfiniteScroll.svelte';
  import CompanyLogo from './CompanyLogo.svelte';
  import CompanyFilterSummary from './filters/CompanyFilterSummary.svelte';
  import CompanyFilterModal from './filters/CompanyFilterModal.svelte';
  import ListToolbar from './ListToolbar.svelte';

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
  let modalOpen = $state(false);

  // `initial` was fetched for page.url; if a shallow-routing back/forward left
  // page.url lagging the address bar, it's stale — reload on the first run instead.
  const initialStale = browser && page.url.search !== location.search;

  // Register this page's store so the header search drives it (the header hosts
  // the text field here — see HeaderListSearch).
  onMount(() => {
    setListSearchTarget(filters);
    return () => {
      setListSearchTarget(null);
      filters.dispose();
    };
  });

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
      <CompanyFilterSummary store={filters} onOpen={() => (modalOpen = true)} />
    </div>
  </aside>

  <div class="min-w-0 flex-1">
    <ListToolbar
      total={companies.status === 'ready' && companies.items.length > 0 ? companies.total : null}
      unit={companies.total === 1 ? 'company' : 'companies'}
      active={filters.active}
      onOpenFilters={() => (modalOpen = true)}
    />
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
      <div class="flex flex-col gap-3">
        {#each companies.items as company (company.slug)}
          {@const industry = company.industries?.[0]}
          {@const hq = company.hq_country ? countryLabel(company.hq_country) : null}
          <a
            href={resolve('/companies/[slug]', { slug: company.slug })}
            class="flex items-start gap-2.5 rounded-lg border border-border px-4 py-3 transition-colors hover:bg-accent"
          >
            <CompanyLogo name={company.name} size="size-8" />
            <div class="flex min-w-0 flex-1 flex-col gap-1">
              <div class="flex items-center justify-between gap-2">
                <span class="truncate font-medium">{company.name}</span>
                <Badge variant="outline" class="shrink-0">{company.job_count} jobs</Badge>
              </div>
              {#if company.tagline}
                <p class="line-clamp-1 text-sm text-muted-foreground">{company.tagline}</p>
              {/if}
              {#if industry || hq}
                <div class="flex flex-wrap gap-1.5">
                  {#if industry}<Badge variant="secondary">{industry}</Badge>{/if}
                  {#if hq}<Badge variant="secondary">{hq}</Badge>{/if}
                </div>
              {/if}
            </div>
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

<CompanyFilterModal store={filters} open={modalOpen} onClose={() => (modalOpen = false)} />
