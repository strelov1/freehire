<script lang="ts">
  import { onMount, untrack, type Snippet } from 'svelte';
  import { Layers } from '@lucide/svelte';
  import { browser } from '$app/environment';
  import { afterNavigate, goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { api, type Slice } from '$lib/api';
  import { isAuthenticated } from '$lib/auth.svelte';
  import { ensureViewedLoaded } from '$lib/viewedJobs.svelte';
  import { ensureSavedLoaded } from '$lib/savedJobs.svelte';
  import { latestOnly } from '$lib/latestOnly';
  import { Paginator } from '$lib/paginated.svelte';
  import { FilterStore, filtersToParams, activeFilterCount, type SortField } from '$lib/filters';
  import { openAuthDialog } from '$lib/auth-dialog.svelte';
  import { loadJobFilters } from '$lib/filterStorage';
  import {
    bannerVisible,
    loadOnboardingState,
    markDone,
    markSeen,
    narrowestFacet,
    type OnboardingLifecycle,
  } from '$lib/onboarding';
  import { consumePendingAlert } from '$lib/saveSearchAlert';
  import OnboardingWizard from './onboarding/OnboardingWizard.svelte';
  import OnboardingBanner from './onboarding/OnboardingBanner.svelte';
  import OnboardingAlertBanner from './onboarding/OnboardingAlertBanner.svelte';
  import { syncOnNavigation } from '$lib/urlSynced.svelte';
  import { setListSearchTarget } from '$lib/listSearch.svelte';
  import type { Job, FacetCounts } from '$lib/types';
  import FilterSummary from './filters/FilterSummary.svelte';
  import FilterModal from './filters/FilterModal.svelte';
  import ListToolbar from './ListToolbar.svelte';
  import States from './States.svelte';
  import JobRow from './JobRow.svelte';
  import LoadMore from './LoadMore.svelte';
  import InfiniteScroll from './InfiniteScroll.svelte';

  // Filters live in the URL; the route `load` searches by them and returns the
  // first page as `initial`, so the rows are in the initial HTML for SSR/share/
  // reload. After hydration the view is client-driven: FilterStore writes the URL
  // synchronously and exposes a debounced `applied` snapshot; a filter change
  // reloads the list + counts client-side off `applied` (no navigation), and
  // infinite scroll pages the rest.
  //
  // `scope` pins extra search params that the user can't change (e.g. the company
  // page passes `{ company_slug }`): they're merged into every search but kept out
  // of `filters`/the URL, so they're not user-selectable facets. `excludeFacets`
  // hides facets that are redundant under that scope (e.g. Source on a company).
  // `sidebarTop` renders above the filter summary in the desktop sidebar (e.g. the
  // company page's facts card); the standalone /jobs list omits it.
  let {
    initial,
    scope = {},
    excludeFacets = [],
    sidebarTop,
  }: {
    initial: Slice<Job>;
    scope?: Record<string, string>;
    excludeFacets?: string[];
    sidebarTop?: Snippet;
  } = $props();

  // Standalone /jobs (no fixed scope) hands its text search to the header; an
  // embedded, scoped instance (e.g. a company page) keeps its own inline input.
  const standalone = $derived(Object.keys(scope).length === 0);

  // Seed filters from the current URL so the server and the hydrated client
  // render the same filtered view. Only the standalone list persists to storage;
  // the embedded company list must not clobber the shared key. Persistence is
  // fixed for the store's life, so the initial `standalone` is captured once.
  const filters = new FilterStore(page.url.searchParams, untrack(() => standalone));

  // CV-similarity sort is offered only on the standalone feed (the company-embedded
  // list never ranks by the visitor's CV). Read off the debounced `applied` sort so
  // it changes in lockstep with the data reload below.
  const cvMode = $derived(standalone && filters.applied.sort === 'cv');

  // CV ranking needs the authenticated recommendations endpoint, so a signed-out
  // user gets a sign-in prompt instead of a feed: the reload effect skips the fetch
  // and the template stands the prompt in for the list. Reading auth only in CV mode
  // means a sign-in there re-triggers the effect while the default feed is untouched.
  const cvSignInPrompt = $derived(cvMode && !isAuthenticated());

  // An empty CV feed is ambiguous — no usable CV vector and a filter matching
  // nothing both return []. Disambiguate on whether a facet filter is applied
  // (keyed on the debounced `applied` set the feed was fetched with, not the live
  // value, so a mid-drag change can't flash the wrong empty state).
  const appliedActive = $derived(activeFilterCount(filters.applied));

  // The user's (debounced) facet filters plus the fixed `scope` params
  // (company_slug, …). Reads `applied` so typing doesn't fetch per keystroke. In CV
  // mode two params are dropped so the request matches what the recommendations
  // endpoint actually honors: `sort=cv` (a frontend routing signal — see
  // makePaginator) and the free-text `q` (the endpoint ranks by the CV vector and
  // ignores query text). Dropping `q` also keeps the facet counts consistent with
  // the ranked feed instead of counting a q-filtered set the feed never uses.
  const scopedParams = () => {
    const p = filtersToParams(filters.applied);
    if (filters.applied.sort === 'cv') {
      p.delete('sort');
      p.delete('q');
    }
    for (const [k, v] of Object.entries(scope)) p.set(k, v);
    return p;
  };

  // In CV-sort mode the feed ranks by the signed-in user's CV vector via the
  // recommendations endpoint (facet filters still constrain the set); otherwise it
  // browses via keyword search. Both take the same facet params and return the same
  // Job shape, so only the fetch fn differs. CV sort is standalone-only.
  const makePaginator = () =>
    new Paginator<Job>((limit, offset) =>
      cvMode ? api.recommendations(scopedParams(), limit, offset) : api.searchJobs(scopedParams(), limit, offset),
    );

  // Seeded with the server-rendered first page (an intentional one-time snapshot
  // of the initial prop); "load more" and filter changes fetch client-side.
  const seeded = makePaginator();
  seeded.seed(untrack(() => initial));
  let jobs = $state.raw(seeded);

  // The live facet distribution (value → count per facet), feeding the dynamic
  // selects (skills, countries) so the user sees which values exist and how many
  // jobs each has under the current filters. A failed fetch leaves the prior
  // counts — the selects degrade to plain (countless) options, never break.
  // latestOnly stops a slow earlier response overwriting a newer one.
  let counts = $state.raw<FacetCounts | null>(null);
  const refreshCounts = latestOnly(
    () => api.facetCounts(scopedParams()),
    (c) => (counts = c),
  );

  let modalOpen = $state(false);
  let started = false;

  // Onboarding: the one-time nudge banner + wizard, standalone-only. The lifecycle
  // lives in localStorage (client-only); seed it at init on the client so a returning
  // (dismissed/completed) visitor never flashes a banner before mount. The banner is
  // the sole entry — once dismissed or completed it retires; there is no persistent
  // re-open control.
  let wizardOpen = $state(false);
  let onboardingState = $state<OnboardingLifecycle>(browser ? loadOnboardingState() : 'unseen');
  // The ephemeral post-onboarding Telegram-alert offer (set after the wizard, or on
  // mount to resume a pending alert after sign-in). Dismissible; not persisted.
  let alertBanner = $state<{ query: string; autostart: boolean } | null>(null);
  // Show only to an un-nudged visitor with no active facet filters AND no text query,
  // so a shared search/filter link is never interrupted (activeFilterCount ignores the
  // query, so check it explicitly). Gated on `browser`: never SSR the banner.
  const showBanner = $derived(
    browser && standalone && bannerVisible(onboardingState, filters.active > 0 || filters.value.q.trim() !== ''),
  );

  function dismissBanner() {
    markSeen();
    onboardingState = 'seen';
  }
  function cancelWizard() {
    wizardOpen = false;
    markSeen();
    onboardingState = loadOnboardingState(); // markSeen never downgrades a completed run
  }
  function completeWizard(query: string) {
    // Apply through the same store path as a saved search — feed, counts, and
    // localStorage all reconfigure via the existing effect; then the banner retires.
    filters.apply(query);
    markDone();
    onboardingState = 'done';
    wizardOpen = false;
    // Peak intent: offer to keep this feed as a Telegram alert.
    alertBanner = { query, autostart: false };
  }
  // Narrow-feed relief: the single narrowest applied facet to drop (skills → regions →
  // seniority; never the role), or null if none. Shared by the empty-state guard and
  // the relax action so they can't disagree. Never broadens on its own.
  const relaxTarget = $derived(narrowestFacet(filters.value));
  function relaxFeed() {
    if (relaxTarget) filters.clearFacet(relaxTarget);
  }

  // Resume a save the user started before signing in. Reactive on the session so it
  // fires whether auth resolved server-side (an OAuth full-page redirect — mount is
  // already authed) or flipped in-tab (an in-dialog sign-in re-resolves page.data.user
  // without a remount). Consumed exactly once; standalone list only; skipped if a banner
  // is already showing.
  $effect(() => {
    if (!untrack(() => standalone) || alertBanner) return; // effect: client-only, no browser guard
    if (!isAuthenticated()) return;
    const pending = consumePendingAlert();
    if (pending !== null) alertBanner = { query: pending, autostart: true };
  });

  // Live disjunctive facet counts for the staged filter set (built by the modal),
  // merged with the fixed scope params (e.g. company_slug) so every control's counts —
  // and the "Show N jobs" total — match the list. Disjunctive so a selected facet
  // still shows its siblings' counts.
  const stagedCounts = (params: URLSearchParams) => {
    const p = new URLSearchParams(params);
    for (const [k, v] of Object.entries(scope)) p.set(k, v);
    return api.facetCounts(p, { disjunctive: true });
  };

  // The server-rendered `initial` was searched for `page.url`. After a back/forward
  // onto a shallow-routing entry that lags page.url behind the address bar, `initial`
  // is stale (it holds the pre-filter page) while the filters seed from the real URL.
  // Detect that mismatch so the first effect run reloads instead of keeping `initial`.
  const initialStale = browser && page.url.search !== location.search;

  // For a signed-in user, load the set of already-viewed and already-saved slugs so
  // JobRow can dim seen cards and fill saved bookmarks (no-op when signed out — the
  // sets stay empty). Cleanup: cancel any pending debounced reload so it can't fire
  // after this view is gone.
  onMount(() => {
    if (isAuthenticated()) {
      ensureViewedLoaded();
      ensureSavedLoaded();
    }
    // Register this page's store so the header search drives it. This holds for the
    // standalone /jobs list AND the company page's embedded, scoped list — on
    // /companies/:slug the header search filters that company's postings (there's
    // no inline box), so both modes route their text search through the header.
    // The adapter also exposes `filterScope` (the store + a reactive counts getter)
    // so the header renders its Location & work-format popover on these jobs-backed
    // lists; the company list registers a bare store with no filterScope, so the
    // popover stays hidden there.
    setListSearchTarget({
      get value() {
        return filters.value;
      },
      setQuery: (q) => filters.setQuery(q),
      filterScope: { store: filters, counts: () => counts, variant: 'jobs' },
    });
    return () => {
      setListSearchTarget(null);
      filters.dispose();
    };
  });

  function reloadList() {
    const next = makePaginator();
    next.start();
    jobs = next;
  }

  // Enter swipe mode carrying the current filters + query (same param shape the
  // list uses), so the deck reflects exactly what's on screen. `scope` params are
  // fixed context (e.g. company_slug) and go along too.
  function openSwipe() {
    const params = scopedParams();
    const qs = params.toString();
    goto(resolve('/jobs/swipe') + (qs ? `?${qs}` : ''));
  }

  // Reload list + counts whenever the debounced filters change — a settled
  // keystroke, an immediate facet toggle, or a back/forward re-seed. Skip the
  // first run for the list (the SSR `initial` already seeded page one); still
  // fetch counts on mount since they aren't server-rendered into this view.
  $effect(() => {
    void filters.applied; // track the debounced snapshot
    const blocked = cvSignInPrompt; // read in the tracked scope so a sign-in refetches
    untrack(() => {
      refreshCounts();
      if (!started) {
        started = true;
        // Keep the SSR `initial` page unless it was loaded for a different URL than
        // the address bar (stale shallow-routing restore) or the feed is in CV mode
        // — the SSR seed is the newest-sorted keyword feed, wrong for CV ranking.
        if (!initialStale && !cvMode) return;
      }
      if (blocked) return;
      reloadList();
    });
  });

  // Re-seed the filters from the URL on every real navigation (initial load, the
  // "Jobs" nav link, back/forward). On the standalone list a navigation that lands
  // on a bare /jobs (empty URL) instead restores the last persisted filters, so an
  // ordinary navigation never drops them ("Clear all" persisted an empty set, so it
  // stays cleared).
  //
  // afterNavigate (not a URL-tracking $effect) because it runs only after the router
  // is initialized: apply()'s replaceState is safe on a client-side navigation (a
  // hydration-time $effect would throw "before router is initialized"), its `applied`
  // change lands after the reload effect's first pass, and our own shallow replaceState
  // writes don't re-fire it — so there's no restore loop. Restore is skipped on the
  // initial `enter` navigation (a hard load / refresh): the router isn't ready for
  // replaceState yet, and the SSR already rendered that exact URL, so we just re-seed
  // from it. Every restore-worthy case — the "Jobs" nav link, cross-route entry,
  // back/forward — is a client-side navigation, where it works. location.search is the
  // address-bar truth (page.url can lag after shallow routing). The company-embedded
  // list (persist off) keeps the plain re-seed — unchanged.
  if (untrack(() => standalone)) {
    afterNavigate((nav) => {
      const stored = nav.type !== 'enter' && location.search === '' ? loadJobFilters() : '';
      if (stored) filters.apply(stored);
      else filters.syncFromUrl();
    });
  } else {
    syncOnNavigation(filters);
  }
</script>

<!-- The feed sort control, handed to ListToolbar so it sits in the shared toolbar
     (mobile) / above the list (desktop). Offered only to a signed-in user on the
     standalone feed — "Recommended" needs a CV, so a signed-out visitor has no use
     for it (a shared ?sort=cv link still shows the sign-in prompt). URL value stays `cv`. -->
{#snippet sortSelect()}
  <label class="flex shrink-0 items-center gap-1.5 text-sm text-muted-foreground">
    <span class="hidden sm:inline">Sort</span>
    <select
      aria-label="Sort jobs"
      class="rounded-lg border border-input bg-transparent py-2 pl-2 pr-1 text-sm text-foreground transition-colors focus-visible:border-ring focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50 md:py-1 dark:bg-input/30"
      value={filters.value.sort}
      onchange={(e) => filters.setSort(e.currentTarget.value as SortField)}
    >
      <option value="posted_at">Newest</option>
      <option value="cv">Recommended</option>
    </select>
  </label>
{/snippet}

<div class="flex gap-6">
  <aside class="hidden w-72 shrink-0 md:block">
    <div class="sticky top-6 flex max-h-[calc(100vh-5rem)] flex-col gap-4 overflow-y-auto">
      {#if !standalone && jobs.status === 'ready'}
        <!-- Company view: the (filtered) open-job count as the sidebar's lead stat.
             The inline count above the list is hidden on desktop (shown only on
             mobile, where there's no sidebar), so it lives here instead. -->
        <div class="rounded-xl border border-border bg-card px-4 py-3">
          <p class="text-3xl font-semibold leading-none tracking-tight tabular-nums">
            {jobs.total.toLocaleString()}
          </p>
          <p class="mt-1 text-xs font-medium uppercase tracking-wide text-muted-foreground">
            {jobs.total === 1 ? 'open job' : 'open jobs'}
          </p>
        </div>
      {/if}
      {@render sidebarTop?.()}
      <div class="rounded-xl border border-border bg-card p-4">
        <FilterSummary store={filters} exclude={excludeFacets} onOpen={() => (modalOpen = true)} canSave={standalone} />
      </div>
    </div>
  </aside>

  <div class="min-w-0 flex-1">
    <ListToolbar
      total={!cvSignInPrompt && jobs.items.length > 0 ? jobs.total : null}
      unit={jobs.total === 1 ? 'job' : 'jobs'}
      active={filters.active}
      onOpenFilters={() => (modalOpen = true)}
      onSwipe={standalone ? openSwipe : undefined}
      showDesktopTotal={standalone}
      sortControl={standalone && isAuthenticated() ? sortSelect : undefined}
    />

    <!-- Onboarding nudges sit UNDER the sort toolbar so the feed controls stay at the
         top; each shows once (until dismissed or completed), then retires. Never blocks
         the feed below. -->
    {#if showBanner || alertBanner}
      <div class="mt-3">
        {#if showBanner}
          <OnboardingBanner onOpen={() => (wizardOpen = true)} onDismiss={dismissBanner} />
        {/if}
        {#if alertBanner}
          <OnboardingAlertBanner
            query={alertBanner.query}
            autostart={alertBanner.autostart}
            onDismiss={() => (alertBanner = null)}
          />
        {/if}
      </div>
    {/if}

    {#if cvSignInPrompt}
      <!-- CV ranking needs the authenticated recommendations endpoint; the reload
           effect skips the fetch here, so this prompt stands in for the feed. -->
      <div class="rounded-xl border border-border bg-card p-6 text-center">
        <p class="text-sm font-medium text-foreground">Sign in for recommendations</p>
        <p class="mt-1 text-sm text-muted-foreground">
          Recommended ranks jobs by similarity to your uploaded résumé.
          <button
            type="button"
            onclick={() => openAuthDialog()}
            class="font-medium text-foreground underline underline-offset-4 hover:no-underline">Sign in</button
          >
          to use it.
        </p>
      </div>
    {:else if jobs.status === 'loading'}
      <States state="loading" />
    {:else if jobs.status === 'error'}
      <States state="error" message="Failed to load jobs." />
    {:else if jobs.items.length === 0}
      {#if cvMode && appliedActive === 0}
        <!-- CV mode, no facet filter: an empty feed means no usable CV vector (the
             recommendations endpoint returns [] for no/stale CV), so prompt an
             upload rather than showing a bare "no matches". -->
        <div class="rounded-xl border border-border bg-card p-6 text-center">
          <p class="text-sm font-medium text-foreground">No recommendations yet</p>
          <p class="mt-1 text-sm text-muted-foreground">
            Add or update your CV on your
            <a
              href={resolve('/my/profile')}
              class="font-medium text-foreground underline underline-offset-4 hover:no-underline">profile</a
            >
            to get jobs ranked by how well they match your experience.
          </p>
        </div>
      {:else}
        <States state="empty" message="No matching jobs." />
        {#if standalone && relaxTarget}
          <!-- No semantic fallback in this slice: offer an honest one-step broaden
               instead of silently widening the feed. -->
          <div class="mt-4 flex justify-center">
            <button
              type="button"
              onclick={relaxFeed}
              class="inline-flex items-center gap-1.5 rounded-lg bg-brand px-4 py-2 text-sm font-semibold text-brand-foreground transition-opacity hover:opacity-90"
            >
              Broaden search
            </button>
          </div>
        {/if}
      {/if}
    {:else}
      <div class="flex flex-col gap-3">
        {#each jobs.items as job (job.public_slug)}
          <JobRow {job} />
        {/each}
      </div>

      {#if jobs.hasMore}
        <!-- Scroll-to-bottom auto-load; the button stays as the accessible
             fallback (keyboard/screen-reader, and retry on a failed load). -->
        <InfiniteScroll onLoad={() => jobs.loadMore()} enabled={!jobs.loadingMore && !jobs.loadMoreError} />
        <LoadMore loading={jobs.loadingMore} error={jobs.loadMoreError} onclick={() => jobs.loadMore()} />
      {/if}
    {/if}
  </div>
</div>

{#if standalone}
  <!-- Desktop swipe-mode entry: an icon-only button pinned to the right viewport edge
       (mobile uses the inline toolbar / scroll-revealed tab, so this is md-only). Fixed,
       so it exists only while the standalone list is mounted and stays reachable while
       scrolling; kept below the z-40 mobile overlays. -->
  <button
    type="button"
    onclick={openSwipe}
    aria-label="Swipe mode"
    title="Swipe mode"
    class="fixed right-0 top-16 z-30 hidden items-center rounded-l-xl border border-r-0 border-border bg-secondary py-2.5 pl-2.5 pr-2 text-secondary-foreground shadow-md transition-colors hover:bg-accent md:flex"
  >
    <Layers class="size-4 shrink-0" />
  </button>
{/if}

<FilterModal
  store={filters}
  {counts}
  exclude={excludeFacets}
  savedSearches={standalone}
  open={modalOpen}
  onClose={() => (modalOpen = false)}
  {stagedCounts}
/>

{#if standalone}
  <OnboardingWizard open={wizardOpen} {counts} onComplete={completeWizard} onCancel={cancelWizard} />
{/if}
