<script lang="ts">
  import { untrack } from 'svelte';
  import { browser } from '$app/environment';
  import { page } from '$app/state';
  import { api } from '$lib/api';
  import type { CatalogueMember } from '$lib/generated/contracts';
  import { Paginator } from '$lib/paginated.svelte';
  import { pageCount, pageOffset } from '$lib/pagination';
  import { TalentFilterStore } from '$lib/talentFilters';
  import { syncOnNavigation } from '$lib/urlSynced.svelte';
  import { talentFiltersToParams } from '$lib/talentFacetModel';
  import type { Slice } from '$lib/api';
  import ListToolbar from './ListToolbar.svelte';
  import Pagination from './Pagination.svelte';
  import States from './States.svelte';
  import TalentCard from './TalentCard.svelte';
  import TalentFilterModal from './filters/TalentFilterModal.svelte';
  import TalentFilterSummary from './filters/TalentFilterSummary.svelte';

  // The public Talent Network catalogue, on the same chrome as the job and company
  // lists: a sidebar of applied filters, a toolbar with the count, and a pager.
  //
  // HYBRID, and deliberately, the same shape CompaniesView has. `initial` is
  // server-rendered for the requested URL, so a shared or crawled link arrives already
  // filtered in the HTML. After that the filter store owns the URL through
  // `replaceState` — shallow routing, which does NOT re-run `load` — so subsequent
  // filter changes reload the list client-side off `applied`. Getting that wrong is not
  // subtle: the address bar would change and the list would not.

  let {
    initial,
    currentPage,
  }: { initial: Slice<CatalogueMember>; currentPage: number } = $props();

  const filters = new TalentFilterStore(page.url.searchParams);

  const makePaginator = () =>
    new Paginator<CatalogueMember>((limit, offset) => {
      const params = talentFiltersToParams(filters.applied);
      params.set('limit', String(limit));
      params.set('offset', String(offset));
      return api.listTalent(params.toString());
    });

  // Seeded from the server-rendered page — a one-time snapshot of the props, re-taken by
  // the effect below when the filters actually change.
  const seeded = makePaginator();
  untrack(() => seeded.seed(initial, pageOffset(currentPage)));
  let members = $state.raw(seeded);

  let activePage = $state(untrack(() => currentPage));
  let modalOpen = $state(false);
  let started = false;

  // The applied filters as a string, so a re-seed to the same set can be told from a real
  // change. Seeded with what the route searched with, not left empty: an empty one would
  // make the first client run look like a new query and snap page 3 back to 1.
  let lastSearchKey = untrack(() => talentFiltersToParams(filters.applied).toString());

  // `initial` was fetched for page.url; if a shallow-routing back/forward left page.url
  // lagging the address bar, it is stale — reload on the first run instead of trusting it.
  const initialStale = browser && page.url.search !== location.search;

  $effect(() => {
    const key = talentFiltersToParams(filters.applied).toString();
    const changed = key !== lastSearchKey;

    if (!started) {
      started = true;
      lastSearchKey = key;
      if (!initialStale) return;
    } else if (!changed) {
      return;
    }

    // A genuinely new filter starts at the first page: staying on page four of a
    // narrower result lands on an empty page that reads as no matches.
    if (changed) activePage = 1;
    lastSearchKey = key;

    const next = makePaginator();
    members = next;
    void next.start(pageOffset(activePage));
  });

  // Re-seed when a page link is followed: SvelteKit REUSES this component across
  // `?page=N`, so the props arrive again and the state seeded from them does not. Without
  // this the pager highlights page 2 while the list still shows page 1 — the props
  // changed and nothing read them. No fetch: the route already listed exactly this page.
  // CompaniesView and JobsView both carry this effect; copying only the filter one is how
  // it went missing here.
  $effect(() => {
    const nextPage = currentPage;
    const slice = initial;
    untrack(() => {
      if (nextPage === activePage) return;
      activePage = nextPage;
      const next = makePaginator();
      next.seed(slice, pageOffset(nextPage));
      members = next;
    });
  });

  // Browser back/forward re-seeds the filters from the URL. Without it, a `replaceState`
  // written by the store and then stepped over leaves the store and the address bar
  // disagreeing about what is filtered.
  syncOnNavigation(filters);

  const total = $derived(members.total ?? 0);
  const pages = $derived(pageCount(members.total));
</script>

<div class="mx-auto flex w-full max-w-6xl gap-6 px-4 py-8">
  <aside class="hidden w-72 shrink-0 md:block">
    <!-- `top-20` is the site header's own `h-14` plus 24px of air, matching the companies
         sidebar: at `top-6` the opaque sticky header covers the card's first rows. -->
    <div
      class="sticky top-20 max-h-[calc(100vh-6.5rem)] overflow-y-auto rounded-xl border border-border bg-card p-4"
    >
      <TalentFilterSummary store={filters} onOpen={() => (modalOpen = true)} />
    </div>
  </aside>

  <div class="min-w-0 flex-1">
    <header class="mb-4">
      <h1 class="text-2xl font-semibold tracking-tight">Talent Network</h1>
    </header>

    <ListToolbar
      total={members.status === 'ready' && members.items.length > 0 ? members.total : null}
      unit={total === 1 ? 'candidate' : 'candidates'}
    />

    {#if filters.value.facets.tz?.length}
      <!-- Stated rather than left to be discovered: a timezone filter drops everyone whose
      zone is unknown, which otherwise reads as a small talent pool rather than a missing
      field. -->
      <p class="mb-3 text-xs text-muted-foreground">
        Filtering by timezone hides candidates who have not set one.
      </p>
    {/if}

    {#if members.status === 'loading'}
      <States state="loading" />
    {:else if members.status === 'error'}
      <States state="error" message="Failed to load candidates." />
    {:else if members.items.length === 0}
      <States
        state="empty"
        message="Nobody matches yet. Try widening the filters — the catalogue grows as candidates join."
      />
    {:else}
      <div class="flex flex-col gap-3">
        <!-- Keyed by handle: unique per member and stable across pages, unlike an index,
        which would make Svelte reuse a card for a different person on navigation. -->
        {#each members.items as member (member.handle)}
          <TalentCard {member} />
        {/each}
      </div>

      {#if pages > 1}
        <Pagination
          current={activePage}
          total={pages}
          pathname={page.url.pathname}
          params={filters.params}
        />
      {/if}
    {/if}
  </div>
</div>

<TalentFilterModal store={filters} open={modalOpen} onClose={() => (modalOpen = false)} />
