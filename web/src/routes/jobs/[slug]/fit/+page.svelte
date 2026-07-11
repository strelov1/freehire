<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { resolve } from '$app/paths';
  import { ArrowLeft, Sparkles, RefreshCw, FileText, Check, Loader, ChevronDown } from '@lucide/svelte';
  import { api } from '$lib/api';
  import { isAuthenticated } from '$lib/auth.svelte';
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
  // A fresh cached analysis already covers the page; anything else needs a live compute.
  const needsCompute = $derived(!data.fit?.analysis || isStale);
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
      if (!stream.done) stream = reduceFitEvent(stream, 'error', { message: 'Connection lost' });
      stop();
    };
  }

  function stop() {
    streaming = false;
    es?.close();
    es = null;
  }

  onMount(() => {
    if (isAuthenticated() && needsCompute && (data.fit?.has_cv ?? true)) start();
  });
  onDestroy(stop);

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
  const toneChip: Record<Tone, string> = {
    strong: 'border-emerald-500/30 bg-emerald-500/10 text-emerald-700 dark:text-emerald-400',
    good: 'border-emerald-500/30 bg-emerald-500/10 text-emerald-700 dark:text-emerald-400',
    moderate: 'border-amber-500/30 bg-amber-500/10 text-amber-700 dark:text-amber-500',
    weak: 'border-amber-500/30 bg-amber-500/10 text-amber-700 dark:text-amber-500',
    poor: 'border-destructive/30 bg-destructive/10 text-destructive',
  };

</script>

<svelte:head><title>Fit analysis · {data.job.title}</title></svelte:head>

