<script lang="ts">
  import { Bell } from '@lucide/svelte';
  import { resolve } from '$app/paths';
  import { ApiError } from '#lib/api.js';
  import { isAuthenticated } from '#lib/auth.svelte.js';
  import { savedSearches } from '#lib/savedSearches.svelte.js';
  import { notifications } from '#lib/notifications.svelte.js';
  import { ensureSaved, matchedSavedSearch } from '#lib/saveSearchAlert.js';
  import { Button } from '#lib/ui/index.js';
  import AlertChannels from './filters/AlertChannels.svelte';

  // The watchlist's alert is the saved search `watchlist=true`: the server resolves it to
  // the owner's list, so subscribing it to a channel mails or messages the new postings of
  // every watched company as one digest section. Creating it is the saved-search save;
  // the channel choice is the same control every saved search uses.
  const QUERY = 'watchlist=true';

  let busy = $state(false);
  let error = $state<string | null>(null);

  $effect(() => {
    if (isAuthenticated()) {
      void savedSearches.ensureLoaded();
      void notifications.ensureLoaded();
    }
  });

  const matched = $derived(matchedSavedSearch(QUERY, savedSearches.items));
  const subscribed = $derived(notifications.subscriptions.some((s) => s.saved_search_id === matched?.id));

  async function create() {
    if (busy) return;
    busy = true;
    error = null;
    try {
      await ensureSaved(QUERY, savedSearches);
    } catch (e) {
      error = e instanceof ApiError ? e.message : 'Could not create the alert. Please try again.';
    } finally {
      busy = false;
    }
  }
</script>

<section class="flex flex-col gap-3 rounded-2xl border border-border bg-card p-5" aria-labelledby="watchlist-alerts">
  <div class="flex flex-col gap-1">
    <h2 id="watchlist-alerts" class="flex items-center gap-2 text-base font-semibold tracking-tight">
      <Bell class="size-4" aria-hidden="true" /> Alerts
    </h2>
    <p class="text-sm text-muted-foreground">
      Get a digest of new jobs at the companies on your watchlist, as a section of its own.
    </p>
  </div>
  {#if matched}
    <AlertChannels savedSearchId={matched.id} showLabel={false} />
    <p class="text-sm text-muted-foreground">
      {subscribed ? 'You are subscribed.' : 'Pick a channel to start receiving the digest.'}
      <a href={resolve('/my/notifications/searches')} class="font-medium text-foreground underline underline-offset-2">
        Manage alerts
      </a>
    </p>
  {:else}
    <Button variant="primary" onclick={create} disabled={busy} class="self-start">
      <Bell class="size-4" aria-hidden="true" />
      {busy ? 'Creating…' : 'Alert me about new jobs'}
    </Button>
  {/if}
  {#if error}
    <p class="text-sm text-destructive">{error}</p>
  {/if}
</section>
