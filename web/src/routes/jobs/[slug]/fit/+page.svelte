<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { resolve } from '$app/paths';
  import { ArrowLeft, RefreshCw, FileText, Check, Loader } from '@lucide/svelte';
  import { api } from '$lib/api';
  import { isAuthenticated } from '$lib/auth.svelte';
  import CompanyLogo from '$lib/components/CompanyLogo.svelte';
  import {
    verdictTone,
    requirementStatusMeta,
    initFitStream,
    reduceFitEvent,
    type FitStreamState,
    type Tone,
  } from '$lib/jobFit';
  import { Button } from '$lib/ui';

  let { data } = $props();

  // Stream state, seeded from the SSR-cached analysis so a fresh cache paints instantly.
  let stream = $state<FitStreamState>(seed());
  let streaming = $state(false);
  let showThinking = $state(false);
  let es: EventSource | null = null;
  // The thinking panel tails the model's reasoning: keep it pinned to the newest tokens.
  let thinkingEl = $state<HTMLElement | null>(null);
  $effect(() => {
    stream.thinking; // track new reasoning
    if (thinkingEl) thinkingEl.scrollTop = thinkingEl.scrollHeight;
  });

  function seed(): FitStreamState {
    const s = initFitStream();
    if (data.fit && !data.fit.has_cv) s.hasCV = false;
    if (data.fit?.analysis) {
      s.analysis = data.fit.analysis;
      s.requirements = data.fit.analysis.requirement_match;
      s.done = true;
      s.stages = s.stages.map((x) => ({ ...x, state: 'done' as const }));
    }
    return s;
  }

  const analysis = $derived(stream.analysis);
  const isStale = $derived(data.fit?.stale ?? false);
  // Only auto-run when there is NO cached analysis at all (cold). A stale cache still
  // paints instantly and offers a manual Recompute — so a refresh never silently burns
  // three LLM calls, and a fresh cache never recomputes.
  const coldStart = $derived(!data.fit?.analysis);
  const dimensions = $derived(analysis?.dimensions ?? []);
  const requirements = $derived(analysis?.requirement_match?.length ? analysis.requirement_match : stream.requirements);

  function start() {
    stop();
    stream = initFitStream();
    streaming = true;
    showThinking = true;
    const source = new EventSource(api.jobFitStreamUrl(data.job.public_slug), { withCredentials: true });
    es = source;
    // NB: our server error event is `stream_error`, never `error` — `error` is
    // EventSource's own reserved connection-error event (handled by onerror below).
    for (const name of ['meta', 'stage_start', 'stage_done', 'thinking', 'requirements', 'dimensions', 'final', 'stream_error']) {
      source.addEventListener(name, (e) => {
        stream = reduceFitEvent(stream, name, JSON.parse((e as MessageEvent).data));
        if (name === 'final' || name === 'stream_error') stop();
      });
    }
    // A connection drop before the final event is a real failure (not the normal close,
    // which we trigger via stop() on `final`). Surface it and don't let it auto-reconnect.
    source.onerror = () => {
      if (!stream.done) stream = reduceFitEvent(stream, 'stream_error', { message: 'Connection lost' });
      stop();
    };
  }

  function stop() {
    streaming = false;
    es?.close();
    es = null;
  }

  onMount(() => {
    if (isAuthenticated() && coldStart && (data.fit?.has_cv ?? true)) start();
  });
  onDestroy(stop);

  // Radial gauge geometry for the overall score.
  const GAUGE_R = 54;
  const GAUGE_C = 2 * Math.PI * GAUGE_R;

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
  const toneStroke: Record<Tone, string> = {
    strong: 'stroke-emerald-500',
    good: 'stroke-emerald-500',
    moderate: 'stroke-amber-500',
    weak: 'stroke-amber-500',
    poor: 'stroke-destructive',
  };
  const toneChip: Record<Tone, string> = {
    strong: 'border-emerald-500/30 bg-emerald-500/10 text-emerald-700 dark:text-emerald-400',
    good: 'border-emerald-500/30 bg-emerald-500/10 text-emerald-700 dark:text-emerald-400',
    moderate: 'border-amber-500/30 bg-amber-500/10 text-amber-700 dark:text-amber-500',
    weak: 'border-amber-500/30 bg-amber-500/10 text-amber-700 dark:text-amber-500',
    poor: 'border-destructive/30 bg-destructive/10 text-destructive',
  };
  const toneGlow: Record<Tone, string> = {
    strong: 'rgba(16,185,129,0.10)',
    good: 'rgba(16,185,129,0.10)',
    moderate: 'rgba(245,158,11,0.10)',
    weak: 'rgba(245,158,11,0.10)',
    poor: 'rgba(239,68,68,0.10)',
  };
