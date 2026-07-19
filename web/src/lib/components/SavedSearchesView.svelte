<script lang="ts">
  import { Check, Pencil, Share2, Trash2 } from '@lucide/svelte';
  import { ApiError } from '$lib/api';
  import { isAuthenticated } from '$lib/auth.svelte';
  import { openAuthDialog } from '$lib/auth-dialog.svelte';
  import { savedSearches } from '$lib/savedSearches.svelte';
  import { notifications } from '$lib/notifications.svelte';
  import type { SavedSearch } from '$lib/types';
  import { Button, Input } from '$lib/ui';
  import ProviderIcon from './ProviderIcon.svelte';
  import AlertChannels from './filters/AlertChannels.svelte';
  import States from './States.svelte';

  // The account page for saved searches and their alerts: the Telegram connection at
  // the top, then each saved search as a card with its actions (open / rename / share /
  // delete) and its per-channel alert toggles (the shared AlertChannels). Merges the
  // former separate Notifications page — a subscription is always tied to a saved
  // search, so it's managed per-row.

  let status = $state<'loading' | 'error' | 'ready'>('loading');
  const items = $derived(savedSearches.items);

  // Telegram connection (moved here from the standalone notifications page).
  const telegram = $derived(notifications.telegram);
  let connecting = $state(false);
  let connectBusy = $state(false);

  async function connect() {
    connectBusy = true;
    error = null;
    try {
      const url = await notifications.link();
      window.open(url, '_blank', 'noopener');
      connecting = true;
    } catch (e) {
      error = e instanceof ApiError ? e.message : 'Could not start the connection. Please try again.';
    } finally {
      connectBusy = false;
    }
  }

  async function recheckLink() {
    connectBusy = true;
    error = null;
    try {
      await notifications.refreshTelegram();
      if (notifications.telegram.linked) connecting = false;
    } catch {
      error = 'Could not check the connection. Please try again.';
    } finally {
      connectBusy = false;
    }
  }

  async function disconnect() {
    if (!window.confirm('Disconnect Telegram? You will stop receiving alerts.')) return;
    connectBusy = true;
    error = null;
    try {
      await notifications.unlink();
    } catch (e) {
      error = e instanceof ApiError ? e.message : 'Could not disconnect. Please try again.';
    } finally {
      connectBusy = false;
    }
  }

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
      await Promise.all([savedSearches.ensureLoaded(), notifications.ensureLoaded()]);
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
      <h1 class="text-2xl font-semibold tracking-tight">Saved searches &amp; alerts</h1>
      <p class="text-sm text-muted-foreground">
        Each saved set of filters can send you its new jobs — in Telegram, by email, or both.
        Reuse one anytime, or share it as a public board. Create new saved searches from the
        filters panel on the jobs page.
      </p>
    </div>

    {#if error}
      <p class="text-sm text-destructive">{error}</p>
    {/if}

    {#if status === 'loading'}
      <States state="loading" />
    {:else if status === 'error'}
      <States state="error" message="Couldn't load your saved searches." />
    {:else}
      <!-- Telegram connection -->
      <section class="flex items-center gap-3 rounded-xl border border-border p-4">
        <div class="flex size-10 shrink-0 items-center justify-center rounded-full bg-secondary text-foreground">
          <ProviderIcon provider="telegram" />
        </div>
        <div class="min-w-0 flex-1">
          <h2 class="text-sm font-semibold tracking-tight">Telegram</h2>
          {#if !telegram.enabled}
            <p class="text-xs text-muted-foreground">Not available on this server yet.</p>
          {:else if telegram.linked}
            <p class="flex items-center gap-1 text-xs font-medium text-green-600">
              <Check class="size-3.5" aria-hidden="true" /> Connected
            </p>
          {:else if connecting}
            <p class="text-xs text-muted-foreground">Tap “Start” in the bot, then confirm.</p>
          {:else}
            <p class="text-xs text-muted-foreground">Connect to receive your job alerts here.</p>
          {/if}
        </div>
        {#if telegram.enabled}
          {#if telegram.linked}
            <Button variant="ghost" size="sm" class="shrink-0" onclick={disconnect} disabled={connectBusy}>
              Disconnect
            </Button>
          {:else if connecting}
            <Button variant="secondary" size="sm" class="shrink-0" onclick={recheckLink} disabled={connectBusy}>
              {connectBusy ? 'Checking…' : 'I’ve connected'}
            </Button>
          {:else}
            <Button variant="primary" size="sm" class="shrink-0" onclick={connect} disabled={connectBusy}>
              Connect
            </Button>
          {/if}
        {/if}
      </section>

      {#if items.length === 0}
        <States
          state="empty"
          message="No saved searches yet. Save a filter set from the jobs page to see it here."
        />
      {:else}
        <div class="flex flex-col gap-3">
        {#each items as s (s.id)}
          <article class="flex flex-col rounded-xl border border-border p-4 transition-colors hover:border-muted-foreground/30">
            <div class="flex items-start gap-3">
              <div class="flex min-w-0 flex-1 flex-col gap-0.5">
                <span class="truncate text-sm font-medium">{s.name}</span>
                <span class="text-xs text-muted-foreground">
                  {s.query === '' ? 'All jobs' : 'Custom filters'}
                  {#if s.public_slug}· <span class="font-medium text-brand-strong">Shared</span>{/if}
                </span>
              </div>
              <div class="flex shrink-0 items-center gap-1">
                <Button variant="secondary" size="sm" href={`/?${s.query}`}>Open</Button>
                <button
                  type="button"
                  aria-label="Rename “{s.name}”"
                  title="Rename"
                  onclick={() => rename(s)}
                  class="flex size-8 items-center justify-center rounded-lg text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
                >
                  <Pencil class="size-4" />
                </button>
                {#if !s.public_slug}
                  <button
                    type="button"
                    aria-label="Share “{s.name}”"
                    title="Share as a public board"
                    onclick={() => startShare(s)}
                    class="flex size-8 items-center justify-center rounded-lg text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
                  >
                    <Share2 class="size-4" />
                  </button>
                {/if}
                <button
                  type="button"
                  aria-label="Delete “{s.name}”"
                  title="Delete"
                  onclick={() => remove(s)}
                  class="flex size-8 items-center justify-center rounded-lg text-muted-foreground transition-colors hover:bg-destructive/10 hover:text-destructive"
                >
                  <Trash2 class="size-4" />
                </button>
              </div>
            </div>

            <!-- Per-search alerts: the shared toggle chips (Telegram / Email). -->
            <div class="mt-3 border-t border-dashed border-border pt-3">
              <AlertChannels savedSearchId={s.id} />
            </div>

            {#if s.public_slug}
              <!-- Shared: the public link, its author label, copy, and unshare. -->
              <div class="mt-3 flex flex-wrap items-center gap-2 rounded-lg bg-secondary/50 px-3 py-2">
                <a
                  href={`/b/${s.public_slug}`}
                  class="min-w-0 truncate text-xs text-brand-strong underline-offset-4 hover:underline"
                >
                  /b/{s.public_slug}
                </a>
                {#if s.author_label}
                  <span class="text-xs text-muted-foreground">by {s.author_label}</span>
                {/if}
                <Button variant="ghost" size="sm" class="ml-auto" onclick={() => copyLink(s)}>
                  {copiedId === s.id ? 'Copied' : 'Copy link'}
                </Button>
                <Button variant="ghost" size="sm" disabled={busyId === s.id} onclick={() => unshare(s.id)}>
                  Unshare
                </Button>
              </div>
            {:else if shareEditId === s.id}
              <!-- Private + sharing: optional author label, then confirm. -->
              <div class="mt-3 flex flex-wrap items-center gap-2">
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
          </article>
        {/each}
        </div>
      {/if}
    {/if}
  </div>
{/if}
