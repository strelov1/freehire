<script lang="ts">
  // The tailoring context panel: a resizable right-hand pane whose tabs are divided by what
  // each one MEASURES, so no tab mixes numbers taken against different baselines.
  //
  //  - Job Match — the live, deterministic score of the document being edited against this
  //    vacancy, with the cached LLM match analysis beneath it — refreshed by every autopilot
  //    run, not a frozen snapshot.
  //  - Score — what tailoring did to the CV's ATS readability against the base CV, and the
  //    last autopilot run's log.
  //  - Job — the vacancy's own text.
  //
  // Every tab here MEASURES the document or is the thing it is measured against; what CHANGES
  // the document — its text, its template, its typography — lives in the left panel.
  //
  // JD and the match analysis reuse the SAME components the job page / match page use, so
  // they read identically. Splitter width is clamped by the vitest-covered clampWidth.
  import { resolve } from '$app/paths';
  import { ExternalLink, PanelRightClose, PanelRightOpen } from '@lucide/svelte';
  import { clampWidth, keyResizeWidth, PANEL_MAX_WIDTH, PANEL_MIN_WIDTH } from './geometry';
  import { tablist } from '#lib/actions/tablist.js';
  import JobDescription from '#lib/components/JobDescription.svelte';
  import MatchAnalysisFull from '#lib/components/MatchAnalysisFull.svelte';
  import { companyLogoUrl } from '#lib/logo.js';
  import { EntityLogo } from '#lib/ui/index.js';
  import RevisionHistory from './RevisionHistory.svelte';
  import AutopilotReport from './AutopilotReport.svelte';
  import AtsDelta from './AtsDelta.svelte';
  import JobMatch from './JobMatch.svelte';
  import { scoreTone, type JobMatchTone } from './jobmatch';
  import CoverLetter from './CoverLetter.svelte';
  import type { Analysis, AutopilotEntry, RevisionView } from '#lib/generated/contracts.js';
  import type { Job, MatchAnalysisResponse } from '#lib/types.js';
  import type { CvAtsDelta, CvJobMatch } from '#lib/cv.js';

  type Tab = 'jd' | 'jobmatch' | 'score' | 'letter' | 'history';

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
    autoRunAnalysis = false,
    atsDelta = null,
    jobMatch = null,
    onRerunAutopilot,
    revisions = [],
    onPreviewRevision,
    onUndoRevision,
    onUndoRevisionRun,
    onReseed,
    cvId,
    resetBusy = false,
    resetError = '',
  }: {
    job: Job;
    analysis: Analysis | null;
    tab?: Tab;
    mobileVisible?: boolean;
    /** The last unattended run's log, shown in the Score tab. The match analysis (Job Match
     *  tab) is refreshed by every run — see prepareAutopilotRun on the server. */
    autopilotReport?: AutopilotEntry[];
    autopilotBusy?: boolean;
    /** True only for the tailoring workspace's own cold start — see `+page.svelte`'s
     *  `coldStartRunning`. Opens the match analysis's own visible SSE stream (the stage
     *  stepper) at the same moment the autopilot's CV-edit run starts, instead of leaving
     *  it to autopilot's invisible pre-run compute (see match_analysis_coordinator.go for
     *  how the two are kept from both running the chain). */
    autoRunAnalysis?: boolean;
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
    /** Rebuild this tailored CV from the current seed — the experience bank first, the
     *  résumé's own extract only for what the bank doesn't track. Page owns confirm + flush. */
    onReseed?: () => Promise<void>;
    /** True while a reset round-trip (or a turn) is in flight — disables the control. */
    resetBusy?: boolean;
    /** Last reset failure message; cleared by the page on a new attempt. */
    resetError?: string;
    /** The CV whose vacancy the Letter tab writes for. */
    cvId: string;
  } = $props();

  const tabs: [Tab, string][] = [
    ['jobmatch', 'Job Match'],
    ['score', 'ATS'],
    ['letter', 'Letter'],
    ['history', 'History'],
    ['jd', 'Job'],
  ];
  // 400 rather than the 340 floor: at the floor the tab strip wraps "Job Match" onto two lines
  // and the vacancy title truncates after a couple of words, so the panel opened looking cramped.
  // Each measuring tab carries its current number, so the score is readable from whichever tab
  // is open and the three measurements stop competing for one heading. Absent when the score
  // is — a badge reading "–" would be a state to ignore, which is what the panel avoids.
  const badges = $derived<Partial<Record<Tab, number>>>({
    jobmatch: jobMatch?.available ? jobMatch.score?.overall : undefined,
    score: atsDelta?.available ? atsDelta.delta?.tailored : undefined,
  });
  // The same thresholds the Job match card colours its own number by, so a badge and the card
  // it summarises never disagree about whether a score is good.
  const badgeTone: Record<JobMatchTone, string> = {
    strong: 'text-brand-strong',
    moderate: 'text-warning-strong',
    poor: 'text-destructive',
  };
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
    if (resizing) width = clampWidth(window.innerWidth - e.clientX, PANEL_MIN_WIDTH, PANEL_MAX_WIDTH);
  }
  // Also the pointercancel handler: a touch the browser takes back (a scroll, a system gesture)
  // must end the drag, or the next unrelated move would keep resizing. A cancelled pointer has
  // already lost its capture, hence the check.
  function stopResize(e: PointerEvent) {
    resizing = false;
    const el = e.currentTarget as HTMLElement;
    if (el.hasPointerCapture(e.pointerId)) el.releasePointerCapture(e.pointerId);
  }
  // The panel sits on the right, so moving the splitter left (ArrowLeft) widens it.
  function keyResize(e: KeyboardEvent) {
    const next = keyResizeWidth(width, e.key, 'ArrowLeft');
    if (next === null) return;
    e.preventDefault();
    width = next;
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
  <!-- Splitter: drag left/right to resize the panel, or focus it and use the arrow keys. -->
  <!-- A focusable separator is the ARIA window-splitter widget; the lint reads every separator as static. -->
  <!-- svelte-ignore a11y_no_noninteractive_tabindex, a11y_no_noninteractive_element_interactions -->
  <div
    class="hidden w-1.5 shrink-0 cursor-col-resize touch-none bg-border/50 transition-colors hover:bg-border focus-visible:bg-ring focus-visible:outline-none lg:block"
    role="separator"
    tabindex="0"
    aria-orientation="vertical"
    aria-label="Resize panel"
    aria-valuenow={width}
    aria-valuemin={PANEL_MIN_WIDTH}
    aria-valuemax={PANEL_MAX_WIDTH}
    onpointerdown={startResize}
    onpointermove={doResize}
    onpointerup={stopResize}
    onpointercancel={stopResize}
    onkeydown={keyResize}
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
    <div role="tablist" aria-label="Measure this CV" use:tablist={tab} class="flex min-w-0 items-center gap-1">
    {#each tabs as [id, label] (id)}
      <button
        type="button"
        role="tab"
        id="tailor-right-tab-{id}"
        aria-selected={tab === id}
        aria-controls="tailor-right-panel"
        onclick={() => (tab = id)}
        class={[
          'whitespace-nowrap rounded px-2 py-1 transition-colors',
          tab === id
            ? 'bg-brand-muted font-semibold text-brand-strong'
            : 'text-muted-foreground hover:text-foreground',
        ]}
      >
        {label}
        {#if badges[id] !== undefined}
          {@const value = badges[id] ?? 0}
          <span class="ml-1 text-xs font-medium tabular-nums {badgeTone[scoreTone(value)]}">{value}</span>
          <span class="sr-only">out of 100</span>
        {/if}
      </button>
    {/each}
    </div>
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
    <EntityLogo
      name={job.company || 'Unknown company'}
      src={companyLogoUrl(job.company) ?? undefined}
      shape="square"
      size="sm"
    />
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

  <div id="tailor-right-panel" role="tabpanel" aria-labelledby="tailor-right-tab-{tab}" class="min-h-0 flex-1 overflow-auto">
    {#if tab === 'history'}
      <div>
        {#if onReseed}
          <div class="border-b border-border px-4 py-3">
            <p class="text-xs leading-snug text-muted-foreground">
              Replace this CV’s content from your current seed (experience bank and résumé,
              and refresh your base CV too). Template and typography stay; this is not “undo
              last agent edit” alone — History undo covers that. Edits are undoable from the
              history below.
            </p>
            <button
              type="button"
              disabled={resetBusy || autopilotBusy}
              onclick={() => void onReseed()}
              class="mt-2 inline-flex items-center rounded-md border border-border bg-background px-2.5 py-1 text-xs font-medium text-foreground transition-colors hover:bg-muted disabled:cursor-not-allowed disabled:opacity-50"
            >
              {resetBusy ? 'Resetting…' : 'Reset changes'}
            </button>
            {#if resetError}
              <p class="mt-1.5 text-xs text-destructive">{resetError}</p>
            {/if}
          </div>
        {/if}
        <RevisionHistory {revisions} onPreview={onPreviewRevision} onUndo={onUndoRevision} onUndoRun={onUndoRevisionRun} />
      </div>
    {:else if tab === 'letter'}
      <!-- An artefact, not a measurement: this tab carries no score and no delta, because there
           is no baseline a letter could be read against. -->
      <CoverLetter {cvId} />
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
        <!-- The match analysis used to be a frozen base-profile snapshot; it is now refreshed
             by every autopilot run, so it says that instead of implying it never moves. A
             heading rather than a card: the analysis below already draws its own surface. -->
        <div class="mb-2 mt-6 flex items-baseline justify-between gap-2">
          <h3 class="text-sm font-semibold text-foreground">Match analysis</h3>
          <p class="text-xs text-muted-foreground">Refreshed after every autopilot run</p>
        </div>
        <!-- Keyed on whether an analysis is present: MatchAnalysisFull seeds its internal state
             from `initial` once, on mount, and does not re-read it if the prop changes later —
             so once the visible stream (or, on the rare race it loses, autopilot's invisible
             pre-run) lands an analysis after the tab first rendered empty, this forces a clean
             remount to pick it up rather than staying stuck showing the pending stepper
             forever. -->
        {#key !!analysis}
          <MatchAnalysisFull {job} initial={fit} autoRun={autoRunAnalysis} stacked />
        {/key}
      </div>
    {/if}
  </div>
</aside>
