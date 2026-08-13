<script lang="ts">
  import { untrack } from 'svelte';
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import { api } from '$lib/api';
  import { isAuthenticated } from '$lib/auth.svelte';
  import { FilterStore, filtersToParams } from '$lib/filters';
  import FilterSummary from '$lib/components/filters/FilterSummary.svelte';
  import FilterModal from '$lib/components/filters/FilterModal.svelte';
  import FilterEdgeTab from '$lib/components/FilterEdgeTab.svelte';
  import MarketPulseView from '$lib/components/MarketPulseView.svelte';
  import States from '$lib/components/States.svelte';
  import { TabStrip, tabStripId } from '$lib/ui';
  import VerdictView from '$lib/components/VerdictView.svelte';
  import { profileStore } from '$lib/profile.svelte';
  import type { FacetCounts, Verdict } from '$lib/types';
  import { Button } from '$lib/ui';

  const profile = $derived(profileStore.profile);

  // Skills are the measured set (from the profile), never a market filter — hide the
  // skills facet so the sidebar can't turn them into one.
  const excludeFacets = ['skills'];

  const TABS = [
    { id: 'coverage', label: 'Coverage' },
    { id: 'trend', label: 'Skill trend' },
  ] as const;
  const PANEL_ID = 'market-pulse-panel';
  let tab = $state<'coverage' | 'trend'>('coverage');

  let filters = $state<FilterStore | null>(null);
  let verdict = $state<Verdict | null>(null);
  let counts = $state<FacetCounts | null>(null);
  let loadError = $state(false);
  let modalOpen = $state(false);

  // Job-count preview for the modal's staged filters — the same facet call, total only.
  const previewCount = (params: URLSearchParams) => api.facetCounts(params).then((c) => c.total);

  // Seed the comparison filter from the profile's specializations (unless the URL already
  // carries a category) — so it opens on the profile's own role, which the user can then
  // change to compare against another position without touching the saved profile.
  function buildFilters(specializations: string[]): FilterStore {
    // eslint-disable-next-line svelte/prefer-svelte-reactivity -- transient: seeds a FilterStore once, never stored as reactive state
    const seed = new URLSearchParams(page.url.searchParams);
    if (!seed.getAll('category').some((c) => c !== '')) {
      for (const spec of specializations) seed.append('category', spec);
    }
    return new FilterStore(seed);
  }

  // Link a gap skill to the job search under the current comparison role plus that skill.
  function gapHref(skill: string): string {
    // eslint-disable-next-line svelte/prefer-svelte-reactivity -- transient: builds an href string once, never stored as reactive state
    const params = filters ? filtersToParams(filters.applied) : new URLSearchParams();
    params.append('skills', skill);
    return `/?${params}`;
  }

  // (Re)load the profile once the session resolves.
  $effect(() => {
    if (isAuthenticated()) void profileStore.ensureLoaded();
  });

  // Build the filter only on the profile null↔exists transition, never on a plain edit —
  // mirrors Profile's own filter lifecycle.
  $effect(() => {
    const p = profile;
    untrack(() => {
      if (p && !filters) {
        filters = buildFilters(p.specializations);
      } else if (!p && filters) {
        filters.dispose();
        filters = null;
      }
    });
  });

  // Reload the verdict + facet counts whenever the applied (debounced) filters change.
  // No filter (no profile) → nothing to compute.
  $effect(() => {
    const f = filters;
    if (!f) return;
    void f.applied;
    void reload();
  });

  // reloadGeneration guards against out-of-order responses: fast filter changes can have
  // an older request resolve after a newer one, so only the latest reload commits.
  let reloadGeneration = 0;
  async function reload() {
    if (!filters) return;
    const gen = ++reloadGeneration;
    const params = filtersToParams(filters.applied);
    const [v, c] = await Promise.allSettled([api.getProfileVerdict(params), api.facetCounts(params)]);
    if (gen !== reloadGeneration) return;
    if (v.status !== 'fulfilled') {
      loadError = true;
      return;
    }
    verdict = v.value;
    counts = c.status === 'fulfilled' ? c.value : null;
    loadError = false;
  }
</script>

<svelte:head>
  <title>Market pulse — freehire</title>
</svelte:head>

<!-- The account shell (my/+layout) owns the container, auth gate, and noindex. -->
{#if !isAuthenticated()}
  <p class="py-12 text-center text-sm text-muted-foreground">Sign in to view your market pulse.</p>
{:else}
  <div class="mb-6 flex flex-col gap-1">
    <h1 class="text-2xl font-semibold tracking-tight">Market pulse</h1>
    <p class="text-sm text-muted-foreground">
      How you compare to the live market — role coverage and your own skill-demand trend.
    </p>
  </div>

  <div class="flex gap-6">
    <main class="flex min-w-0 flex-1 flex-col gap-6">
      <TabStrip tabs={TABS} active={tab} onSelect={(id) => (tab = id)} label="Market pulse sections" panelId={PANEL_ID} />

      <div id={PANEL_ID} role="tabpanel" aria-labelledby={tabStripId(PANEL_ID, tab)} class="flex flex-col gap-6">
        {#if tab === 'trend'}
          <MarketPulseView />
        {:else if !profileStore.loaded}
          <States state="loading" />
        {:else if profile === null}
          <div class="flex flex-col items-center gap-3 py-12 text-center">
            <p class="text-sm font-medium text-foreground">Complete your profile to see market coverage</p>
            <p class="max-w-sm text-sm text-muted-foreground">
              Coverage compares your CV's skills against the live market for a role you choose —
              add a profile first.
            </p>
            <Button variant="primary" href={resolve('/my/profile')}>Go to profile</Button>
          </div>
        {:else if loadError}
          <States state="error" message="Couldn't load the report." />
        {:else if verdict === null}
          <States state="loading" />
        {:else}
          <VerdictView {verdict} {gapHref} />
        {/if}
      </div>
    </main>

    <!-- Filters refine the Coverage comparison only, so the summary sidebar shows on
         that tab alone — to the right of the content, clear of the account nav sidebar. -->
    {#if filters && tab === 'coverage'}
      <aside class="hidden w-72 shrink-0 md:block">
        <div class="sticky top-6 flex max-h-[calc(100vh-5rem)] flex-col gap-4 overflow-y-auto">
          <div class="rounded-xl border border-border bg-card p-4">
            <FilterSummary
              store={filters}
              exclude={excludeFacets}
              onOpen={() => (modalOpen = true)}
              description="Narrow the market to see how it reshapes your CV — pick roles, regions and seniority to compare against."
            />
          </div>
        </div>
      </aside>
    {/if}
  </div>

  {#if filters && tab === 'coverage'}
    <FilterEdgeTab active={filters.active} onclick={() => (modalOpen = true)} side="right" class="top-[5.5rem]" />
    <FilterModal
      store={filters}
      {counts}
      exclude={excludeFacets}
      savedSearches
      open={modalOpen}
      onClose={() => (modalOpen = false)}
      {previewCount}
    />
  {/if}
{/if}
