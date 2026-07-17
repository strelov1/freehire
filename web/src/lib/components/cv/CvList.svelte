<script lang="ts">
  import { onMount } from 'svelte';
  import { FileText, Download, Trash2 } from '@lucide/svelte';
  import { api, ApiError } from '$lib/api';
  import { Button } from '$lib/ui';
  import { cvTitle, type CvTailoredItem } from '$lib/cv';

  // The tailored-CV landing: every CV the caller built for a specific vacancy, each a shortcut
  // back into its tailoring workspace (which resumes the same agent session). CVs are created
  // only from a vacancy's match page, so there is no create action here.

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
    if (!window.confirm(`Delete “${cvTitle(cv.title)}”? This cannot be undone.`)) return;
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
    <ul class="divide-y divide-border rounded-lg border border-border">
      {#each items as cv (cv.id)}
        <li class="flex items-center justify-between gap-4 p-4">
          <a href="/tailor/{cv.job_slug}?cv={cv.id}" class="min-w-0 flex-1">
            <span class="block truncate font-medium">{cvTitle(cv.title)}</span>
            <span class="text-xs text-muted-foreground">Updated {fmt(cv.updated_at)} · {cv.template_id}</span>
          </a>
          <div class="flex items-center gap-1">
            <Button variant="ghost" size="icon" href={api.cvPdfUrl(cv.id)} aria-label="Download PDF">
              <Download class="h-4 w-4" />
            </Button>
            <Button variant="ghost" size="icon" aria-label="Delete" onclick={() => remove(cv)}>
              <Trash2 class="h-4 w-4" />
            </Button>
          </div>
        </li>
      {/each}
    </ul>
  {/if}
</div>
