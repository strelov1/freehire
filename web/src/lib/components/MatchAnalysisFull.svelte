<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { resolve } from '$app/paths';
  import { RefreshCw, FileText, Check, Loader, TriangleAlert } from '@lucide/svelte';
  import { refuses, resetsAtLabel } from '$lib/allowance';
  import { api } from '$lib/api';
  import { track } from '$lib/analytics';
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
  import { renderMarkdown } from '$lib/markdown';
  import { Button } from '$lib/ui';

  // The full AI match report + live SSE stream. Caller: `ArtifactPanel`'s Job Match tab, with
  // `autoRun` wired to the tailoring workspace's own `coldStartRunning` — a cold start opens
  // this component's stream immediately, in parallel with the autopilot's CV-edit run, so the
  // stage stepper actually animates instead of the compute happening invisibly elsewhere. A
  // concurrent invisible compute (autopilot's pre-run) can still win the race on occasion; see
  // internal/handler/match_analysis_coordinator.go for how that's kept from double-running the
  // chain, and its `followMatchAnalysis` for what this component renders when it does.
  // `initial` seeds from an SSR-cached analysis for an instant paint when the caller has one;
  // when absent, the cached analysis is fetched on mount. `autoRun` starts the stream on a
  // cold start — the "never silently recompute a cached analysis" gate. `stacked` forces
  // every multi-column section into a single column regardless of viewport width — for
  // callers narrower than the viewport-based `lg:`/`sm:` breakpoints assume (the tailoring
  // artifact panel).
  let {
    job,
    initial = null,
    autoRun = true,
    stacked = false,
  }: {
    job: Job;
    initial?: MatchAnalysisResponse | null;
    autoRun?: boolean;
    stacked?: boolean;
  } = $props();

  let fit = $state<MatchAnalysisResponse | null>(initial);

  function seedFrom(f: MatchAnalysisResponse | null): MatchStreamState {
    const s = initMatchStream();
    if (f && !f.has_cv) s.hasCV = false;
    if (f?.analysis) {
      s.analysis = f.analysis;
      s.requirements = f.analysis.requirement_match;
      s.hiddenSignals = f.analysis.hidden_signals;
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
  // A brand-new job can't be analysed once today's allowance would REFUSE one — the stream
  // would 402. A recompute of an already-cached analysis stays free, so this gates cold
  // starts only (never the Recompute button).
  //
  // `refuses`, not `isSpent`: through the shadow run the count reads as spent while the
  // stream still opens, and blocking on the count alone would put a wall in front of an
  // analysis the server was about to run — and keep it out of the numbers being measured.
  const allowance = $derived(fit?.allowance ?? null);
  const blockedNew = $derived(coldStart && refuses(allowance));
  const dimensions = $derived(analysis?.dimensions ?? []);
  const requirements = $derived(
    analysis?.requirement_match?.length ? analysis.requirement_match : stream.requirements,
  );
  const hiddenSignals = $derived(
    analysis?.hidden_signals?.length ? analysis.hidden_signals : stream.hiddenSignals,
  );
  // Coverage tally for the ATS-view header: covered folds in synonym-only matches (both are
  // positive, matching the hero's count), `addit` is the fixable near-miss, `gap` the genuine
  // miss. `synonym` is a sub-count of covered, surfaced as a legend note on the coverage meter.
  const reqTally = $derived.by(() => {
    const t = { covered: 0, synonym: 0, addit: 0, gap: 0 };
    for (const r of requirements) {
      if (r.status === 'covered') t.covered++;
      else if (r.status === 'synonym-only') {
        t.covered++;
        t.synonym++;
      } else if (r.status === 'missing-have') t.addit++;
      else if (r.status === 'missing-gap') t.gap++;
    }
    return t;
  });
  // Split the ledger so the misses stand out: `attentionReqs` are the fixable
  // near-misses + genuine gaps, pulled into a highlighted callout; `coveredReqs`
  // are everything already satisfied.
  const attentionReqs = $derived(
    requirements.filter((r) => r.status === 'missing-have' || r.status === 'missing-gap'),
  );
  const coveredReqs = $derived(
    requirements.filter((r) => r.status === 'covered' || r.status === 'synonym-only'),
  );
  const reqTotal = $derived(reqTally.covered + reqTally.addit + reqTally.gap);

  function start() {
    stop();
    // Tracked at the start, not on `final`: the run costs a credit the moment it
    // begins, and an analysis that stalls or times out is exactly the one worth
    // seeing in the funnel.
    track('match_run', { slug: job.public_slug });
    stream = initMatchStream();
    streaming = true;
    showThinking = true;
    const source = new EventSource(api.matchAnalysisStreamUrl(job.public_slug), { withCredentials: true });
    es = source;
    // NB: our server error event is `stream_error`, never `error` — `error` is
    // EventSource's own reserved connection-error event (handled by onerror below).
    for (const name of ['meta', 'stage_start', 'stage_done', 'thinking', 'requirements', 'dimensions', 'final', 'stream_error']) {
      source.addEventListener(name, (e) => {
        // A malformed/truncated frame must not throw out of the listener: an
        // uncaught error here would skip the stop() below, so a bad `final` frame
        // would hang the UI in `streaming` and leak the EventSource. Degrade to
        // stream_error — the same terminal path as a connection drop.
        let data: unknown;
        try {
          data = JSON.parse((e as MessageEvent).data);
        } catch {
          stream = reduceMatchEvent(stream, 'stream_error', { message: 'Analysis failed' });
          stop();
          return;
        }
        stream = reduceMatchEvent(stream, name, data);
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

  // Poll the cached analysis after a dropped stream: the server's background compute lands the
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
    // The card has no SSR analysis — fetch the cache, then seed from it (the stream was
    // seeded from a null `initial`, so there is nothing to preserve).
    if (!fit && isAuthenticated()) {
      try {
        fit = await api.getMatchAnalysis(job.public_slug);
        stream = seedFrom(fit);
      } catch {
        /* best-effort: an unconfigured/failing match-analysis endpoint leaves the empty state */
      }
    }
    // Read the cache fields directly rather than via $derived: onMount is not a reactive
    // context, and a stale derived read here would leave a cold Job Match tab never
    // opening its stream (stages stuck on "pending" forever).
    const hasAnalysis = !!fit?.analysis;
    const noAllowance = !hasAnalysis && refuses(fit?.allowance);
    if (
      isAuthenticated() &&
      !hasAnalysis &&
      (fit?.has_cv ?? true) &&
      !noAllowance &&
      autoRun
    ) {
      start();
    }
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
    moderate: 'text-warning-strong',
    weak: 'text-warning-strong',
    poor: 'text-destructive',
  };
  const toneBar: Record<Tone, string> = {
    strong: 'bg-brand',
    good: 'bg-brand',
    moderate: 'bg-warning',
    weak: 'bg-warning',
    poor: 'bg-destructive',
  };
  const toneStroke: Record<Tone, string> = {
    strong: 'stroke-brand',
    good: 'stroke-brand',
    moderate: 'stroke-warning',
    weak: 'stroke-warning',
    poor: 'stroke-destructive',
  };
  const toneChip: Record<Tone, string> = {
    strong: 'border-brand/30 bg-brand-muted text-brand-strong',
    good: 'border-brand/30 bg-brand-muted text-brand-strong',
    moderate: 'border-warning/30 bg-warning/10 text-warning-strong',
    weak: 'border-warning/30 bg-warning/10 text-warning-strong',
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
      Sign in to analyse your match for this role.
    </p>
  {:else if !stream.hasCV}
    <div class="flex flex-col items-center gap-4 rounded-xl border border-dashed border-border bg-card p-10 text-center">
      <FileText class="size-8 text-muted-foreground" />
      <p class="text-sm text-muted-foreground">Upload a CV to analyse your match for this role.</p>
      <Button variant="primary" size="sm" href={resolve('/my/profile')}>Upload CV</Button>
    </div>
  {:else}
    <!-- The recommendation card — defined once, placed near the top in the stacked panel (where
         the punchline should lead) and at the bottom on the full-width page. -->
    {#snippet verdictCard()}
      <section class="relative rounded-2xl border border-border bg-secondary/40 p-6 sm:p-8">
        <span class="{headingClass} text-muted-foreground">The verdict</span>
        <div
          class="verdict-prose mt-2 border-l-2 border-foreground/20 pl-4 font-medium leading-relaxed {stacked
            ? 'text-base'
            : 'text-lg'}"
        >
          <!-- eslint-disable-next-line svelte/no-at-html-tags -- DOMPurify-sanitized markdown -->
          {@html renderMarkdown(analysis?.recommendation ?? '')}
        </div>
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
      <p class="fit-reveal rounded-lg border border-dashed border-warning/40 bg-warning/5 px-4 py-2.5 text-xs text-warning-strong" style="--i:2">
        Your CV or this role changed since this ran — recompute for an up-to-date read.
      </p>
    {/if}

    <!-- Today's analyses are spent AND the ceiling is live: a fresh one can't run (a
         recompute of an already-analysed role stays available on those pages). While the
         ceiling is only being counted this stays hidden and the analysis runs. -->
    {#if blockedNew}
      <div class="fit-reveal flex flex-col items-center gap-2 rounded-xl border border-dashed border-border bg-card p-10 text-center" style="--i:1">
        <p class="text-sm font-medium">You've used today's job analyses.</p>
        <p class="text-xs text-muted-foreground">
          More at {resetsAtLabel(allowance)}. Analyses you've already run stay available.
        </p>
      </div>
    {/if}

    <!-- Streaming: stage stepper + thinking. An idle cold start (autoRun=false — a resumed
         CV, the standalone page) used to render the same pending stepper with nothing driving
         it, which looked like Extract & Match / Recruiter verdict were hung. Show an explicit
         Run when idle. -->
    {#if !blockedNew && (streaming || recovering)}
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
    {:else if !blockedNew && !analysis && !stream.error}
      <div class="flex flex-col items-center gap-3 rounded-xl border border-dashed border-border bg-card p-8 text-center">
        <p class="text-sm text-muted-foreground">
          Run the three-stage match analysis — Extract &amp; Match, Recruiter verdict, then Adversarial audit.
        </p>
        <Button variant="primary" size="sm" onclick={start} disabled={streaming || recovering}>
          <RefreshCw class="size-3.5" /> Run analysis
        </Button>
      </div>
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
        <section class="flex flex-col gap-5">
          <!-- Coverage meter: the plain text tally becomes a segmented bar, so the
               covered/gap split reads at a glance instead of as three numbers. -->
          <div class="flex flex-col gap-2.5">
            <h2 class="{headingClass} text-muted-foreground">Requirements · ATS view</h2>
            {#if reqTotal}
              <div class="flex items-baseline gap-2">
                <span class="text-xl font-bold tabular-nums leading-none"
                  >{reqTally.covered}<span class="font-medium text-muted-foreground">/{reqTotal}</span></span
                >
                <span class="text-sm text-muted-foreground">requirements covered</span>
              </div>
              <div class="flex h-2 overflow-hidden rounded-full bg-secondary">
                {#if reqTally.covered}<div class="fit-meter h-full bg-brand" style="width: {(reqTally.covered / reqTotal) * 100}%"></div>{/if}
                {#if reqTally.addit}<div class="fit-meter h-full bg-warning" style="width: {(reqTally.addit / reqTotal) * 100}%"></div>{/if}
                {#if reqTally.gap}<div class="fit-meter h-full bg-destructive" style="width: {(reqTally.gap / reqTotal) * 100}%"></div>{/if}
              </div>
              <div class="flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-muted-foreground">
                {#if reqTally.covered}<span class="flex items-center gap-1.5"><span class="size-2 rounded-full bg-brand"></span>{reqTally.covered} covered</span>{/if}
                {#if reqTally.addit}<span class="flex items-center gap-1.5"><span class="size-2 rounded-full bg-warning"></span>{reqTally.addit} to add</span>{/if}
                {#if reqTally.gap}<span class="flex items-center gap-1.5"><span class="size-2 rounded-full bg-destructive"></span>{reqTally.gap} gap</span>{/if}
                {#if reqTally.synonym}<span class="flex items-center gap-1.5"><span class="size-2 rounded-full border border-dotted border-brand-strong"></span>{reqTally.synonym} via synonym</span>{/if}
              </div>
            {/if}
          </div>

          <!-- Needs attention: the near-misses + genuine gaps pulled out and
               highlighted, so the few misses are seen at a glance instead of hunted
               for in the covered ledger. A tone-coloured rule + label carries the
               status, replacing the repeated pills. -->
          {#if attentionReqs.length}
            <div class="rounded-xl border border-destructive/25 bg-destructive/5 p-4">
              <h3 class="mb-2 text-[0.7rem] font-semibold uppercase tracking-wider text-destructive/90">
                Needs attention · {attentionReqs.length}
              </h3>
              <ul class={['grid gap-x-10 gap-y-0.5', !stacked && 'sm:grid-cols-2']}>
                {#each attentionReqs as r, i (i)}
                  {@const meta = requirementStatusMeta(r.status)}
                  <li class="flex items-center gap-2.5 py-1.5">
                    <span class="h-4 w-[3px] shrink-0 rounded-full {toneBar[meta.tone]}"></span>
                    <span class="min-w-0 flex-1 text-sm font-medium leading-snug text-foreground">{r.text}</span>
                    {#if r.priority && r.priority.toLowerCase() !== 'required'}
                      <span class="shrink-0 text-[0.6rem] font-medium lowercase tracking-wide text-muted-foreground/70">{r.priority}</span>
                    {/if}
                    <span class="shrink-0 text-xs font-semibold {toneText[meta.tone]}">{meta.label}</span>
                  </li>
                {/each}
              </ul>
            </div>
          {/if}

          <!-- Covered ledger: a quiet checklist — a tone glyph (✓ / ≈ for a synonym
               match) carries the status, so 20+ rows read as a scannable matrix
               instead of a wall of identical pills. `preferred` sits on the right. -->
          {#if coveredReqs.length}
            <div class="flex flex-col gap-2.5">
              <h3 class="text-[0.7rem] font-semibold uppercase tracking-wider text-muted-foreground">
                Covered · {reqTally.covered}
              </h3>
              <ul class={['grid gap-x-10', !stacked && 'sm:grid-cols-2 xl:grid-cols-3']}>
                {#each coveredReqs as r, i (i)}
                  {@const syn = r.status === 'synonym-only'}
                  {@const weak = r.evidence_strength === 'keyword'}
                  <li class="flex items-baseline gap-2.5 border-b border-border/60 py-2">
                    <span class="shrink-0 text-xs leading-snug {syn ? 'text-muted-foreground' : 'text-brand-strong'}">{syn ? '≈' : '✓'}</span>
                    <span class={['min-w-0 flex-1 text-sm leading-snug', syn && 'border-b border-dotted border-brand-strong/50']}>{r.text}</span>
                    {#if r.evidence_strength}
                      <span
                        class={['shrink-0 text-[0.6rem] font-medium lowercase tracking-wide', weak ? 'text-warning-strong' : 'text-muted-foreground/60']}
                        title={weak ? 'Only a bare mention in your CV — back it with a concrete result' : 'Backed by ' + r.evidence_strength + ' evidence in your CV'}
                      >{r.evidence_strength}</span>
                    {/if}
                    {#if r.priority && r.priority.toLowerCase() !== 'required'}
                      <span class="shrink-0 text-[0.6rem] font-medium lowercase tracking-wide text-muted-foreground/60">{r.priority}</span>
                    {/if}
                  </li>
                {/each}
              </ul>
            </div>
          {/if}
        </section>

        <!-- Hidden signals: unstated culture/pace/team-stage reads from the posting's own
             wording, quoted verbatim alongside the interpretation. Omitted entirely when the
             posting was too generic to read anything from. -->
        {#if hiddenSignals.length}
          <section class="flex flex-col gap-5">
            <h2 class="{headingClass} text-muted-foreground">Hidden signals</h2>
            <ul class="flex flex-col gap-3">
              {#each hiddenSignals as sig, i (i)}
                <li class="rounded-lg border border-border/60 bg-secondary/30 p-3.5">
                  <p class="text-sm italic leading-relaxed text-muted-foreground">"{sig.quote}"</p>
                  <p class="mt-1.5 text-sm font-medium leading-relaxed text-foreground">{sig.insight}</p>
                </li>
              {/each}
            </ul>
          </section>
        {/if}
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
  /* Multi-paragraph verdict: marked emits <p>s; keep breaks without collapsing. */
  .verdict-prose :global(p + p) {
    margin-top: 0.75em;
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
