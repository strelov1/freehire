<script lang="ts">
  import { onMount } from 'svelte';
  import { resolve } from '$app/paths';
  import { FileText, Download, Trash2, ArrowRight } from '@lucide/svelte';
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
    <div class="rounded-lg border border-dashed border-border p-8 sm:p-10">
      <div class="mx-auto max-w-md">
        <FileText class="mx-auto h-8 w-8 text-muted-foreground" />
        <p class="mt-3 text-center font-medium">No tailored CVs yet</p>
        <p class="mt-1 text-center text-sm text-muted-foreground">
          A tailored CV starts from a vacancy’s fit analysis. Here’s how:
        </p>
        <ol class="mx-auto mt-5 flex max-w-sm flex-col gap-3 text-left text-sm">
          <li class="flex items-start gap-3">
            <span class="flex size-6 shrink-0 items-center justify-center rounded-full bg-muted text-xs font-semibold">1</span>
            <span>Open a vacancy you want to apply to.</span>
          </li>
          <li class="flex items-start gap-3">
            <span class="flex size-6 shrink-0 items-center justify-center rounded-full bg-muted text-xs font-semibold">2</span>
            <span>Run the fit check — press <strong class="font-medium text-foreground">Analyze match</strong> on the job page.</span>
          </li>
          <li class="flex items-start gap-3">
            <span class="flex size-6 shrink-0 items-center justify-center rounded-full bg-muted text-xs font-semibold">3</span>
            <span>On the result, choose <strong class="font-medium text-foreground">Tailor my CV</strong> — your tailored copy appears here.</span>
          </li>
        </ol>
        <div class="mt-6 text-center">
          <a
            href={resolve('/')}
            class="inline-flex items-center gap-1.5 rounded-lg bg-foreground px-4 py-2 text-sm font-medium text-background transition-opacity hover:opacity-90"
          >
            Browse jobs <ArrowRight class="size-4" />
          </a>
        </div>
      </div>
    </div>
  {:else}
    <ul class="flex flex-col gap-3">
      {#each items as cv (cv.id)}
        <li
          class="group relative flex items-center gap-4 rounded-xl border border-border bg-card p-4 transition-colors hover:border-border/80 hover:bg-muted/30"
        >
          <!-- The whole card opens the workspace; the action buttons stop propagation below. -->
          <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- resolve() applied to the path; the rule can't see through the appended ?cv= query -->
          <a href={`${resolve('/tailor/[slug]', { slug: cv.job_slug })}?cv=${cv.id}`} class="absolute inset-0" aria-label="Open {cv.job_title}"></a>
          <CompanyLogo name={cv.job_company} size="size-11" />
          <div class="min-w-0 flex-1">
            <p class="truncate font-medium">{cv.job_title}</p>
            <p class="truncate text-sm text-muted-foreground">{cv.job_company}</p>
            <p class="mt-0.5 text-xs text-muted-foreground/80">Updated {fmt(cv.updated_at)}</p>
          </div>
          <div class="relative z-10 flex items-center gap-1">
            <!-- eslint-disable svelte/no-navigation-without-resolve -- external CV PDF API URL, not an internal route -->
            <a
              href={api.cvPdfUrl(cv.id)}
              target="_blank"
              rel="noopener"
              aria-label="Open PDF"
              title="Open PDF"
              class="flex size-9 items-center justify-center rounded-lg text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
            >
              <!-- eslint-enable svelte/no-navigation-without-resolve -->
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
