<script lang="ts">
  import { Check, Download, TriangleAlert, ZoomIn, ZoomOut } from '@lucide/svelte';

  // The tailoring workspace's preview toolbar: zoom (`zoomPct`, or fit to width while
  // `fitMode`), the rendered page count, the gaps still left in the CV (`placeholders`, the
  // first one named) and the PDF download (`pdfUrl`, guarded by `ondownload`).
  let {
    zoomPct,
    fitMode = $bindable(),
    pageCount,
    placeholders,
    pdfUrl,
    onzoomin,
    onzoomout,
    ongap,
    ondownload,
  }: {
    zoomPct: number;
    fitMode: boolean;
    pageCount: number | null;
    placeholders: { token: string }[];
    pdfUrl: string;
    onzoomin: () => void;
    onzoomout: () => void;
    ongap: () => void;
    ondownload: (e: MouseEvent) => void;
  } = $props();
</script>

<div
  class="flex flex-wrap items-center justify-between gap-x-2 gap-y-1.5 border-b border-border bg-background px-3 py-1.5 text-sm"
>
  <div class="flex flex-wrap items-center gap-2">
    <div class="flex items-center gap-1">
      <button
        type="button"
        onclick={onzoomout}
        aria-label="Zoom out"
        class="rounded p-1 text-muted-foreground transition-colors hover:text-foreground"
      >
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
      <button
        type="button"
        onclick={onzoomin}
        aria-label="Zoom in"
        class="rounded p-1 text-muted-foreground transition-colors hover:text-foreground"
      >
        <ZoomIn class="size-4" />
      </button>
    </div>
    {#if pageCount !== null}
      <span
        class={[
          'inline-flex items-center gap-1 text-xs tabular-nums',
          pageCount > 2 ? 'text-warning-strong' : 'text-muted-foreground',
        ]}
      >
        {#if pageCount > 2}
          <TriangleAlert class="size-3.5" aria-hidden="true" />
          {pageCount} pages · aim for 1–2
        {:else}
          <Check class="size-3.5 text-success-strong" aria-hidden="true" />
          {pageCount}
          {pageCount === 1 ? 'page' : 'pages'}
        {/if}
      </span>
    {/if}
  </div>
  <div class="flex flex-wrap items-center gap-2">
    {#if placeholders.length > 0}
      <button
        type="button"
        onclick={ongap}
        title="Go to the next gap in the editor"
        class="inline-flex items-center gap-1.5 rounded-md bg-warning-muted px-2.5 py-1 text-xs font-medium text-warning-strong transition-colors hover:bg-warning-muted/70"
      >
        <TriangleAlert class="size-3.5" aria-hidden="true" />
        <!-- Names one of the slots: "placeholder" alone did not say what was wrong. -->
        {placeholders.length === 1
          ? `1 gap: ${placeholders[0]?.token}`
          : `${placeholders.length} gaps like ${placeholders[0]?.token}`}
      </button>
    {/if}
    <!-- eslint-disable svelte/no-navigation-without-resolve -- external CV PDF API URL, not an internal route -->
    <a
      href={pdfUrl}
      target="_blank"
      rel="noopener"
      onclick={ondownload}
      class="inline-flex items-center gap-1.5 rounded-md border border-border px-2.5 py-1 text-xs font-medium text-foreground transition-colors hover:bg-muted"
    >
      <!-- eslint-enable svelte/no-navigation-without-resolve -->
      <Download class="size-4" /> Download PDF
    </a>
  </div>
</div>
