<script lang="ts">
  import { Bell, Check } from '@lucide/svelte';
  import { ApiError } from '$lib/api';
  import { isAuthenticated } from '$lib/auth.svelte';
  import { openAuthDialog } from '$lib/auth-dialog.svelte';
  import { notifications } from '$lib/notifications.svelte';
  import { Button } from '$lib/ui';
  import States from './States.svelte';

  // The notification settings page: manage the Telegram connection and the list of
  // filter subscriptions. Creating a subscription happens from the filters panel
  // (it needs a concrete saved search); here the user connects/disconnects
  // Telegram and pauses/resumes/removes existing subscriptions.

  let status = $state<'loading' | 'ready'>('loading');
  let error = $state<string | null>(null);
  let busyId = $state<number | null>(null);
  let connecting = $state(false);
  let connectBusy = $state(false);

  const telegram = $derived(notifications.telegram);
  const subs = $derived(notifications.subscriptions);

  $effect(() => {
    if (isAuthenticated()) {
      void notifications.ensureLoaded().then(() => (status = 'ready'));
    }
  });

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

  async function recheck() {
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
    if (!window.confirm('Disconnect Telegram? You will stop receiving notifications.')) return;
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

  async function toggleActive(id: number, active: boolean) {
    busyId = id;
    error = null;
    try {
      await notifications.setActive(id, active);
    } catch (e) {
      error = e instanceof ApiError ? e.message : 'Could not update. Please try again.';
    } finally {
      busyId = null;
    }
  }

  async function remove(id: number, name: string) {
    if (!window.confirm(`Remove notifications for “${name}”?`)) return;
    busyId = id;
    error = null;
    try {
      await notifications.unsubscribe(id);
    } catch (e) {
      error = e instanceof ApiError ? e.message : 'Could not remove. Please try again.';
    } finally {
      busyId = null;
    }
  }
</script>

<div class="flex flex-col gap-6">
  <div class="flex items-center gap-2">
    <Bell class="size-5" aria-hidden="true" />
    <h1 class="text-xl font-semibold tracking-tight">Notifications</h1>
  </div>

  {#if !isAuthenticated()}
    <p class="text-sm text-muted-foreground">
      <button type="button" class="font-medium text-primary hover:underline" onclick={() => openAuthDialog()}>
        Sign in
      </button>
      to manage notifications.
    </p>
  {:else if status === 'loading'}
    <States state="loading" />
  {:else}
    <!-- Telegram connection -->
    <section class="flex flex-col gap-2 rounded-lg border border-border p-4">
      <h2 class="text-sm font-semibold">Telegram</h2>
      {#if !telegram.enabled}
        <p class="text-sm text-muted-foreground">Telegram notifications aren’t available on this server yet.</p>
      {:else if telegram.linked}
        <p class="flex items-center gap-1.5 text-sm text-muted-foreground">
          <Check class="size-4 text-green-600" aria-hidden="true" /> Connected.
        </p>
        <Button variant="ghost" size="sm" class="self-start" onclick={disconnect} disabled={connectBusy}>
          Disconnect
        </Button>
      {:else if connecting}
        <p class="text-sm text-muted-foreground">Opened Telegram — tap “Start” in the bot, then:</p>
        <Button variant="secondary" size="sm" class="self-start" onclick={recheck} disabled={connectBusy}>
          {connectBusy ? 'Checking…' : 'I’ve connected'}
        </Button>
      {:else}
        <p class="text-sm text-muted-foreground">Connect Telegram to receive job alerts there.</p>
        <Button variant="secondary" size="sm" class="self-start" onclick={connect} disabled={connectBusy}>
          Connect Telegram
        </Button>
      {/if}
    </section>

    <!-- Subscriptions -->
    <section class="flex flex-col gap-2">
      <h2 class="text-sm font-semibold">Your subscriptions</h2>
      {#if subs.length === 0}
        <p class="text-sm text-muted-foreground">
          No subscriptions yet. Open the filters, save a search, then turn on “Notify me on Telegram”.
        </p>
      {:else}
        <ul class="flex flex-col divide-y divide-border rounded-lg border border-border">
          {#each subs as sub (sub.id)}
            <li class="flex items-center justify-between gap-3 px-4 py-3">
              <div class="min-w-0">
                <p class="truncate text-sm font-medium">{sub.saved_search_name ?? 'Saved search'}</p>
                <p class="text-xs text-muted-foreground">
                  {sub.channel}{sub.active ? '' : ' · paused'}
                </p>
              </div>
              <div class="flex shrink-0 items-center gap-2">
                <Button
                  variant="ghost"
                  size="sm"
                  onclick={() => toggleActive(sub.id, !sub.active)}
                  disabled={busyId === sub.id}
                >
                  {sub.active ? 'Pause' : 'Resume'}
                </Button>
                <Button
                  variant="ghost"
                  size="sm"
                  onclick={() => remove(sub.id, sub.saved_search_name ?? 'this search')}
                  disabled={busyId === sub.id}
                >
                  Remove
                </Button>
              </div>
            </li>
          {/each}
        </ul>
      {/if}
    </section>

    {#if error}
      <p class="text-sm text-destructive">{error}</p>
    {/if}
  {/if}
</div>
