<script lang="ts">
  import { ArrowLeft, Check, TrendingUp } from '@lucide/svelte';
  import type { Verdict } from '$lib/types';
  import { Badge } from '$lib/ui';

  // The verdict is the backend's market-coverage computation for the selected role:
  // how many of the role's open vacancies the profile's skills reach, and which missing
  // skill unlocks the most new vacancies. `role` is a humanized label for the header;
  // `gapHref`, when given, links a gap skill to the matching job search.
  let {
    verdict,
    name,
    role,
    gapHref,
  }: {
    verdict: Verdict;
    name: string;
    role: string;
    gapHref?: (skill: string) => string;
  } = $props();

  const gaps = $derived(verdict.gaps ?? []);
  const uncovered = $derived(Math.max(verdict.total - verdict.covered, 0));
</script>

<div class="flex flex-col gap-8">
  <!-- Header -->
  <div class="flex flex-col gap-3">
    <a
      href="/my/profiles"
      class="inline-flex w-fit items-center gap-1 text-sm text-muted-foreground transition-colors hover:text-foreground"
    >
      <ArrowLeft class="size-4" />
      Profiles
    </a>
    <div class="flex flex-col gap-1">
      <h1 class="text-3xl font-semibold tracking-tight">Your market coverage</h1>
      <p class="text-sm text-muted-foreground">{name}{role ? ` · ${role}` : ''}</p>
    </div>
  </div>

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
      <div class="h-full rounded bg-primary transition-all" style="width: {verdict.coverage_percent}%"></div>
    </div>
    <p class="text-sm text-muted-foreground">
      Vacancies for this role that mention at least one of your skills. Add the skills below to
      reach more.
    </p>
  </div>

  <!-- Biggest wins -->
  {#if gaps.length > 0}
    <div class="flex flex-col gap-3">
      <div class="flex items-center gap-2">
        <TrendingUp class="size-4 text-muted-foreground" />
        <h2 class="text-sm font-semibold uppercase tracking-[0.14em] text-muted-foreground">
          Add a skill · what unlocks the most vacancies
        </h2>
      </div>
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
    </div>
  {:else}
    <div class="flex items-center gap-2 rounded-lg border border-border bg-card/50 p-4 text-sm text-muted-foreground">
      <Check class="size-4 text-primary" />
      No in-demand skills left to add for this role — your stack already reaches its open vacancies.
    </div>
  {/if}
</div>
