<script lang="ts">
  import { onMount } from 'svelte';
  import { FileText, Download, Trash2 } from '@lucide/svelte';
  import { api, ApiError } from '$lib/api';
  import CompanyLogo from '$lib/components/CompanyLogo.svelte';
  import { type CvTailoredItem } from '$lib/cv';

  // The tailored-CV landing: one company card per CV the caller built for a vacancy, styled like
  // the saved-jobs cards. The card opens the tailoring workspace (which resumes the same agent
  // session); the PDF and delete actions sit on the card without triggering the open.

  let status = $state<'loading' | 'error' | 'ready'>('loading');
  let error = $state<string | null>(null);
  let items = $state<CvTailoredItem[]>([]);

  onMount(load);

  async function load() {
    status = 'loading';
    try {
      items = await api.listCvs();
      status = 'ready';
    } catch (e) {
      error = e instanceof ApiError ? e.message : 'Could not load your CVs.';
      status = 'error';
    }
  }

  async function remove(cv: CvTailoredItem) {
    if (!window.confirm(`Delete your tailored CV for “${cv.job_title}”? This cannot be undone.`)) return;
    try {
      await api.deleteCv(cv.id);
      items = items.filter((i) => i.id !== cv.id);
    } catch (e) {
      error = e instanceof ApiError ? e.message : 'Could not delete this CV.';
    }
  }

  const fmt = (iso: string) => new Date(iso).toLocaleDateString();
</script>

<div class="space-y-6">
  <div>
    <h1 class="text-2xl font-semibold">Tailored CVs</h1>
    <p class="text-sm text-muted-foreground">
      CVs you tailored for specific roles. Open one to resume its tailoring session, or start a new
      one from any vacancy’s match page.
    </p>
  </div>

  {#if error}<p class="text-sm text-destructive">{error}</p>{/if}

  {#if status === 'loading'}
    <p class="text-muted-foreground">Loading…</p>
  {:else if status === 'ready' && items.length === 0}
    <div class="rounded-lg border border-dashed border-border p-10 text-center">
      <FileText class="mx-auto h-8 w-8 text-muted-foreground" />
      <p class="mt-2 font-medium">No tailored CVs yet</p>
      <p class="text-sm text-muted-foreground">
        Open a vacancy, run the match analysis, and choose “Tailor my CV” to build one here.
      </p>
    </div>
  {:else}
    <ul class="flex flex-col gap-3">
      {#each items as cv (cv.id)}
        <li
          class="group relative flex items-center gap-4 rounded-xl border border-border bg-card p-4 transition-colors hover:border-border/80 hover:bg-muted/30"
        >
          <!-- The whole card opens the workspace; the action buttons stop propagation below. -->
          <a href="/tailor/{cv.job_slug}?cv={cv.id}" class="absolute inset-0" aria-label="Open {cv.job_title}"></a>
          <CompanyLogo name={cv.job_company} size="size-11" />
          <div class="min-w-0 flex-1">
            <p class="truncate font-medium">{cv.job_title}</p>
            <p class="truncate text-sm text-muted-foreground">{cv.job_company}</p>
            <p class="mt-0.5 text-xs text-muted-foreground/80">Updated {fmt(cv.updated_at)}</p>
          </div>
          <div class="relative z-10 flex items-center gap-1">
            <a
              href={api.cvPdfUrl(cv.id)}
              target="_blank"
              rel="noopener"
              aria-label="Open PDF"
              title="Open PDF"
              class="flex size-9 items-center justify-center rounded-lg text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
            >
              <Download class="h-4 w-4" />
            </a>
            <button
              type="button"
              aria-label="Delete"
              title="Delete"
              onclick={() => remove(cv)}
              class="flex size-9 items-center justify-center rounded-lg text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
            >
              <Trash2 class="h-4 w-4" />
            </button>
          </div>
        </li>
      {/each}
    </ul>
  {/if}
</div>
