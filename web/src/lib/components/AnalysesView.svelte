<script lang="ts">
  import { resolve } from '$app/paths';
  import { ArrowRight } from '@lucide/svelte';
  import { api } from '$lib/api';
  import { isAuthenticated } from '$lib/auth.svelte';
  import { verdictTone, type Tone } from '$lib/matchAnalysis';
  import type { MyAnalysisItem } from '$lib/types';
  import { companyLogoUrl } from '$lib/logo';
  import { EntityLogo } from '$lib/ui';
  import States from './States.svelte';

  // The Activity → Matches tab: the jobs the caller has run the AI match analysis on. Read-only —
  // never triggers the LLM (each row links to the Tailor workspace, which owns compute/recompute).
  // The AI-credits balance lives on its own page (/my/credits), not inline here.
  let status = $state<'loading' | 'error' | 'ready'>('loading');
  let items = $state<MyAnalysisItem[]>([]);

  $effect(() => {
    if (!isAuthenticated()) return;
    status = 'loading';
    api
      .myAnalyses()
      .then((r) => {
        items = r.items;
        status = 'ready';
      })
      .catch(() => {
        status = 'error';
      });
  });

  const toneText: Record<Tone, string> = {
    strong: 'text-brand-strong',
    good: 'text-brand-strong',
    moderate: 'text-warning-strong',
    weak: 'text-warning-strong',
    poor: 'text-destructive',
  };

  const fmtDate = (iso: string) =>
    new Date(iso).toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: 'numeric' });
</script>

{#if status === 'loading'}
  <States state="loading" />
{:else if status === 'error'}
  <States state="error" message="Couldn't load your analyses." />
{:else if items.length === 0}
  <States state="empty" message="No AI match analyses yet. Open a job and run “Analyse match with AI”." />
{:else}
  <ul class="flex flex-col gap-3">
    {#each items as it (it.slug)}
      {@const tone = verdictTone(it.overall_score)}
      <li>
        <a
          href={resolve('/tailor/[slug]', { slug: it.slug })}
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
                <span class="shrink-0 rounded-full border border-warning/40 px-1.5 py-0.5 text-[0.65rem] font-semibold uppercase text-warning-strong">Stale</span>
              {/if}
            </span>
            <span class="flex items-center gap-1.5 text-xs text-muted-foreground">
              <EntityLogo
                name={it.company || 'Unknown company'}
                src={companyLogoUrl(it.company) ?? undefined}
                shape="square"
                size="xs"
              />
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