<div class="mx-auto flex w-full max-w-5xl flex-col gap-6 px-4 py-6">
  <a href={resolve('/jobs/[slug]', { slug: data.job.public_slug })} class="flex w-fit items-center gap-1.5 text-sm text-muted-foreground hover:text-foreground">
    <ArrowLeft class="size-4" />Back to job
  </a>

  <div class="flex flex-col gap-1">
    <span class="flex items-center gap-1.5 text-xs font-semibold uppercase tracking-wide text-muted-foreground">
      <Sparkles class="size-3.5" />AI fit analysis
    </span>
    <h1 class="text-2xl font-bold leading-tight">{data.job.title}</h1>
    <p class="text-sm text-muted-foreground">{data.job.company}</p>
  </div>

  {#if !isAuthenticated()}
    <p class="rounded-lg border border-border bg-card p-6 text-sm text-muted-foreground">Sign in to analyse your fit for this role.</p>
  {:else if !stream.hasCV}
    <div class="flex flex-col items-start gap-3 rounded-lg border border-border bg-card p-6">
      <span class="flex items-center gap-1.5 text-sm text-muted-foreground"><FileText class="size-4" />Upload a CV to analyse this role.</span>
      <Button variant="primary" size="sm" href={resolve('/my/profile')}>Upload CV</Button>
    </div>
  {:else}
    <!-- Hero: overall + verdict -->
    {#if analysis}
      {@const tone = verdictTone(analysis.overall_score)}
      <div class="flex flex-wrap items-center justify-between gap-4 rounded-xl border border-border bg-card p-6">
        <div class="flex items-baseline gap-3">
          <span class="text-5xl font-bold tabular-nums leading-none {toneText[tone]}">{analysis.overall_score}%</span>
          <span class="rounded-full border px-3 py-1 text-sm font-semibold {toneChip[tone]}">{analysis.verdict}</span>
        </div>
        <Button variant="ghost" size="sm" onclick={start} disabled={streaming}>
          <RefreshCw class="size-4 {streaming ? 'animate-spin' : ''}" />{streaming ? 'Analysing…' : 'Recompute'}
        </Button>
      </div>
    {/if}

    {#if isStale && !streaming}
      <p class="rounded border border-dashed border-amber-500/40 bg-amber-500/5 px-3 py-2 text-xs text-amber-700 dark:text-amber-500">
        Your CV or this job changed since this ran — recompute for an up-to-date read.
      </p>
    {/if}

    <!-- Stage stepper (while streaming) -->
    {#if streaming || (!analysis && !stream.error)}
      <div class="flex flex-col gap-3 rounded-xl border border-border bg-card p-5">
        <div class="flex flex-wrap items-center gap-x-6 gap-y-2">
          {#each stream.stages as st (st.n)}
            <span class="flex items-center gap-2 text-sm {st.state === 'done' ? 'text-foreground' : st.state === 'active' ? 'text-primary' : 'text-muted-foreground'}">
              {#if st.state === 'done'}<Check class="size-4 text-emerald-500" />
              {:else if st.state === 'active'}<Loader class="size-4 animate-spin" />
              {:else}<span class="size-4 rounded-full border border-current opacity-40"></span>{/if}
              {st.n}. {st.label}
            </span>
          {/each}
        </div>

        {#if stream.thinking}
          <div class="flex flex-col gap-1 border-t border-border pt-3">
            <button type="button" class="flex w-fit items-center gap-1 text-xs font-medium text-muted-foreground" onclick={() => (showThinking = !showThinking)}>
              <ChevronDown class="size-3.5 transition-transform {showThinking ? 'rotate-180' : ''}" />Thinking
            </button>
            {#if showThinking}
              <p class="max-h-40 overflow-y-auto whitespace-pre-wrap font-mono text-xs leading-relaxed text-muted-foreground">{stream.thinking}</p>
            {/if}
          </div>
        {/if}
      </div>
    {/if}

    {#if stream.error && !analysis}
      <p class="rounded-lg border border-destructive/30 bg-destructive/5 p-6 text-sm text-destructive">
        {stream.error}. <button class="underline" onclick={start}>Try again</button>
      </p>
    {/if}

    <!-- Detail: dimensions + ATS requirements -->
    {#if analysis}
      <div class="grid gap-6 lg:grid-cols-[minmax(0,3fr)_minmax(0,2fr)]">
        <section class="flex flex-col gap-4">
          <h2 class="text-sm font-semibold uppercase tracking-wide text-muted-foreground">Dimensions</h2>
          {#each dimensions as d (d.key)}
            {@const dt = verdictTone(d.score)}
            <div class="flex flex-col gap-1.5">
              <div class="flex items-baseline justify-between gap-2">
                <span class="text-sm font-medium">{d.label}</span>
                <span class="tabular-nums text-sm font-semibold {toneText[dt]}">{d.score}</span>
              </div>
              <div class="flex h-2 overflow-hidden rounded bg-secondary">
                <div class="h-full {toneBar[dt]} transition-all" style="width: {d.score}%"></div>
              </div>
              {#if d.comment}<p class="text-sm text-muted-foreground">{d.comment}</p>{/if}
            </div>
          {/each}
        </section>

        <section class="flex flex-col gap-3">
          <h2 class="text-sm font-semibold uppercase tracking-wide text-muted-foreground">Requirements (ATS view)</h2>
          <ul class="flex flex-col divide-y divide-border rounded-lg border border-border">
            {#each requirements as r, i (i)}
              {@const meta = requirementStatusMeta(r.status)}
              <li class="flex items-start justify-between gap-3 p-2.5 text-sm">
                <span class="min-w-0"><span class="text-muted-foreground">[{r.priority}]</span> {r.text}</span>
                <span class="mt-0.5 shrink-0 rounded-full border px-2 py-0.5 text-xs font-medium {toneChip[meta.tone]}">{meta.label}</span>
              </li>
            {/each}
          </ul>
        </section>
      </div>

      <div class="grid gap-6 sm:grid-cols-2">
        {#if analysis.strengths.length}
          <section class="flex flex-col gap-2">
            <h2 class="text-sm font-semibold uppercase tracking-wide text-muted-foreground">Strengths</h2>
            <ul class="flex flex-col gap-1 text-sm">
              {#each analysis.strengths as s, i (i)}<li class="flex gap-2"><span class="text-emerald-500">+</span>{s}</li>{/each}
            </ul>
          </section>
        {/if}
        {#if analysis.gaps.length}
          <section class="flex flex-col gap-2">
            <h2 class="text-sm font-semibold uppercase tracking-wide text-muted-foreground">Gaps</h2>
            <ul class="flex flex-col gap-1 text-sm">
              {#each analysis.gaps as g, i (i)}<li class="flex gap-2"><span class="text-destructive">−</span>{g}</li>{/each}
            </ul>
          </section>
        {/if}
      </div>

      {#if analysis.recommendation}
        <section class="rounded-xl border border-border bg-card p-5">
          <h2 class="mb-1 text-sm font-semibold uppercase tracking-wide text-muted-foreground">Recommendation</h2>
          <p class="text-sm">{analysis.recommendation}</p>
        </section>
      {/if}
    {/if}
  {/if}
</div>
