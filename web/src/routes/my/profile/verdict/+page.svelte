<script lang="ts">
  import { ArrowLeft, Pencil, Sparkles } from '@lucide/svelte';
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import { facetCounts, getATSReport, getProfileVerdict, runATSReview } from '$lib/api';
  import { isAuthenticated } from '$lib/auth.svelte';
  import { openAuthDialog } from '$lib/auth-dialog.svelte';
  import { categoryLabel } from '$lib/facets';
  import { FilterStore, filtersToParams } from '$lib/filters';
  import ATSReportView from '$lib/components/ATSReportView.svelte';
  import FiltersPanel from '$lib/components/FiltersPanel.svelte';
  import States from '$lib/components/States.svelte';
  import VerdictView from '$lib/components/VerdictView.svelte';
  import { profileStore } from '$lib/profile.svelte';
  import type { ATSResponse, FacetCounts, Verdict } from '$lib/types';
  import { Button } from '$lib/ui';

  const profileHref = resolve('/my/profile');
  const profile = $derived(profileStore.profile);

  // Skills are the measured set (from the profile), never a role filter — hide the
  // skills facet so the sidebar can't turn them into one.
  const excludeFacets = ['skills'];

  let status = $state<'loading' | 'error' | 'ready'>('loading');
  let filters = $state<FilterStore | null>(null);
  let verdict = $state<Verdict | null>(null);
  let counts = $state<FacetCounts | null>(null);
  let ats = $state<ATSResponse | null>(null);
  let loadError = $state(false);
  let tab = $state<'coverage' | 'cv'>('coverage');

  // AI review state.
  let reviewBusy = $state(false);
  let reviewUnavailable = $state(false);

  // Run the optional LLM review over the stored CV; folds content-quality + findings
  // into the report. When the server has no LLM the report comes back unchanged — flag
  // that so the UI stops offering the button.
  async function runReview() {
    reviewBusy = true;
    reviewUnavailable = false;
    try {
      const params = filters ? filtersToParams(filters.applied) : undefined;
      const next = await runATSReview(params);
      ats = next;
      if (next.has_cv && next.report && next.report.content_quality == null) {
        reviewUnavailable = true;
      }
    } catch {
      reviewUnavailable = true;
    } finally {
      reviewBusy = false;
    }
  }

  // Build the filter store once the profile is known, seeding its role from the
  // profile's specializations when the URL carries no category — so the panel opens on
  // the profile's own role, which the user can then change without touching the profile.
  // With no profile, leave `filters` null so the page shows the set-up prompt instead of
  // firing verdict/ATS requests that would 404.
  async function init() {
    status = 'loading';
    try {
      await profileStore.ensureLoaded();
      const p = profileStore.profile;
      if (p) {
        const seed = new URLSearchParams(page.url.searchParams);
        if (!seed.getAll('category').some((c) => c !== '')) {
          for (const spec of p.specializations) seed.append('category', spec);
        }
        filters = new FilterStore(seed);
      }
      status = 'ready';
    } catch {
      status = 'error';
    }
  }

  $effect(() => {
    // Re-init when the session resolves.
    if (isAuthenticated()) void init();
  });

  // Reload the verdict + facet counts whenever the applied (debounced) filters change.
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
        getProfileVerdict(params),
        facetCounts(params),
        getATSReport(params),
      ]);
      if (gen !== reloadGeneration) return; // a newer reload started — discard stale results.
      [verdict, counts, ats] = [v, c, a];
      loadError = false;
    } catch {
      if (gen === reloadGeneration) loadError = true;
    }
  }

  // Humanized role label: the selected categories, falling back to the profile's own
  // specializations when none are selected.
  const role = $derived.by(() => {
    const selected = filters?.applied.facets['category']?.values ?? [];
    const source = selected.length ? selected : (profile?.specializations ?? []);
    return source.map(categoryLabel).join(', ');
  });

  // Link a gap skill to the job search under the current role filter plus that skill.
  function gapHref(skill: string): string {
    const params = filters ? filtersToParams(filters.applied) : new URLSearchParams();
    params.append('skills', skill);
    return `/jobs?${params}`;
  }
</script>

<svelte:head>
  <title>Market coverage — freehire</title>
  <!-- Personal page: keep it out of search results. -->
  <meta name="robots" content="noindex" />
</svelte:head>

