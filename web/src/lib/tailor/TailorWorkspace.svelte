<script lang="ts">
  // The dedicated CV workspace: a three-column surface — left panel tabbed between the
  // Chat, the structured Editor, the Experience bank and Design, a centre live HTML CV preview
  // (zoom, page count, Download PDF), and a right context panel that measures the CV against the
  // vacancy (Job Match, Score, Cover letter, History, Job). Two modes:
  //  - bootstrap (no ?cv): create the tailored CV + a seeded agent session and store the
  //    session id on the CV. Nothing is sent: the empty chat offers two ways to start.
  //  - resume (?cv=<id>): reuse the existing CV + its stored session — re-attach, NO kickoff.
  //
  // The page owns the CV document in memory so the Editor and the centre preview share one object:
  // typing re-renders the preview instantly, autosave persists in the background, and an agent turn
  // refetches and replaces the document.
  import { onMount, onDestroy, tick } from 'svelte';
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import CollapsedPanelRail from './CollapsedPanelRail.svelte';
  import DesignPanel from './DesignPanel.svelte';
  import EditorPanel from './EditorPanel.svelte';
  import { announce } from '#lib/announce.svelte.js';
  import { api, ApiError } from '#lib/api.js';
  import { offerCvRefresh, TAILOR_REFRESH_MESSAGE } from '#lib/cvRefreshOffer.js';
  import { askCvRefresh } from '#lib/cvRefreshDialog.svelte.js';
  import { track } from '#lib/analytics.js';
  import AssistantChat from '#lib/assistant/AssistantChat.svelte';
  import ArtifactPanel from '#lib/tailor/ArtifactPanel.svelte';
  import CliEditDialog from '#lib/components/cv/CliEditDialog.svelte';
  import CvHtmlPreview from '#lib/tailor/CvHtmlPreview.svelte';
  import ExperiencePanel from './ExperiencePanel.svelte';
  import AccountNavRail from '#lib/components/AccountNavRail.svelte';
  import { isLeftView, leftTabs, mobileTabsFor, type LeftTab, type MobileView } from './workspaceTabs';
  import { PreviewZoom } from './previewZoom.svelte';
  import GapsDownloadDialog from './GapsDownloadDialog.svelte';
  import ResetCvDialog from './ResetCvDialog.svelte';
  import { revealGap } from './revealGap';
  import { Headshot } from './headshot.svelte';
  import { readAtsDelta, readCvFonts, readJobMatch } from './workspaceReads';
  import { CvRevisions } from './cvRevisions.svelte';
  import LeftPanelBar from './LeftPanelBar.svelte';
  import { workspaceOpenError } from './workspaceOpenError';
  import { openWorkspace } from './openWorkspace';
  import { PdfExport } from './pdfExport.svelte';
  import { TailorOutcomes } from './outcome';
  import PanelSplitter from './PanelSplitter.svelte';
  import PreviewToolbar from './PreviewToolbar.svelte';
  import TailorMobileTabs from './TailorMobileTabs.svelte';
  import { undoRun, openingActions, baseOpeningActions } from '#lib/tailor/autopilot.js';
  import { findPlaceholders } from '#lib/tailor/readiness.js';
  import { fillViewport } from '#lib/actions/fillViewport.js';
  import {
    toEditable,
    emptyDocument,
    type CvRecord,
    type CvAtsDelta,
    type CvFont,
    type CvJobMatch,
    type TailorResult,
  } from '#lib/cv.js';
  import type { Analysis, AutopilotEntry, Document } from '#lib/generated/contracts.js';
  import type { Job } from '#lib/types.js';

  // Two modes, one component. With `slug` it is the tailoring workspace for a vacancy. With
  // `baseCvId` it is the BASE mode: the candidate's general CV, with no vacancy — so none of the
  // vacancy panes (job, match analysis, ATS delta against the base, cover letter, the autopilot
  // run) — while the preview, the section editor, template/style/margins, the experience bank,
  // the revision history with undo and the agent chat stay. The agent there runs under the `base`
  // preset: the same CV tools and evidence gate, a prompt that edits the general CV.
  let { slug = '', baseCvId = undefined }: { slug?: string; baseCvId?: string } = $props();
  const baseMode = $derived(baseCvId !== undefined);

  const cvParam = $derived(page.url.searchParams.get('cv'));
  // The base CV chosen before arriving (confirm dialog / intake / a base card). The tailor
  // request is made on load and again on reload, so the choice rides in the URL.
  const baseParam = $derived(page.url.searchParams.get('base') ?? undefined);

  let status = $state<'loading' | 'ready' | 'error'>('loading');
  let errorMsg = $state('');
  let sessionId = $state<string | undefined>(undefined);
  let resuming = $state(false);
  let cvId = $state('');
  // True exactly on a bootstrap that just created the tailored CV — the empty chat runs the
  // autopilot pass itself instead of offering the two-action menu. Never true when resuming
  // an existing CV (?cv=<id>).
  let coldStartRunning = $state(false);
  // The CV's own consent flag, read with the document and written by its own endpoint — it is
  // not part of the document, so autosave neither carries nor overwrites it.
  let tracerLinksEnabled = $state(false);
  let analysis = $state<Analysis | null>(null);
  let job = $state<Job | null>(null);

  // Page-owned CV state: the single client source of truth the Editor binds and the preview reads.
  let doc = $state<Document>(emptyDocument());
  let title = $state('');
  let templateId = $state('classic-ats');
  // The typefaces a CV may pick, fetched once. The preview needs each entry's CSS stack and the
  // Settings picker needs its label, so both read this one list rather than hard-coding it.
  let fonts = $state<CvFont[]>([]);
  let cvLoaded = $state(false);
  // Autosave lifecycle mirrors the old standalone editor: 'idle' before the first change, then
  // saving → saved (or error).
  let saveState = $state<'idle' | 'saving' | 'saved' | 'error'>('idle');
  let saveError = $state('');
  // Bumped on every persisted change (autosave, agent turn, template switch) to cache-bust the PDF.
  let pdfVersion = $state(0);

  // The last unattended run: its per-requirement log, whether it can still be undone, and
  // whether one is in flight. A run holds the editor closed — it rewrites the same document
  // the Editor tab holds in memory and saves on a debounce, so both writing at once loses one
  // side's work silently.
  let autopilotReport = $state<AutopilotEntry[] | undefined>(undefined);
  // What tailoring did to the CV's ATS readiness, refreshed at the two moments the document
  // has just changed most: the workspace opening, and an agent turn finishing. Null until the
  // first read, and after any failure — the panel renders it as an absence either way, so a
  // score nobody can compute never becomes an error the candidate has to dismiss.
  let atsDelta = $state<CvAtsDelta | null>(null);
  // How well the current document matches this vacancy. Unlike the delta this is one render
  // rather than two and calls no model, so it is refreshed on every persisted edit as well —
  // it is the one number in the workspace that is supposed to move while the candidate types.
  let jobMatch = $state<CvJobMatch | null>(null);
  let runActive = $state(false);
  // Any turn, not just a run: "Run again" would silently do nothing while one is in flight.
  let turnActive = $state(false);
  // The chat owns starting a turn; "Run again" beside the report reaches it through here.
  let chatRef = $state<AssistantChat>();

  // Left panel: which tab is shown, and its resizable width. The chat stays mounted across tab
  // switches (hidden, not unmounted) so its live session is never dropped.
  let leftTab = $state<LeftTab>('chat');
  let leftWidth = $state(350);
  // Folded to a rail so the centre CV preview can take the width. Desktop-only: below lg the
  // columns already show one at a time, and collapsing there would hide a view with no way back.
  let leftCollapsed = $state(false);
  let cliDialogOpen = $state(false);
  let leftPanelEl = $state<HTMLElement>();

  // The right context panel's tab, lifted here so the mobile tab bar can drive it (on desktop the
  // panel's own tab bar sets it via the same binding).
  let artifactTab = $state<'jd' | 'jobmatch' | 'score' | 'letter' | 'history'>(
    baseCvId !== undefined ? 'history' : 'jobmatch',
  );

  // Mobile-only navigation: one flat tab bar below lg (see mobileTabs.ts).
  const mobileTabs = $derived(mobileTabsFor(baseMode));
  // Defaults to the same tab the desktop right panel opens on (artifactTab above), so a
  // cold start's match-analysis animation is the first thing a mobile visitor sees too,
  // not hidden behind Chat.
  let mobileView = $state<MobileView>(baseCvId !== undefined ? 'chat' : 'jobmatch');

  // Below lg the account icon rail collapses into a drawer opened by the burger in the mobile tab
  // bar; AccountNavRail owns the drawer and binds this open flag.
  let navOpen = $state(false);
  function pickMobile(v: MobileView) {
    mobileView = v;
    if (isLeftView(v)) leftTab = v;
    else if (v !== 'preview') artifactTab = v;
  }

  // Centre preview zoom (see PreviewZoom).
  const preview = new PreviewZoom();
  const pdfUrl = $derived(`${api.cvPdfUrl(cvId)}?v=${pdfVersion}`);

  // Readiness of the PDF, the one thing here the seeker actually sends. The page count comes
  // from the preview's own pagination; a template slot the agent left behind (`[result]`)
  // would print verbatim, so the download stops to say so rather than letting it go out.
  let pageCount = $state<number | null>(null);
  const placeholders = $derived(findPlaceholders(doc));
  let placeholdersShown = $state(false);
  const showingPlaceholders = $derived(placeholdersShown && placeholders.length > 0);
  let previewScroller = $state<HTMLElement | null>(null);

  // Each press of the gap chip goes to the next slot, in reading order, and wraps — so a CV
  // with three gaps is fixed by pressing it three times, not by hunting for underlines.
  let gapCursor = 0;

  // Underlines every slot on the sheet and puts the cursor on one of them in the Editor, with
  // the bracketed text selected, so typing replaces it. The field is found by the same address
  // the check produced (`experience[1].bullets[0]`), which the Editor tags each input with.
  async function goToGap(index = gapCursor) {
    if (placeholders.length === 0) return;
    const gap = placeholders[index % placeholders.length];
    if (!gap) return;
    gapCursor = (index % placeholders.length) + 1;
    placeholdersShown = true;
    leftCollapsed = false;
    pickMobile('editor');
    await tick();
    revealGap(gap, previewScroller, leftPanelEl, `Gap ${gapCursor} of ${placeholders.length}: ${gap.token}`);
  }

  const pdf = new PdfExport({
    url: () => pdfUrl,
    flush: flushForExport,
    onExported: () => outcomes.pdfExported(),
    hasGaps: () => placeholders.length > 0,
  });

  // The member's headshot, for the templates that print one (see Headshot).
  const headshot = new Headshot();

  // Offered whenever the conversation is empty — the chat renders them only then. A CV opened
  // by id can carry a conversation nobody has spoken to, and that case looked like lost history.
  const opening = $derived(baseMode ? baseOpeningActions() : openingActions());
  const sessionLabel = $derived(job ? `${job.title} · ${job.company}` : baseMode ? title || undefined : undefined);

  // Hydrate the page-owned CV state from a CV record (marking the snapshot as the persisted
  // baseline so the autosave effect doesn't fire on load).
  function hydrate(rec: CvRecord) {
    title = rec.title;
    templateId = rec.template_id;
    doc = toEditable(rec.document);
    autopilotReport = rec.autopilot_report;
    tracerLinksEnabled = rec.tracer_links_enabled;
    lastSnapshot = snapshot();
    cvLoaded = true;
  }
  const loadCv = async () => hydrate(await api.getCv(cvId));

  // First-result analytics: a completed turn that changed the saved CV; a PDF that arrived.
  const outcomes = new TailorOutcomes({
    cvId: () => cvId,
    slug: () => (baseMode ? undefined : slug),
    document: () => doc,
    emit: track,
  });

  // The two scores, re-read after every change that moves them (see workspaceReads.ts).
  async function refreshAtsDelta() {
    if (baseMode) return; // a base has no baseline to be compared against
    atsDelta = await readAtsDelta(cvId);
  }

  async function refreshJobMatch() {
    if (baseMode) return; // no vacancy to match against
    jobMatch = await readJobMatch(cvId);
  }

  // Shared tail of every path that reaches a usable workspace (resume or bootstrap): flips to
  // ready and kicks off the accessory reads that don't block first paint.
  function finishReady() {
    status = 'ready';
    // Not awaited: the workspace is usable before the comparison lands, and the comparison
    // costs two renders. The history is the same kind of accessory read.
    void refreshAtsDelta();
    void refreshJobMatch();
    void history.load();
    void readCvFonts().then((f) => (fonts = f));
  }

  // Applies a tailoring bootstrap's result (fresh or retried) to the page state and settles
  // the CV into the address bar.
  async function applyTailorResult(tailor: TailorResult) {
    cvId = tailor.tailor_cv_id;
    analysis = tailor.analysis;
    sessionId = tailor.session_id;
    coldStartRunning = tailor.cold_start_running;
    await loadCv(); // bootstrap has no CV record in hand yet — fetch the tailored copy
    // Put the CV in the address, replacing this entry rather than adding one. A reload of
    // the bare /tailor/<slug> is a bootstrap request, and until the address names the CV
    // the candidate is one F5 away from an empty workspace. Back still leaves the page.
    // eslint-disable-next-line svelte/no-navigation-without-resolve -- resolve() supplies the path; the rule can't see through the appended ?cv= query
    void goto(`${resolve('/tailor/[slug]', { slug })}?cv=${cvId}`, {
      replaceState: true,
      reset: false,
    });
  }

  onMount(async () => {
    try {
      const o = await openWorkspace({ slug, baseCvId, cvParam, baseParam });
      resuming = o.resuming;
      job = o.job ?? null;
      if (o.tailor) await applyTailorResult(o.tailor);
      else if (o.cv) {
        cvId = o.cvId;
        analysis = o.analysis ?? null;
        sessionId = o.sessionId;
        hydrate(o.cv);
      }
      finishReady();
    } catch (e) {
      errorMsg = workspaceOpenError(e, baseMode);
      status = 'error';
    }
  });

  // ---- Revision history ---- (see CvRevisions)
  const history = new CvRevisions(
    () => cvId,
    () => flushPendingSave(),
    () => afterUndo(),
  );

  async function afterUndo() {
    await loadCv();
    pdfVersion += 1;
    void history.load();
    void refreshAtsDelta();
    void refreshJobMatch();
  }

  let resetBusy = $state(false);
  let resetError = $state('');
  // The one condition that forbids replacing the whole document: another reset already in
  // flight, or any agent turn. All three write the same document, so a second writer loses
  // one side's work silently. Every entry point to a reset reads this, so none can drift.
  const resetLocked = $derived(resetBusy || turnActive || runActive);

  async function applyReseed() {
    resetBusy = true;
    resetError = '';
    try {
      await undoRun({
        flush: flushPendingSave,
        undo: async () => {
          const rec = await api.reseedCv(cvId);
          hydrate(rec);
        },
        refetch: async () => {
          pdfVersion += 1;
          void history.load();
          void refreshAtsDelta();
          void refreshJobMatch();
        },
      });
      announce('Changes reset');
    } catch (e) {
      resetError = e instanceof ApiError ? e.message : 'Could not reset changes.';
      // Shown in two places (the History tab and the Experience tab), spoken once here.
      announce(resetError, 'assertive');
    } finally {
      resetBusy = false;
    }
  }

  let confirmResetOpen = $state(false);

  async function reseed() {
    confirmResetOpen = true;
  }

  // A bank edit offers the same whole-document reset the History tab's control offers, so it
  // answers to the same guard. While that guard is up the offer is skipped rather than shown:
  // asking and then doing nothing reads as a broken control, and the next bank edit asks again.
  function offerRefreshAfterBankEdit() {
    if (resetLocked) return;
    void offerCvRefresh({
      message: TAILOR_REFRESH_MESSAGE,
      apply: applyReseed,
      confirm: askCvRefresh,
    });
  }

  // ---- Autosave (folded in from the old standalone CvEditor) ----
  // A JSON snapshot of the last-persisted state; the effect compares against it to detect real
  // edits (and skip the initial load), and persist() advances it on success.
  let lastSnapshot = '';
  const snapshot = () => JSON.stringify({ title, templateId, doc });

  // The save in flight, so a flush (agent turn, PDF export) can wait for it: the timer is already
  // cleared by then, and the server would still hold the old document.
  let inflight: Promise<void> | null = null;
  function persist() {
    const p: Promise<void> = write().finally(() => {
      if (inflight === p) inflight = null;
    });
    inflight = p;
    return p;
  }

  async function write() {
    const snap = snapshot(); // capture NOW; edits during the round-trip re-trigger the effect
    saveState = 'saving';
    try {
      await api.updateCv(cvId, { title, template_id: templateId, document: doc });
      lastSnapshot = snap;
      saveState = 'saved';
      pdfVersion += 1;
      void history.load();
      // Chained off the SAVE, not off the effect that schedules it: read any earlier and the
      // score would describe the document as it was before this edit landed.
      void refreshJobMatch();
    } catch (e) {
      saveState = 'error';
      saveError = e instanceof ApiError ? e.message : 'Could not save. Please try again.';
    }
  }

  // Debounced autosave: any edit schedules a save 800ms later, resetting the timer on each
  // keystroke. There are no Save buttons — the CV persists on its own.
  let saveTimer: ReturnType<typeof setTimeout> | null = null;
  $effect(() => {
    if (!cvLoaded) return;
    if (snapshot() === lastSnapshot) return; // subscribes to title/templateId/doc
    if (saveTimer) clearTimeout(saveTimer);
    saveTimer = setTimeout(() => {
      saveTimer = null;
      void persist();
    }, 800);
  });

  onDestroy(() => {
    if (saveTimer) clearTimeout(saveTimer);
    if (cvLoaded && snapshot() !== lastSnapshot) {
      void api.updateCv(cvId, { title, template_id: templateId, document: doc }).catch(() => {});
    }
  });

  // Refreshes just the document, live, as a run's cv_edit calls resolve — so the preview fills
  // in while the run is still going instead of waiting for it to finish. Deliberately lighter
  // than onTurnComplete: the accessory reads (ATS delta, job match, history.list) stay end-of-turn
  // only, or a 30-step run would fire ten of each.
  async function onDocumentEdited() {
    outcomes.documentEdited();
    try {
      await loadCv();
      pdfVersion += 1;
    } catch {
      /* best-effort; onTurnComplete reconciles at the end regardless */
    }
  }

  // After an agent turn the CV may have changed server-side: flush any pending human edit, then
  // refetch and replace the shared document so the Editor and preview reflect it.
  async function onTurnComplete(turn: { completed: boolean }) {
    // Before any await: the chat starts a queued turn right after this returns its promise.
    const ended = outcomes.endTurn();
    await flushPendingSave();
    try {
      await loadCv();
      // Judged on the document just re-read from the server.
      outcomes.turnEnded(ended, turn.completed);
      pdfVersion += 1;
      void history.load(true);
      void refreshAtsDelta();
      void refreshJobMatch();
      // A cold start's own visible match-analysis stream (in ArtifactPanel) normally lands the
      // analysis well before this fires — this is only a fallback for the rare case that
      // stream lost the lead race and is showing a synthesized burst read straight from the
      // cache; that cache is exactly what this re-fetches.
      if (!analysis && !baseMode) {
        analysis = (await api.getMatchAnalysis(slug).catch(() => null))?.analysis ?? null;
      }
    } catch {
      outcomes.turnEnded(ended, false); // saved document unconfirmed
      /* best-effort refresh; the next edit or reload will reconcile */
    }
  }

  // Write a pending human edit NOW rather than 800ms from now. Anything that changes the CV
  // server-side has to do this first, or the debounce fires afterwards and overwrites it.
  async function flushPendingSave() {
    if (inflight) await inflight;
    if (!saveTimer) return;
    clearTimeout(saveTimer);
    saveTimer = null;
    if (snapshot() !== lastSnapshot) await persist();
  }

  // The PDF is rendered from the saved CV, so a save that failed must stop the export rather
  // than let it download the old one. Ordinary autosave failures keep showing in the save state.
  async function flushForExport() {
    await flushPendingSave();
    if (saveState === 'error') throw new Error(saveError);
  }

  // A template switch is persisted by the gallery via setCvTemplate; mirror the new id into the
  // page's own state so the next autosave (which also writes template_id) doesn't revert it.
  function onTemplateSelected(id: string) {
    templateId = id;
    lastSnapshot = snapshot();
    pdfVersion += 1;
  }
