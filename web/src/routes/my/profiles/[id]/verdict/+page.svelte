<script lang="ts">
  import { ArrowUp } from '@lucide/svelte';
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import { ApiError, analyzeProfileResume, getProfileVerdict } from '$lib/api';
  import { isAuthenticated } from '$lib/auth.svelte';
  import { openAuthDialog } from '$lib/auth-dialog.svelte';
  import { categoryLabel } from '$lib/facets';
  import States from '$lib/components/States.svelte';
  import VerdictView from '$lib/components/VerdictView.svelte';
  import { searchProfiles } from '$lib/searchProfiles.svelte';
  import type { Verdict } from '$lib/types';
  import { Button } from '$lib/ui';

  const id = $derived(Number(page.params.id));
  const profile = $derived(searchProfiles.items.find((p) => p.id === id));
  const role = $derived(profile ? profile.specializations.map(categoryLabel).join(', ') : '');

  let status = $state<'loading' | 'error' | 'ready'>('loading');
  let verdict = $state<Verdict | null>(null);

  // Résumé analysis (the LLM coherence layer) — dropzone state.
  let resumeBusy = $state(false);
  let resumeError = $state<string | null>(null);
  let dragActive = $state(false);
  let fileName = $state<string | null>(null);
  let fileInput = $state<HTMLInputElement | null>(null);

  async function load() {
    status = 'loading';
    try {
      await searchProfiles.ensureLoaded();
      verdict = await getProfileVerdict(id);
      status = 'ready';
    } catch {
      status = 'error';
    }
  }

  $effect(() => {
    if (isAuthenticated()) void load();
  });

  function isPdf(file: File): boolean {
    return file.type === 'application/pdf' || file.name.toLowerCase().endsWith('.pdf');
  }

  // Analyze the résumé for coherence + advice. Best-effort like the backend: a failure
  // shows a message but the deterministic verdict already on screen is untouched.
  async function analyze(file: File) {
    resumeBusy = true;
    resumeError = null;
    fileName = file.name;
    try {
      verdict = await analyzeProfileResume(id, file);
    } catch (err) {
      resumeError =
        err instanceof ApiError ? err.message : 'Could not analyze the résumé. Please try again.';
    } finally {
      resumeBusy = false;
    }
  }

  function onFile(e: Event) {
    const target = e.currentTarget as HTMLInputElement;
    const file = target.files?.[0];
    target.value = '';
    if (file) void analyze(file);
  }

  function onDrop(e: DragEvent) {
    e.preventDefault();
    dragActive = false;
    const file = e.dataTransfer?.files?.[0];
    if (!file) return;
    if (!isPdf(file)) {
      resumeError = 'Please drop a PDF file.';
      return;
    }
    void analyze(file);
  }

  function onDragOver(e: DragEvent) {
    e.preventDefault();
    dragActive = true;
  }

  function onDragLeave(e: DragEvent) {
    e.preventDefault();
    dragActive = false;
  }
</script>

<svelte:head>
  <title>Résumé verdict — freehire</title>
  <!-- Personal page: keep it out of search results. -->
  <meta name="robots" content="noindex" />
</svelte:head>

<div class="mx-auto w-full max-w-3xl px-4 py-6">
  {#if !isAuthenticated()}
    <div class="flex flex-col items-center gap-3 py-12 text-center">
      <p class="text-sm text-muted-foreground">Sign in to see your verdict.</p>
      <Button variant="primary" onclick={() => openAuthDialog()}>Sign in</Button>
    </div>
  {:else if status === 'loading'}
    <States state="loading" />
  {:else if status === 'error'}
    <States state="error" message="Couldn't load the verdict." />
  {:else if profile === undefined || verdict === null}
    <div class="flex flex-col items-center gap-3 py-12 text-center">
      <p class="text-sm text-muted-foreground">This profile no longer exists.</p>
      <Button variant="primary" href={resolve('/my/profiles')}>Back to profiles</Button>
    </div>
  {:else}
    <div class="flex flex-col gap-8">
      <VerdictView {verdict} name={profile.name} {role} />

      <!-- Résumé coherence: upload once to add the Coherence Score + gap advice. -->
      <div class="flex flex-col gap-2">
        <h2 class="text-sm font-semibold uppercase tracking-[0.14em] text-muted-foreground">
          {verdict.coherence == null ? 'Add a coherence check' : 'Refresh the coherence check'}
        </h2>
        <input
          type="file"
          accept="application/pdf,.pdf"
          class="hidden"
          bind:this={fileInput}
          onchange={onFile}
        />
        <button
          type="button"
          onclick={() => fileInput?.click()}
          ondragover={onDragOver}
          ondragleave={onDragLeave}
          ondrop={onDrop}
          disabled={resumeBusy}
          class="flex flex-col items-center justify-center gap-3 rounded-xl border-2 border-dashed px-6 py-8 text-center transition-colors disabled:opacity-70 {dragActive
            ? 'border-primary bg-primary/5'
            : 'border-border hover:border-primary/60'}"
        >
          <span class="flex size-11 items-center justify-center rounded-full bg-primary text-primary-foreground">
            <ArrowUp class="size-5" />
          </span>
          <span class="flex flex-col gap-0.5">
            <span class="text-sm font-semibold">{resumeBusy ? 'Analyzing…' : 'Drop your résumé PDF here'}</span>
            {#if !resumeBusy}
              <span class="text-xs text-muted-foreground">
                or <span class="text-primary underline">choose from disk</span>
              </span>
            {/if}
          </span>
          {#if fileName}
            <span class="text-xs text-primary">✓ {fileName}</span>
          {/if}
        </button>
        <span class="text-xs text-muted-foreground">
          Checks whether your Experience backs up your skills · parsed on the server and discarded.
        </span>
        {#if resumeError}
          <p class="text-sm text-destructive">{resumeError}</p>
        {/if}
      </div>
    </div>
  {/if}
</div>
