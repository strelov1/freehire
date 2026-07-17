<script lang="ts">
  // The tailoring artifact panel: a resizable right-hand pane with three tabs — the live CV
  // PDF (refreshed via refreshKey after each agent turn), the vacancy's job description, and
  // the fit verdict (score + honest-wall requirement split). Splitter width is clamped by the
  // vitest-covered clampWidth; the verdict split by splitRequirements.
  import { clampWidth } from './geometry';
  import { splitRequirements } from './verdict';
  import { api } from '$lib/api';
  import type { Analysis } from '$lib/generated/contracts';

  let {
    cvId,
    jobDescription,
    analysis,
    refreshKey = 0,
  }: {
    cvId: number;
    jobDescription: string;
    analysis: Analysis | null;
    refreshKey?: number;
  } = $props();

  type Tab = 'cv' | 'jd' | 'verdict';
  const tabs: [Tab, string][] = [
    ['cv', 'CV'],
    ['jd', 'Job description'],
    ['verdict', 'Verdict'],
  ];
  let tab = $state<Tab>('cv');
  let width = $state(480);
  let resizing = false;

  const cvUrl = $derived(`${api.cvPdfUrl(cvId)}?v=${refreshKey}`);
  const reqs = $derived(splitRequirements(analysis));

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
    {#if tab === 'cv'}
      <iframe src={cvUrl} title="CV preview" class="h-full w-full"></iframe>
    {:else if tab === 'jd'}
      <div class="whitespace-pre-wrap p-4 text-sm leading-relaxed text-foreground">
        {jobDescription || 'No job description.'}
      </div>
    {:else}
      <div class="flex flex-col gap-4 p-4 text-sm">
        {#if analysis}
          <div>
            <span class="text-2xl font-bold text-foreground">{analysis.overall_score}</span>
            <span class="text-muted-foreground">/ 100 · {analysis.verdict}</span>
          </div>
          {#if analysis.recommendation}
            <p class="leading-relaxed text-muted-foreground">{analysis.recommendation}</p>
          {/if}
          <div>
            <h3 class="mb-1 font-medium text-foreground">Reframe — you already have this</h3>
            <ul class="space-y-1 text-muted-foreground">
              {#each reqs.missingHave as r (r.text)}
                <li>• {r.text}</li>
              {:else}
                <li>—</li>
              {/each}
            </ul>
          </div>
          <div>
            <h3 class="mb-1 font-medium text-foreground">Gaps — the agent should ask first</h3>
            <ul class="space-y-1 text-muted-foreground">
              {#each reqs.missingGap as r (r.text)}
                <li>• {r.text}</li>
              {:else}
                <li>—</li>
              {/each}
            </ul>
          </div>
        {:else}
          <p class="text-muted-foreground">No analysis available.</p>
        {/if}
      </div>
    {/if}
  </div>
</aside>
