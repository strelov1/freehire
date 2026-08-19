<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { Search } from '@lucide/svelte';
  import { api, ApiError } from '$lib/api';
  import type { Job } from '$lib/types';
  import { companyLogoUrl } from '$lib/logo';
  import { Button, Dialog, EntityLogo } from '$lib/ui';

  // Three ways into tailoring: pick one of our own vacancies (a search/select — no backend
  // call, straight to the workspace), paste an external job posting URL, or paste the JD
  // text itself. The URL and text tabs both call the same resolve endpoint and land on the
  // same workspace; it doesn't know or care which tab produced its slug.

  let { onClose }: { onClose: () => void } = $props();

  let open = $state(true);
  $effect(() => {
    if (!open) onClose();
  });

  let tab = $state<'existing' | 'url' | 'text'>('existing');

  const DEBOUNCE_MS = 250;
  const RESULTS_LIMIT = 8;

  let query = $state('');
  let results = $state.raw<Job[]>([]);
  let searching = $state(false);
  let reqToken = 0;

  $effect(() => {
    const q = query.trim();
    const id = setTimeout(() => runSearch(q), DEBOUNCE_MS);
    return () => clearTimeout(id);
  });

  async function runSearch(q: string) {
    const mine = ++reqToken;
    if (!q) {
      results = [];
      searching = false;
      return;
    }
    searching = true;
    const page = await api.searchJobs(new URLSearchParams({ q }), RESULTS_LIMIT, 0);
    if (mine !== reqToken) return; // superseded by a newer query
    results = page.items;
    searching = false;
  }

  function selectJob(job: Job) {
    open = false;
    void goto(resolve('/tailor/[slug]', { slug: job.public_slug }));
  }

  let url = $state('');
  let text = $state('');
  let title = $state('');
  let company = $state('');
  let submitting = $state(false);
  let error = $state<string | null>(null);

  function messageFor(e: unknown): string {
    if (e instanceof ApiError) {
      if (e.status === 422) {
        return "We couldn't read a vacancy from that link — double-check it, or paste the description as text instead.";
      }
      if (e.status === 401) return 'Please sign in first.';
    }
    return 'Something went wrong. Please try again.';
  }

  async function resolveAndOpen(input: Parameters<typeof api.resolveJd>[0]) {
    error = null;
    submitting = true;
    try {
      const slug = await api.resolveJd(input);
      open = false;
      await goto(resolve('/tailor/[slug]', { slug }));
    } catch (e) {
      error = messageFor(e);
    } finally {
      submitting = false;
    }
  }

  function submitUrl(e: SubmitEvent) {
    e.preventDefault();
    if (!url.trim()) return;
    void resolveAndOpen({ url: url.trim() });
  }

  function submitText(e: SubmitEvent) {
    e.preventDefault();
    if (!text.trim()) return;
    void resolveAndOpen({
      text: text.trim(),
      title: title.trim() || undefined,
      company: company.trim() || undefined,
    });
  }

  const fieldClass =
    'rounded-md border border-border bg-background px-3 py-2 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring';
</script>

<Dialog bind:open title="Tailor a CV for a job" class="sm:max-w-lg">
  <div class="flex gap-4 border-b border-border text-sm">
    {#each [{ id: 'existing', label: 'Our vacancy' }, { id: 'url', label: 'Link' }, { id: 'text', label: 'Paste text' }] as t (t.id)}
      <button
        type="button"
        onclick={() => (tab = t.id as typeof tab)}
        class="-mb-px border-b-2 px-1 py-2 transition-colors {tab === t.id
          ? 'border-brand font-medium text-foreground'
          : 'border-transparent text-muted-foreground hover:text-foreground'}"
      >
        {t.label}
      </button>
    {/each}
  </div>

  <div class="pt-4">
    {#if tab === 'existing'}
      <div class="flex flex-col gap-3">
        <label class="flex flex-col gap-1.5 text-sm">
          <span class="font-medium">Search our catalog</span>
          <div class="flex items-center gap-2 {fieldClass}">
            <Search class="size-4 shrink-0 text-muted-foreground" />
            <input
              bind:value={query}
              type="text"
              placeholder="Job title or company…"
              autocomplete="off"
              class="min-w-0 flex-1 bg-transparent outline-none placeholder:text-muted-foreground"
            />
          </div>
        </label>
        {#if searching}
          <p class="text-sm text-muted-foreground">Searching…</p>
        {:else if query.trim() && results.length === 0}
          <p class="text-sm text-muted-foreground">No matches.</p>
        {:else if results.length > 0}
          <ul class="flex max-h-72 flex-col gap-1 overflow-y-auto">
            {#each results as job (job.public_slug)}
              <li>
                <button
                  type="button"
                  onclick={() => selectJob(job)}
                  class="flex w-full items-center gap-3 rounded-md px-2 py-2 text-left transition-colors hover:bg-accent"
                >
                  <EntityLogo
                    name={job.company || 'Unknown company'}
                    src={companyLogoUrl(job.company) ?? undefined}
                    shape="square"
                    size="sm"
                  />
                  <span class="min-w-0 flex-1">
                    <span class="block truncate text-sm font-medium">{job.title}</span>
                    <span class="block truncate text-xs text-muted-foreground">{job.company}</span>
                  </span>
                </button>
              </li>
            {/each}
          </ul>
        {/if}
      </div>
    {:else if tab === 'url'}
      <form class="flex flex-col gap-4" onsubmit={submitUrl}>
        <label class="flex flex-col gap-1.5 text-sm">
          <span class="font-medium">Job posting URL</span>
          <input bind:value={url} type="url" required placeholder="https://…" class={fieldClass} />
        </label>
        {#if error}<p role="alert" class="text-sm text-destructive">{error}</p>{/if}
        <Button type="submit" variant="primary" disabled={submitting}>
          {submitting ? 'Resolving…' : 'Continue'}
        </Button>
      </form>
    {:else}
      <form class="flex flex-col gap-4" onsubmit={submitText}>
        <label class="flex flex-col gap-1.5 text-sm">
          <span class="font-medium">Job description</span>
          <textarea
            bind:value={text}
            required
            rows="8"
            placeholder="Paste the job description here…"
            class="resize-y {fieldClass}"
          ></textarea>
        </label>
        <div class="grid grid-cols-2 gap-3">
          <label class="flex flex-col gap-1.5 text-sm">
            <span class="font-medium"
              >Title <span class="font-normal text-muted-foreground">(optional)</span></span
            >
            <input bind:value={title} type="text" class={fieldClass} />
          </label>
          <label class="flex flex-col gap-1.5 text-sm">
            <span class="font-medium"
              >Company <span class="font-normal text-muted-foreground">(optional)</span></span
            >
            <input bind:value={company} type="text" class={fieldClass} />
          </label>
        </div>
        {#if error}<p role="alert" class="text-sm text-destructive">{error}</p>{/if}
        <Button type="submit" variant="primary" disabled={submitting}>
          {submitting ? 'Resolving…' : 'Continue'}
        </Button>
      </form>
    {/if}
  </div>
</Dialog>
