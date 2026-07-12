<script lang="ts">
  import { Check, TrendingUp } from '@lucide/svelte';
  import type { Verdict } from '$lib/types';
  import { Badge } from '$lib/ui';

  // The verdict is the backend's market-coverage computation for the selected role:
  // how many of the role's open vacancies the profile's skills reach, which missing
  // skill unlocks the most new vacancies, and a breakdown of the role's top in-demand
  // skills scored against the CV. `gapHref`, when given, links a gap skill to the
  // matching job search. The page owns the header/tabs; this renders the body.
  let {
    verdict,
    gapHref,
  }: {
    verdict: Verdict;
    gapHref?: (skill: string) => string;
  } = $props();

  const gaps = $derived(verdict.gaps ?? []);
  const skills = $derived(verdict.skills ?? []);
  const bundles = $derived(verdict.bundles ?? []);
  const uncovered = $derived(Math.max(verdict.total - verdict.covered, 0));

  // The two detail lists (gaps to add vs the role's top skills) are tabbed so only one
  // shows at a time — keeps the coverage view compact under the headline + stats.
  let sub = $state<'gaps' | 'skills'>('gaps');

  // Status → badge styling (STRONG green / HIDDEN amber / ADJACENT blue / MISSING red).
  const statusStyle: Record<string, string> = {
    strong: 'bg-brand/10 text-primary',
    hidden: 'bg-amber-500/10 text-amber-600 dark:text-amber-500',
    adjacent: 'bg-sky-500/10 text-sky-600 dark:text-sky-400',
    missing: 'bg-destructive/10 text-destructive',
  };
</script>

