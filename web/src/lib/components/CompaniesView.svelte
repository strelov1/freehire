<script lang="ts">
  import { onMount, untrack } from 'svelte';
  import { Star } from '@lucide/svelte';
  import { browser } from '$app/environment';
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import { api, type Slice } from '$lib/api';
  import { Paginator } from '$lib/paginated.svelte';
  import { syncOnNavigation } from '$lib/urlSynced.svelte';
  import { CompanyFilterStore, companyFiltersToParams, type CompanySortField } from '$lib/companyFilters';
  import { setListSearchTarget } from '$lib/listSearch.svelte';
  import type { CompanyListItem } from '$lib/types';
  import { Badge, CountryFlag, EntityLogo } from '$lib/ui';
  import { countryLabel } from '$lib/facets';
  import { companyLogoUrl } from '$lib/logo';
  import { pageCount, pageOffset } from '$lib/pagination';
  import States from './States.svelte';
  import Pagination from './Pagination.svelte';
  import BackerBadge from './BackerBadge.svelte';
  import CompanyFilterSummary from './filters/CompanyFilterSummary.svelte';
  import CompanyFilterModal from './filters/CompanyFilterModal.svelte';
  import ListToolbar from './ListToolbar.svelte';

  // The requested page is server-rendered (route `load`) for the current filters,
  // so the rows are in the initial HTML. `currentPage` is the `?page=N` that load
  // served: page links are the only way through the list, so it is required rather
  // than optional — see JobsView, which holds the same contract.
  let { initial, currentPage }: { initial: Slice<CompanyListItem>; currentPage: number } = $props();

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

  // Seeded from the server-rendered page — an intentional one-time snapshot of the
  // props, which the effects below re-take when the page or the query changes.
  const seeded = makePaginator();
  untrack(() => seeded.seed(initial, pageOffset(currentPage)));
  let companies = $state.raw(seeded);

  // The page being read. Starts at the route's `?page=N` and is reset only by a
  // genuinely NEW query — see the effect below for why "the filters changed" is not
  // the same question.
  let activePage = $state(untrack(() => currentPage));
  let modalOpen = $state(false);
  let started = false;
  // The applied filters as a string, so a re-seed to the same set can be told from a
  // real change. Seeded with what the route searched with, not left empty: an empty
  // one would make the first client run look like a new query and snap page 3 to 1.
  let lastSearchKey = untrack(() => companyFiltersToParams(filters.applied).toString());

  // `initial` was fetched for page.url; if a shallow-routing back/forward left
  // page.url lagging the address bar, it's stale — reload on the first run instead.
  const initialStale = browser && page.url.search !== location.search;

  // Register this page's store so the header search drives it (the header hosts
  // the text field here — see HeaderListSearch). The adapter also exposes a
  // `companies` filter scope so the header's Location popover shows region +
  // remote-hiring pills here (no facet counts are fetched on this page, so counts
  // is null and the pills render countless).
  onMount(() => {
    setListSearchTarget({
      get value() {
        return filters.value;
      },
      setQuery: (q) => filters.setQuery(q),
      filterScope: { store: filters, counts: () => null, variant: 'companies' },
      openFilters: () => (modalOpen = true),
      activeFilters: () => filters.active,
    });
    return () => {
      setListSearchTarget(null);
      filters.dispose();
    };
  });

  function reload(offset = 0) {
    companies = makePaginator();
    companies.start(offset);
  }

  // Reload whenever the debounced filters change (typing settled, a facet toggled,
  // or back/forward re-seeded them). Skip the first run: the SSR `initial` already
  // rendered the page the URL asked for.
  $effect(() => {
    void filters.applied; // track the debounced filters
    untrack(() => {
      if (!started) {
        started = true;
        if (!initialStale) return;
      }
      // "The filters object changed" is not "the visitor searched for something
      // else": the store re-seeds on navigation and rewrites the URL, and both fire
      // this effect with the filters unchanged. Only a changed query means a
      // different result set, and so page 1; anything else reloads the page being
      // read. Without the distinction `/companies?page=3` rendered page 3 and snapped
      // back to page 1 the moment it hydrated. Mirrors JobsView.
      const searchKey = companyFiltersToParams(filters.applied).toString();
      const sameQuery = searchKey === lastSearchKey;
      lastSearchKey = searchKey;
      if (!sameQuery) activePage = 1;
      reload(sameQuery ? pageOffset(activePage) : 0);
    });
  });

  // Re-seed when a page link is followed: SvelteKit reuses this component across
  // `?page=N`, so the props arrive again and the state seeded from them does not.
  // No fetch — the route already listed exactly this page. See JobsView, which
  // carries the same effect and the longer explanation.
  $effect(() => {
    const nextPage = currentPage;
    const slice = initial;
    untrack(() => {
      if (nextPage === activePage) return;
      activePage = nextPage;
      const next = makePaginator();
      next.seed(slice, pageOffset(nextPage));
      companies = next;
    });
  });

  // Browser back/forward re-seeds the filters from the URL.
  syncOnNavigation(filters);
