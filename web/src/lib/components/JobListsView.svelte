<script lang="ts">
  import { AlignLeft, Pencil, Share2, Trash2 } from '@lucide/svelte';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { ApiError } from '$lib/api';
  import { isAuthenticated } from '$lib/auth.svelte';
  import { signinUrl } from '$lib/signin';
  import { jobLists } from '$lib/jobLists.svelte';
  import type { JobList } from '$lib/types';
  import { Button, ConfirmDialog, Input } from '$lib/ui';
  import States from './States.svelte';

  // The account page for job lists: create a named list, rename it, edit its
  // description, publish/unpublish it as a public read-only page, and delete it.
  // Adding/removing specific jobs happens from the job card's "Add to list" control,
  // not here — this page manages the lists themselves.

  let status = $state<'loading' | 'error' | 'ready'>('loading');
  const items = $derived(jobLists.items);
  let error = $state<string | null>(null);

  async function load() {
    status = 'loading';
    try {
      await jobLists.ensureLoaded();
      status = 'ready';
    } catch {
      status = 'error';
    }
  }

  // Load once the session is confirmed; reset the per-user cache on sign-out so a
  // different user does not see the previous one's lists.
  $effect(() => {
    if (isAuthenticated()) {
      void load();
    } else {
      jobLists.reset();
    }
  });

  // Create flow: a small inline form, collapsed by default.
  let creating = $state(false);
  let newName = $state('');
  let newDescription = $state('');
  let createBusy = $state(false);

  function startCreate() {
    creating = true;
    newName = '';
    newDescription = '';
    error = null;
  }

  async function confirmCreate() {
    const name = newName.trim();
    if (!name) return;
    createBusy = true;
    error = null;
    try {
      await jobLists.create(name, newDescription.trim());
      creating = false;
    } catch (err) {
      error = err instanceof ApiError ? err.message : 'Could not create this list. Please try again.';
    } finally {
      createBusy = false;
    }
  }

  async function rename(l: JobList) {
    const next = window.prompt('Rename job list', l.name)?.trim();
    if (!next || next === l.name) return;
    error = null;
    try {
      await jobLists.update(l.id, { name: next });
    } catch (err) {
      error = err instanceof ApiError ? err.message : 'Could not rename this list. Please try again.';
    }
  }

  async function editDescription(l: JobList) {
    const next = window.prompt('Edit description', l.description);
    if (next === null || next === l.description) return;
    error = null;
    try {
      await jobLists.update(l.id, { description: next.trim() });
    } catch (err) {
      error = err instanceof ApiError ? err.message : 'Could not update the description. Please try again.';
    }
  }

  let busyId = $state<number | null>(null);
  let copiedId = $state<number | null>(null);

  function listUrl(slug: string): string {
    return `${location.origin}${resolve('/l/[slug]', { slug })}`;
  }

  async function share(id: number) {
    busyId = id;
    error = null;
    try {
      await jobLists.share(id);
    } catch (err) {
      error = err instanceof ApiError ? err.message : 'Could not share this list. Please try again.';
    } finally {
      busyId = null;
    }
  }

  async function unshare(id: number) {
    busyId = id;
    error = null;
    try {
      await jobLists.unshare(id);
    } catch {
      error = 'Could not unshare this list. Please try again.';
    } finally {
      busyId = null;
    }
  }

  async function copyLink(l: JobList) {
    try {
      await navigator.clipboard.writeText(listUrl(l.public_slug));
      copiedId = l.id;
      setTimeout(() => {
        if (copiedId === l.id) copiedId = null;
      }, 1500);
    } catch {
      error = 'Could not copy the link.';
    }
  }

  let removeTarget = $state<JobList | null>(null);
  let confirmRemoveOpen = $state(false);

  function requestRemove(l: JobList) {
    removeTarget = l;
    confirmRemoveOpen = true;
  }

  async function remove() {
    const l = removeTarget;
    if (!l) return;
    error = null;
    try {
      await jobLists.remove(l.id);
    } catch {
      error = 'Could not delete this list. Please try again.';
    }
  }
</script>

