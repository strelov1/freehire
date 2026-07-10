<script lang="ts">
  import { resolve } from '$app/paths';
  import { ChevronDown, Sparkles, FileText, RefreshCw } from '@lucide/svelte';
  import { api } from '$lib/api';
  import { verdictTone, requirementStatusMeta, type Tone } from '$lib/jobFit';
  import type { JobFitResponse } from '$lib/types';
  import { Button } from '$lib/ui';

  // The AI fit analysis for one job, lazily loaded on expand and computed only on an
  // explicit action (never on page open). Self-contained: it owns its fetch/compute
  // state so the parent Profile-match block just renders it below the deterministic bar.
  let { slug }: { slug: string } = $props();

  let expanded = $state(false);
  let loading = $state(false);
  let computing = $state(false);
  let data = $state<JobFitResponse | null>(null);
  let loaded = false; // whether the initial GET has run for the current slug

  // Reset when navigating to another job so a stale analysis never bleeds across pages.
  $effect(() => {
    slug; // track
    expanded = false;
    loading = false;
    computing = false;
    data = null;
    loaded = false;
    computeReturnedEmpty = false;
  });

  async function toggle() {
    expanded = !expanded;
    if (!expanded || loaded) return;
    loaded = true;
    loading = true;
    // Capture the slug this request is for; a fast navigation to another job resets
    // state (the effect above) and must not be overwritten by this in-flight response.
    const forSlug = slug;
    try {
      const res = await api.getJobFit(forSlug);
      if (forSlug === slug) data = res;
    } catch {
      // Best-effort: leave data null; the block offers a compute.
    } finally {
      if (forSlug === slug) loading = false;
    }
  }

  async function compute() {
    computing = true;
    const forSlug = slug;
    try {
      const res = await api.runJobFit(forSlug);
      if (forSlug === slug) {
        data = res;
        computeReturnedEmpty = res.has_cv && !res.analysis;
      }
    } catch {
      // Swallowed: the compute button stays available to retry.
    } finally {
      if (forSlug === slug) computing = false;
    }
  }

  const analysis = $derived(data?.analysis ?? null);
  // The backend initializes every array (never a nil slice → always `[]` on the wire),
  // but normalize defensively so a hand-written/legacy cache row can never null-deref
  // and tear down the page.
  const dimensions = $derived(analysis?.dimensions ?? []);
  const requirements = $derived(analysis?.requirement_match ?? []);
  const strengths = $derived(analysis?.strengths ?? []);
  const gaps = $derived(analysis?.gaps ?? []);
  // A compute that returned no analysis (LLM unconfigured/failed) — distinct from
  // "not analyzed yet", so we surface an unavailable note instead of looping the button.
  let computeReturnedEmpty = $state(false);

  const toneText: Record<Tone, string> = {
    strong: 'text-emerald-600 dark:text-emerald-400',
    good: 'text-emerald-600 dark:text-emerald-400',
    moderate: 'text-amber-600 dark:text-amber-500',
    weak: 'text-amber-600 dark:text-amber-500',
    poor: 'text-destructive',
  };
  const toneBar: Record<Tone, string> = {
    strong: 'bg-emerald-500',
    good: 'bg-emerald-500',
    moderate: 'bg-amber-500',
    weak: 'bg-amber-500',
    poor: 'bg-destructive',
  };
  const toneChip: Record<Tone, string> = {
    strong: 'border-emerald-500/30 bg-emerald-500/10 text-emerald-700 dark:text-emerald-400',
    good: 'border-emerald-500/30 bg-emerald-500/10 text-emerald-700 dark:text-emerald-400',
    moderate: 'border-amber-500/30 bg-amber-500/10 text-amber-700 dark:text-amber-500',
    weak: 'border-amber-500/30 bg-amber-500/10 text-amber-700 dark:text-amber-500',
    poor: 'border-destructive/30 bg-destructive/10 text-destructive',
  };

  const profileHref = resolve('/my/profile');
</script>

