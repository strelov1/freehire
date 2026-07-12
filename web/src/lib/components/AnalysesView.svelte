<script lang="ts">
  import { resolve } from '$app/paths';
  import { ArrowRight } from '@lucide/svelte';
  import { api } from '$lib/api';
  import { isAuthenticated } from '$lib/auth.svelte';
  import { verdictTone, type Tone } from '$lib/jobFit';
  import type { MyAnalysisItem, JobFitQuota } from '$lib/types';
  import CompanyLogo from './CompanyLogo.svelte';
  import States from './States.svelte';

  // The Tracking → AI fit tab: the jobs the caller has run the AI fit analysis on, with
  // their monthly quota. Read-only — never triggers the LLM (each row links to the fit
  // page, which owns compute/recompute).
  let status = $state<'loading' | 'error' | 'ready'>('loading');
  let items = $state<MyAnalysisItem[]>([]);
  let quota = $state<JobFitQuota | null>(null);

  $effect(() => {
    if (!isAuthenticated()) return;
    status = 'loading';
    api
      .myAnalyses()
      .then((r) => {
        items = r.items;
        quota = r.quota;
        status = 'ready';
      })
      .catch(() => {
        status = 'error';
      });
  });

  const toneText: Record<Tone, string> = {
    strong: 'text-brand-strong',
    good: 'text-brand-strong',
    moderate: 'text-amber-600 dark:text-amber-500',
    weak: 'text-amber-600 dark:text-amber-500',
    poor: 'text-destructive',
  };

  const fmtDate = (iso: string) =>
    new Date(iso).toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: 'numeric' });
</script>

{#if quota}
  <div
    class="flex items-center justify-between gap-3 rounded-lg border border-border bg-secondary/40 px-4 py-2.5 text-sm"
  >
    <span class="text-muted-foreground">
      <strong class="font-semibold text-foreground">{quota.used}/{quota.limit}</strong> AI analyses used this month
    </span>
    <span class="text-xs text-muted-foreground">{quota.remaining} left</span>
  </div>
{/if}

{#if status === 'loading'}
  <States state="loading" />
{:else if status === 'error'}
  <States state="error" message="Couldn't load your analyses." />
{:else if items.length === 0}
  <States state="empty" message="No AI fit analyses yet. Open a job and run “Analyse fit with AI”." />
{:else}
  <ul class="flex flex-col gap-3">
    {#each items as it (it.slug)}
      {@const tone = verdictTone(it.overall_score)}
      <li>
        <a
          href={resolve('/jobs/[slug]/fit', { slug: it.slug })}
          class="group flex items-center gap-4 rounded-lg border border-border p-3.5 transition-colors hover:border-brand/40 hover:bg-accent/40"
        >
          <span class="w-12 shrink-0 text-center text-2xl font-bold tabular-nums leading-none {toneText[tone]}">
            {it.overall_score}<span class="text-xs font-medium text-muted-foreground">%</span>
          </span>
          <div class="flex min-w-0 flex-1 flex-col gap-1">
            <span class="flex items-center gap-2">
              <span class="truncate font-medium">{it.title}</span>
              {#if it.closed}
                <span class="shrink-0 rounded-full border border-border px-1.5 py-0.5 text-[0.65rem] font-semibold uppercase text-muted-foreground">Closed</span>
              {/if}
              {#if it.stale}
                <span class="shrink-0 rounded-full border border-amber-500/40 px-1.5 py-0.5 text-[0.65rem] font-semibold uppercase text-amber-700 dark:text-amber-500">Stale</span>
              {/if}
            </span>
            <span class="flex items-center gap-1.5 text-xs text-muted-foreground">
              <CompanyLogo name={it.company} size="size-4" />
              <span class="truncate">{it.company}</span>
              <span aria-hidden="true">·</span>
              <span class="shrink-0">{fmtDate(it.analysed_at)}</span>
            </span>
          </div>
          <span class="flex shrink-0 items-center gap-1 text-sm font-medium {toneText[tone]}">
            {it.verdict}
            <ArrowRight class="size-3.5 transition-transform group-hover:translate-x-0.5" />
          </span>
        </a>
      </li>
    {/each}
  </ul>
{/if}
