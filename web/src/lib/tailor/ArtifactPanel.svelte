<script lang="ts">
  // The tailoring context panel: a resizable right-hand pane whose tabs are divided by what
  // each one MEASURES, so no tab mixes numbers taken against different baselines.
  //
  //  - Job Match — the live, deterministic score of the document being edited against this
  //    vacancy, with the cached LLM fit analysis beneath it, labelled as the snapshot of the
  //    base profile that it is.
  //  - Score — what tailoring did to the CV's ATS readability against the base CV, and the
  //    last autopilot run's log.
  //  - Job — the vacancy's own text.
  //
  // Every tab here MEASURES the document or is the thing it is measured against; what CHANGES
  // the document — its text, its template, its typography — lives in the left panel.
  //
  // JD and the fit analysis reuse the SAME components the job page / fit page use, so they
  // read identically. Splitter width is clamped by the vitest-covered clampWidth.
  import { resolve } from '$app/paths';
  import { ExternalLink, PanelRightClose, PanelRightOpen } from '@lucide/svelte';
  import { clampWidth } from './geometry';
  import JobDescription from '$lib/components/JobDescription.svelte';
  import MatchAnalysisFull from '$lib/components/MatchAnalysisFull.svelte';
  import CompanyLogo from '$lib/components/CompanyLogo.svelte';
  import RevisionHistory from './RevisionHistory.svelte';
  import AutopilotReport from './AutopilotReport.svelte';
  import AtsDelta from './AtsDelta.svelte';
  import JobMatch from './JobMatch.svelte';
  import type { Analysis, AutopilotEntry, RevisionView } from '$lib/generated/contracts';
  import type { Job, MatchAnalysisResponse } from '$lib/types';
  import type { CvAtsDelta, CvJobMatch } from '$lib/cv';

  type Tab = 'jd' | 'jobmatch' | 'score' | 'history';

  let {
    job,
    analysis,
    // Which tab is active — bindable so the page's mobile tab bar can drive it (on desktop the
    // panel's own tab bar sets it). mobileVisible is the page's per-breakpoint show/hide on mobile;
    // at lg the aside is always shown regardless (lg:flex overrides the mobile hidden).
    tab = $bindable('jobmatch'),
    mobileVisible = false,
    autopilotReport = undefined,
    autopilotBusy = false,
    atsDelta = null,
    jobMatch = null,
    onRerunAutopilot,
    revisions = [],
    onPreviewRevision,
    onUndoRevision,
    onUndoRevisionRun,
  }: {
    job: Job;
    analysis: Analysis | null;
    tab?: Tab;
    mobileVisible?: boolean;
    /** The last unattended run's log, shown in the Score tab. The fit analysis is untouched
     *  by a run — it measures the base profile, not this tailored copy. */
    autopilotReport?: AutopilotEntry[];
    autopilotBusy?: boolean;
    /** What tailoring did to the CV's ATS readiness. Null renders nothing — an unavailable
     *  delta is an absence, not an error state. */
    atsDelta?: CvAtsDelta | null;
    /** How well the current document matches this vacancy. Same absence rule as the delta. */
    jobMatch?: CvJobMatch | null;
    onRerunAutopilot: () => void;
    /** The CV's history, newest first. */
    revisions?: RevisionView[];
    /** Which entry's edits the preview should underline. The highlight belongs to the
     *  document, so it travels through to the page rather than living in this panel. */
    onPreviewRevision: (revision: RevisionView | null) => void;
    /** Undoing is the page's to run: it owns the debounced save that has to be flushed
     *  first, and the re-read that follows. */
    onUndoRevision: (revision: RevisionView) => Promise<void>;
    onUndoRevisionRun: (batchId: string) => Promise<void>;
  } = $props();

  const tabs: [Tab, string][] = [
    ['jobmatch', 'Job Match'],
    ['score', 'Score'],
    ['history', 'History'],
    ['jd', 'Job'],
  ];
  // 400 rather than the 340 floor: at the floor the tab strip wraps "Job Match" onto two lines
  // and the vacancy title truncates after a couple of words, so the panel opened looking cramped.
  let width = $state(400);
  let resizing = false;
  // Collapsed to a rail so the centre CV preview can take the width. Desktop-only: below lg
  // the columns already show one at a time, and collapsing there would hide a view with no
  // way back to it.
  let collapsed = $state(false);

  // Seed MatchAnalysisFull from the already-cached analysis so it paints read-only (no recompute burn).
  const fit = $derived<MatchAnalysisResponse>({ has_cv: true, stale: false, analysis });

  function startResize(e: PointerEvent) {
    resizing = true;
    (e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
  }
  function doResize(e: PointerEvent) {
    // The panel hugs the right edge, so its width is the distance from the cursor to that edge.
    if (resizing) width = clampWidth(window.innerWidth - e.clientX, 340, 720);
  }
  function stopResize(e: PointerEvent) {
    resizing = false;
    (e.currentTarget as HTMLElement).releasePointerCapture(e.pointerId);
  }
</script>

{#if collapsed}
  <!-- The collapsed rail. Desktop-only, and it carries the one control that brings the panel
       back — a collapse with no visible way out is a lost panel. -->
  <div
    class="hidden shrink-0 flex-col items-center gap-2 border-l border-border bg-background px-1.5 py-2 lg:flex"
  >
    <button
      type="button"
      onclick={() => (collapsed = false)}
      aria-label="Expand the context panel"
      class="rounded p-1 text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
    >
      <PanelRightOpen class="size-4" />
    </button>
  </div>
{:else}
  <!-- Splitter: drag left/right to resize the panel. -->
  <div
    class="hidden w-1.5 shrink-0 cursor-col-resize bg-border/50 transition-colors hover:bg-border lg:block"
    role="separator"
    aria-orientation="vertical"
    aria-label="Resize panel"
    onpointerdown={startResize}
    onpointermove={doResize}
    onpointerup={stopResize}
  ></div>
{/if}

<aside
  class={[
    'w-full min-h-0 flex-1 flex-col border-l border-border bg-background lg:w-[var(--w)] lg:flex-none',
    mobileVisible ? 'flex' : 'hidden',
    collapsed ? 'lg:hidden' : 'lg:flex',
  ]}
  style="--w: {width}px"
>
  <!-- Own tab bar is desktop-only; on mobile the page's flat tab bar drives the tab. The
       collapse control sits at its end, beside the edge the panel folds towards. -->
  <div class="hidden items-center gap-1 border-b border-border px-2 py-1.5 text-sm lg:flex">
    {#each tabs as [id, label] (id)}
      <button
        type="button"
        onclick={() => (tab = id)}
        class={[
          'rounded px-2 py-1 transition-colors',
          tab === id
            ? 'bg-brand-muted font-semibold text-brand-strong'
            : 'text-muted-foreground hover:text-foreground',
        ]}
      >
        {label}
      </button>
    {/each}
    <button
      type="button"
      onclick={() => (collapsed = true)}
      aria-label="Collapse the context panel"
      class="ml-auto rounded p-1 text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
    >
      <PanelRightClose class="size-4" />
    </button>
  </div>

  <!-- The vacancy header sits above the tabs rather than inside the Job tab: the link out to
       the posting is worth reaching from every tab, and a candidate reading a score should not
       have to change tabs to see what they are being scored against. -->
  <div class="flex items-start gap-2.5 border-b border-border px-3 py-2.5">
    <CompanyLogo name={job.company} size="size-8" />
    <div class="min-w-0 flex-1">
      <h2 class="truncate text-sm font-semibold leading-snug text-foreground" title={job.title}>
        {job.title}
      </h2>
      <p class="truncate text-xs text-muted-foreground">{job.company}</p>
    </div>
    {#if job.public_slug}
      <a
        href={resolve('/jobs/[slug]', { slug: job.public_slug })}
        target="_blank"
        rel="noopener"
        class="inline-flex shrink-0 items-center gap-1 rounded-md border border-border px-2 py-1 text-xs font-medium text-foreground transition-colors hover:bg-muted"
      >
        View Job <ExternalLink class="size-3" aria-hidden="true" />
      </a>
    {/if}
  </div>

  <div class="min-h-0 flex-1 overflow-auto">
    {#if tab === 'history'}
      <RevisionHistory {revisions} onPreview={onPreviewRevision} onUndo={onUndoRevision} onUndoRun={onUndoRevisionRun} />
    {:else if tab === 'jd'}
      <div class="p-4">
        {#if job.description}
          <JobDescription html={job.description} />
        {:else}
          <p class="text-sm text-muted-foreground">No job description.</p>
        {/if}
      </div>
    {:else if tab === 'score'}
      <div class="p-4">
        <!-- Readability, and the run that changed it. The job-anchored score lives in its own
             tab: the two answer different questions against different baselines, and stacking
             them under one heading is what made the previous Verdict tab unreadable. -->
        <AtsDelta data={atsDelta} />
        <AutopilotReport report={autopilotReport} busy={autopilotBusy} onRerun={onRerunAutopilot} />
      </div>
    {:else}
      <div class="p-4">
        <JobMatch data={jobMatch} />
        <!-- The fit analysis measures the candidate's BASE profile, not the document being
             edited, and says so. An unlabelled fit score beside a live one teaches the
             candidate that tailoring does not move the number — true of that score, false of
             this surface as a whole. -->
        <div class="mb-3 rounded-lg border border-border bg-muted/30 px-2.5 py-2">
          <h3 class="text-sm font-semibold text-foreground">Fit analysis</h3>
          <p class="mt-0.5 text-[11px] leading-snug text-muted-foreground">
            A snapshot of your base profile against this vacancy. It does not move as you edit
            this CV — recompute it below to take it again.
          </p>
        </div>
        <MatchAnalysisFull {job} initial={fit} autoRun={false} stacked />
      </div>
    {/if}
  </div>
</aside>