</script>

<svelte:head><title>Fit analysis · {data.job.title}</title></svelte:head>

<div class="mx-auto flex w-full max-w-5xl flex-col gap-8 px-4 py-8 sm:py-10">
  <!-- Editorial masthead -->
  <header class="fit-reveal flex flex-col gap-4" style="--i:0">
    <a
      href={resolve('/jobs/[slug]', { slug: data.job.public_slug })}
      class="flex w-fit items-center gap-1.5 text-xs font-medium text-muted-foreground transition-colors hover:text-foreground"
    >
      <ArrowLeft class="size-3.5" />Back to role
    </a>
    <div class="flex flex-col gap-2.5 border-b border-border pb-6">
      <h1 class="text-xl font-bold leading-tight tracking-tight sm:text-2xl">{data.job.title}</h1>
      <div class="flex items-center gap-2">
        <CompanyLogo name={data.job.company} size="size-5" />
        <p class="text-sm text-muted-foreground">{data.job.company}</p>
      </div>
    </div>
  </header>

  {#if !isAuthenticated()}
    <p class="rounded-xl border border-border bg-card p-8 text-center text-sm text-muted-foreground">
      Sign in to analyse your fit for this role.
    </p>
  {:else if !stream.hasCV}
    <div class="flex flex-col items-center gap-4 rounded-xl border border-dashed border-border bg-card p-10 text-center">
      <FileText class="size-8 text-muted-foreground" />
      <p class="text-sm text-muted-foreground">Upload a CV to analyse your fit for this role.</p>
      <Button variant="primary" size="sm" href={resolve('/my/profile')}>Upload CV</Button>
    </div>
  {:else}
    <!-- Verdict hero: radial gauge + verdict -->
    {#if analysis}
      {@const tone = verdictTone(analysis.overall_score)}
      {@const covered = requirements.filter((r) => r.status === 'covered' || r.status === 'synonym-only').length}
      <section
        class="fit-reveal relative overflow-hidden rounded-2xl border border-border bg-card p-6 sm:p-8"
        style="--i:1"
      >
        <div
          class="pointer-events-none absolute -right-24 -top-24 size-72 rounded-full blur-3xl"
          style="background: radial-gradient(circle, {toneGlow[tone]}, transparent 70%)"
          aria-hidden="true"
        ></div>
        <div class="relative flex flex-col items-center gap-8 sm:flex-row sm:items-center sm:gap-10">
          <!-- Radial gauge -->
          <div class="relative shrink-0">
            <svg width="140" height="140" viewBox="0 0 140 140" class="-rotate-90">
              <circle cx="70" cy="70" r={GAUGE_R} fill="none" class="stroke-secondary" stroke-width="10" />
              <circle
                cx="70" cy="70" r={GAUGE_R} fill="none"
                class="fit-arc {toneStroke[tone]}" stroke-width="10" stroke-linecap="round"
                stroke-dasharray="{(GAUGE_C * analysis.overall_score) / 100} {GAUGE_C}"
              />
            </svg>
            <div class="absolute inset-0 flex flex-col items-center justify-center">
              <span class="text-4xl font-bold tabular-nums leading-none tracking-tight {toneText[tone]}">{analysis.overall_score}</span>
              <span class="text-xs font-medium text-muted-foreground">/ 100</span>
            </div>
          </div>
          <!-- Verdict copy -->
          <div class="flex flex-1 flex-col items-center gap-3 text-center sm:items-start sm:text-left">
            <span class="rounded-full border px-3 py-1 text-sm font-semibold uppercase tracking-wide {toneChip[tone]}">{analysis.verdict}</span>
            <p class="text-sm text-muted-foreground">
              Across <strong class="font-semibold text-foreground">{dimensions.length}</strong> dimensions,
              covering <strong class="font-semibold text-foreground">{covered}</strong> of
              <strong class="font-semibold text-foreground">{requirements.length}</strong> requirements an ATS screens for.
            </p>
            <Button variant="ghost" size="sm" onclick={start} disabled={streaming}>
              <RefreshCw class="size-3.5 {streaming ? 'animate-spin' : ''}" />{streaming ? 'Analysing…' : 'Recompute'}
            </Button>
          </div>
        </div>
      </section>
    {/if}

    {#if isStale && !streaming}
      <p class="fit-reveal rounded-lg border border-dashed border-amber-500/40 bg-amber-500/5 px-4 py-2.5 text-xs text-amber-700 dark:text-amber-500" style="--i:2">
        Your CV or this role changed since this ran — recompute for an up-to-date read.
      </p>
    {/if}

    <!-- Streaming: stage stepper + thinking -->
    {#if streaming || (!analysis && !stream.error)}
      <section class="rounded-2xl border border-border bg-card p-6 sm:px-8">
        <!-- Stage stepper: evenly-spaced nodes over a single connecting rail. -->
        <div class="relative flex">
          <div class="absolute inset-x-[16.67%] top-4 h-px bg-border" aria-hidden="true"></div>
          {#each stream.stages as st (st.n)}
            <div class="relative z-10 flex flex-1 flex-col items-center gap-2">
              <span
                class="flex size-8 shrink-0 items-center justify-center rounded-full border bg-card text-sm font-semibold transition-colors
                {st.state === 'done' ? 'border-emerald-500 bg-emerald-500 text-white' : st.state === 'active' ? 'border-primary text-primary' : 'border-border text-muted-foreground'}"
              >
                {#if st.state === 'done'}<Check class="size-4" />
                {:else if st.state === 'active'}<Loader class="size-4 animate-spin" />
                {:else}{st.n}{/if}
              </span>
              <span class="max-w-[7rem] text-center text-xs font-medium leading-tight sm:text-sm {st.state === 'pending' ? 'text-muted-foreground' : 'text-foreground'}">{st.label}</span>
            </div>
          {/each}
        </div>

        {#if stream.thinking}
          <div class="mt-6 border-t border-border pt-4">
            <button type="button" class="flex items-center gap-2 text-xs font-semibold uppercase tracking-wide text-muted-foreground" onclick={() => (showThinking = !showThinking)}>
              <span class="relative flex size-2">
                <span class="absolute inline-flex size-full animate-ping rounded-full bg-primary/60"></span>
                <span class="relative inline-flex size-2 rounded-full bg-primary"></span>
              </span>
              Thinking {showThinking ? '▾' : '▸'}
            </button>
            {#if showThinking}
              <p bind:this={thinkingEl} class="mt-2 max-h-48 overflow-y-auto whitespace-pre-wrap font-mono text-xs leading-relaxed text-muted-foreground">{stream.thinking}</p>
            {/if}
          </div>
        {/if}
      </section>
    {/if}

    {#if stream.error && !analysis}
      <div class="rounded-xl border border-destructive/30 bg-destructive/5 p-8 text-center text-sm text-destructive">
        <p>{stream.error}.</p>
        <button class="mt-2 font-semibold underline underline-offset-4" onclick={start}>Try again</button>
      </div>
    {/if}

    <!-- The report -->
    {#if analysis}
      <div class="grid gap-8 lg:grid-cols-[minmax(0,3fr)_minmax(0,2fr)]">
        <!-- Dimensions -->
        <section class="flex flex-col gap-5">
          <h2 class="text-[0.7rem] font-semibold uppercase tracking-[0.2em] text-muted-foreground">Dimensions</h2>
          <div class="flex flex-col gap-5">
            {#each dimensions as d, i (d.key)}
              {@const dt = verdictTone(d.score)}
              <div class="fit-reveal flex flex-col gap-2" style="--i:{i + 2}">
                <div class="flex items-baseline justify-between gap-3">
                  <span class="text-sm font-semibold">{d.label}</span>
                  <span class="text-lg font-bold tabular-nums leading-none {toneText[dt]}">{d.score}</span>
                </div>
                <div class="h-1.5 overflow-hidden rounded-full bg-secondary">
                  <div class="fit-meter h-full rounded-full {toneBar[dt]}" style="width: {d.score}%"></div>
                </div>
                {#if d.comment}<p class="text-sm leading-relaxed text-muted-foreground">{d.comment}</p>{/if}
              </div>
            {/each}
          </div>
        </section>

        <!-- ATS requirements -->
        <section class="flex flex-col gap-5">
          <h2 class="text-[0.7rem] font-semibold uppercase tracking-[0.2em] text-muted-foreground">Requirements · ATS view</h2>
          <ul class="flex flex-col divide-y divide-border overflow-hidden rounded-xl border border-border">
            {#each requirements as r, i (i)}
              {@const meta = requirementStatusMeta(r.status)}
              <li class="flex items-start justify-between gap-3 px-3.5 py-2.5">
                <span class="min-w-0 text-sm leading-snug">
                  <span class="mr-1 text-[0.65rem] font-semibold uppercase tracking-wide text-muted-foreground">{r.priority}</span>
                  {r.text}
                </span>
                <span class="mt-px shrink-0 rounded-full border px-2 py-0.5 text-[0.7rem] font-semibold {toneChip[meta.tone]}">{meta.label}</span>
              </li>
            {/each}
          </ul>
        </section>
      </div>

      {#if analysis.strengths.length || analysis.gaps.length}
        <div class="grid gap-8 border-t border-border pt-8 sm:grid-cols-2">
          {#if analysis.strengths.length}
            <section class="flex flex-col gap-3">
              <h2 class="text-[0.7rem] font-semibold uppercase tracking-[0.2em] text-emerald-600 dark:text-emerald-400">Strengths</h2>
              <ul class="flex flex-col gap-2.5">
                {#each analysis.strengths as s, i (i)}
                  <li class="flex gap-2.5 text-sm leading-relaxed">
                    <span class="mt-0.5 flex size-4 shrink-0 items-center justify-center rounded-full bg-emerald-500/15 text-emerald-600 dark:text-emerald-400">+</span>
                    {s}
                  </li>
                {/each}
              </ul>
            </section>
          {/if}
          {#if analysis.gaps.length}
            <section class="flex flex-col gap-3">
              <h2 class="text-[0.7rem] font-semibold uppercase tracking-[0.2em] text-destructive">Gaps</h2>
              <ul class="flex flex-col gap-2.5">
                {#each analysis.gaps as g, i (i)}
                  <li class="flex gap-2.5 text-sm leading-relaxed">
                    <span class="mt-0.5 flex size-4 shrink-0 items-center justify-center rounded-full bg-destructive/15 text-destructive">−</span>
                    {g}
                  </li>
                {/each}
              </ul>
            </section>
          {/if}
        </div>
      {/if}

      {#if analysis.recommendation}
        <section class="relative rounded-2xl border border-border bg-secondary/40 p-6 sm:p-8">
          <span class="text-[0.7rem] font-semibold uppercase tracking-[0.2em] text-muted-foreground">The verdict</span>
          <p class="mt-2 border-l-2 border-foreground/20 pl-4 text-lg font-medium leading-relaxed">{analysis.recommendation}</p>
        </section>
      {/if}
    {/if}
  {/if}
</div>

<style>
  /* Staggered entrance — one orchestrated reveal, not scattered micro-motion. */
  .fit-reveal {
    animation: fit-in 0.5s cubic-bezier(0.16, 1, 0.3, 1) backwards;
    animation-delay: calc(var(--i, 0) * 55ms);
  }
  @keyframes fit-in {
    from {
      opacity: 0;
      transform: translateY(8px);
    }
  }
  /* Meters and the gauge arc sweep in. */
  .fit-meter {
    animation: fit-grow 0.7s cubic-bezier(0.16, 1, 0.3, 1) backwards;
    animation-delay: 0.15s;
  }
  @keyframes fit-grow {
    from {
      width: 0 !important;
    }
  }
  .fit-arc {
    transition: stroke-dasharray 0.9s cubic-bezier(0.16, 1, 0.3, 1);
  }
  @media (prefers-reduced-motion: reduce) {
    .fit-reveal,
    .fit-meter,
    .fit-arc {
      animation: none;
      transition: none;
    }
  }
</style>