{#if !isAuthenticated()}
  <div class="flex flex-col items-center gap-3 py-12 text-center">
    <p class="text-sm text-muted-foreground">Sign in to manage your job lists.</p>
    <Button variant="primary" href={signinUrl({ returnTo: page.url.pathname + page.url.search, mode: 'login' })}>Sign in</Button>
  </div>
{:else}
  <div class="flex flex-col gap-6">
    <div class="flex flex-col gap-1">
      <h1 class="text-2xl font-semibold tracking-tight">Job lists</h1>
      <p class="text-sm text-muted-foreground">
        Group specific jobs into named lists — independent of the "Save" star — and
        optionally share one as a public, read-only page.
      </p>
    </div>

    {#if error}
      <p class="text-sm text-destructive">{error}</p>
    {/if}

    {#if status === 'loading'}
      <States state="loading" />
    {:else if status === 'error'}
      <States state="error" message="Couldn't load your job lists." />
    {:else}
      {#if creating}
        <div class="flex flex-col gap-2 rounded-xl border border-border p-4">
          <Input bind:value={newName} placeholder="List name" maxlength={100} />
          <Input bind:value={newDescription} placeholder="Description (optional)" maxlength={2000} />
          <div class="flex items-center gap-2">
            <Button variant="primary" size="sm" disabled={createBusy || !newName.trim()} onclick={confirmCreate}>
              {createBusy ? 'Creating…' : 'Create list'}
            </Button>
            <Button variant="ghost" size="sm" onclick={() => (creating = false)}>Cancel</Button>
          </div>
        </div>
      {:else}
        <Button variant="secondary" size="sm" class="self-start" onclick={startCreate}>New list</Button>
      {/if}

      {#if items.length === 0}
        <States
          state="empty"
          message="No job lists yet. Create one, or add a job to a new list from its card."
        />
      {:else}
        <div class="flex flex-col gap-3">
        {#each items as l (l.id)}
          <article class="flex flex-col rounded-xl border border-border p-4 transition-colors hover:border-muted-foreground/30">
            <div class="flex items-start gap-3">
              <div class="flex min-w-0 flex-1 flex-col gap-0.5">
                <span class="truncate text-sm font-medium">{l.name}</span>
                <span class="text-xs text-muted-foreground">
                  {l.job_count} {l.job_count === 1 ? 'job' : 'jobs'}
                  {#if l.public_slug}· <span class="font-medium text-brand-strong">Shared</span>{/if}
                </span>
                {#if l.description}
                  <span class="mt-1 text-xs text-muted-foreground">{l.description}</span>
                {/if}
              </div>
              <div class="flex shrink-0 items-center gap-1">
                <button
                  type="button"
                  aria-label="Rename “{l.name}”"
                  title="Rename"
                  onclick={() => rename(l)}
                  class="flex size-8 items-center justify-center rounded-lg text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
                >
                  <Pencil class="size-4" />
                </button>
                <button
                  type="button"
                  aria-label="Edit description of “{l.name}”"
                  title="Edit description"
                  onclick={() => editDescription(l)}
                  class="flex size-8 items-center justify-center rounded-lg text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
                >
                  <AlignLeft class="size-4" />
                </button>
                {#if !l.public_slug}
                  <button
                    type="button"
                    aria-label="Share “{l.name}”"
                    title="Share as a public page"
                    disabled={busyId === l.id}
                    onclick={() => share(l.id)}
                    class="flex size-8 items-center justify-center rounded-lg text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
                  >
                    <Share2 class="size-4" />
                  </button>
                {/if}
                <button
                  type="button"
                  aria-label="Delete “{l.name}”"
                  title="Delete"
                  onclick={() => requestRemove(l)}
                  class="flex size-8 items-center justify-center rounded-lg text-muted-foreground transition-colors hover:bg-destructive/10 hover:text-destructive"
                >
                  <Trash2 class="size-4" />
                </button>
              </div>
            </div>

            {#if l.public_slug}
              <!-- Shared: the public link, copy, and unshare. -->
              <div class="mt-3 flex flex-wrap items-center gap-2 rounded-lg bg-secondary/50 px-3 py-2">
                <a
                  href={resolve('/l/[slug]', { slug: l.public_slug })}
                  class="min-w-0 truncate text-xs text-brand-strong underline-offset-4 hover:underline"
                >
                  /l/{l.public_slug}
                </a>
                <Button variant="ghost" size="sm" class="ml-auto" onclick={() => copyLink(l)}>
                  {copiedId === l.id ? 'Copied' : 'Copy link'}
                </Button>
                <Button variant="ghost" size="sm" disabled={busyId === l.id} onclick={() => unshare(l.id)}>
                  Unshare
                </Button>
              </div>
            {/if}
          </article>
        {/each}
        </div>
      {/if}
    {/if}
  </div>

  <ConfirmDialog
    bind:open={confirmRemoveOpen}
    title={`Delete job list “${removeTarget?.name ?? ''}”?`}
    confirmLabel="Delete"
    variant="destructive"
    onConfirm={remove}
  />
{/if}