<section class="flex flex-col gap-3 border-t border-border pt-4">
  <button
    type="button"
    class="flex items-center justify-between gap-2 text-left"
    aria-expanded={expanded}
    aria-controls="job-fit-panel"
    onclick={toggle}
  >
    <span class="flex items-center gap-1.5 text-xs font-semibold uppercase tracking-wide text-muted-foreground">
      <Sparkles class="size-3.5 shrink-0" />AI fit analysis
    </span>
    <ChevronDown class="size-4 shrink-0 text-muted-foreground transition-transform {expanded ? 'rotate-180' : ''}" />
  </button>

  {#if expanded}
    <div id="job-fit-panel" class="flex flex-col gap-3">
    {#if loading}
      <div class="h-2 animate-pulse rounded bg-secondary"></div>
      <div class="h-2 w-2/3 animate-pulse rounded bg-secondary"></div>
    {:else if data && !data.has_cv}
      <div class="flex items-center justify-between gap-2">
        <span class="flex items-center gap-1.5 text-xs text-muted-foreground">
          <FileText class="size-3.5 shrink-0" />Upload a CV to analyse this role
        </span>
        <Button variant="primary" size="sm" href={profileHref}>Upload CV</Button>
      </div>
    {:else if computeReturnedEmpty}
      <p class="text-sm text-muted-foreground">AI analysis is unavailable right now. Please try again later.</p>
    {:else if !analysis}
      <p class="text-sm text-muted-foreground">
        A recruiter-style read of your CV against this role — title fit, relevant experience, and the gaps an ATS flags.
      </p>
      <Button variant="primary" size="sm" onclick={compute} disabled={computing}>
        {computing ? 'Analysing…' : 'Analyse fit with AI'}
      </Button>
    {:else}
      {@const tone = verdictTone(analysis.overall_score)}
      {#if data?.stale}
        <div class="flex items-center justify-between gap-2 rounded border border-dashed border-amber-500/40 bg-amber-500/5 px-2.5 py-2">
          <span class="text-xs text-amber-700 dark:text-amber-500">Your CV or this job changed since this ran.</span>
          <Button variant="ghost" size="sm" onclick={compute} disabled={computing}>
            <RefreshCw class="size-3.5 {computing ? 'animate-spin' : ''}" />
            {computing ? 'Redoing…' : 'Recompute'}
          </Button>
        </div>
      {/if}

      <div class="flex items-baseline justify-between gap-2">
        <span class="text-2xl font-bold tabular-nums leading-none {toneText[tone]}">{analysis.overall_score}%</span>
        <span class="text-sm font-medium {toneText[tone]}">{analysis.verdict}</span>
      </div>

      <div class="flex flex-col gap-2">
        {#each dimensions as d (d.key)}
          {@const dt = verdictTone(d.score)}
          <div class="flex flex-col gap-1">
            <div class="flex items-baseline justify-between gap-2 text-xs">
              <span class="text-muted-foreground">{d.label}</span>
              <span class="tabular-nums font-medium {toneText[dt]}">{d.score}</span>
            </div>
            <div class="flex h-1.5 overflow-hidden rounded bg-secondary">
              <div class="h-full {toneBar[dt]} transition-all" style="width: {d.score}%"></div>
            </div>
            {#if d.comment}<p class="text-xs text-muted-foreground">{d.comment}</p>{/if}
          </div>
        {/each}
      </div>

      {#if requirements.length}
        <div class="flex flex-col gap-1.5">
          <span class="text-xs font-medium text-muted-foreground">Requirements (ATS view)</span>
          <ul class="flex flex-col gap-1">
            {#each requirements as r, i (i)}
              {@const meta = requirementStatusMeta(r.status)}
              <li class="flex items-center justify-between gap-2 text-xs">
                <span class="min-w-0 truncate">{r.text}</span>
                <span class="shrink-0 rounded-full border px-2 py-0.5 font-medium {toneChip[meta.tone]}">{meta.label}</span>
              </li>
            {/each}
          </ul>
        </div>
      {/if}

      {#if strengths.length}
        <div class="flex flex-col gap-1">
          <span class="text-xs font-medium text-muted-foreground">Strengths</span>
          <ul class="flex flex-col gap-0.5 text-xs">
            {#each strengths as s, i (i)}<li class="flex gap-1.5"><span class="text-emerald-500">+</span>{s}</li>{/each}
          </ul>
        </div>
      {/if}

      {#if gaps.length}
        <div class="flex flex-col gap-1">
          <span class="text-xs font-medium text-muted-foreground">Gaps</span>
          <ul class="flex flex-col gap-0.5 text-xs">
            {#each gaps as g, i (i)}<li class="flex gap-1.5"><span class="text-destructive">−</span>{g}</li>{/each}
          </ul>
        </div>
      {/if}

      {#if analysis.recommendation}
        <p class="text-xs text-muted-foreground italic">{analysis.recommendation}</p>
      {/if}
    {/if}
    </div>
  {/if}
</section>