<div class="flex flex-col gap-8">
  <!-- Coverage headline -->
  <div class="flex flex-col gap-4 rounded-xl border border-border bg-card p-6 animate-in fade-in slide-in-from-bottom-2 duration-500">
    <div class="flex flex-wrap items-end justify-between gap-2">
      <div class="flex items-baseline gap-2">
        <span class="text-6xl font-semibold tabular-nums leading-none">{verdict.coverage_percent}%</span>
        <span class="text-sm font-medium text-muted-foreground">
          {verdict.covered.toLocaleString('en-US')} of {verdict.total.toLocaleString('en-US')} open vacancies
        </span>
      </div>
      <Badge variant="outline" class="uppercase tracking-[0.14em]">
        {uncovered.toLocaleString('en-US')} out of reach
      </Badge>
    </div>
    <div class="h-2 overflow-hidden rounded bg-secondary">
      <div class="h-full rounded bg-brand transition-all" style="width: {verdict.coverage_percent}%"></div>
    </div>
    <p class="text-sm text-muted-foreground">
      Vacancies for this role that mention at least one of your skills. Add the skills below to
      reach more.
    </p>
  </div>

  <!-- Market-anchored stats -->
  <div class="grid gap-3 sm:grid-cols-3">
    <div class="flex flex-col gap-1 rounded-xl border border-border bg-card p-4">
      {#if verdict.must_have_total > 0}
        <span class="text-3xl font-semibold tabular-nums leading-none">
          {verdict.must_have_covered}<span class="text-lg text-muted-foreground">/{verdict.must_have_total}</span>
        </span>
      {:else}
        <!-- No skill clears the must-have frequency threshold for this role, so the
             covered/total ratio is undefined — show a dash instead of a broken "0/0". -->
        <span
          class="text-3xl font-semibold leading-none text-muted-foreground"
          title="No single skill appears in the majority of this role's vacancies, so there's no must-have to measure."
        >
          —
        </span>
      {/if}
      <span class="text-xs font-medium uppercase tracking-wide text-muted-foreground">Must-have skills covered</span>
    </div>
    <div class="flex flex-col gap-1 rounded-xl border border-border bg-card p-4">
      <span class="text-3xl font-semibold tabular-nums leading-none">{verdict.stack_match_percent}%</span>
      <span class="text-xs font-medium uppercase tracking-wide text-muted-foreground">Stack match</span>
    </div>
    <div class="flex flex-col gap-1 rounded-xl border border-border bg-card p-4">
      <span class="text-3xl font-semibold tabular-nums leading-none">{verdict.coherence_percent}%</span>
      <span class="text-xs font-medium uppercase tracking-wide text-muted-foreground">Coherence</span>
    </div>
  </div>

  <!-- Skill bundles: the market expects combinations, not isolated skills -->
  {#if bundles.length > 0}
    <div class="flex flex-col gap-2">
      <h2 class="text-base font-semibold tracking-tight">Skill bundles the market expects</h2>
      <div class="flex flex-wrap gap-2">
        {#each bundles as b (b.name)}
          <span
            class="inline-flex items-center gap-1.5 rounded-lg border border-border px-3 py-1.5 text-sm {b.has
              ? 'bg-brand/5 text-foreground'
              : 'bg-card text-muted-foreground'}"
          >
            {#if b.has}<Check class="size-3.5 text-primary" />{/if}
            <span class="font-medium">{b.label}</span>
            <span class="tabular-nums text-xs text-muted-foreground">{b.covered}/{b.total}</span>
          </span>
        {/each}
      </div>
    </div>
  {/if}

  <!-- Detail lists, tabbed to stay compact -->
  <div class="flex flex-col gap-3">
    <div class="flex gap-5 border-b border-border">
      <button
        type="button"
        onclick={() => (sub = 'gaps')}
        class="-mb-px flex items-center gap-1.5 border-b-2 px-1 pb-2 text-sm font-medium transition-colors {sub === 'gaps'
          ? 'border-brand text-foreground'
          : 'border-transparent text-muted-foreground hover:text-foreground'}"
      >
        <TrendingUp class="size-4" />
        Add a skill
      </button>
      <button
        type="button"
        onclick={() => (sub = 'skills')}
        class="-mb-px border-b-2 px-1 pb-2 text-sm font-medium transition-colors {sub === 'skills'
          ? 'border-brand text-foreground'
          : 'border-transparent text-muted-foreground hover:text-foreground'}"
      >
        Top market skills
      </button>
    </div>

    {#if sub === 'gaps'}
      {#if gaps.length > 0}
        <ul class="flex flex-col gap-2">
          {#each gaps as gap, i (gap.name)}
            <li
              class="flex items-center gap-3 rounded-lg border border-border bg-card p-3 sm:p-4 animate-in fade-in slide-in-from-bottom-1 duration-300"
              style="animation-delay: {Math.min(i * 30, 400)}ms"
            >
              <span class="w-6 shrink-0 text-sm tabular-nums text-muted-foreground">{i + 1}</span>
              <div class="flex min-w-0 flex-1 flex-col">
                {#if gapHref}
                  <a href={gapHref(gap.name)} class="truncate text-sm font-medium hover:underline">{gap.name}</a>
                {:else}
                  <span class="truncate text-sm font-medium">{gap.name}</span>
                {/if}
              </div>
              <div class="flex shrink-0 flex-col items-end">
                <span class="inline-flex items-center gap-1 text-sm font-semibold tabular-nums">
                  <TrendingUp class="size-3.5" />
                  +{gap.new_vacancies.toLocaleString('en-US')}
                </span>
                <span class="text-xs text-muted-foreground">+{gap.unlock_percent}% of the role</span>
              </div>
            </li>
          {/each}
        </ul>
      {:else}
        <div class="flex items-center gap-2 rounded-lg border border-border bg-card/50 p-4 text-sm text-muted-foreground">
          <Check class="size-4 text-primary" />
          No in-demand skills left to add for this role — your stack already reaches its open vacancies.
        </div>
      {/if}
    {:else if skills.length > 0}
      <ul class="flex flex-col gap-2">
        {#each skills as skill, i (skill.name)}
          <li class="flex flex-col gap-1.5 rounded-lg border border-border bg-card p-3 sm:p-4">
            <div class="flex items-center gap-3">
              <span class="w-6 shrink-0 text-sm tabular-nums text-muted-foreground">{i + 1}</span>
              <div class="flex min-w-0 flex-1 flex-wrap items-center gap-2">
                {#if gapHref && skill.status !== 'strong'}
                  <a href={gapHref(skill.name)} class="truncate text-sm font-medium hover:underline">{skill.name}</a>
                {:else}
                  <span class="truncate text-sm font-medium">{skill.name}</span>
                {/if}
                {#if skill.must_have}
                  <span
                    class="rounded bg-secondary px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-muted-foreground"
                  >
                    Must-have
                  </span>
                {/if}
                <span class="text-xs text-muted-foreground">{skill.market_frequency}% of roles</span>
              </div>
              <span
                class="shrink-0 rounded px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wide {statusStyle[
                  skill.status
                ]}"
              >
                {skill.status}
              </span>
            </div>
            {#if skill.advice}
              <p class="pl-9 text-xs leading-relaxed text-muted-foreground">{skill.advice}</p>
            {/if}
          </li>
        {/each}
      </ul>
    {/if}
  </div>
</div>
