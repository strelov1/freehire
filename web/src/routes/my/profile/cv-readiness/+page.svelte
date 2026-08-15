<script lang="ts">
  import { untrack } from 'svelte';
  import { ScanSearch } from '@lucide/svelte';
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import { api } from '$lib/api';
  import { FilterStore, filtersToParams } from '$lib/filters';
  import ATSReportView from '$lib/components/ATSReportView.svelte';
  import FilterSummary from '$lib/components/filters/FilterSummary.svelte';
  import FilterModal from '$lib/components/filters/FilterModal.svelte';
  import FilterEdgeTab from '$lib/components/FilterEdgeTab.svelte';
  import States from '$lib/components/States.svelte';
  import { profileStore } from '$lib/profile.svelte';
  import type { ATSResponse, FacetCounts } from '$lib/types';
  import { Button } from '$lib/ui';

  // CV readiness: the ATS-readiness score and the optional AI review, scored against a
  // role the reader picks. Reachable by URL only — it is deliberately not one of the
  // profile's tabs (see the TABS list in ../+layout.svelte), kept because the report is
  // still worth having, not because anything links to it.
  const profile = $derived(profileStore.profile);

  // Skills are the measured set (from the profile), never a market filter — hide the
  // skills facet so the sidebar can't turn them into one.
  const excludeFacets = ['skills'];

  let filters = $state<FilterStore | null>(null);
  let counts = $state<FacetCounts | null>(null);
  let ats = $state<ATSResponse | null>(null);
  let loadError = $state(false);
  let modalOpen = $state(false);

  // AI review state.
  let reviewBusy = $state(false);
  let reviewUnavailable = $state(false);

  // Job-count preview for the modal's staged filters — the same facet call, total only.
  const previewCount = (params: URLSearchParams) => api.facetCounts(params).then((c) => c.total);

  // Run the optional LLM review over the stored CV; folds content-quality + suggestions
  // into the report. When the server has no LLM the report comes back unreviewed — flag
  // that so the UI stops offering the button.
  async function runReview() {
    reviewBusy = true;
    reviewUnavailable = false;
    try {
      const params = filters ? filtersToParams(filters.applied) : undefined;
      const next = await api.runATSReview(params);
      ats = next;
      if (next.has_cv && next.report && !next.report.reviewed) {
        reviewUnavailable = true;
      }
    } catch {
      reviewUnavailable = true;
    } finally {
      reviewBusy = false;
    }
  }

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

  // Build the filter only on the profile null↔exists transition, never on a plain edit —
  // so refining the comparison role survives a skills/role save.
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

  // Reload the facet counts + ATS report whenever the applied (debounced) filters
  // change. No filter (no profile) → nothing to compute.
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
    // Settled separately: a Meili facet-settings lag (new filterable attr not applied yet)
    // must not blank the ATS report when that endpoint is fine. Facet counts degrade to
    // empty; the report still loads.
    const [a, c] = await Promise.allSettled([api.getATSReport(params), api.facetCounts(params)]);
    if (gen !== reloadGeneration) return;
    if (a.status !== 'fulfilled') {
      loadError = true;
      return;
    }
    ats = a.value;
    counts = c.status === 'fulfilled' ? c.value : null;
    loadError = false;
  }
</script>

<svelte:head>
  <title>CV readiness — freehire</title>
</svelte:head>

<!-- Run / Re-run AI review control, rendered inside the report's own section header
     (via ATSReportView's `action` slot) rather than above it. -->
{#snippet reviewAction()}
  {#if ats?.report && !ats.report.reviewed && !reviewUnavailable}
    <Button variant="primary" onclick={runReview} disabled={reviewBusy}>
      <ScanSearch class="size-4 {reviewBusy ? 'animate-pulse' : ''}" />
      {reviewBusy ? 'Reviewing…' : 'Run AI review'}
    </Button>
  {:else if ats?.report?.reviewed}
    <Button variant="ghost" onclick={runReview} disabled={reviewBusy}>
      <ScanSearch class="size-4 {reviewBusy ? 'animate-pulse' : ''}" />
      {reviewBusy ? 'Reviewing…' : 'Re-run AI review'}
    </Button>
  {/if}
{/snippet}

<div class="flex gap-6">
  <main class="flex min-w-0 flex-1 flex-col gap-6">
    {#if loadError}
      <States state="error" message="Couldn't load the report." />
    {:else if ats === null}
      <States state="loading" />
    {:else if ats.has_cv && ats.report}
      <div class="flex flex-col gap-5">
        {#if reviewUnavailable}
          <p class="text-xs text-muted-foreground">AI review is not available right now.</p>
        {/if}
        <ATSReportView report={ats.report} action={reviewAction} />
      </div>
    {:else}
      <!-- No CV yet: uploaded from the profile's Settings section. -->
      <div class="flex flex-col items-start gap-2 rounded-xl border border-dashed border-border p-6">
        <p class="text-sm font-medium">Add your CV to score its ATS readiness</p>
        <p class="text-sm text-muted-foreground">
          Upload your CV in
          <a class="font-medium text-foreground underline underline-offset-2" href={resolve('/my/profile')}>
            profile settings
          </a>
          to check ATS readability and this role's keywords.
        </p>
      </div>
    {/if}
  </main>

  <!-- Filters refine the keyword-match role — to the right of the content, clear of the
       account nav sidebar. -->
  {#if filters}
    <aside class="hidden w-72 shrink-0 md:block">
      <div class="sticky top-6 flex max-h-[calc(100vh-5rem)] flex-col gap-4 overflow-y-auto">
        <div class="rounded-xl border border-border bg-card p-4">
          <FilterSummary
            store={filters}
            exclude={excludeFacets}
            onOpen={() => (modalOpen = true)}
            description="Compare your CV's keyword strength against a role, region or seniority you choose."
          />
        </div>
      </div>
    </aside>
  {/if}
</div>

{#if filters}
  <FilterEdgeTab
    active={filters.active}
    onclick={() => (modalOpen = true)}
    side="right"
    class="top-[5.5rem]"
  />
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