</script>

<svelte:head
  ><title
    >{baseMode ? `Edit CV${title ? ` · ${title}` : ''}` : `Tailor CV${job ? ` · ${job.title}` : ''}`} — freehire</title
  ></svelte:head
>

<!-- Full-width workspace loses the account shell nav; the same left-edge icon rail as
     the Agent page brings the account sections back. It stays put across every state. -->
<div use:fillViewport class="flex h-[calc(100dvh-3.5rem)]">
  <AccountNavRail collapsible bind:open={navOpen} />
  {#if status === 'loading'}
    <div class="flex min-w-0 flex-1 items-center justify-center text-sm text-muted-foreground">
      {baseMode
        ? 'Opening your CV…'
        : resuming
          ? 'Re-opening your tailoring session…'
          : 'Preparing your tailoring session…'}
    </div>
  {:else if status === 'error'}
    <div class="flex min-w-0 flex-1 flex-col items-center justify-center gap-3 p-6 text-center">
      <p class="max-w-md text-sm text-destructive">{errorMsg}</p>
      {#if baseMode}
        <a href={resolve('/my/cvs')} class="text-sm text-brand hover:underline">Back to your CVs</a>
      {:else}
        <a href={resolve('/jobs/[slug]', { slug })} class="text-sm text-brand hover:underline">Back to the role</a>
      {/if}
    </div>
  {:else}
    <div class="flex min-w-0 flex-1 flex-col lg:flex-row">
      <TailorMobileTabs tabs={mobileTabs} current={mobileView} bind:navOpen onPick={pickMobile} />

      <!-- LEFT: Editor / Chat tabs (chat stays mounted across tab switches). On mobile it shows only
           when its tab is picked; at lg it's a splitter-sized column always shown. The width rides a
           CSS var so the inline style never overrides the mobile w-full. -->
      {#if leftCollapsed}
        <CollapsedPanelRail onExpand={() => (leftCollapsed = false)} />
      {/if}
      <section
        bind:this={leftPanelEl}
        class={[
          'w-full min-h-0 flex-1 flex-col border-r border-border bg-background lg:w-[var(--lw)] lg:flex-none',
          isLeftView(mobileView) ? 'flex' : 'hidden',
          leftCollapsed ? 'lg:hidden' : 'lg:flex',
        ]}
        style="--lw: {leftWidth}px"
      >
        <!-- Own tab bar (and save status) is desktop-only; the mobile bar drives the tab there. -->
        <LeftPanelBar
          tabs={leftTabs}
          bind:current={leftTab}
          {saveState}
          {saveError}
          oncollapse={() => (leftCollapsed = true)}
        />
        <div id="tailor-left-panel" role="tabpanel" aria-labelledby="tailor-left-tab-{leftTab}" class="min-h-0 flex-1">
          <div class="h-full overflow-auto p-4" class:hidden={leftTab !== 'editor'}>
            <EditorPanel bind:doc bind:title {runActive} onOpenCli={() => (cliDialogOpen = true)} />
          </div>
          <!-- The experience bank as its owner sees it on /my/profile — same component, so
               checking, confirming, or editing what the assistant knows never means leaving the
               workspace. -->
          <div class="h-full overflow-auto p-4" class:hidden={leftTab !== 'experience'}>
            <ExperiencePanel {resetError} onBankMutated={offerRefreshAfterBankEdit} />
          </div>
          <div class="h-full overflow-auto p-4" class:hidden={leftTab !== 'design'}>
            <DesignPanel {cvId} bind:doc {fonts} bind:tracerLinksEnabled {onTemplateSelected} />
          </div>
          <!-- Mounts before ArtifactPanel's MatchAnalysisFull (below, in the right panel) —
               deliberately: on a cold start MatchAnalysisFull opens its match-analysis SSE
               stream synchronously from its own onMount, while this chat's autopilot kickoff
               needs a network round trip (listSessions) before it fires, so the visible stream
               reliably wins the race to lead the match-analysis compute (see
               match_analysis_coordinator.go). Correctness never depends on this order — the
               backend coordinator does — this only keeps the common case looking right. -->
          <div class="flex min-h-0 h-full" class:hidden={leftTab !== 'chat'}>
            <AssistantChat
              bind:this={chatRef}
              session={sessionId}
              openingActions={coldStartRunning ? undefined : opening}
              autoRun={coldStartRunning}
              {sessionLabel}
              showSessionRail={false}
              {onTurnComplete}
              {onDocumentEdited}
              onRunStateChange={(running) => (runActive = running)}
              onTurnStateChange={(active) => ((turnActive = active), outcomes.turnState(active))}
              beforeTurn={flushPendingSave}
            />
          </div>
        </div>
      </section>

      <PanelSplitter bind:width={leftWidth} panel={leftPanelEl} collapsed={leftCollapsed} />

      <!-- CENTRE: live HTML preview + zoom + Download PDF. On mobile it shows only when the Preview
           tab is picked; at lg it's always shown as the middle column. -->
      <div
        class={['min-w-0 min-h-0 flex-1 flex-col bg-muted/30 lg:flex', mobileView === 'preview' ? 'flex' : 'hidden']}
      >
        <PreviewToolbar
          zoomPct={preview.zoomPct}
          bind:fitMode={preview.fitMode}
          {pageCount}
          {placeholders}
          {pdfUrl}
          onzoomin={() => preview.zoomIn()}
          onzoomout={() => preview.zoomOut()}
          ongap={() => goToGap()}
          {pdf}
        />
        <div bind:this={previewScroller} bind:clientWidth={preview.width} class="min-h-0 flex-1 overflow-auto p-6">
          <CvHtmlPreview
            {doc}
            {templateId}
            zoom={preview.zoom}
            {fonts}
            photoSrc={headshot.src}
            highlightPaths={showingPlaceholders ? placeholders.map((p) => p.path) : (history.pinned?.paths ?? [])}
            onPageCount={(n) => (pageCount = n)}
          />
        </div>
      </div>

      <!-- RIGHT: Job Match / Score / Cover letter / History / Job (renders its own splitter). Shown on mobile
           only when one of its tabs is picked; always shown at lg. -->
      <ArtifactPanel
        {job}
        baseTitle={title}
        {analysis}
        autoRunAnalysis={coldStartRunning}
        {autopilotReport}
        autopilotBusy={turnActive || runActive}
        {atsDelta}
        {jobMatch}
        onRerunAutopilot={() => chatRef?.startRun()}
        revisions={history.list}
        onPreviewRevision={(r) => {
          history.pinned = r;
          // Picking a revision is asking "what changed"; the gap underlines would drown it.
          placeholdersShown = false;
        }}
        onUndoRevision={history.undo}
        onUndoRevisionRun={history.undoRun}
        onReseed={reseed}
        resetBusy={resetLocked}
        {resetError}
        {cvId}
        bind:tab={artifactTab}
        mobileVisible={mobileView === 'jd' ||
          mobileView === 'jobmatch' ||
          mobileView === 'score' ||
          mobileView === 'letter' ||
          mobileView === 'history'}
      />
    </div>
  {/if}
  {#if cliDialogOpen}
    <CliEditDialog {cvId} onClose={() => (cliDialogOpen = false)} />
  {/if}
</div>

<ResetCvDialog bind:open={confirmResetOpen} {baseMode} onConfirm={applyReseed} />

<GapsDownloadDialog
  bind:open={pdf.confirmOpen}
  {placeholders}
  onConfirm={pdf.run}
  onFix={() => {
    pdf.confirmOpen = false;
    void goToGap(0);
  }}
/>
