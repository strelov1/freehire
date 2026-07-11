<script lang="ts">
  import { resolve } from '$app/paths';
  import { ArrowRight, FileText } from '@lucide/svelte';
  import { api } from '$lib/api';
  import { isAuthenticated } from '$lib/auth.svelte';
  import { verdictTone, type Tone } from '$lib/jobFit';
  import type { JobFitResponse } from '$lib/types';
  import { Button } from '$lib/ui';

  // A compact fit summary in the Profile-match sidebar: the overall %, verdict, and the
  // single biggest gap when an analysis is cached, with a link to the full-page analysis
  // (which runs the live streaming compute). It never computes inline.
  let { slug }: { slug: string } = $props();

  let data = $state<JobFitResponse | null>(null);

  // Read the cached summary (never runs the model); re-fetch on navigation with a guard
  // so a slow response for a previous job can't overwrite the current one.
  $effect(() => {
    const forSlug = slug;
    data = null;
    if (!isAuthenticated()) return;
    api.getJobFit(forSlug)
      .then((d) => {
        if (forSlug === slug) data = d;
      })
      .catch(() => {});
  });

  const analysis = $derived(data?.analysis ?? null);
  const topGap = $derived(analysis?.gaps?.[0] ?? null);

  const toneText: Record<Tone, string> = {
    strong: 'text-emerald-600 dark:text-emerald-400',
    good: 'text-emerald-600 dark:text-emerald-400',
    moderate: 'text-amber-600 dark:text-amber-500',
    weak: 'text-amber-600 dark:text-amber-500',
    poor: 'text-destructive',
  };
</script>

<section class="flex flex-col gap-3 border-t border-border pt-4" aria-label="AI fit analysis">
  <p class="text-xs font-semibold uppercase tracking-wide text-muted-foreground">AI fit analysis</p>

  {#if data && !data.has_cv}
    <div class="flex items-center justify-between gap-2">
      <span class="flex items-center gap-1.5 text-xs text-muted-foreground"><FileText class="size-3.5 shrink-0" />Upload a CV to analyse</span>
      <Button variant="primary" size="sm" href={resolve('/my/profile')}>Upload CV</Button>
    </div>
  {:else if analysis}
    {@const tone = verdictTone(analysis.overall_score)}
    <a href={resolve('/jobs/[slug]/fit', { slug })} class="group flex flex-col gap-2 rounded-lg border border-border p-3 transition-colors hover:border-primary/40 hover:bg-accent/40">
      <div class="flex items-baseline justify-between gap-2">
        <span class="text-2xl font-bold tabular-nums leading-none {toneText[tone]}">{analysis.overall_score}%</span>
        <span class="text-sm font-medium {toneText[tone]}">{analysis.verdict}</span>
      </div>
      {#if topGap}
        <p class="text-xs text-muted-foreground"><span class="font-medium">Top gap:</span> {topGap}</p>
      {/if}
      <span class="flex items-center gap-1 text-xs font-medium text-primary">
        View full analysis <ArrowRight class="size-3.5 transition-transform group-hover:translate-x-0.5" />
      </span>
    </a>
  {:else}
    <p class="text-sm text-muted-foreground">A recruiter-style read of your CV against this role — title fit, experience, and the gaps an ATS flags.</p>
    <Button variant="primary" size="sm" href={resolve('/jobs/[slug]/fit', { slug })}>Analyse fit with AI</Button>
  {/if}
</section>
