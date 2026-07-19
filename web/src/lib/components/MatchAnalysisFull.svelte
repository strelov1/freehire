<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { resolve } from '$app/paths';
  import { RefreshCw, FileText, Check, Loader, TriangleAlert } from '@lucide/svelte';
  import { api } from '$lib/api';
  import { isAuthenticated } from '$lib/auth.svelte';
  import {
    verdictTone,
    requirementStatusMeta,
    initMatchStream,
    reduceMatchEvent,
    type MatchStreamState,
    type Tone,
  } from '$lib/matchAnalysis';
  import type { Job, MatchAnalysisResponse } from '$lib/types';
  import { Button } from '$lib/ui';

  // The full AI fit report + live SSE stream, shared by the /match page and the
  // tracking card's Job Match tab. `initial` seeds from an SSR-cached fit (the page
  // passes its load data for an instant paint); when absent (the card, which has no
  // SSR), the cached fit is fetched on mount. `autoRun` starts the stream on a cold
  // start — the same "never silently recompute a cached analysis" gate as the page.
  // `stacked` forces every multi-column section into a single column regardless of viewport
  // width — for the narrow tailoring artifact panel, where the viewport-based `lg:`/`sm:`
  // breakpoints would otherwise cram two columns into ~480px.
  let {
    job,
    initial = null,
    autoRun = true,
    stacked = false,
  }: { job: Job; initial?: MatchAnalysisResponse | null; autoRun?: boolean; stacked?: boolean } = $props();

  let fit = $state<MatchAnalysisResponse | null>(initial);

  function seedFrom(f: MatchAnalysisResponse | null): MatchStreamState {
    const s = initMatchStream();
    if (f && !f.has_cv) s.hasCV = false;
    if (f?.analysis) {
      s.analysis = f.analysis;
      s.requirements = f.analysis.requirement_match;
      s.done = true;
      s.stages = s.stages.map((x) => ({ ...x, state: 'done' as const }));
    }
    return s;
  }

  // Seeded synchronously from `initial` so an SSR-cached page paints instantly; the
  // card re-seeds in onMount after its client-side fetch resolves.
  let stream = $state<MatchStreamState>(seedFrom(initial));
  let streaming = $state(false);
  let showThinking = $state(false);
  // While true, the stream dropped mid-compute and we're polling the cache for the result
  // the server finishes on its own (see the EventSource error handler and recoverFromDrop).
  let recovering = $state(false);
  let destroyed = false;
  let es: EventSource | null = null;
  // The thinking panel tails the model's reasoning: keep it pinned to the newest tokens.
  let thinkingEl = $state<HTMLElement | null>(null);
  $effect(() => {
    void stream.thinking; // track new reasoning
    if (thinkingEl) thinkingEl.scrollTop = thinkingEl.scrollHeight;
  });

  const analysis = $derived(stream.analysis);
  const isStale = $derived(fit?.stale ?? false);
  // Only auto-run when there is NO cached analysis at all (cold). A stale cache still
  // paints instantly and offers a manual Recompute — so a refresh never silently burns
  // three LLM calls, and a fresh cache never recomputes.
  const coldStart = $derived(!fit?.analysis);
  // A brand-new job can't be analysed once the monthly AI credits are spent — the stream
  // would 402. A recompute of an already-cached analysis stays free, so this gates cold
  // starts only (never the Recompute button).
  const credits = $derived(fit?.credits ?? null);
  const blockedNew = $derived(coldStart && !!credits && credits.remaining <= 0);
  const dimensions = $derived(analysis?.dimensions ?? []);
  const requirements = $derived(
    analysis?.requirement_match?.length ? analysis.requirement_match : stream.requirements,
  );
  // Coverage tally for the ATS-view header: covered folds in synonym-only matches (both are
  // positive, matching the hero's count), `addit` is the fixable near-miss, `gap` the genuine miss.
  const reqTally = $derived.by(() => {
    const t = { covered: 0, addit: 0, gap: 0 };
    for (const r of requirements) {
      if (r.status === 'covered' || r.status === 'synonym-only') t.covered++;
      else if (r.status === 'missing-have') t.addit++;
      else if (r.status === 'missing-gap') t.gap++;
    }
    return t;
  });

  function start() {
    stop();
    stream = initMatchStream();
    streaming = true;
    showThinking = true;
    const source = new EventSource(api.matchAnalysisStreamUrl(job.public_slug), { withCredentials: true });
    es = source;
    // NB: our server error event is `stream_error`, never `error` — `error` is
    // EventSource's own reserved connection-error event (handled by onerror below).
    for (const name of ['meta', 'stage_start', 'stage_done', 'thinking', 'requirements', 'dimensions', 'final', 'stream_error']) {
      source.addEventListener(name, (e) => {
        stream = reduceMatchEvent(stream, name, JSON.parse((e as MessageEvent).data));
        if (name === 'final' || name === 'stream_error') stop();
      });
    }
    // A mid-stream drop is common on mobile: a backgrounded tab gets frozen, or a Wi-Fi↔cellular
    // handoff resets the socket. The server keeps computing on a background context and caches
    // the result regardless of the client, so recover by polling the cache rather than dead-ending
    // or re-running three paid LLM stages. Only a drop after the compute actually began is
    // recoverable; a rejected connection (no stage ever started) is a genuine failure.
    // NB: the reserved `error` event — distinct from our server's `stream_error` above.
    source.addEventListener('error', () => {
      stop();
      if (stream.done) return;
      if (stream.stages.some((s) => s.state !== 'pending')) void recoverFromDrop();
      else stream = reduceMatchEvent(stream, 'stream_error', { message: 'Connection lost' });
    });
  }

  function stop() {
    streaming = false;
    recovering = false;
    es?.close();
    es = null;
  }

  const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms));

  // Poll the cached fit after a dropped stream: the server's background compute lands the
  // analysis in the cache even with no client attached, so a re-read recovers it without a
  // fresh (charged) recompute. Attempts are spent only while the tab is visible, so a long
  // background freeze on mobile doesn't exhaust the budget before the user returns; stop()
  // (Try again / Recompute / unmount) clears `recovering` to cancel an in-flight loop.
  async function recoverFromDrop() {
    recovering = true;
    let delay = 3_000;
    for (let attempts = 0; attempts < 30; ) {
      await sleep(delay);
      if (destroyed || !recovering) return;
      if (typeof document !== 'undefined' && document.visibilityState === 'hidden') continue;
      attempts++;
      try {
        const f = await api.getMatchAnalysis(job.public_slug);
        if (f?.analysis) {
          fit = f;
          stream = seedFrom(f);
          recovering = false;
          return;
        }
      } catch {
        /* transient — keep polling until the attempt budget is spent */
      }
      delay = Math.min(Math.round(delay * 1.5), 15_000);
    }
    recovering = false;
    stream = reduceMatchEvent(stream, 'stream_error', { message: 'Connection lost' });
  }

  onMount(async () => {
    // The card has no SSR fit — fetch the cache, then seed from it (the stream was
    // seeded from a null `initial`, so there is nothing to preserve).
    if (!fit && isAuthenticated()) {
      try {
        fit = await api.getMatchAnalysis(job.public_slug);
        stream = seedFrom(fit);
      } catch {
        /* best-effort: an unconfigured/failing fit endpoint leaves the empty state */
      }
    }
    if (isAuthenticated() && coldStart && (fit?.has_cv ?? true) && !blockedNew && autoRun) start();
  });
  onDestroy(() => {
    destroyed = true;
    stop();
  });

  // Radial gauge geometry for the overall score.
  const GAUGE_R = 54;
  const GAUGE_C = 2 * Math.PI * GAUGE_R;

  const toneText: Record<Tone, string> = {
    strong: 'text-brand-strong',
    good: 'text-brand-strong',
    moderate: 'text-amber-600 dark:text-amber-500',
    weak: 'text-amber-600 dark:text-amber-500',
    poor: 'text-destructive',
  };
  const toneBar: Record<Tone, string> = {
    strong: 'bg-brand',
    good: 'bg-brand',
    moderate: 'bg-amber-500',
    weak: 'bg-amber-500',
    poor: 'bg-destructive',
  };
  const toneStroke: Record<Tone, string> = {
    strong: 'stroke-brand',
    good: 'stroke-brand',
    moderate: 'stroke-amber-500',
    weak: 'stroke-amber-500',
    poor: 'stroke-destructive',
  };
  const toneChip: Record<Tone, string> = {
    strong: 'border-brand/30 bg-brand-muted text-brand-strong',
    good: 'border-brand/30 bg-brand-muted text-brand-strong',
    moderate: 'border-amber-500/30 bg-amber-500/10 text-amber-700 dark:text-amber-500',
    weak: 'border-amber-500/30 bg-amber-500/10 text-amber-700 dark:text-amber-500',
    poor: 'border-destructive/30 bg-destructive/10 text-destructive',
  };
  const toneGlow: Record<Tone, string> = {
    strong: 'rgba(120,140,21,0.16)',
    good: 'rgba(120,140,21,0.16)',
    moderate: 'rgba(245,158,11,0.10)',
    weak: 'rgba(245,158,11,0.10)',
    poor: 'rgba(239,68,68,0.10)',
  };

  // Section headings read as tiny wide-tracked uppercase labels on the full-width /match page;
  // in the narrow stacked panel that looks cramped, so use a plain readable weight there.
  const headingClass = $derived(
    stacked ? 'text-sm font-semibold' : 'text-[0.7rem] font-semibold uppercase tracking-[0.2em]',
  );
