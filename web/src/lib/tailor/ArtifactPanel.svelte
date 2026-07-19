<script lang="ts">
  // The tailoring artifact panel: a resizable right-hand pane with three tabs — the live CV PDF
  // (refreshed via refreshKey after each agent turn), the vacancy's job description, and the fit
  // verdict. JD and verdict reuse the SAME components the job page / fit page use, so they read
  // identically. Splitter width is clamped by the vitest-covered clampWidth.
  import { clampWidth } from './geometry';
  import { ExternalLink, RefreshCw } from '@lucide/svelte';
  import { api } from '$lib/api';
  import JobDescription from '$lib/components/JobDescription.svelte';
  import MatchAnalysisFull from '$lib/components/MatchAnalysisFull.svelte';
  import CompanyLogo from '$lib/components/CompanyLogo.svelte';
  import CvEditor from '$lib/components/cv/CvEditor.svelte';
  import type { Analysis } from '$lib/generated/contracts';
  import type { Job, MatchAnalysisResponse } from '$lib/types';

  let {
    cvId,
    job,
    analysis,
    refreshKey = 0,
  }: {
    cvId: number;
    job: Job;
    analysis: Analysis | null;
    refreshKey?: number;
  } = $props();

  type Tab = 'cv' | 'edit' | 'jd' | 'verdict';
  const tabs: [Tab, string][] = [
    ['cv', 'CV'],
    ['edit', 'Edit'],
    ['jd', 'Job description'],
    ['verdict', 'Verdict'],
  ];
  let tab = $state<Tab>('cv');
  let width = $state(480);
  let resizing = false;

  // Cache-bust the PDF on both agent turns (refreshKey, from the host) and manual autosaves in
  // the Edit tab (cvVersion, bumped locally) so the preview never shows a stale render.
  let cvVersion = $state(0);
  const cvUrl = $derived(`${api.cvPdfUrl(cvId)}?v=${refreshKey + cvVersion}`);
  // Seed MatchAnalysisFull from the already-cached analysis so it paints read-only (no recompute burn).
  const fit = $derived<MatchAnalysisResponse>({ has_cv: true, stale: false, analysis });

  function startResize(e: PointerEvent) {
    resizing = true;
    (e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
  }
  function doResize(e: PointerEvent) {
    // The panel hugs the right edge, so its width is the distance from the cursor to that edge.
    if (resizing) width = clampWidth(window.innerWidth - e.clientX, 360, 900);
  }
  function stopResize(e: PointerEvent) {
    resizing = false;
    (e.currentTarget as HTMLElement).releasePointerCapture(e.pointerId);
  }
</script>

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

<aside
  class="hidden shrink-0 flex-col border-l border-border bg-background lg:flex"
  style="width: {width}px"
>
  <div class="flex items-center justify-between border-b border-border px-2 py-1.5 text-sm">
    <div class="flex items-center gap-1">
      {#each tabs as [id, label] (id)}
        <button
          type="button"
          onclick={() => (tab = id)}
          class={[
            'rounded px-2 py-1 transition-colors',
            tab === id ? 'bg-muted font-medium text-foreground' : 'text-muted-foreground hover:text-foreground',
          ]}
        >
          {label}
        </button>
      {/each}
    </div>
    <div class="flex items-center gap-1">
      <button
        type="button"
        onclick={() => (cvVersion += 1)}
        title="Reload the CV PDF (after editing it via the CLI)"
        class="inline-flex items-center gap-1 rounded px-2 py-1 text-muted-foreground transition-colors hover:text-foreground"
      >
        <RefreshCw class="size-4" />
        <span class="hidden xl:inline">Refresh</span>
      </button>
      <a
        href={cvUrl}
        target="_blank"
        rel="noopener"
        title="Open the CV PDF in a new tab"
        class="inline-flex items-center gap-1 rounded px-2 py-1 text-muted-foreground transition-colors hover:text-foreground"
      >
        <ExternalLink class="size-4" />
        <span class="hidden xl:inline">Open PDF</span>
      </a>
    </div>
  </div>

  <div class="min-h-0 flex-1 overflow-auto">
    {#if tab === 'cv'}
      <iframe src={cvUrl} title="CV preview" class="h-full w-full"></iframe>
    {:else if tab === 'edit'}
      <div class="p-4">
        <CvEditor id={cvId} embedded onSaved={() => (cvVersion += 1)} />
      </div>
    {:else if tab === 'jd'}
      <div class="p-4">
        <!-- Role header: logo + title + company, so the JD reads as a real posting. -->
        <div class="mb-4 flex items-start gap-3 border-b border-border pb-4">
          <CompanyLogo name={job.company} size="size-10" />
          <div class="min-w-0">
            <h2 class="text-base font-semibold leading-snug text-foreground">{job.title}</h2>
            <p class="text-sm text-muted-foreground">{job.company}</p>
          </div>
        </div>
        {#if job.description}
          <JobDescription html={job.description} />
        {:else}
          <p class="text-sm text-muted-foreground">No job description.</p>
        {/if}
      </div>
    {:else}
      <div class="p-4">
        <MatchAnalysisFull {job} initial={fit} autoRun={false} stacked />
      </div>
    {/if}
  </div>
</aside>