</script>

<!-- The catalog sort control, handed to ListToolbar so it sits in the shared toolbar
     (mobile) / above the list (desktop) — same shape as the jobs feed's sortSelect.
     "Highest rated" forces the Postgres path server-side even alongside a search/
     facet (see companyFacetModel.ts's CompanySortField doc comment) — rating isn't
     a Meili-sortable attribute yet. -->
{#snippet sortSelect()}
  <label class="flex shrink-0 items-center gap-1.5 text-sm text-muted-foreground">
    <span class="hidden sm:inline">Sort</span>
    <select
      aria-label="Sort companies"
      class="rounded-lg border border-input bg-transparent py-2 pl-2 pr-1 text-sm text-foreground transition-colors focus-visible:border-ring focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50 md:py-1 dark:bg-input/30"
      value={filters.value.sort}
      onchange={(e) => filters.setSort(e.currentTarget.value as CompanySortField)}
    >
      <option value="job_count">Most active</option>
      <option value="rating">Highest rated</option>
    </select>
  </label>
{/snippet}

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
      sortControl={sortSelect}
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
            <EntityLogo name={company.name} src={companyLogoUrl(company.name) ?? undefined} shape="square" size="sm" />
            <div class="flex min-w-0 flex-1 flex-col gap-1">
              <div class="flex items-center justify-between gap-2">
                <span class="truncate font-medium">{company.name}</span>
                <!-- The whole card is a link, so the marks stay display-only here.
                     They sit with the name because a backer is a fact about the
                     company, the same placement the job feed card uses. -->
                <BackerBadge collections={company.collections} class="ml-0.5" />
                <span class="ml-auto flex shrink-0 items-center gap-1.5">
                  {#if company.feedback_rating_avg !== null}
                    <Badge variant="secondary" class="flex items-center gap-1">
                      <Star class="size-3" fill="currentColor" aria-hidden="true" />
                      {company.feedback_rating_avg.toFixed(1)}
                    </Badge>
                  {/if}
                  <Badge variant="outline">{company.job_count} jobs</Badge>
                </span>
              </div>
              {#if company.tagline}
                <p class="line-clamp-1 text-sm text-muted-foreground">{company.tagline}</p>
              {/if}
              {#if industry || hq}
                <div class="flex flex-wrap gap-1.5">
                  {#if industry}<Badge variant="secondary">{industry}</Badge>{/if}
                  {#if hq}<Badge variant="secondary" class="gap-1.5"><CountryFlag code={company.hq_country ?? ''} label={hq} />{hq}</Badge>{/if}
                </div>
              {/if}
            </div>
          </a>
        {/each}
      </div>

      <!-- Page links, and the only way through the list: a scroll-to-bottom auto-load
           used to sit here, which grew the page every time the reader neared the end
           of it and put the footer permanently out of reach. -->
      <Pagination
        current={activePage}
        total={pageCount(companies.total)}
        pathname={page.url.pathname}
        params={filters.params}
      />
    {/if}
  </div>
</div>

<CompanyFilterModal store={filters} open={modalOpen} onClose={() => (modalOpen = false)} />