</script>

<div class="flex flex-col gap-8">
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
    <!-- The recommendation card — defined once, placed near the top in the stacked panel (where
         the punchline should lead) and at the bottom on the full-width page. -->
    {#snippet verdictCard()}
      <section class="relative rounded-2xl border border-border bg-secondary/40 p-6 sm:p-8">
        <span class="{headingClass} text-muted-foreground">The verdict</span>
        <p class="mt-2 border-l-2 border-foreground/20 pl-4 text-lg font-medium leading-relaxed">{analysis?.recommendation}</p>
      </section>
    {/snippet}

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

    <!-- Stacked panel: lead with the verdict. -->
    {#if stacked && analysis?.recommendation}
      {@render verdictCard()}
    {/if}

    {#if isStale && !streaming}
      <p class="fit-reveal rounded-lg border border-dashed border-amber-500/40 bg-amber-500/5 px-4 py-2.5 text-xs text-amber-700 dark:text-amber-500" style="--i:2">
        Your CV or this role changed since this ran — recompute for an up-to-date read.
      </p>
    {/if}

    <!-- Monthly AI credits spent: a fresh analysis can't run (recompute of an
         already-analysed role stays available on those pages). -->
    {#if blockedNew}
      <div class="fit-reveal flex flex-col items-center gap-2 rounded-xl border border-dashed border-border bg-card p-10 text-center" style="--i:1">
        <p class="text-sm font-medium">You're out of AI credits for this month.</p>
        <p class="text-xs text-muted-foreground">
          Your credits renew {credits ? new Date(credits.resets_at).toLocaleDateString(undefined, { month: 'short', day: 'numeric' }) : 'next month'}. Analyses you've already run stay available.
        </p>
      </div>
    {/if}

    <!-- Streaming: stage stepper + thinking -->
    {#if !blockedNew && (streaming || (!analysis && !stream.error))}
      <section class="rounded-2xl border border-border bg-card p-6 sm:px-8">
        <!-- Stage stepper: evenly-spaced nodes over a single connecting rail. -->
        <div class="relative flex">
          <div class="absolute inset-x-[16.67%] top-4 h-px bg-border" aria-hidden="true"></div>
          {#each stream.stages as st (st.n)}
            <div class="relative z-10 flex flex-1 flex-col items-center gap-2">
              <span
                class="flex size-8 shrink-0 items-center justify-center rounded-full border text-sm font-semibold transition-colors
                {st.state === 'done' ? 'border-brand bg-brand text-brand-foreground' : st.state === 'active' ? 'border-brand bg-card text-primary' : 'border-border bg-card text-muted-foreground'}"
              >
                {#if st.state === 'done'}<Check class="size-4" />
                {:else if st.state === 'active'}<Loader class="size-4 animate-spin" />
                {:else}{st.n}{/if}
              </span>
              <span class="max-w-[7rem] text-center text-xs font-medium leading-tight sm:text-sm {st.state === 'pending' ? 'text-muted-foreground' : 'text-foreground'}">{st.label}</span>
            </div>
          {/each}
        </div>

        {#if recovering}
          <p class="mt-6 flex items-center justify-center gap-2 border-t border-border pt-4 text-xs text-muted-foreground">
            <Loader class="size-3.5 animate-spin" aria-hidden="true" /> Connection dropped — waiting for your result…
          </p>
        {/if}

        {#if stream.thinking}
          <div class="mt-6 border-t border-border pt-4">
            <button type="button" class="flex items-center gap-2 text-xs font-semibold uppercase tracking-wide text-muted-foreground" onclick={() => (showThinking = !showThinking)}>
              <span class="relative flex size-2">
                <span class="absolute inline-flex size-full animate-ping rounded-full bg-brand/60"></span>
                <span class="relative inline-flex size-2 rounded-full bg-brand"></span>
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
      {@const msg = stream.error.charAt(0).toUpperCase() + stream.error.slice(1)}
      <div class="flex flex-col items-center gap-3 rounded-xl border border-destructive/20 bg-destructive/5 p-8 text-center">
        <span class="flex size-10 items-center justify-center rounded-full bg-destructive/10 text-destructive">
          <TriangleAlert class="size-5" aria-hidden="true" />
        </span>
        <p class="text-sm font-medium text-foreground">{msg}.</p>
        <button
          class="inline-flex items-center gap-1.5 rounded-lg bg-brand px-4 py-2 text-sm font-semibold text-brand-foreground transition-opacity hover:opacity-90"
          onclick={start}
        >
          <RefreshCw class="size-4" aria-hidden="true" /> Try again
        </button>
      </div>
    {/if}

    <!-- The report — full-width stacked sections, each flowing into its own multi-column
         grid, so the short Dimensions breakdown and the long ATS checklist stay balanced
         instead of one column towering over an empty other. -->
    {#if analysis}
      <div class="flex flex-col gap-10">
        <!-- Dimensions: 6 scored rows read as a 2-column breakdown rather than a tall list. -->
        <section class="flex flex-col gap-5">
          <h2 class="{headingClass} text-muted-foreground">Dimensions</h2>
          <div class={['grid gap-x-10 gap-y-6', !stacked && 'sm:grid-cols-2']}>
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

        <!-- ATS requirements: a light multi-column ledger with a coverage tally, so 29 terse
             rows read as a scannable matrix instead of a boxed-in tower. -->
        <section class="flex flex-col gap-4">
          <div class="flex flex-wrap items-baseline justify-between gap-x-4 gap-y-1.5">
            <h2 class="{headingClass} text-muted-foreground">Requirements · ATS view</h2>
            <div class="flex items-center gap-3 text-xs font-medium tabular-nums">
              {#if reqTally.covered}<span class="text-brand-strong">{reqTally.covered} covered</span>{/if}
              {#if reqTally.addit}<span class="text-amber-600 dark:text-amber-500">{reqTally.addit} to add</span>{/if}
              {#if reqTally.gap}<span class="text-destructive">{reqTally.gap} gap</span>{/if}
            </div>
          </div>
          <ul class={['grid gap-x-10', !stacked && 'sm:grid-cols-2 xl:grid-cols-3']}>
            {#each requirements as r, i (i)}
              {@const meta = requirementStatusMeta(r.status)}
              {@const showPriority = !!r.priority && r.priority.toLowerCase() !== 'required'}
              <li class="flex items-start justify-between gap-3 border-b border-border/60 py-2">
                <span class="min-w-0 text-sm leading-snug">
                  {#if showPriority}<span class="mr-1 text-[0.65rem] font-semibold uppercase tracking-wide text-muted-foreground">{r.priority}</span>{/if}
                  {r.text}
                </span>
                <span class="mt-px shrink-0 rounded-full border px-2 py-0.5 text-[0.7rem] font-semibold {toneChip[meta.tone]}">{meta.label}</span>
              </li>
            {/each}
          </ul>
        </section>
      </div>

      {#if analysis.strengths.length || analysis.gaps.length}
        <div class={['grid gap-8 border-t border-border pt-8', !stacked && 'sm:grid-cols-2']}>
          {#if analysis.strengths.length}
            <section class="flex flex-col gap-3">
              <h2 class="{headingClass} text-brand-strong">Strengths</h2>
              <ul class="flex flex-col gap-2.5">
                {#each analysis.strengths as s, i (i)}
                  <li class="flex gap-2.5 text-sm leading-relaxed">
                    <span class="mt-0.5 flex size-4 shrink-0 items-center justify-center rounded-full bg-brand-muted text-brand-strong">+</span>
                    {s}
                  </li>
                {/each}
              </ul>
            </section>
          {/if}
          {#if analysis.gaps.length}
            <section class="flex flex-col gap-3">
              <h2 class="{headingClass} text-destructive">Gaps</h2>
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

      {#if !stacked && analysis.recommendation}
        {@render verdictCard()}
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
