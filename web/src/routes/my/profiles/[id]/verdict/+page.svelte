<script lang="ts">
  import { ArrowUp, FileText, RefreshCw, Sparkles } from '@lucide/svelte';
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import {
    ApiError,
    extractResumeSkills,
    facetCounts,
    getATSReport,
    getProfileVerdict,
    runATSReview,
  } from '$lib/api';
  import { isAuthenticated } from '$lib/auth.svelte';
  import { openAuthDialog } from '$lib/auth-dialog.svelte';
  import { categoryLabel } from '$lib/facets';
  import { FilterStore, filtersToParams } from '$lib/filters';
  import ATSReportView from '$lib/components/ATSReportView.svelte';
  import FiltersPanel from '$lib/components/FiltersPanel.svelte';
  import States from '$lib/components/States.svelte';
  import VerdictView from '$lib/components/VerdictView.svelte';
  import { searchProfiles } from '$lib/searchProfiles.svelte';
  import type { ATSResponse, FacetCounts, Verdict } from '$lib/types';
  import { Button } from '$lib/ui';

  const id = $derived(Number(page.params.id));
  const profile = $derived(searchProfiles.items.find((p) => p.id === id));

  // Skills are the measured set (from the profile), never a role filter — hide the
  // skills facet so the sidebar can't turn them into one.
  const excludeFacets = ['skills'];

  let status = $state<'loading' | 'error' | 'ready'>('loading');
  let filters = $state<FilterStore | null>(null);
  let verdict = $state<Verdict | null>(null);
  let counts = $state<FacetCounts | null>(null);
  let ats = $state<ATSResponse | null>(null);
  let loadError = $state(false);

  // CV upload state (the ATS report needs a stored CV).
  let cvBusy = $state(false);
  let cvError = $state<string | null>(null);
  let fileInput = $state<HTMLInputElement | null>(null);
  let dragActive = $state(false);

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
      const next = await runATSReview(id, params);
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
  async function init() {
    status = 'loading';
    try {
      await searchProfiles.ensureLoaded();
      const p = searchProfiles.items.find((x) => x.id === id);
      const seed = new URLSearchParams(page.url.searchParams);
      if (p && !seed.getAll('category').some((c) => c !== '')) {
        for (const spec of p.specializations) seed.append('category', spec);
      }
      filters = new FilterStore(seed);
      status = 'ready';
    } catch {
      status = 'error';
    }
  }

  $effect(() => {
    // Re-init when the session resolves or the profile id changes (SvelteKit may reuse
    // this component across [id] navigations).
    void id;
    if (isAuthenticated()) void init();
  });

  // Reload the verdict + facet counts whenever the applied (debounced) filters change.
  $effect(() => {
    const f = filters;
    if (!f) return;
    void f.applied;
    void reload();
  });

  async function reload() {
    if (!filters) return;
    const params = filtersToParams(filters.applied);
    try {
      [verdict, counts, ats] = await Promise.all([
        getProfileVerdict(id, params),
        facetCounts(params),
        getATSReport(id, params),
      ]);
      loadError = false;
    } catch {
      loadError = true;
    }
  }

  function isPdf(file: File): boolean {
    return file.type === 'application/pdf' || file.name.toLowerCase().endsWith('.pdf');
  }

  // Store the CV (via the skill-extract path, which also stores it when storage is on),
  // then reload so the ATS report scores it.
  async function uploadCV(file: File) {
    cvBusy = true;
    cvError = null;
    try {
      await extractResumeSkills(file);
      await reload();
    } catch (err) {
      cvError = err instanceof ApiError ? err.message : 'Could not read the CV. Please try again.';
    } finally {
      cvBusy = false;
    }
  }

  function onCVFile(e: Event) {
    const target = e.currentTarget as HTMLInputElement;
    const file = target.files?.[0];
    target.value = '';
    if (file) void uploadCV(file);
  }

  function onDrop(e: DragEvent) {
    e.preventDefault();
    dragActive = false;
    const file = e.dataTransfer?.files?.[0];
    if (!file) return;
    if (!isPdf(file)) {
      cvError = 'Please drop a PDF file.';
      return;
    }
    void uploadCV(file);
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
  {:else if profile === undefined || filters === null}
    <div class="flex flex-col items-center gap-3 py-12 text-center">
      <p class="text-sm text-muted-foreground">This profile no longer exists.</p>
      <Button variant="primary" href={resolve('/my/profiles')}>Back to profiles</Button>
    </div>
  {:else}
    <div class="grid gap-6 md:grid-cols-[260px_1fr]">
      <aside class="md:sticky md:top-6 md:self-start">
        <FiltersPanel store={filters} exclude={excludeFacets} {counts} />
      </aside>
      <main>
        {#if loadError}
          <States state="error" message="Couldn't load the coverage." />
        {:else if verdict === null}
          <States state="loading" />
        {:else}
          <div class="flex flex-col gap-10">
            <VerdictView {verdict} name={profile.name} {role} {gapHref} />

            <!-- CV readiness (ATS report). Needs a stored CV; prompt an upload when absent. -->
            <div class="border-t border-border pt-8">
              <input
                type="file"
                accept="application/pdf,.pdf"
                class="hidden"
                bind:this={fileInput}
                onchange={onCVFile}
              />
              {#if ats?.has_cv && ats.report}
                <div class="flex flex-col gap-4">
                  <ATSReportView report={ats.report} />
                  <div class="flex flex-wrap items-center gap-2">
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
                    <Button variant="ghost" onclick={() => fileInput?.click()} disabled={cvBusy}>
                      <RefreshCw class="size-4 {cvBusy ? 'animate-spin' : ''}" />
                      {cvBusy ? 'Uploading…' : 'Replace CV'}
                    </Button>
                  </div>
                  {#if reviewUnavailable}
                    <p class="text-xs text-muted-foreground">AI review is not available right now.</p>
                  {/if}
                </div>
              {:else}
                <div class="flex flex-col gap-2">
                  <h2 class="text-sm font-semibold uppercase tracking-[0.14em] text-muted-foreground">
                    CV readiness
                  </h2>
                  <button
                    type="button"
                    onclick={() => fileInput?.click()}
                    ondragover={(e) => {
                      e.preventDefault();
                      dragActive = true;
                    }}
                    ondragleave={(e) => {
                      e.preventDefault();
                      dragActive = false;
                    }}
                    ondrop={onDrop}
                    disabled={cvBusy}
                    class="flex flex-col items-center justify-center gap-3 rounded-xl border-2 border-dashed px-6 py-8 text-center transition-colors disabled:opacity-70 {dragActive
                      ? 'border-primary bg-primary/5'
                      : 'border-border hover:border-primary/60'}"
                  >
                    <span class="flex size-11 items-center justify-center rounded-full bg-primary text-primary-foreground">
                      {#if cvBusy}
                        <FileText class="size-5" />
                      {:else}
                        <ArrowUp class="size-5" />
                      {/if}
                    </span>
                    <span class="flex flex-col gap-0.5">
                      <span class="text-sm font-semibold">
                        {cvBusy ? 'Reading your CV…' : 'Upload your CV to score it'}
                      </span>
                      {#if !cvBusy}
                        <span class="text-xs text-muted-foreground">
                          Drop a PDF here, or <span class="text-primary underline">choose from disk</span>
                        </span>
                      {/if}
                    </span>
                  </button>
                  <span class="text-xs text-muted-foreground">
                    Checks ATS readability + this role's keywords. Parsed on the server; CV text is
                    not stored.
                  </span>
                </div>
              {/if}
              {#if cvError}
                <p class="mt-2 text-sm text-destructive">{cvError}</p>
              {/if}
            </div>
          </div>
        {/if}
      </main>
    </div>
  {/if}
</div>
