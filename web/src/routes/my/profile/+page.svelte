<script lang="ts">
  import { untrack } from 'svelte';
  import { ScanSearch, Trash2 } from '@lucide/svelte';
  import { page } from '$app/state';
  import { api } from '$lib/api';
  import { isAuthenticated } from '$lib/auth.svelte';
  import { FilterStore, filtersToParams } from '$lib/filters';
  import ATSReportView from '$lib/components/ATSReportView.svelte';
  import FilterSummary from '$lib/components/filters/FilterSummary.svelte';
  import FilterModal from '$lib/components/filters/FilterModal.svelte';
  import FilterEdgeTab from '$lib/components/FilterEdgeTab.svelte';
  import ProfileForm from '$lib/components/ProfileForm.svelte';
  import ResumeStructuredView from '$lib/components/ResumeStructuredView.svelte';
  import States from '$lib/components/States.svelte';
  import VerdictView from '$lib/components/VerdictView.svelte';
  import { profileStore } from '$lib/profile.svelte';
  import type { ATSResponse, FacetCounts, ResumeStructured, Verdict } from '$lib/types';
  import { Button } from '$lib/ui';

  const profile = $derived(profileStore.profile);

  // Skills are the measured set (from the profile), never a market filter — hide the
  // skills facet so the sidebar can't turn them into one.
  const excludeFacets = ['skills'];

  let status = $state<'loading' | 'error' | 'ready'>('loading');
  let filters = $state<FilterStore | null>(null);
  let verdict = $state<Verdict | null>(null);
  let counts = $state<FacetCounts | null>(null);
  let ats = $state<ATSResponse | null>(null);
  // The read-only structured résumé parsed from the CV (null when none is current: no
  // résumé, unconfigured LLM, background extraction not yet landed, or stale). Fetched
  // independently of the filter-driven reload.
  let structured = $state<ResumeStructured | null>(null);
  let loadError = $state(false);
  let tab = $state<'profile' | 'coverage' | 'readiness'>('profile');
  let modalOpen = $state(false);
  let actionError = $state<string | null>(null);

  // Optimistic CV flag: a résumé upload stores the CV server-side before the next ATS
  // fetch resolves (and before any profile exists during set-up), so reflect it at once.
  let cvUploaded = $state(false);
  const hasCv = $derived((ats?.has_cv ?? false) || cvUploaded);

  // Job-count preview for the modal's staged filters — the same facet call, total only.
  const previewCount = (params: URLSearchParams) => api.facetCounts(params).then((c) => c.total);

  // AI review state.
  let reviewBusy = $state(false);
  let reviewUnavailable = $state(false);

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
    const seed = new URLSearchParams(page.url.searchParams);
    if (!seed.getAll('category').some((c) => c !== '')) {
      for (const spec of specializations) seed.append('category', spec);
    }
    return new FilterStore(seed);
  }

  async function load() {
    status = 'loading';
    try {
      await profileStore.ensureLoaded();
      status = 'ready';
    } catch {
      status = 'error';
    }
    void loadStructured();
  }

  // Fetch the read-only structured résumé independently of the filter-driven reload.
  // Best-effort: any failure (or none current) leaves the section hidden, never an error.
  async function loadStructured() {
    try {
      structured = (await api.getResume()).structured;
    } catch {
      structured = null;
    }
  }

  // (Re)load once the session resolves.
  $effect(() => {
    if (isAuthenticated()) void load();
  });

  // Build the filter only on the profile null↔exists transition, never on a plain edit —
  // so refining the comparison role survives a skills/role save. Delete drops it back to
  // the set-up form.
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

  // Reload the verdict + facet counts + ATS report whenever the applied (debounced)
  // filters change. No filter (no profile) → nothing to compute.
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
    try {
      const [v, c, a] = await Promise.all([
        api.getProfileVerdict(params),
        api.facetCounts(params),
        api.getATSReport(params),
      ]);
      if (gen !== reloadGeneration) return; // a newer reload started — discard stale results.
      [verdict, counts, ats] = [v, c, a];
      loadError = false;
    } catch {
      if (gen === reloadGeneration) loadError = true;
    }
  }

  // ProfileForm callbacks: a save re-fetches coverage; a CV upload also refreshes the
  // stored-CV state. During set-up both reloads are no-ops (reload bails without a
  // filter), but cvUploaded still flips so the drop-zone shows the uploaded state at once.
  const handleSaved = () => void reload();
  function handleCvUploaded() {
    cvUploaded = true;
    void reload();
    // The structured résumé is derived in the background (seconds), so it usually is not
    // ready this instant; re-fetch anyway — it lands on the next profile visit otherwise.
    void loadStructured();
  }

  // Link a gap skill to the job search under the current comparison role plus that skill.
  function gapHref(skill: string): string {
    const params = filters ? filtersToParams(filters.applied) : new URLSearchParams();
    params.append('skills', skill);
    return `/?${params}`;
  }

  async function remove() {
    if (!window.confirm('Delete your profile?')) return;
    actionError = null;
    try {
      await profileStore.clear();
      cvUploaded = false;
    } catch {
      actionError = 'Could not delete the profile. Please try again.';
    }
  }
