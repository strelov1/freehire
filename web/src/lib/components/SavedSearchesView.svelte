<script lang="ts">
  import { Check, Mail } from '@lucide/svelte';
  import { ApiError } from '$lib/api';
  import { isAuthenticated } from '$lib/auth.svelte';
  import { openAuthDialog } from '$lib/auth-dialog.svelte';
  import { savedSearches } from '$lib/savedSearches.svelte';
  import { notifications } from '$lib/notifications.svelte';
  import type { SavedSearch } from '$lib/types';
  import { Button, Input } from '$lib/ui';
  import ProviderIcon from './ProviderIcon.svelte';
  import SaveSearchAlert from './filters/SaveSearchAlert.svelte';
  import States from './States.svelte';

  // The account page for saved searches and their alerts: the Telegram connection at
  // the top, then each saved search with its share/board controls and its own Telegram
  // alert toggle (the shared SaveSearchAlert). Merges the former separate Notifications
  // page — a subscription is always tied to a saved search, so it's managed per-row.

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
  // The row whose email alert is being toggled.
  let emailBusyId = $state<number | null>(null);

  // Toggle the email alert for an (already saved) search: subscribe on the account
  // email, or unsubscribe if it is already on. No linking step — unlike Telegram,
  // email delivers to the account address with no connect flow.
  async function toggleEmail(s: SavedSearch) {
    emailBusyId = s.id;
    error = null;
    try {
      const existing = notifications.forSavedSearch(s.id, 'email');
      if (existing) await notifications.unsubscribe(existing.id);
      else await notifications.subscribe(s.id, 'email');
    } catch (e) {
      error = e instanceof ApiError ? e.message : 'Could not update the email alert. Please try again.';
    } finally {
      emailBusyId = null;
    }
  }

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
        Reuse a saved filter set, turn on its Telegram or email alert, or share it as a public
        board anyone can open. Create new saved searches from the filters panel on the jobs page.
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
        <ul class="flex flex-col divide-y divide-border rounded-lg border border-border">
        {#each items as s (s.id)}
          {@const emailSub = notifications.forSavedSearch(s.id, 'email')}
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
                <Button variant="ghost" size="sm" href={`/?${s.query}`}>Open</Button>
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

            <!-- Per-search Telegram alert. The search is already saved here, so the
                 shared control shows the alert offer / subscribed state (with turn-off). -->
            <div class="flex">
              <SaveSearchAlert query={s.query} variant="full" />
            </div>

            <!-- Per-search email alert. No linking step — email goes to the account
                 address — so this is a plain subscribe/turn-off toggle. -->
            <div class="flex flex-col gap-1.5">
              {#if emailSub}
                <p class="flex items-center gap-1.5 text-sm">
                  <Check class="size-4 text-primary" aria-hidden="true" />
                  <span>You’ll get new jobs by email</span>
                </p>
                <button
                  type="button"
                  onclick={() => toggleEmail(s)}
                  disabled={emailBusyId === s.id}
                  class="self-start text-xs font-medium text-muted-foreground transition-colors hover:text-foreground disabled:opacity-50"
                >
                  Turn off
                </button>
              {:else}
                <Button
                  variant="secondary"
                  size="sm"
                  onclick={() => toggleEmail(s)}
                  disabled={emailBusyId === s.id}
                  class="justify-center gap-1.5 self-start"
                >
                  <Mail class="size-4" aria-hidden="true" />
                  {emailBusyId === s.id ? 'Setting up…' : 'Email me new jobs'}
                </Button>
              {/if}
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
    {/if}
  </div>
{/if}
