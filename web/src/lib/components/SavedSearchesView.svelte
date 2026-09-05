<script lang="ts">
  import { Pencil, Trash2, Check } from '@lucide/svelte';
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import { ApiError } from '$lib/api';
  import { isAuthenticated } from '$lib/auth.svelte';
  import { signinUrl } from '$lib/signin';
  import { savedSearches } from '$lib/savedSearches.svelte';
  import { notifications } from '$lib/notifications.svelte';
  import { profileStore } from '$lib/profile.svelte';
  import type { SavedSearch } from '$lib/types';
  import { Button, ConfirmDialog } from '$lib/ui';
  import { toSearchString } from '$lib/urlSearchString';
  import { ProviderIcon } from '$lib/ui';
  import AlertChannels from './filters/AlertChannels.svelte';
  import ProfileAlertToggle from './ProfileAlertToggle.svelte';
  import States from './States.svelte';

  // The account page for saved searches and their alerts: the Telegram connection at
  // the top, then the built-in "notify me about jobs matching my profile" toggle (needs
  // a candidate profile to derive filters from — hidden for an account with none), then
  // each saved search as a card with its actions (open / rename / delete) and its
  // per-channel alert toggles (the shared AlertChannels). Merges the former separate
  // Notifications page — a subscription is always tied to a saved search, so it's
  // managed per-row. Public sharing (formerly "boards") is retired in favor of job
  // lists, which share specific jobs rather than a live query.

  let status = $state<'loading' | 'error' | 'ready'>('loading');
  const items = $derived(savedSearches.items);
  const profile = $derived(profileStore.profile);

  // Telegram connection status. Connecting/disconnecting the bot itself lives on
  // Integrations (/my/integrations) now — this page only reads the status to show
  // beside the per-search toggles below.
  const telegram = $derived(notifications.telegram);

  let error = $state<string | null>(null);

  async function load() {
    status = 'loading';
    try {
      await Promise.all([
        savedSearches.ensureLoaded(),
        notifications.ensureLoaded(),
        profileStore.ensureLoaded(),
      ]);
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

  // The stored query may carry a %2C from URLSearchParams' own encoding (see
  // urlSearchString.ts); normalize it back to a literal comma for the "Open" link
  // so a multi-value facet reads the same compact way it does everywhere else.
  function openHref(query: string): string {
    return `/jobs?${toSearchString(new URLSearchParams(query))}`;
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

  let removeTarget = $state<SavedSearch | null>(null);
  let confirmRemoveOpen = $state(false);

  function requestRemove(s: SavedSearch) {
    removeTarget = s;
    confirmRemoveOpen = true;
  }

  async function remove() {
    const s = removeTarget;
    if (!s) return;
    error = null;
    try {
      await savedSearches.remove(s.id);
    } catch {
      error = 'Could not delete this search. Please try again.';
    }
  }
</script>

{#if !isAuthenticated()}
  <div class="flex flex-col items-center gap-3 py-12 text-center">
    <p class="text-sm text-muted-foreground">Sign in to manage your saved searches.</p>
    <Button variant="primary" href={signinUrl({ returnTo: page.url.pathname + page.url.search, mode: 'login' })}>Sign in</Button>
  </div>
{:else}
  <div class="flex flex-col gap-6">
    <div class="flex flex-col gap-1">
      <h1 class="text-2xl font-semibold tracking-tight">Saved searches &amp; alerts</h1>
      <p class="text-sm text-muted-foreground">
        Each saved set of filters can send you its new jobs — in Telegram, by email, or both.
        Reuse one anytime. Create new saved searches from the filters panel on the jobs page.
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
      <!-- Telegram connection: status only, pointing to Integrations for connect/disconnect. -->
      {#if telegram.enabled}
        <section class="flex items-center gap-3 rounded-xl border border-border p-4">
          <div class="flex size-10 shrink-0 items-center justify-center rounded-full bg-secondary text-foreground">
            <ProviderIcon provider="telegram" />
          </div>
          <div class="min-w-0 flex-1">
            <h2 class="text-sm font-semibold tracking-tight">Telegram</h2>
            {#if telegram.linked}
              <p class="flex items-center gap-1 text-xs font-medium text-green-600">
                <Check class="size-3.5" aria-hidden="true" /> Connected
              </p>
            {:else}
              <p class="text-xs text-muted-foreground">Connect to receive your job alerts here.</p>
            {/if}
          </div>
          <Button variant="secondary" size="sm" class="shrink-0" href={resolve('/my/integrations')}>
            {telegram.linked ? 'Manage in Integrations' : 'Connect in Integrations'}
          </Button>
        </section>
      {/if}

      {#if profile}
        <ProfileAlertToggle {profile} />
      {/if}

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
                </span>
              </div>
              <div class="flex shrink-0 items-center gap-1">
                <Button variant="secondary" size="sm" href={openHref(s.query)}>Open</Button>
                <button
                  type="button"
                  aria-label="Rename “{s.name}”"
                  title="Rename"
                  onclick={() => rename(s)}
                  class="flex size-8 items-center justify-center rounded-lg text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
                >
                  <Pencil class="size-4" />
                </button>
                <button
                  type="button"
                  aria-label="Delete “{s.name}”"
                  title="Delete"
                  onclick={() => requestRemove(s)}
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
          </article>
        {/each}
        </div>
      {/if}
    {/if}
  </div>

  <ConfirmDialog
    bind:open={confirmRemoveOpen}
    title={`Delete saved search “${removeTarget?.name ?? ''}”?`}
    confirmLabel="Delete"
    variant="destructive"
    onConfirm={remove}
  />
{/if}