<div class="mx-auto w-full max-w-5xl px-4 py-6">
  {#if !isAuthenticated()}
    <div class="flex flex-col items-center gap-3 py-12 text-center">
      <p class="text-sm text-muted-foreground">Sign in to see your coverage.</p>
      <Button variant="primary" onclick={() => openAuthDialog()}>Sign in</Button>
    </div>
  {:else if status === 'loading'}
    <States state="loading" />
  {:else if status === 'error'}
    <States state="error" message="Couldn't load the verdict." />
  {:else if profile === null || filters === null}
    <div class="flex flex-col items-center gap-3 py-12 text-center">
      <p class="text-sm text-muted-foreground">Set up your profile to see market coverage.</p>
      <Button variant="primary" href={profileHref}>Go to profile</Button>
    </div>
  {:else}
    <div class="grid gap-6 md:grid-cols-[260px_1fr]">
      <aside class="md:sticky md:top-6 md:self-start">
        <FiltersPanel store={filters} exclude={excludeFacets} {counts} />
      </aside>
      <main class="flex flex-col gap-6">
        <!-- Page header -->
        <div class="flex flex-col gap-3">
          <a
            href={profileHref}
            class="inline-flex w-fit items-center gap-1 text-sm text-muted-foreground transition-colors hover:text-foreground"
          >
            <ArrowLeft class="size-4" />
            Profile
          </a>
          <div class="flex flex-col gap-1">
            <h1 class="text-3xl font-semibold tracking-tight">Market coverage</h1>
            {#if role}<p class="text-sm text-muted-foreground">{role}</p>{/if}
          </div>
        </div>

        <!-- Tabs -->
        <div class="flex gap-5 border-b border-border">
          <button
            type="button"
            onclick={() => (tab = 'coverage')}
            class="-mb-px border-b-2 px-1 pb-2.5 text-sm font-medium transition-colors {tab === 'coverage'
              ? 'border-primary text-foreground'
              : 'border-transparent text-muted-foreground hover:text-foreground'}"
          >
            Market coverage
          </button>
          <button
            type="button"
            onclick={() => (tab = 'cv')}
            class="-mb-px border-b-2 px-1 pb-2.5 text-sm font-medium transition-colors {tab === 'cv'
              ? 'border-primary text-foreground'
              : 'border-transparent text-muted-foreground hover:text-foreground'}"
          >
            CV readiness
          </button>
        </div>

        <!-- Body -->
        {#if loadError}
          <States state="error" message="Couldn't load the report." />
        {:else if verdict === null}
          <States state="loading" />
        {:else if tab === 'coverage'}
          <VerdictView {verdict} {gapHref} />
        {:else if ats?.has_cv && ats.report}
          <!-- CV readiness -->
          <div class="flex flex-col gap-5">
            <div class="flex flex-wrap items-center justify-between gap-3">
              <p class="text-sm text-muted-foreground">How ATS-ready your CV is for this role.</p>
              {#if ats.report.content_quality == null && !reviewUnavailable}
                <Button variant="primary" onclick={runReview} disabled={reviewBusy}>
                  <Sparkles class="size-4 {reviewBusy ? 'animate-pulse' : ''}" />
                  {reviewBusy ? 'Reviewing…' : 'Run AI review'}
                </Button>
              {:else if ats.report.content_quality != null}
                <Button variant="ghost" onclick={runReview} disabled={reviewBusy}>
                  <Sparkles class="size-4 {reviewBusy ? 'animate-pulse' : ''}" />
                  {reviewBusy ? 'Reviewing…' : 'Re-run AI review'}
                </Button>
              {/if}
            </div>
            {#if reviewUnavailable}
              <p class="text-xs text-muted-foreground">AI review is not available right now.</p>
            {/if}
            <ATSReportView report={ats.report} />
            <a
              href={profileHref}
              class="w-fit text-xs text-muted-foreground underline-offset-2 transition-colors hover:text-foreground hover:underline"
            >
              Update your CV in your profile
            </a>
          </div>
        {:else}
          <!-- No CV yet: managed on the profile. -->
          <div class="flex flex-col items-start gap-3 rounded-xl border border-dashed border-border p-6">
            <p class="text-sm font-medium">Add your CV to score its ATS readiness</p>
            <p class="text-sm text-muted-foreground">
              Upload your CV on the profile to check ATS readability and this role's keywords.
            </p>
            <Button variant="primary" href={profileHref}>
              <Pencil class="size-4" />
              Go to profile
            </Button>
          </div>
        {/if}
      </main>
    </div>
  {/if}
</div>
