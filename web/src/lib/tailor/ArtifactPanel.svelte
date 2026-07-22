<script lang="ts">
  // The tailoring context panel: a resizable right-hand pane with three tabs — the template
  // gallery, the vacancy's job description, and the fit verdict. The CV itself now renders in the
  // centre column (live HTML) and edits in the left panel, so this panel carries only context.
  // JD and verdict reuse the SAME components the job page / fit page use, so they read identically.
  // Splitter width is clamped by the vitest-covered clampWidth.
  import { clampWidth } from './geometry';
  import JobDescription from '$lib/components/JobDescription.svelte';
  import MatchAnalysisFull from '$lib/components/MatchAnalysisFull.svelte';
  import CompanyLogo from '$lib/components/CompanyLogo.svelte';
  import TemplateGallery from './TemplateGallery.svelte';
  import type { Analysis } from '$lib/generated/contracts';
  import type { Job, MatchAnalysisResponse } from '$lib/types';

  let {
    cvId,
    job,
    analysis,
    onTemplateSelected,
  }: {
    cvId: number;
    job: Job;
    analysis: Analysis | null;
    onTemplateSelected: (id: string) => void;
  } = $props();

  type Tab = 'templates' | 'jd' | 'verdict';
  const tabs: [Tab, string][] = [
    ['templates', 'Templates'],
    ['jd', 'Job description'],
    ['verdict', 'Verdict'],
  ];
  let tab = $state<Tab>('templates');
  let width = $state(340);
  let resizing = false;

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
  <div class="flex items-center gap-1 border-b border-border px-2 py-1.5 text-sm">
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

  <div class="min-h-0 flex-1 overflow-auto">
    {#if tab === 'templates'}
      <div class="p-4">
        <TemplateGallery {cvId} onSelected={onTemplateSelected} />
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
