<script lang="ts">
  import { ArrowLeft, Check, TrendingUp } from '@lucide/svelte';
  import type { Verdict } from '$lib/types';
  import { Badge } from '$lib/ui';

  // The verdict is computed by the backend (deterministic market comparison + optional
  // LLM coherence/advice). name/role are the owning profile's label and a humanized
  // specialization string for the header and the "of <role> roles" copy.
  let { verdict, name, role }: { verdict: Verdict; name: string; role: string } = $props();

  const skills = $derived(verdict.skills ?? []);
  const strong = $derived(skills.filter((s) => s.have));
  // The gaps that open the most of the market, biggest win first — the whole point:
  // not "random missing skills" but the ones that unlock the most vacancies.
  const topGaps = $derived(
    skills
      .filter((s) => !s.have && s.unlock != null)
      .toSorted((a, b) => (b.unlock ?? 0) - (a.unlock ?? 0)),
  );

  // Stack Match always shows; Coherence only when the résumé has been analyzed.
  const stats = $derived(
    [
      { label: 'Stack Match', value: verdict.stack_match },
      ...(verdict.coherence != null ? [{ label: 'Coherence Score', value: verdict.coherence }] : []),
    ].map((s, i) => ({ ...s, delay: 80 + i * 80 })),
  );
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
      <h1 class="text-3xl font-semibold tracking-tight">Here's your verdict</h1>
      <p class="text-sm text-muted-foreground">{name} · {role}</p>
    </div>
  </div>

  <!-- Scoreboard -->
  <div class="grid gap-3 sm:grid-cols-[1.4fr_1fr]">
    <!-- Must-haves -->
    <div
      class="flex flex-col justify-between gap-4 rounded-xl border border-border bg-card p-5 animate-in fade-in slide-in-from-bottom-2 duration-500"
    >
      <div class="flex items-center justify-between gap-2">
        <Badge variant="outline" class="uppercase tracking-[0.14em]">
          {verdict.must_have_total - verdict.must_have_covered} must-haves away
        </Badge>
      </div>
      <div>
        <div class="flex items-baseline gap-1">
          <span class="text-6xl font-semibold tabular-nums leading-none">{verdict.must_have_covered}</span>
          <span class="text-3xl font-medium tabular-nums text-muted-foreground">/{verdict.must_have_total}</span>
        </div>
        <p class="mt-2 text-sm font-medium">Must-have skills covered</p>
        <p class="text-sm text-muted-foreground">
          You're close. Closing these gaps puts you past most ATS filters.
        </p>
      </div>
    </div>

    <!-- Stack match (+ coherence when analyzed) -->
    <div class="grid gap-3">
      {#each stats as stat (stat.label)}
        <div
          class="flex flex-col gap-2 rounded-xl border border-border bg-card p-5 animate-in fade-in slide-in-from-bottom-2 duration-500"
          style="animation-delay: {stat.delay}ms"
        >
          <div class="flex items-baseline justify-between">
            <span class="text-4xl font-semibold tabular-nums leading-none">{stat.value}%</span>
            <span class="text-xs font-medium uppercase tracking-wide text-muted-foreground">{stat.label}</span>
          </div>
          <div class="h-1.5 overflow-hidden rounded bg-secondary">
            <div class="h-full rounded bg-primary" style="width: {stat.value}%"></div>
          </div>
        </div>
      {/each}
    </div>
  </div>

  <!-- Legend -->
  <p class="rounded-lg border border-border bg-card/50 p-4 text-xs leading-relaxed text-muted-foreground">
    <span class="font-semibold text-foreground">Must-haves</span> are the non-negotiables for this role —
    cover them all and you're past the first screen.
    <span class="font-semibold text-foreground">Stack Match</span> shows your breadth across the top-20 skills.
    <span class="font-semibold text-foreground">Coherence Score</span> checks that every skill you claim is
    also backed by your Experience — a low score is buzzword stuffing.
  </p>

  <!-- Biggest wins -->
  {#if topGaps.length > 0}
    <div class="flex flex-col gap-3">
      <div class="flex items-center gap-2">
        <TrendingUp class="size-4 text-muted-foreground" />
        <h2 class="text-sm font-semibold uppercase tracking-[0.14em] text-muted-foreground">
          Biggest wins · what unlocks the most roles
        </h2>
      </div>
      <div class="grid gap-2 sm:grid-cols-3">
        {#each topGaps.slice(0, 3) as gap, i (gap.rank)}
          <div
            class="flex flex-col gap-1 rounded-xl border border-border bg-card p-4 animate-in fade-in slide-in-from-bottom-2 duration-500"
            style="animation-delay: {i * 70}ms"
          >
            <span class="text-3xl font-semibold tabular-nums leading-none">+{gap.unlock}%</span>
            <span class="text-xs text-muted-foreground">of {role} roles</span>
            <span class="mt-1 line-clamp-2 text-sm font-medium">{gap.name}</span>
          </div>
        {/each}
      </div>
    </div>
  {/if}

  <!-- Full breakdown -->
  <div class="flex flex-col gap-3">
    <h2 class="text-sm font-semibold uppercase tracking-[0.14em] text-muted-foreground">
      Full breakdown · top market skills
    </h2>
    <ul class="flex flex-col gap-2">
      {#each skills as skill, i (skill.rank)}
        <li
          class="flex items-start gap-3 rounded-lg border border-border bg-card p-3 sm:p-4 animate-in fade-in slide-in-from-bottom-1 duration-300"
          style="animation-delay: {Math.min(i * 30, 400)}ms"
        >
          <span class="w-6 shrink-0 pt-0.5 text-sm tabular-nums {skill.have ? 'text-muted-foreground' : 'text-foreground'}">
            {skill.rank}
          </span>

          <div class="flex min-w-0 flex-1 flex-col gap-1">
            <div class="flex flex-wrap items-center gap-x-2 gap-y-1">
              <span class="text-sm font-medium">{skill.name}</span>
              {#if skill.must_have}
                <Badge variant="outline" class="uppercase tracking-wide">must-have</Badge>
              {/if}
            </div>
            {#if skill.advice}
              <p class="text-xs leading-relaxed text-muted-foreground">{skill.advice}</p>
            {/if}
          </div>

          <div class="flex shrink-0 flex-col items-end gap-1 pt-0.5">
            {#if skill.have}
              <span class="inline-flex items-center gap-1 text-xs font-medium text-muted-foreground">
                <Check class="size-3.5" />
                Covered
              </span>
            {:else}
              <span class="rounded-md border border-border px-2 py-0.5 text-xs font-medium text-muted-foreground">
                Gap
              </span>
              {#if skill.unlock != null}
                <span class="inline-flex items-center gap-1 text-xs font-semibold tabular-nums">
                  <TrendingUp class="size-3" />
                  +{skill.unlock}% roles
                </span>
              {/if}
            {/if}
          </div>
        </li>
      {/each}
    </ul>
  </div>

  <!-- Your superpowers -->
  {#if strong.length > 0}
    <div class="flex flex-col gap-3">
      <h2 class="text-sm font-semibold uppercase tracking-[0.14em] text-muted-foreground">
        Your superpowers
      </h2>
      <div class="flex flex-wrap gap-1.5">
        {#each strong as skill (skill.rank)}
          <Badge>{skill.name}</Badge>
        {/each}
      </div>
    </div>
  {/if}
</div>