</script>

<svelte:head>
  <title>Profile — freehire</title>
</svelte:head>

<!-- The account shell (my/+layout) owns the container, auth gate, and noindex. -->
{#if status === 'loading'}
  <States state="loading" />
{:else if status === 'error'}
  <States state="error" message="Couldn't load your profile." />
{:else}
  <!-- Header -->
  <div class="mb-6 flex flex-col gap-1">
    <h1 class="text-2xl font-semibold tracking-tight">Profile</h1>
    <p class="text-sm text-muted-foreground">
      Your CV, skills and role — measured against live market demand.
    </p>
  </div>

  {#if actionError}
    <p class="mb-4 text-sm text-destructive">{actionError}</p>
  {/if}

  <!-- Run / Re-run AI review control, rendered inside the CV-readiness section header
       (via ATSReportView's `action` slot) rather than crammed into the tab row. -->
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

  {#if profile === null}
    <!-- Set-up: the inline form only; coverage appears once a profile exists. Full-width to
         match the loaded profile tab, whose main column also spans the container (no aside). -->
    <div class="w-full">
      <ProfileForm profile={null} {hasCv} onSaved={handleSaved} onCvUploaded={handleCvUploaded} />
    </div>
  {:else}
    <div class="flex gap-6">
      <main class="flex min-w-0 flex-1 flex-col gap-6">
        <!-- Tabs -->
        <div class="flex gap-5 border-b border-border">
          <button
            type="button"
            onclick={() => (tab = 'profile')}
            class="-mb-px border-b-2 px-1 pb-2.5 text-sm font-medium transition-colors {tab === 'profile'
              ? 'border-brand text-foreground'
              : 'border-transparent text-muted-foreground hover:text-foreground'}"
          >
            Your CV
          </button>
          <button
            type="button"
            onclick={() => (tab = 'coverage')}
            class="-mb-px border-b-2 px-1 pb-2.5 text-sm font-medium transition-colors {tab === 'coverage'
              ? 'border-brand text-foreground'
              : 'border-transparent text-muted-foreground hover:text-foreground'}"
          >
            Market coverage
          </button>
          <button
            type="button"
            onclick={() => (tab = 'readiness')}
            class="-mb-px border-b-2 px-1 pb-2.5 text-sm font-medium transition-colors {tab === 'readiness'
              ? 'border-brand text-foreground'
              : 'border-transparent text-muted-foreground hover:text-foreground'}"
          >
            CV readiness
          </button>
        </div>

        <!-- Body -->
        {#if tab === 'profile'}
          {#key profile.updated_at}
            <ProfileForm {profile} {hasCv} onSaved={handleSaved} onCvUploaded={handleCvUploaded} />
          {/key}
          <!-- Destructive action lives at the foot of the profile-management tab, out of
               the page header (where it crowded the title on narrow viewports). -->
          <div class="mt-2 flex justify-end border-t border-border pt-4">
            <Button
              variant="ghost"
              size="sm"
              onclick={remove}
              class="text-muted-foreground hover:bg-destructive/10 hover:text-destructive"
            >
              <Trash2 class="size-4" />
              Delete profile
            </Button>
          </div>
        {:else if loadError}
          <States state="error" message="Couldn't load the report." />
        {:else if verdict === null}
          <States state="loading" />
        {:else if tab === 'coverage'}
          <VerdictView {verdict} {gapHref} />
        {:else}
          <!-- CV readiness: the structured résumé we parsed from the CV (read-only,
               omitted when none is current) above the ATS-readiness score. -->
          <div class="flex flex-col gap-6">
            {#if structured}
              <ResumeStructuredView resume={structured} />
            {/if}
            {#if ats?.has_cv && ats.report}
              <div class="flex flex-col gap-5">
                {#if reviewUnavailable}
                  <p class="text-xs text-muted-foreground">AI review is not available right now.</p>
                {/if}
                <ATSReportView report={ats.report} action={reviewAction} />
              </div>
            {:else if !structured}
              <!-- No CV yet: uploaded via the Your CV tab. -->
              <div class="flex flex-col items-start gap-2 rounded-xl border border-dashed border-border p-6">
                <p class="text-sm font-medium">Add your CV to score its ATS readiness</p>
                <p class="text-sm text-muted-foreground">
                  Upload your CV in the <button type="button" class="font-medium text-foreground underline underline-offset-2" onclick={() => (tab = 'profile')}>Your CV</button> tab to check ATS readability and this role's keywords.
                </p>
              </div>
            {/if}
          </div>
        {/if}
      </main>

      <!-- Filters refine the Market coverage comparison only, so the summary sidebar
           shows on that tab alone — to the right of the content, clear of the account
           nav sidebar. -->
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
  {/if}
{/if}
