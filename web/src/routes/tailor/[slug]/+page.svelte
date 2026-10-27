<script lang="ts">
  // The dedicated CV-tailoring workspace: a three-column surface — left panel tabbed between the
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
  import { Check, Download, Menu, PanelLeftClose, PanelLeftOpen, Terminal, TriangleAlert, ZoomIn, ZoomOut } from '@lucide/svelte';
  import { announce } from '#lib/announce.svelte.js';
  import { api, ApiError } from '#lib/api.js';
  import { offerCvRefresh, TAILOR_REFRESH_MESSAGE } from '#lib/cvRefreshOffer.js';
  import { askCvRefresh } from '#lib/cvRefreshDialog.svelte.js';
  import { track } from '#lib/analytics.js';
  import { must } from '#lib/utils.js';
  import AssistantChat from '#lib/assistant/AssistantChat.svelte';
  import ArtifactPanel from '#lib/tailor/ArtifactPanel.svelte';
  import CliEditDialog from '#lib/components/cv/CliEditDialog.svelte';
  import CvHtmlPreview from '#lib/tailor/CvHtmlPreview.svelte';
  import CvSectionForm from '#lib/components/cv/CvSectionForm.svelte';
  import ExperienceBankView from '#lib/components/ExperienceBankView.svelte';
  import MarginSettings from '#lib/components/cv/MarginSettings.svelte';
  import TracerLinksSettings from '#lib/components/cv/TracerLinksSettings.svelte';
  import StyleSettings from '#lib/components/cv/StyleSettings.svelte';
  import TemplateGallery from '#lib/tailor/TemplateGallery.svelte';
  import AccountNavRail from '#lib/components/AccountNavRail.svelte';
  import { ConfirmDialog } from '#lib/ui/index.js';
  import { clampWidth, fitToWidthZoom, keyResizeWidth, PANEL_MAX_WIDTH, PANEL_MIN_WIDTH } from '#lib/tailor/geometry.js';
  import { undoRun, openingActions } from '#lib/tailor/autopilot.js';
  import { findPlaceholders } from '#lib/tailor/readiness.js';
  import { tablist } from '#lib/actions/tablist.js';
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
  import type { Analysis, AutopilotEntry, Document, RevisionView } from '#lib/generated/contracts.js';
  import type { Allowance, Job } from '#lib/types.js';
  import { notInPlan, resetsAtLabel } from '#lib/allowance.js';

  const slug = $derived(page.params.slug ?? '');
  const cvParam = $derived(page.url.searchParams.get('cv'));

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
  // The left panel holds what CHANGES the document — its text, its template, its typography —
  // and the chat that does all three by asking. Measuring the document is the right panel's job.
  type LeftTab = 'chat' | 'editor' | 'experience' | 'design';
  let leftTab = $state<LeftTab>('chat');
  const leftTabs: [LeftTab, string][] = [
    ['chat', 'Chat'],
    ['editor', 'Editor'],
    ['experience', 'Experience'],
    ['design', 'Design'],
  ];
  let leftWidth = $state(350);
  // Folded to a rail so the centre CV preview can take the width. Desktop-only: below lg the
  // columns already show one at a time, and collapsing there would hide a view with no way back.
  let leftCollapsed = $state(false);
  let cliDialogOpen = $state(false);
  let leftPanelEl = $state<HTMLElement>();
  let leftResizing = false;

  // The right context panel's tab, lifted here so the mobile tab bar can drive it (on desktop the
  // panel's own tab bar sets it via the same binding).
  let artifactTab = $state<'jd' | 'jobmatch' | 'score' | 'letter' | 'history'>('jobmatch');

  // Mobile-only navigation: below lg the three columns collapse to one, so a single flat tab bar
  // picks which view fills the screen. At lg it's hidden and every column shows at once as before.
  // mobileView is the sole source of truth for per-region visibility on mobile; picking a tab also
  // syncs the matching column's own selector (mobile → column) so the wide layout shows the same
  // content once revealed. The reverse (a desktop tab change updating mobileView) is not wired —
  // switching a column tab then narrowing across lg resets the mobile view to that tab's default.
  type MobileView = 'chat' | 'editor' | 'experience' | 'design' | 'preview' | 'jd' | 'jobmatch' | 'score' | 'letter' | 'history';
  const mobileTabs: [MobileView, string][] = [
    ['chat', 'Chat'],
    ['editor', 'Editor'],
    ['experience', 'Experience'],
    ['design', 'Design'],
    ['preview', 'Preview'],
    ['jobmatch', 'Job Match'],
    ['score', 'ATS'],
    ['letter', 'Letter'],
    ['history', 'History'],
    ['jd', 'Job'],
  ];
  // Defaults to the same tab the desktop right panel opens on (artifactTab above), so a
  // cold start's match-analysis animation is the first thing a mobile visitor sees too,
  // not hidden behind Chat.
  let mobileView = $state<MobileView>('jobmatch');

  // Below lg the account icon rail collapses into a drawer opened by the burger in the mobile tab
  // bar; AccountNavRail owns the drawer and binds this open flag.
  let navOpen = $state(false);
  function pickMobile(v: MobileView) {
    mobileView = v;
    if (v === 'chat' || v === 'editor' || v === 'experience' || v === 'design') leftTab = v;
    else if (v !== 'preview') artifactTab = v;
  }

  // Centre preview zoom. It opens fitted to the column's width, so the whole sheet is readable
  // whatever the window or the side panels leave; the + and − buttons then step in 10% from
  // there (50–150%), and the percentage puts it back to fit. A fixed 80% start used to promise
  // this and overflowed the column at 1440px with both panels open.
  const SCROLLER_PADDING = 48; // the scroller's p-6, both sides
  let previewWidth = $state(0);
  let fitMode = $state(true);
  let manualZoom = $state(0.8);
  const fitZoom = $derived(fitToWidthZoom(previewWidth - SCROLLER_PADDING));
  const zoom = $derived(fitMode ? fitZoom : manualZoom);
  const zoomPct = $derived(Math.round(zoom * 100));
  const clampZoom = (z: number) => Math.min(1.5, Math.max(0.5, Math.round(z * 10) / 10));
  const setZoom = (z: number) => {
    manualZoom = clampZoom(z);
    fitMode = false;
  };
  const zoomOut = () => setZoom(zoom - 0.1);
  const zoomIn = () => setZoom(zoom + 0.1);
  const pdfUrl = $derived(`${api.cvPdfUrl(cvId)}?v=${pdfVersion}`);

  // Readiness of the PDF, the one thing here the seeker actually sends. The page count comes
  // from the preview's own pagination; a template slot the agent left behind (`[result]`)
  // would print verbatim, so the download stops to say so rather than letting it go out.
  let pageCount = $state<number | null>(null);
  const placeholders = $derived(findPlaceholders(doc));
  const placeholderTokens = $derived([...new Set(placeholders.map((p) => p.token))]);
  let placeholdersShown = $state(false);
  const showingPlaceholders = $derived(placeholdersShown && placeholders.length > 0);
  let confirmDownloadOpen = $state(false);
  let previewScroller = $state<HTMLElement | null>(null);

  async function togglePlaceholders(show = !showingPlaceholders) {
    placeholdersShown = show;
    if (!show) return;
    await tick();
    // The hidden measurement layer renders the same marks, so skip anything inside it.
    const first = Array.from(previewScroller?.querySelectorAll('.cv-lit') ?? []).find(
      (el) => !el.closest('[aria-hidden="true"]'),
    );
    const reduce = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
    first?.scrollIntoView({ block: 'center', behavior: reduce ? 'auto' : 'smooth' });
  }

  function guardDownload(e: MouseEvent) {
    if (placeholders.length === 0) return;
    e.preventDefault();
    confirmDownloadOpen = true;
  }

  // The member's headshot, for the templates that print one. Read once here rather than inside
  // the preview: the photo belongs to the profile, not to the CV being edited, and the same URL
  // feeds every template switch. Null while unknown, absent, or unconfigured — the preview then
  // draws the placeholder, exactly as the PDF does.
  let photoSrc = $state<string | null>(null);
  $effect(() => {
    let cancelled = false;
    void (async () => {
      try {
        const meta = await api.getPhoto();
        if (cancelled || !meta.present) return;
        photoSrc = `/api/v1/me/photo/image?v=${encodeURIComponent(meta.uploaded_at ?? '')}`;
      } catch {
        // Best-effort: without it the preview shows the placeholder.
      }
    })();
    return () => {
      cancelled = true;
    };
  });

  // Offered whenever the conversation is empty — the chat renders them only then. A CV opened
  // by id can carry a conversation nobody has spoken to, and that case looked like lost history.
  const opening = openingActions();
  const sessionLabel = $derived(job ? `${job.title} · ${job.company}` : undefined);

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

  // Read the ATS delta for the current CV. Never throws: the workspace must load and edit
  // whether or not the comparison is available, and a failed read is the same absence as an
  // unavailable one.
  async function refreshAtsDelta() {
    try {
      atsDelta = await api.getCvAtsDelta(cvId);
    } catch {
      atsDelta = null;
    }
  }

  // Read the job-match score for the current CV. Never throws, for the same reason the delta
  // does not: the workspace must load and edit whether or not the score is available.
  async function refreshJobMatch() {
    try {
      jobMatch = await api.getCvJobMatch(cvId);
    } catch {
      jobMatch = null;
    }
  }

  // Shared tail of every path that reaches a usable workspace (resume or bootstrap): flips to
  // ready and kicks off the accessory reads that don't block first paint.
  function finishReady() {
    status = 'ready';
    // Not awaited: the workspace is usable before the comparison lands, and the comparison
    // costs two renders. The history is the same kind of accessory read.
    void refreshAtsDelta();
    void refreshJobMatch();
    void loadRevisions();
    // Not awaited either: an empty list only means the font picker has nothing to offer yet,
    // and the preview falls back to the template's own face meanwhile.
    void api.listCvFonts().then((f) => (fonts = f)).catch(() => {});
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
      if (cvParam) {
        // Resume an existing tailored CV. If it already has a bound session, re-attach it with
        // no kickoff. If it has none (a CV created before session binding), mint a fresh
        // tailoring session for it and let the kickoff orient the agent.
        const existing = cvParam;
        const [j, fit] = await Promise.all([
          api.getJob(slug),
          api.getMatchAnalysis(slug).catch(() => null),
        ]);
        job = j;
        cvId = existing;
        analysis = fit?.analysis ?? null;
        const rec = await api.getCv(existing);
        hydrate(rec); // reuse the same record we fetched for the session id
        if (rec.agent_session_id) {
          resuming = true;
          sessionId = rec.agent_session_id;
          // Resume with an existing session never hits POST /me/cvs/tailor, which is what
          // places the vacancy on the Tracking board. Re-run bootstrap for that side effect
          // only (idempotent: same CV, no second debit). Fire-and-forget so a slow tracking
          // write cannot block the workspace.
          void api.tailorCv(slug).catch(() => {});
        } else {
          // A CV created before session binding: the backend mints a tailoring
          // conversation bound to it and stores it on the CV (and places the vacancy
          // on the board).
          const s = await api.startTailorSession(existing);
          sessionId = s.session_id;
        }
      } else {
        // Bootstrap: reach the tailored CV for this vacancy (the backend returns the existing
        // one when there is one) and the conversation bound to it.
        // Only the bootstrap is tracked as a run: it is the path that mints a tailored
        // CV and spends one of the day's sessions. Re-opening an existing one
        // (startTailorSession above) costs nothing and would inflate the count on every
        // revisit.
        track('tailor_run', { slug });
        const [j, tailor] = await Promise.all([api.getJob(slug), api.tailorCv(slug)]);
        job = j;
        await applyTailorResult(tailor);
      }
      finishReady();
    } catch (e) {
      if (e instanceof ApiError && e.status === 402) {
        // Today's allowance is spent: surface what ran out plus when it starts over. The
        // instant comes from the refusal itself rather than being computed here — the
        // server owns when the day rolls, and a second opinion about it would be wrong
        // for anybody whose clock disagrees.
        //
        // A plan that includes none of the feature gets no reset time at all. The server's
        // own message already says the plan excludes this, and a reset appended under it
        // would name a moment at which the allowance is still zero.
        const at = e.body?.allowance as Allowance | undefined;
        const more = at?.resets_at && !notInPlan(at) ? ` More at ${resetsAtLabel(at)}.` : '';
        errorMsg = `${e.message}${more}`;
      } else {
        errorMsg = e instanceof ApiError ? e.message : 'Could not open the tailoring workspace.';
      }
      status = 'error';
    }
  });

  // ---- Revision history ----
  // The feed is loaded beside the CV and refreshed after anything that writes: a save, an
  // agent turn, an undo. `pinnedRevision` is what the preview underlines; it lives here rather
  // than in the panel because the highlight belongs to the document, not to the tab.
  let revisions = $state<RevisionView[]>([]);
  let pinnedRevision = $state<RevisionView | null>(null);

  async function loadRevisions(highlightNewestRun = false) {
    if (!cvId) return;
    try {
      revisions = await api.listCvRevisions(cvId);
      // After a run, what it changed is underlined without being asked: the candidate's first
      // question is "what did it do to my CV", and the answer is on the page.
      if (highlightNewestRun) {
        pinnedRevision = revisions.find((r) => r.actor === 'agent' && !r.reverted) ?? pinnedRevision;
      }
    } catch {
      // The history is an accessory read: a workspace that cannot list it still edits.
      revisions = [];
    }
  }

  // Undoing goes through undoRun's ordering for the reason it was written: the document is
  // saved on a debounce, so undoing without flushing the pending save first lets the timer
  // write the old text straight back a second later — the undo appears to work and then
  // silently reverses itself. The re-read comes last, and only on success.
  async function undoRevision(revision: RevisionView) {
    await undoRun({
      flush: flushPendingSave,
      undo: () => api.undoCvRevision(cvId, revision.id).then(() => undefined),
      refetch: afterUndo,
    });
  }

  async function undoRevisionRun(batchId: string) {
    await undoRun({
      flush: flushPendingSave,
      undo: () => api.undoCvRevisionRun(cvId, batchId).then(() => undefined),
      refetch: afterUndo,
    });
  }

  async function afterUndo() {
    await loadCv();
    pdfVersion += 1;
    void loadRevisions();
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
          void loadRevisions();
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

  async function persist() {
    const snap = snapshot(); // capture NOW; edits during the round-trip re-trigger the effect
    saveState = 'saving';
    try {
      await api.updateCv(cvId, { title, template_id: templateId, document: doc });
      lastSnapshot = snap;
      saveState = 'saved';
      pdfVersion += 1;
      void loadRevisions();
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
  // than onTurnComplete: the accessory reads (ATS delta, job match, revisions) stay end-of-turn
  // only, or a 30-step run would fire ten of each.
  async function onDocumentEdited() {
    try {
      await loadCv();
      pdfVersion += 1;
    } catch {
      /* best-effort; onTurnComplete reconciles at the end regardless */
    }
  }

  // After an agent turn the CV may have changed server-side: flush any pending human edit, then
  // refetch and replace the shared document so the Editor and preview reflect it.
  async function onTurnComplete() {
    await flushPendingSave();
    try {
      await loadCv();
      pdfVersion += 1;
      void loadRevisions(true);
      void refreshAtsDelta();
      void refreshJobMatch();
      // A cold start's own visible match-analysis stream (in ArtifactPanel) normally lands the
      // analysis well before this fires — this is only a fallback for the rare case that
      // stream lost the lead race and is showing a synthesized burst read straight from the
      // cache; that cache is exactly what this re-fetches.
      if (!analysis) {
        analysis = (await api.getMatchAnalysis(slug).catch(() => null))?.analysis ?? null;
      }
    } catch {
      /* best-effort refresh; the next edit or reload will reconcile */
    }
  }

  // Write a pending human edit NOW rather than 800ms from now. Anything that changes the CV
  // server-side has to do this first, or the debounce fires afterwards and overwrites it.
  async function flushPendingSave() {
    if (!saveTimer) return;
    clearTimeout(saveTimer);
    saveTimer = null;
    if (snapshot() !== lastSnapshot) await persist();
  }

  // A template switch is persisted by the gallery via setCvTemplate; mirror the new id into the
  // page's own state so the next autosave (which also writes template_id) doesn't revert it.
  function onTemplateSelected(id: string) {
    templateId = id;
    lastSnapshot = snapshot();
    pdfVersion += 1;
  }

  // Left-panel splitter: width is the cursor's distance from the panel's own left edge.
  function startLeftResize(e: PointerEvent) {
    leftResizing = true;
    (e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
  }
  function doLeftResize(e: PointerEvent) {
    if (!leftResizing || !leftPanelEl) return;
    const left = leftPanelEl.getBoundingClientRect().left;
    leftWidth = clampWidth(e.clientX - left, PANEL_MIN_WIDTH, PANEL_MAX_WIDTH);
  }
  // Also the pointercancel handler: a touch the browser takes back must end the drag. A
  // cancelled pointer has already lost its capture, hence the check.
  function stopLeftResize(e: PointerEvent) {
    leftResizing = false;
    const el = e.currentTarget as HTMLElement;
    if (el.hasPointerCapture(e.pointerId)) el.releasePointerCapture(e.pointerId);
  }
  // The panel sits on the left, so moving the splitter right (ArrowRight) widens it.
  function keyLeftResize(e: KeyboardEvent) {
    const next = keyResizeWidth(leftWidth, e.key, 'ArrowRight');
    if (next === null) return;
    e.preventDefault();
    leftWidth = next;
  }
</script>

<svelte:head><title>Tailor CV{job ? ` · ${job.title}` : ''} — freehire</title></svelte:head>

<!-- Full-width workspace loses the account shell nav; the same left-edge icon rail as
     the Agent page brings the account sections back. It stays put across every state. -->
<div use:fillViewport class="flex h-[calc(100dvh-3.5rem)]">
  <AccountNavRail collapsible bind:open={navOpen} />
  {#if status === 'loading'}
    <div class="flex min-w-0 flex-1 items-center justify-center text-sm text-muted-foreground">
      {resuming ? 'Re-opening your tailoring session…' : 'Preparing your tailoring session…'}
    </div>
  {:else if status === 'error'}
    <div class="flex min-w-0 flex-1 flex-col items-center justify-center gap-3 p-6 text-center">
      <p class="max-w-md text-sm text-destructive">{errorMsg}</p>
      <a href={resolve('/jobs/[slug]', { slug })} class="text-sm text-brand hover:underline">Back to the role</a>
    </div>
  {:else}
    <div class="flex min-w-0 flex-1 flex-col lg:flex-row">
      <!-- MOBILE TAB BAR: below lg the three columns collapse to one full-screen view; this flat,
           horizontally-scrollable bar switches between all of them. Hidden at lg (columns stack). -->
      <nav class="flex items-center gap-1 overflow-x-auto border-b border-border bg-background px-2 py-1.5 text-sm lg:hidden">
        <!-- Burger opens the account nav drawer (the icon rail is hidden below lg). -->
        <button
          type="button"
          onclick={() => (navOpen = true)}
          aria-label="Open menu"
          aria-expanded={navOpen}
          class="shrink-0 rounded p-1 text-muted-foreground transition-colors hover:text-foreground"
        >
          <Menu class="size-5" />
        </button>
        <span class="mr-0.5 h-5 w-px shrink-0 bg-border" aria-hidden="true"></span>
        {#each mobileTabs as [id, label] (id)}
          <button
            type="button"
            onclick={() => pickMobile(id)}
            aria-current={mobileView === id ? 'page' : undefined}
            class={['shrink-0 rounded px-2 py-1 transition-colors', mobileView === id ? 'bg-muted font-medium text-foreground' : 'text-muted-foreground hover:text-foreground']}
          >
            {label}
          </button>
        {/each}
      </nav>

      <!-- LEFT: Editor / Chat tabs (chat stays mounted across tab switches). On mobile it shows only
           when its tab is picked; at lg it's a splitter-sized column always shown. The width rides a
           CSS var so the inline style never overrides the mobile w-full. -->
      {#if leftCollapsed}
        <!-- The collapsed rail. Desktop-only, and it carries the one control that brings the
             panel back — a collapse with no visible way out is a lost panel. -->
        <div
          class="hidden shrink-0 flex-col items-center gap-2 border-r border-border bg-background px-1.5 py-2 lg:flex"
        >
          <button
            type="button"
            onclick={() => (leftCollapsed = false)}
            aria-label="Expand the editor panel"
            class="rounded p-1 text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
          >
            <PanelLeftOpen class="size-4" />
          </button>
        </div>
      {/if}
      <section
        bind:this={leftPanelEl}
        class={[
          'w-full min-h-0 flex-1 flex-col border-r border-border bg-background lg:w-[var(--lw)] lg:flex-none',
          mobileView === 'chat' || mobileView === 'editor' || mobileView === 'experience' || mobileView === 'design'
            ? 'flex'
            : 'hidden',
          leftCollapsed ? 'lg:hidden' : 'lg:flex',
        ]}
        style="--lw: {leftWidth}px"
      >
        <!-- Own tab bar (and save status) is desktop-only; the mobile bar drives the tab there. -->
        <div class="hidden items-center justify-between gap-2 border-b border-border px-2 py-1.5 text-sm lg:flex">
          <div
            role="tablist"
            aria-label="Edit this CV"
            use:tablist={leftTab}
            class="flex min-w-0 flex-nowrap items-center gap-1 overflow-x-auto"
          >
            {#each leftTabs as [id, label] (id)}
              <button
                type="button"
                role="tab"
                id="tailor-left-tab-{id}"
                aria-selected={leftTab === id}
                aria-controls="tailor-left-panel"
                onclick={() => (leftTab = id)}
                class={['shrink-0 rounded px-2 py-1 transition-colors', leftTab === id ? 'bg-brand-muted font-semibold text-brand-strong' : 'text-muted-foreground hover:text-foreground']}
              >
                {label}
              </button>
            {/each}
          </div>
          <div class="flex shrink-0 items-center gap-1">
            {#if leftTab === 'editor' || leftTab === 'design'}
              <span
                class={['text-xs', saveState === 'error' ? 'text-destructive' : 'text-muted-foreground']}
                aria-live="polite"
                title={saveState === 'error' ? saveError : undefined}
              >
                {#if saveState === 'saving'}Saving…{:else if saveState === 'saved'}Saved{:else if saveState === 'error'}Save failed{/if}
              </span>
            {/if}
            <button
              type="button"
              onclick={() => (leftCollapsed = true)}
              aria-label="Collapse the editor panel"
              class="rounded p-1 text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
            >
              <PanelLeftClose class="size-4" />
            </button>
          </div>
        </div>
        <div id="tailor-left-panel" role="tabpanel" aria-labelledby="tailor-left-tab-{leftTab}" class="min-h-0 flex-1">
          <div class="h-full overflow-auto p-4" class:hidden={leftTab !== 'editor'}>
            {#if runActive}
              <p class="mb-3 rounded-lg border border-border bg-muted/40 px-3 py-2 text-sm text-muted-foreground">
                The agent is editing this CV. Editing is paused until the run finishes.
              </p>
            {/if}
            <!-- A run rewrites this same document server-side while the form holds it in memory
                 and saves on a debounce; letting both write means one side's work vanishes. -->
            <fieldset disabled={runActive} class="contents">
              <CvSectionForm bind:doc bind:title />
            </fieldset>
            <!-- The terminal is another way to edit the same text, so it is offered where the
                 text is edited rather than as an unlabelled glyph in the panel's header. -->
            <button
              type="button"
              onclick={() => (cliDialogOpen = true)}
              class="mt-6 inline-flex items-center gap-1.5 text-sm text-muted-foreground transition-colors hover:text-foreground"
            >
              <Terminal class="size-4" aria-hidden="true" /> Edit this CV from your terminal
            </button>
          </div>
          <!-- The experience bank as its owner sees it on /my/profile — same component, so
               checking, confirming, or editing what the assistant knows never means leaving the
               workspace. -->
          <div class="h-full overflow-auto p-4" class:hidden={leftTab !== 'experience'}>
            <!-- The refresh a bank edit offers is the same reset the History tab's control runs,
                 and it fails into the same `resetError` — but someone who triggered it from here
                 is not looking at that tab, so the failure is said here too. -->
            {#if resetError}
              <p
                class="mb-3 rounded-lg border border-destructive/40 bg-destructive/10 px-3 py-2 text-sm text-destructive-strong"
              >
                {resetError}
              </p>
            {/if}
            <ExperienceBankView onBankMutated={offerRefreshAfterBankEdit} />
          </div>
          <!-- Design: everything that decides how the CV looks, in one place — the template first,
               since it sets the defaults every control below adjusts. All of it writes straight
               into the shared document, so the centre preview re-renders live and autosave
               persists it on the same debounce as any other edit. -->
          <div class="h-full overflow-auto p-4" class:hidden={leftTab !== 'design'}>
            <div class="space-y-8">
              <section class="space-y-3">
                <h2 class="text-sm font-semibold text-foreground">Template</h2>
                <TemplateGallery {cvId} onSelected={onTemplateSelected} />
              </section>
              <section class="space-y-3">
                <h2 class="text-sm font-semibold text-foreground">Typography</h2>
                <StyleSettings bind:style={doc.style} {fonts} />
              </section>
              <section class="space-y-3">
                <h2 class="text-sm font-semibold text-foreground">
                  Page margins <span class="font-normal text-muted-foreground">(inches)</span>
                </h2>
                <MarginSettings bind:margins={doc.margins} />
              </section>
              <!-- Not a presentation choice, and deliberately last: consent to track whoever opens
                   this CV is a different kind of decision from a font size, and it writes on its
                   own rather than riding the document's autosave. -->
              <section class="space-y-3 border-t border-border pt-6">
                <h2 class="text-sm font-semibold text-foreground">Link tracking</h2>
                <TracerLinksSettings {cvId} bind:enabled={tracerLinksEnabled} />
              </section>
            </div>
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
              onTurnStateChange={(active) => (turnActive = active)}
              beforeTurn={flushPendingSave}
            />
          </div>
        </div>
      </section>

      <!-- LEFT SPLITTER: drag, or focus it and use the arrow keys. -->
      <!-- A focusable separator is the ARIA window-splitter widget; the lint reads every separator as static. -->
      <!-- svelte-ignore a11y_no_noninteractive_tabindex, a11y_no_noninteractive_element_interactions -->
      <div
        class={[
          'hidden w-1.5 shrink-0 cursor-col-resize touch-none bg-border/50 transition-colors hover:bg-border focus-visible:bg-ring focus-visible:outline-none',
          leftCollapsed ? 'lg:hidden' : 'lg:block',
        ]}
        role="separator"
        tabindex="0"
        aria-orientation="vertical"
        aria-label="Resize editor panel"
        aria-valuenow={leftWidth}
        aria-valuemin={PANEL_MIN_WIDTH}
        aria-valuemax={PANEL_MAX_WIDTH}
        onpointerdown={startLeftResize}
        onpointermove={doLeftResize}
        onpointerup={stopLeftResize}
        onpointercancel={stopLeftResize}
        onkeydown={keyLeftResize}
      ></div>

      <!-- CENTRE: live HTML preview + zoom + Download PDF. On mobile it shows only when the Preview
           tab is picked; at lg it's always shown as the middle column. -->
      <div
        class={['min-w-0 min-h-0 flex-1 flex-col bg-muted/30 lg:flex', mobileView === 'preview' ? 'flex' : 'hidden']}
      >
        <div class="flex flex-wrap items-center justify-between gap-x-2 gap-y-1.5 border-b border-border bg-background px-3 py-1.5 text-sm">
          <div class="flex flex-wrap items-center gap-2">
            <div class="flex items-center gap-1">
              <button type="button" onclick={zoomOut} aria-label="Zoom out" class="rounded p-1 text-muted-foreground transition-colors hover:text-foreground">
                <ZoomOut class="size-4" />
              </button>
              <button
                type="button"
                onclick={() => (fitMode = true)}
                aria-label="Zoom {zoomPct}%. Fit the page to the width"
                title="Fit to width"
                aria-pressed={fitMode}
                class="w-12 rounded py-0.5 text-center text-xs tabular-nums text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
              >
                {zoomPct}%
              </button>
              <button type="button" onclick={zoomIn} aria-label="Zoom in" class="rounded p-1 text-muted-foreground transition-colors hover:text-foreground">
                <ZoomIn class="size-4" />
              </button>
            </div>
            {#if pageCount !== null}
              <span
                class={['inline-flex items-center gap-1 text-xs tabular-nums', pageCount > 2 ? 'text-warning-strong' : 'text-muted-foreground']}
              >
                {#if pageCount > 2}
                  <TriangleAlert class="size-3.5" aria-hidden="true" />
                  {pageCount} pages · aim for 1–2
                {:else}
                  <Check class="size-3.5 text-success-strong" aria-hidden="true" />
                  {pageCount} {pageCount === 1 ? 'page' : 'pages'}
                {/if}
              </span>
            {/if}
          </div>
          <div class="flex flex-wrap items-center gap-2">
            {#if placeholders.length > 0}
              <button
                type="button"
                onclick={() => togglePlaceholders()}
                aria-pressed={showingPlaceholders}
                class="inline-flex items-center gap-1.5 rounded-md bg-warning-muted px-2.5 py-1 text-xs font-medium text-warning-strong transition-colors hover:bg-warning-muted/70"
              >
                <TriangleAlert class="size-3.5" aria-hidden="true" />
                <!-- Names one of the slots: "placeholder" alone did not say what was wrong. -->
                {placeholders.length === 1 ? `1 gap: ${placeholders[0]?.token}` : `${placeholders.length} gaps like ${placeholders[0]?.token}`}
              </button>
            {/if}
            <!-- eslint-disable svelte/no-navigation-without-resolve -- external CV PDF API URL, not an internal route -->
            <a
              href={pdfUrl}
              target="_blank"
              rel="noopener"
              onclick={guardDownload}
              class="inline-flex items-center gap-1.5 rounded-md border border-border px-2.5 py-1 text-xs font-medium text-foreground transition-colors hover:bg-muted"
            >
              <!-- eslint-enable svelte/no-navigation-without-resolve -->
              <Download class="size-4" /> Download PDF
            </a>
          </div>
        </div>
        <div bind:this={previewScroller} bind:clientWidth={previewWidth} class="min-h-0 flex-1 overflow-auto p-6">
          <CvHtmlPreview
            {doc}
            {templateId}
            {zoom}
            {fonts}
            {photoSrc}
            highlightPaths={showingPlaceholders ? placeholders.map((p) => p.path) : (pinnedRevision?.paths ?? [])}
            onPageCount={(n) => (pageCount = n)}
          />
        </div>
      </div>

      <!-- RIGHT: Job Match / Score / Cover letter / History / Job (renders its own splitter). Shown on mobile
           only when one of its tabs is picked; always shown at lg. -->
      <ArtifactPanel
        job={must(job, 'job')}
        {analysis}
        autoRunAnalysis={coldStartRunning}
        {autopilotReport}
        autopilotBusy={turnActive || runActive}
        {atsDelta}
        {jobMatch}
        onRerunAutopilot={() => chatRef?.startRun()}
        {revisions}
        onPreviewRevision={(r) => (pinnedRevision = r)}
        onUndoRevision={undoRevision}
        onUndoRevisionRun={undoRevisionRun}
        onReseed={reseed}
        resetBusy={resetLocked}
        {resetError}
        {cvId}
        bind:tab={artifactTab}
        mobileVisible={mobileView === 'jd' || mobileView === 'jobmatch' || mobileView === 'score' || mobileView === 'letter' || mobileView === 'history'}
      />
    </div>
  {/if}
  {#if cliDialogOpen}
    <CliEditDialog {cvId} onClose={() => (cliDialogOpen = false)} />
  {/if}
</div>

<ConfirmDialog
  bind:open={confirmResetOpen}
  title="Rebuild this tailored CV from your current seed?"
  description="Your template and typography stay; content edits can be undone from History."
  confirmLabel="Reset"
  onConfirm={applyReseed}
/>

<ConfirmDialog
  bind:open={confirmDownloadOpen}
  title="Your CV still has gaps to fill"
  description="These bracketed slots will print in the PDF exactly as written."
  confirmLabel="Download anyway"
  cancelLabel="Keep editing"
  onConfirm={() => {
    window.open(pdfUrl, '_blank', 'noopener');
  }}
>
  <ul class="flex flex-wrap gap-1.5">
    {#each placeholderTokens as token (token)}
      <li><code class="rounded bg-muted px-1.5 py-0.5 text-xs">{token}</code></li>
    {/each}
  </ul>
  <button
    type="button"
    class="mt-3 text-sm font-medium text-brand-strong underline-offset-4 hover:underline"
    onclick={() => {
      confirmDownloadOpen = false;
      togglePlaceholders(true);
    }}
  >
    Show them on the page
  </button>
</ConfirmDialog>
