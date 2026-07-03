<script lang="ts">
  import { Check, TriangleAlert, X } from '@lucide/svelte';
  import type { ATSReport } from '$lib/types';

  // The deterministic CV ATS-readiness report from the backend: an overall score,
  // readability + keyword-match sub-scores, and a checklist of pass/warn/fail items
  // each with a concrete fix.
  let { report }: { report: ATSReport } = $props();

  const checks = $derived(report.checks ?? []);
  const findings = $derived(report.findings ?? []);

  // Sub-scores: the AI content-quality bar appears only after an AI review has run.
  const subScores = $derived([
    { label: 'Readability', value: report.readability },
    { label: 'Keyword match', value: report.keyword_match },
    ...(report.content_quality != null ? [{ label: 'AI content', value: report.content_quality }] : []),
  ]);

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
      How well an ATS can read your CV and whether it carries this role's keywords.
    </p>
  </div>

  <!-- Scoreboard -->
  <div class="grid gap-3 sm:grid-cols-[1.4fr_1fr]">
    <div class="flex flex-col justify-center gap-1 rounded-xl border border-border bg-card p-5">
      <div class="flex items-baseline gap-1">
        <span class="text-6xl font-semibold tabular-nums leading-none {tone(report.overall)}">{report.overall}</span>
        <span class="text-2xl font-medium text-muted-foreground">/100</span>
      </div>
      <p class="text-sm font-medium">Overall ATS score</p>
    </div>
    <div class="grid gap-3">
      {#each subScores as s (s.label)}
        <div class="flex flex-col gap-2 rounded-xl border border-border bg-card p-4">
          <div class="flex items-baseline justify-between">
            <span class="text-3xl font-semibold tabular-nums leading-none">{s.value}%</span>
            <span class="text-xs font-medium uppercase tracking-wide text-muted-foreground">{s.label}</span>
          </div>
          <div class="h-1.5 overflow-hidden rounded bg-secondary">
            <div class="h-full rounded bg-primary" style="width: {s.value}%"></div>
          </div>
        </div>
      {/each}
    </div>
  </div>

  <!-- Checklist -->
  <ul class="flex flex-col gap-2">
    {#each checks as c (c.id)}
      <li class="flex items-start gap-3 rounded-lg border border-border bg-card p-3 sm:p-4">
        <span class="mt-0.5 shrink-0">
          {#if c.status === 'pass'}
            <Check class="size-4 text-primary" />
          {:else if c.status === 'warn'}
            <TriangleAlert class="size-4 text-amber-500" />
          {:else}
            <X class="size-4 text-destructive" />
          {/if}
        </span>
        <div class="flex min-w-0 flex-1 flex-col gap-0.5">
          <span class="text-sm font-medium">{c.label}</span>
          {#if c.fix && c.status !== 'pass'}
            <span class="text-xs leading-relaxed text-muted-foreground">{c.fix}</span>
          {/if}
        </div>
      </li>
    {/each}
  </ul>

  <!-- AI findings (only after an AI review) -->
  {#if findings.length > 0}
    <div class="flex flex-col gap-2">
      <h3 class="text-xs font-semibold uppercase tracking-[0.14em] text-muted-foreground">AI suggestions</h3>
      <ul class="flex flex-col gap-1.5">
        {#each findings as f, i (i)}
          <li class="flex items-start gap-2 text-sm text-muted-foreground">
            <span class="mt-1 size-1.5 shrink-0 rounded-full bg-primary"></span>
            <span class="leading-relaxed">{f}</span>
          </li>
        {/each}
      </ul>
    </div>
  {/if}
</div>
