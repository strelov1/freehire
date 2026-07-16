<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { FileText, Download, Trash2, Plus } from '@lucide/svelte';
  import { api, ApiError } from '$lib/api';
  import { Button } from '$lib/ui';
  import { cvTitle, type CvMeta } from '$lib/cv';

  // The CV builder landing: the caller's CVs with create / edit / download / delete.
  // Creating seeds from the stored résumé structure when one exists (the server decides).

  let status = $state<'loading' | 'error' | 'ready'>('loading');
  let error = $state<string | null>(null);
  let creating = $state(false);
  let items = $state<CvMeta[]>([]);

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

  async function create() {
    creating = true;
    error = null;
    try {
      const rec = await api.createCv({ seed: true });
      await goto(`/my/cvs/${rec.id}`);
    } catch (e) {
      error = e instanceof ApiError ? e.message : 'Could not create a CV. Please try again.';
      creating = false;
    }
  }

  async function remove(cv: CvMeta) {
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
  <div class="flex items-center justify-between gap-4">
    <div>
      <h1 class="text-2xl font-semibold">CV builder</h1>
      <p class="text-sm text-muted-foreground">Build tailored, ATS-friendly CVs and export them to PDF.</p>
    </div>
    <Button variant="primary" onclick={create} disabled={creating}>
      <Plus class="mr-1 h-4 w-4" /> {creating ? 'Creating…' : 'New CV'}
    </Button>
  </div>

  {#if error}<p class="text-sm text-destructive">{error}</p>{/if}

  {#if status === 'loading'}
    <p class="text-muted-foreground">Loading…</p>
  {:else if status === 'ready' && items.length === 0}
    <div class="rounded-lg border border-dashed border-border p-10 text-center">
      <FileText class="mx-auto h-8 w-8 text-muted-foreground" />
      <p class="mt-2 font-medium">No CVs yet</p>
      <p class="text-sm text-muted-foreground">
        Create your first CV — it starts from your uploaded résumé when you have one.
      </p>
    </div>
  {:else}
    <ul class="divide-y divide-border rounded-lg border border-border">
      {#each items as cv (cv.id)}
        <li class="flex items-center justify-between gap-4 p-4">
          <a href="/my/cvs/{cv.id}" class="min-w-0 flex-1">
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
