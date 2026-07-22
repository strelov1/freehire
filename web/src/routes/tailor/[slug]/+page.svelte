<script lang="ts">
  // The dedicated CV-tailoring workspace: a three-column surface — left panel tabbed between the
  // structured Editor and the Chat, a centre live HTML CV preview (zoom + Download PDF), and a
  // right context panel tabbed between Templates, the Job description, and the Verdict. Two modes:
  //  - bootstrap (no ?cv): create the tailored CV + a seeded agent session, auto-start, and
  //    store the session id on the CV so it can be re-opened.
  //  - resume (?cv=<id>): reuse the existing CV + its stored session — re-attach, NO kickoff.
  //
  // The page owns the CV document in memory so the Editor and the centre preview share one object:
  // typing re-renders the preview instantly, autosave persists in the background, and an agent turn
  // refetches and replaces the document.
  import { onMount, onDestroy } from 'svelte';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { ZoomIn, ZoomOut, Download } from '@lucide/svelte';
  import { api, ApiError } from '$lib/api';
  import { createSession } from '$lib/assistant/api';
  import AssistantChat from '$lib/assistant/AssistantChat.svelte';
  import ArtifactPanel from '$lib/tailor/ArtifactPanel.svelte';
  import CvHtmlPreview from '$lib/tailor/CvHtmlPreview.svelte';
  import CvSectionForm from '$lib/components/cv/CvSectionForm.svelte';
  import AccountNavRail from '$lib/components/AccountNavRail.svelte';
  import { clampWidth } from '$lib/tailor/geometry';
  import { toEditable, type CvRecord } from '$lib/cv';
  import type { Analysis, Document } from '$lib/generated/contracts';
  import type { Job } from '$lib/types';

  const slug = $derived(page.params.slug ?? '');
  const cvParam = $derived(page.url.searchParams.get('cv'));

  let status = $state<'loading' | 'ready' | 'error'>('loading');
  let errorMsg = $state('');
  let sessionId = $state<string | undefined>(undefined);
  let resuming = $state(false);
  let cvId = $state(0);
  let analysis = $state<Analysis | null>(null);
  let job = $state<Job | null>(null);

  // Page-owned CV state: the single client source of truth the Editor binds and the preview reads.
  let doc = $state<Document>({ header: {} });
  let title = $state('');
  let templateId = $state('classic-ats');
  let cvLoaded = $state(false);
  // Autosave lifecycle mirrors the old standalone editor: 'idle' before the first change, then
  // saving → saved (or error).
  let saveState = $state<'idle' | 'saving' | 'saved' | 'error'>('idle');
  let saveError = $state('');
  // Bumped on every persisted change (autosave, agent turn, template switch) to cache-bust the PDF.
  let pdfVersion = $state(0);

  // Left panel: which tab is shown, and its resizable width. The chat stays mounted across tab
  // switches (hidden, not unmounted) so its live session is never dropped.
  let leftTab = $state<'chat' | 'editor'>('chat');
  let leftWidth = $state(440);
  let leftPanelEl = $state<HTMLElement>();
  let leftResizing = false;

  // Centre preview zoom, clamped to 50–150% in 10% steps. Starts at 90% so the full A4 page
  // fits the centre column on load.
  let zoom = $state(0.9);
  const zoomPct = $derived(Math.round(zoom * 100));
  const clampZoom = (z: number) => Math.min(1.5, Math.max(0.5, Math.round(z * 10) / 10));
  const zoomOut = () => (zoom = clampZoom(zoom - 0.1));
  const zoomIn = () => (zoom = clampZoom(zoom + 0.1));
  const pdfUrl = $derived(`${api.cvPdfUrl(cvId)}?v=${pdfVersion}`);

  const kickoff =
    "Let's tailor my CV for this role — review the fit analysis and walk me through the gaps.";
  const sessionLabel = $derived(job ? `${job.title} · ${job.company}` : undefined);

  // Hydrate the page-owned CV state from a CV record (marking the snapshot as the persisted
  // baseline so the autosave effect doesn't fire on load).
  function hydrate(rec: CvRecord) {
    title = rec.title;
    templateId = rec.template_id;
    doc = toEditable(rec.document);
    lastSnapshot = snapshot();
    cvLoaded = true;
  }
  const loadCv = async () => hydrate(await api.getCv(cvId));

  onMount(async () => {
    try {
      if (cvParam) {
        // Resume an existing tailored CV. If it already has a bound session, re-attach it with
        // no kickoff. If it has none (a CV created before session binding), mint a fresh
        // tailoring session for it and let the kickoff orient the agent.
        const existing = Number(cvParam);
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
        } else {
          const s = await api.startTailorSession(existing);
          sessionId = await createSession({
            cli_token: s.cli_token,
            cv_id: existing,
            base_cv_id: s.base_cv_id,
          });
          await api.setCvSession(existing, sessionId).catch(() => {}); // best-effort
        }
      } else {
        // Bootstrap: create the tailored CV + a seeded session, then bind the session to the CV.
        const [j, tailor] = await Promise.all([api.getJob(slug), api.tailorCv(slug)]);
        job = j;
        cvId = tailor.tailor_cv_id;
        analysis = tailor.analysis;
        sessionId = await createSession({
          cli_token: tailor.cli_token,
          cv_id: tailor.tailor_cv_id,
          base_cv_id: tailor.base_cv_id,
        });
        await api.setCvSession(tailor.tailor_cv_id, sessionId).catch(() => {}); // best-effort
        await loadCv(); // bootstrap has no CV record in hand yet — fetch the fresh tailored copy
      }
      status = 'ready';
    } catch (e) {
      if (e instanceof ApiError && e.status === 402) {
        // Out of AI credits: surface the message plus when the monthly grant renews.
        const resetsAt = typeof e.body?.resets_at === 'string' ? e.body.resets_at : null;
        const renews = resetsAt
          ? ` They renew ${new Date(resetsAt).toLocaleDateString(undefined, { month: 'short', day: 'numeric' })}.`
          : '';
        errorMsg = `${e.message}${renews}`;
      } else {
        errorMsg = e instanceof ApiError ? e.message : 'Could not open the tailoring workspace.';
      }
      status = 'error';
    }
  });

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

  // After an agent turn the CV may have changed server-side: flush any pending human edit, then
  // refetch and replace the shared document so the Editor and preview reflect it.
  async function onTurnComplete() {
    if (saveTimer) {
      clearTimeout(saveTimer);
      saveTimer = null;
      if (snapshot() !== lastSnapshot) await persist();
    }
    try {
      await loadCv();
      pdfVersion += 1;
    } catch {
      /* best-effort refresh; the next edit or reload will reconcile */
    }
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
    leftWidth = clampWidth(e.clientX - left, 340, 720);
  }
  function stopLeftResize(e: PointerEvent) {
    leftResizing = false;
    (e.currentTarget as HTMLElement).releasePointerCapture(e.pointerId);
  }
