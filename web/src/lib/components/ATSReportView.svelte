<script lang="ts">
  import { Check, TriangleAlert, X } from '@lucide/svelte';
  import type { ATSReport } from '$lib/types';

  // The CV ATS-readiness report from the backend: an overall score out of 100 built
  // from five weighted categories, each with per-item point attribution, plus the
  // role's strong/recommended keywords and (after an AI review) numbered suggestions.
  let { report }: { report: ATSReport } = $props();

  const categories = $derived(report.categories ?? []);
  const strong = $derived(report.strong_keywords ?? []);
  const recommended = $derived(report.recommended_keywords ?? []);
  const suggestions = $derived(report.suggestions ?? []);

  function tone(score: number): string {
    if (score >= 75) return 'text-primary';
    if (score >= 50) return 'text-amber-500';
    return 'text-destructive';
  }
</script>

<div class="flex flex-col gap-6">
  <div class="flex flex-col gap-1">
    <h2 class="text-sm font-semibold uppercase tracking-[0.14em] text-muted-foreground">CV readiness</h2>
    <p class="text-sm text-muted-foreground">
      How well an ATS reads your CV and whether it carries this role's keywords.
    </p>
  </div>

  <!-- Scoreboard: overall now, potential if you apply the fixes -->
  <div class="flex flex-wrap items-center justify-between gap-4 rounded-xl border border-border bg-card p-5">
    <div class="flex flex-col gap-1">
      <div class="flex items-baseline gap-1">
        <span class="text-6xl font-semibold tabular-nums leading-none {tone(report.overall)}">{report.overall}</span>
        <span class="text-2xl font-medium text-muted-foreground">/100</span>
      </div>
      <p class="text-sm font-medium">Overall ATS score</p>
    </div>
    {#if report.potential > report.overall}
      <div class="flex flex-col items-end gap-0.5">
        <div class="flex items-baseline gap-1">
          <span class="text-3xl font-semibold tabular-nums leading-none text-primary">{report.potential}</span>
          <span class="text-lg font-medium text-muted-foreground">/100</span>
        </div>
        <p class="text-xs text-muted-foreground">if you apply every fix below</p>
      </div>
    {/if}
  </div>

  <!-- Category cards -->
  <div class="grid gap-3 sm:grid-cols-2">
    {#each categories as cat (cat.id)}
      <div class="flex flex-col gap-3 rounded-xl border border-border bg-card p-4">
        <div class="flex items-baseline justify-between gap-2">
          <span class="text-sm font-semibold">{cat.label}</span>
          <span class="text-sm font-semibold tabular-nums {tone(Math.round((cat.score / cat.max) * 100))}">
            {cat.score}<span class="text-muted-foreground">/{cat.max}</span>
          </span>
        </div>
        <div class="h-1.5 overflow-hidden rounded bg-secondary">
          <div class="h-full rounded bg-primary" style="width: {(cat.score / cat.max) * 100}%"></div>
        </div>
        <ul class="flex flex-col gap-1.5">
          {#each cat.items as it, i (i)}
            <li class="flex items-start gap-2 text-xs">
              <span class="mt-0.5 shrink-0">
                {#if it.status === 'pass'}
                  <Check class="size-3.5 text-primary" />
                {:else if it.status === 'warn'}
                  <TriangleAlert class="size-3.5 text-amber-500" />
                {:else}
                  <X class="size-3.5 text-destructive" />
                {/if}
              </span>
              <span class="leading-relaxed {it.status === 'pass' ? 'text-foreground' : 'text-muted-foreground'}">
                <span
                  class="font-medium tabular-nums {it.status === 'pass' ? 'text-primary' : 'text-muted-foreground'}"
                >
                  +{it.points}
                </span>
                {it.text}
              </span>
            </li>
          {/each}
        </ul>
      </div>
    {/each}
  </div>

  <!-- Keywords -->
  {#if strong.length > 0 || recommended.length > 0}
    <div class="grid gap-4 sm:grid-cols-2">
      {#if strong.length > 0}
        <div class="flex flex-col gap-2">
          <h3 class="text-xs font-semibold uppercase tracking-[0.14em] text-muted-foreground">Strong keywords</h3>
          <div class="flex flex-wrap gap-1.5">
            {#each strong as s (s)}
              <span class="rounded-full bg-primary/10 px-2.5 py-1 text-xs font-medium text-primary">{s}</span>
            {/each}
          </div>
        </div>
      {/if}
      {#if recommended.length > 0}
        <div class="flex flex-col gap-2">
          <h3 class="text-xs font-semibold uppercase tracking-[0.14em] text-muted-foreground">
            Recommended keywords ({recommended.length})
          </h3>
          <div class="flex flex-wrap gap-1.5">
            {#each recommended as s (s)}
              <span class="rounded-full bg-destructive/10 px-2.5 py-1 text-xs font-medium text-destructive">{s}</span>
            {/each}
          </div>
        </div>
      {/if}
    </div>
  {/if}

  <!-- AI suggestions (only after a review) -->
  {#if suggestions.length > 0}
    <div class="flex flex-col gap-2">
      <h3 class="text-xs font-semibold uppercase tracking-[0.14em] text-muted-foreground">Suggestions</h3>
      <ol class="flex flex-col gap-1.5">
        {#each suggestions as s, i (i)}
          <li class="flex items-start gap-2 text-sm text-muted-foreground">
            <span class="mt-px shrink-0 font-medium tabular-nums text-foreground">{i + 1}.</span>
            <span class="leading-relaxed">{s}</span>
          </li>
        {/each}
      </ol>
    </div>
  {/if}
</div>
