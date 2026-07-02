<script lang="ts">
  import { ArrowUp, FileText, RefreshCw, Trash2 } from '@lucide/svelte';
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import {
    ApiError,
    analyzeProfileResume,
    deleteResume,
    getProfileVerdict,
    getResumeStatus,
    putResume,
    rerunProfileCoherence,
  } from '$lib/api';
  import { isAuthenticated } from '$lib/auth.svelte';
  import { openAuthDialog } from '$lib/auth-dialog.svelte';
  import { categoryLabel } from '$lib/facets';
  import States from '$lib/components/States.svelte';
  import VerdictView from '$lib/components/VerdictView.svelte';
  import { searchProfiles } from '$lib/searchProfiles.svelte';
  import type { ResumeStatus, Verdict } from '$lib/types';
  import { Button } from '$lib/ui';
  import { formatDate } from '$lib/utils';

  const id = $derived(Number(page.params.id));
  const profile = $derived(searchProfiles.items.find((p) => p.id === id));
  const role = $derived(profile ? profile.specializations.map(categoryLabel).join(', ') : '');

  let status = $state<'loading' | 'error' | 'ready'>('loading');
  let verdict = $state<Verdict | null>(null);
  // Résumé storage status: drives whether we show "re-run from storage" (stored once) or
  // a single upload prompt, and — when storage is unconfigured — the degraded per-request
  // upload path.
  let resume = $state<ResumeStatus | null>(null);

  // Résumé action state (upload/re-run/delete).
  let resumeBusy = $state(false);
  let resumeError = $state<string | null>(null);
  let dragActive = $state(false);
  let fileInput = $state<HTMLInputElement | null>(null);

  async function load() {
    status = 'loading';
    try {
      await searchProfiles.ensureLoaded();
      [verdict, resume] = await Promise.all([getProfileVerdict(id), getResumeStatus()]);
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

  function errText(err: unknown): string {
    return err instanceof ApiError ? err.message : 'Something went wrong. Please try again.';
  }

  // Store (or replace) the résumé once, then score coherence from it — the single upload
  // point when storage is configured.
  async function storeAndScore(file: File) {
    resumeBusy = true;
    resumeError = null;
    try {
      resume = await putResume(file);
      verdict = await rerunProfileCoherence(id);
    } catch (err) {
      resumeError = errText(err);
    } finally {
      resumeBusy = false;
    }
  }

  // Re-run coherence against the already-stored résumé — no upload.
  async function rerun() {
    resumeBusy = true;
    resumeError = null;
    try {
      verdict = await rerunProfileCoherence(id);
    } catch (err) {
      resumeError = errText(err);
    } finally {
      resumeBusy = false;
    }
  }

  // Delete the stored résumé (keeps any coherence already on screen; the user can upload
  // again to refresh it).
  async function removeResume() {
    resumeBusy = true;
    resumeError = null;
    try {
      await deleteResume();
      resume = { enabled: true, present: false, uploaded_at: null };
    } catch (err) {
      resumeError = errText(err);
    } finally {
      resumeBusy = false;
    }
  }

  // Degraded path (storage unconfigured): analyze a per-request upload without storing.
  async function analyzeDegraded(file: File) {
    resumeBusy = true;
    resumeError = null;
    try {
      verdict = await analyzeProfileResume(id, file);
    } catch (err) {
      resumeError = errText(err);
    } finally {
      resumeBusy = false;
    }
  }

  // Route a chosen file to the right path: store-and-score when storage is on, otherwise
  // the degraded per-request analysis.
  function handleFile(file: File) {
    if (resume?.enabled) void storeAndScore(file);
    else void analyzeDegraded(file);
  }

  function onFile(e: Event) {
    const target = e.currentTarget as HTMLInputElement;
    const file = target.files?.[0];
    target.value = '';
    if (file) handleFile(file);
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
    handleFile(file);
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

      <!-- Résumé coherence: one upload, reused. When a résumé is stored we re-run against
           it (no re-upload); otherwise we prompt a single upload. With storage off we fall
           back to a per-request upload (parsed and discarded). -->
      <div class="flex flex-col gap-2">
        <h2 class="text-sm font-semibold uppercase tracking-[0.14em] text-muted-foreground">
          Résumé coherence
        </h2>

        {#if resume?.enabled && resume.present}
          <!-- Stored résumé: show it, re-run from it, replace or delete. -->
          <div class="flex flex-wrap items-center gap-3 rounded-xl border border-border px-4 py-3">
            <span class="flex size-9 items-center justify-center rounded-full bg-primary/10 text-primary">
              <FileText class="size-4" />
            </span>
            <span class="flex flex-col">
              <span class="text-sm font-semibold">Your résumé is stored</span>
              {#if resume.uploaded_at}
                <span class="text-xs text-muted-foreground">Uploaded {formatDate(resume.uploaded_at)}</span>
              {/if}
            </span>
            <div class="ml-auto flex items-center gap-2">
              <Button variant="primary" onclick={rerun} disabled={resumeBusy}>
                <RefreshCw class="size-4 {resumeBusy ? 'animate-spin' : ''}" />
                {resumeBusy ? 'Scoring…' : verdict.coherence == null ? 'Run coherence' : 'Re-run'}
              </Button>
              <Button variant="ghost" onclick={() => fileInput?.click()} disabled={resumeBusy}>
                Replace
              </Button>
              <Button variant="ghost" onclick={removeResume} disabled={resumeBusy}>
                <Trash2 class="size-4" />
              </Button>
            </div>
          </div>
          <input type="file" accept="application/pdf,.pdf" class="hidden" bind:this={fileInput} onchange={onFile} />
        {:else}
          <!-- No stored résumé (or storage off): a single upload prompt. -->
          <input type="file" accept="application/pdf,.pdf" class="hidden" bind:this={fileInput} onchange={onFile} />
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
          </button>
          <span class="text-xs text-muted-foreground">
            Checks whether your Experience backs up your skills · {resume?.enabled
              ? 'stored once so you can re-run without uploading again'
              : 'parsed on the server and discarded'}.
          </span>
        {/if}

        {#if resumeError}
          <p class="text-sm text-destructive">{resumeError}</p>
        {/if}
      </div>
    </div>
  {/if}
</div>