</script>

<svelte:head><title>Tailor CV{job ? ` · ${job.title}` : ''} — freehire</title></svelte:head>

<!-- Full-width workspace loses the account shell nav; the same left-edge icon rail as
     the Agent page brings the account sections back. It stays put across every state. -->
<div class="flex h-[calc(100svh-3.5rem)]">
  <AccountNavRail />
  {#if status === 'loading'}
    <div class="flex min-w-0 flex-1 items-center justify-center text-sm text-muted-foreground">
      {resuming ? 'Re-opening your tailoring session…' : 'Preparing your tailoring session…'}
    </div>
  {:else if status === 'error'}
    <div class="flex min-w-0 flex-1 flex-col items-center justify-center gap-3 p-6 text-center">
      <p class="max-w-md text-sm text-destructive">{errorMsg}</p>
      <a href={resolve('/match/[slug]', { slug })} class="text-sm text-brand hover:underline">Back to the fit analysis</a>
    </div>
  {:else}
    <div class="flex min-w-0 flex-1">
      <!-- LEFT: Editor / Chat tabs (chat stays mounted across tab switches). Full-width below lg
           (the centre preview + right panel are lg-only), a splitter-sized column at lg+. The
           width rides a CSS var so the inline style never overrides the mobile w-full. -->
      <section
        bind:this={leftPanelEl}
        class="flex w-full shrink-0 flex-col border-r border-border bg-background lg:w-[var(--lw)]"
        style="--lw: {leftWidth}px"
      >
        <div class="flex items-center justify-between gap-2 border-b border-border px-2 py-1.5 text-sm">
          <div class="flex items-center gap-1">
            <button
              type="button"
              onclick={() => (leftTab = 'editor')}
              class={['rounded px-2 py-1 transition-colors', leftTab === 'editor' ? 'bg-muted font-medium text-foreground' : 'text-muted-foreground hover:text-foreground']}
            >
              Editor
            </button>
            <button
              type="button"
              onclick={() => (leftTab = 'chat')}
              class={['rounded px-2 py-1 transition-colors', leftTab === 'chat' ? 'bg-muted font-medium text-foreground' : 'text-muted-foreground hover:text-foreground']}
            >
              Chat
            </button>
          </div>
          {#if leftTab === 'editor'}
            <span
              class={['pr-1 text-xs', saveState === 'error' ? 'text-destructive' : 'text-muted-foreground']}
              aria-live="polite"
              title={saveState === 'error' ? saveError : undefined}
            >
              {#if saveState === 'saving'}Saving…{:else if saveState === 'saved'}Saved{:else if saveState === 'error'}Save failed{/if}
            </span>
          {/if}
        </div>
        <div class="min-h-0 flex-1">
          <div class="h-full overflow-auto p-4" class:hidden={leftTab !== 'editor'}>
            <CvSectionForm bind:doc bind:title />
          </div>
          <div class="flex min-h-0 h-full" class:hidden={leftTab !== 'chat'}>
            <AssistantChat
              session={sessionId}
              kickoff={resuming ? undefined : kickoff}
              {sessionLabel}
              showSessionRail={false}
              requireBeta={false}
              {onTurnComplete}
            />
          </div>
        </div>
      </section>

      <!-- LEFT SPLITTER -->
      <div
        class="hidden w-1.5 shrink-0 cursor-col-resize bg-border/50 transition-colors hover:bg-border lg:block"
        role="separator"
        aria-orientation="vertical"
        aria-label="Resize editor panel"
        onpointerdown={startLeftResize}
        onpointermove={doLeftResize}
        onpointerup={stopLeftResize}
      ></div>

      <!-- CENTRE: live HTML preview + zoom + Download PDF. lg-only — below lg the left panel
           (chat + editor) takes the whole surface. -->
      <div class="hidden min-w-0 flex-1 flex-col bg-muted/30 lg:flex">
        <div class="flex items-center justify-between gap-2 border-b border-border bg-background px-3 py-1.5 text-sm">
          <div class="flex items-center gap-1">
            <button type="button" onclick={zoomOut} aria-label="Zoom out" class="rounded p-1 text-muted-foreground transition-colors hover:text-foreground">
              <ZoomOut class="size-4" />
            </button>
            <span class="w-12 text-center text-xs tabular-nums text-muted-foreground">{zoomPct}%</span>
            <button type="button" onclick={zoomIn} aria-label="Zoom in" class="rounded p-1 text-muted-foreground transition-colors hover:text-foreground">
              <ZoomIn class="size-4" />
            </button>
          </div>
          <!-- eslint-disable svelte/no-navigation-without-resolve -- external CV PDF API URL, not an internal route -->
          <a
            href={pdfUrl}
            target="_blank"
            rel="noopener"
            class="inline-flex items-center gap-1.5 rounded-md border border-border px-2.5 py-1 text-xs font-medium text-foreground transition-colors hover:bg-muted"
          >
            <!-- eslint-enable svelte/no-navigation-without-resolve -->
            <Download class="size-4" /> Download PDF
          </a>
        </div>
        <div class="min-h-0 flex-1 overflow-auto p-6">
          <CvHtmlPreview {doc} {templateId} {zoom} />
        </div>
      </div>

      <!-- RIGHT: Templates / Job description / Verdict (renders its own splitter). -->
      <ArtifactPanel {cvId} job={job!} {analysis} {onTemplateSelected} />
    </div>
  {/if}
</div>
