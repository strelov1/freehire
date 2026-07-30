<script lang="ts">
  // What tailoring did to the CV's ATS readiness, measured on the rendered PDF's text layer —
  // the artifact a parser reads, not the document we meant to render. The tailored copy is
  // compared against the base CV it came from, with the template, the margins and the keyword
  // baseline held identical, so the number is the content's contribution and nothing else.
  //
  // Informational: it gates nothing. The candidate can still render, download and send a CV
  // whose score fell — they simply get told that it did, and which category fell.
  //
  // An unavailable delta renders nothing at all. A workspace that says "ATS score unavailable"
  // teaches the candidate to ignore the panel; an absence teaches nothing.
  import { TrendingDown, TrendingUp, Minus } from '@lucide/svelte';
  import { viewAtsDelta, toneOf, type AtsDeltaTone } from './atsdelta';
  import type { CvAtsDelta } from '$lib/cv';

  let { data }: { data: CvAtsDelta | null | undefined } = $props();

  const view = $derived(viewAtsDelta(data));

  const toneClass: Record<AtsDeltaTone, string> = {
    down: 'text-amber-600 dark:text-amber-400',
    up: 'text-emerald-600 dark:text-emerald-400',
    flat: 'text-muted-foreground',
  };
  const overallTone = $derived(view ? toneOf(view.overall.change, view.regressed) : 'flat');
</script>

{#if view}
  <section class="mb-4 rounded-xl border border-border bg-muted/30 p-3">
    <header class="mb-2 flex flex-wrap items-center justify-between gap-2">
      <h3 class="text-sm font-semibold text-foreground">ATS readability</h3>
      <span class="inline-flex items-center gap-1 text-sm font-semibold {toneClass[overallTone]}">
        {#if overallTone === 'down'}
          <TrendingDown class="size-4" aria-hidden="true" />
        {:else if overallTone === 'up'}
          <TrendingUp class="size-4" aria-hidden="true" />
        {:else}
          <Minus class="size-4" aria-hidden="true" />
        {/if}
        {view.overall.base} → {view.overall.tailored}
        <span class="font-normal">({view.overall.text})</span>
      </span>
    </header>

    {#if view.warning}
      <p
        class="mb-2 rounded-lg bg-amber-500/10 px-2 py-1.5 text-xs text-amber-700 dark:text-amber-300"
        role="status"
      >
        {view.warning}
      </p>
    {/if}

    <ul class="space-y-1">
      {#each view.rows as row (row.id)}
        <li class="flex items-center justify-between gap-2 text-xs">
          <span class="text-muted-foreground">{row.label}</span>
          <span class="tabular-nums {toneClass[toneOf(row.change)]}">
            {row.base} → {row.tailored}
            <span class="text-muted-foreground">({row.text})</span>
          </span>
        </li>
      {/each}
    </ul>

    <p class="mt-2 text-[11px] leading-snug text-muted-foreground">
      Measured on the rendered PDF, against your base CV as it stands now.
    </p>
  </section>
{/if}
