<script lang="ts">
  import { ApiError } from '$lib/api';
  import { isAuthenticated } from '$lib/auth.svelte';
  import { openAuthDialog } from '$lib/auth-dialog.svelte';
  import { savedSearches } from '$lib/savedSearches.svelte';
  import type { SavedSearch } from '$lib/types';
  import { Button, Input } from '$lib/ui';
  import States from './States.svelte';

  let status = $state<'loading' | 'error' | 'ready'>('loading');
  const items = $derived(savedSearches.items);

  // Share flow: clicking "Share" on a row reveals an optional author-label input for that
  // row (an inline per-row edit id); confirming publishes the board.
  let shareEditId = $state<number | null>(null);
  let authorLabel = $state('');
  let busyId = $state<number | null>(null);
  let error = $state<string | null>(null);
  // The row whose link was just copied, to flip its button label briefly.
  let copiedId = $state<number | null>(null);

  async function load() {
    status = 'loading';
    try {
      await savedSearches.ensureLoaded();
      status = 'ready';
    } catch {
      status = 'error';
    }
  }

  // Load once the session is confirmed; reset the per-user cache on sign-out so a different
  // user does not see the previous one's searches.
  $effect(() => {
    if (isAuthenticated()) {
      void load();
    } else {
      savedSearches.reset();
    }
  });

  // The public board URL for a shared set. Browser-only (uses location.origin); the list
  // renders after auth on the client, so origin is always available here.
  function boardUrl(slug: string): string {
    return `${location.origin}/b/${slug}`;
  }

  function startShare(s: SavedSearch) {
    shareEditId = s.id;
    authorLabel = s.author_label;
    error = null;
  }

  async function confirmShare(id: number) {
    busyId = id;
    error = null;
    try {
      await savedSearches.share(id, authorLabel.trim());
      shareEditId = null;
      authorLabel = '';
    } catch (err) {
      error = err instanceof ApiError ? err.message : 'Could not share this search. Please try again.';
    } finally {
      busyId = null;
    }
  }

  async function unshare(id: number) {
    busyId = id;
    error = null;
    try {
      await savedSearches.unshare(id);
    } catch {
      error = 'Could not unshare this search. Please try again.';
    } finally {
      busyId = null;
    }
  }

  async function rename(s: SavedSearch) {
    const next = window.prompt('Rename saved search', s.name)?.trim();
    if (!next || next === s.name) return;
    error = null;
    try {
      await savedSearches.update(s.id, { name: next });
    } catch (err) {
      error = err instanceof ApiError ? err.message : 'Could not rename this search. Please try again.';
    }
  }

  async function remove(s: SavedSearch) {
    if (!window.confirm(`Delete saved search “${s.name}”?`)) return;
    error = null;
    try {
      await savedSearches.remove(s.id);
    } catch {
      error = 'Could not delete this search. Please try again.';
    }
  }

  async function copyLink(s: SavedSearch) {
    try {
      await navigator.clipboard.writeText(boardUrl(s.public_slug));
      copiedId = s.id;
      setTimeout(() => {
        if (copiedId === s.id) copiedId = null;
      }, 1500);
    } catch {
      error = 'Could not copy the link.';
    }
  }
</script>

{#if !isAuthenticated()}
  <div class="flex flex-col items-center gap-3 py-12 text-center">
    <p class="text-sm text-muted-foreground">Sign in to manage your saved searches.</p>
    <Button variant="primary" onclick={() => openAuthDialog()}>Sign in</Button>
  </div>
{:else}
  <div class="flex flex-col gap-6">
    <div class="flex flex-col gap-1">
      <h1 class="text-2xl font-semibold tracking-tight">Saved searches</h1>
      <p class="text-sm text-muted-foreground">
        Reuse a saved filter set, or share it as a public board anyone can open. Create new
        saved searches from the filters panel on the jobs page.
      </p>
    </div>

    {#if error}
      <p class="text-sm text-destructive">{error}</p>
    {/if}

    {#if status === 'loading'}
      <States state="loading" />
    {:else if status === 'error'}
      <States state="error" message="Couldn't load your saved searches." />
    {:else if items.length === 0}
      <States
        state="empty"
        message="No saved searches yet. Save a filter set from the jobs page to see it here."
      />
    {:else}
      <ul class="flex flex-col divide-y divide-border rounded-lg border border-border">
        {#each items as s (s.id)}
          <li class="flex flex-col gap-2 px-4 py-3">
            <div class="flex items-start justify-between gap-3">
              <div class="flex min-w-0 flex-col gap-0.5">
                <span class="truncate text-sm font-medium">{s.name}</span>
                <span class="text-xs text-muted-foreground">
                  {s.query === '' ? 'All jobs' : 'Custom filters'}
                  {#if s.public_slug}· <span class="text-primary">Shared</span>{/if}
                </span>
              </div>
              <div class="flex shrink-0 flex-wrap items-center justify-end gap-1">
                <Button variant="ghost" size="sm" href={`/jobs?${s.query}`}>Open</Button>
                <Button variant="ghost" size="sm" onclick={() => rename(s)}>Rename</Button>
                {#if s.public_slug}
                  <Button
                    variant="ghost"
                    size="sm"
                    disabled={busyId === s.id}
                    onclick={() => unshare(s.id)}
                  >
                    Unshare
                  </Button>
                {:else}
                  <Button variant="ghost" size="sm" onclick={() => startShare(s)}>Share</Button>
                {/if}
                <Button variant="ghost" size="sm" onclick={() => remove(s)}>Delete</Button>
              </div>
            </div>

            {#if s.public_slug}
              <!-- Shared: show the public link and (when set) the author label. -->
              <div class="flex flex-wrap items-center gap-2 rounded bg-secondary/50 px-2 py-1.5">
                <a
                  href={`/b/${s.public_slug}`}
                  class="min-w-0 truncate text-xs text-primary underline-offset-4 hover:underline"
                >
                  /b/{s.public_slug}
                </a>
                {#if s.author_label}
                  <span class="text-xs text-muted-foreground">by {s.author_label}</span>
                {/if}
                <Button variant="ghost" size="sm" class="ml-auto" onclick={() => copyLink(s)}>
                  {copiedId === s.id ? 'Copied' : 'Copy link'}
                </Button>
              </div>
            {:else if shareEditId === s.id}
              <!-- Private + sharing: optional author label, then confirm. -->
              <div class="flex flex-wrap items-center gap-2">
                <Input
                  bind:value={authorLabel}
                  placeholder="Author label (optional)"
                  maxlength={60}
                  class="w-56"
                />
                <Button
                  variant="primary"
                  size="sm"
                  disabled={busyId === s.id}
                  onclick={() => confirmShare(s.id)}
                >
                  {busyId === s.id ? 'Sharing…' : 'Create board'}
                </Button>
                <Button variant="ghost" size="sm" onclick={() => (shareEditId = null)}>Cancel</Button>
              </div>
            {/if}
          </li>
        {/each}
      </ul>
    {/if}
  </div>
{/if}
