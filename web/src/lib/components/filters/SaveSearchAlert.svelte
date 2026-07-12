<script lang="ts">
  import { onMount } from 'svelte';
  import { Bell, Bookmark, Check } from '@lucide/svelte';
  import { ApiError } from '$lib/api';
  import { isAuthenticated } from '$lib/auth.svelte';
  import { openAuthDialog } from '$lib/auth-dialog.svelte';
  import { savedSearches } from '$lib/savedSearches.svelte';
  import { notifications } from '$lib/notifications.svelte';
  import { alertStateFor, ensureSaved, matchedSavedSearch, setPendingAlert } from '$lib/saveSearchAlert';
  import { Button } from '$lib/ui';

  // The centralized "save this search, then get its new jobs in Telegram" control,
  // driven by a query string so the same behavior serves the sidebar, the onboarding
  // banner, and the modal's "My filters" tab. Save is the primary, standalone action
  // (works without any subscription); once saved, the Telegram alert is offered as a
  // follow-on. Orchestration state lives in saveSearchAlert.ts.
  //
  // `quick` = the compact onboarding offer (no turn-off). `full` = adds a turn-off
  // toggle once subscribed (sidebar / modal / saved-searches page).
  let {
    query,
    variant = 'full',
    autostart = false,
  }: {
    query: string;
    variant?: 'quick' | 'full';
    /** Run the save on mount — used to resume a pending save after sign-in. */
    autostart?: boolean;
  } = $props();

  let connecting = $state(false);
  let busy = $state(false);
  let error = $state<string | null>(null);

  // The primary Save button is prominent in the `full` placement (modal / sidebar)
  // and compact in the `quick` onboarding banner. Derived once so both save states
  // (sign-in-first and unsaved) share one definition.
  const saveBig = $derived(variant === 'full');
  const saveBtnClass = $derived(
    saveBig
      ? 'w-full justify-center gap-2 rounded-xl text-[0.95rem] font-semibold shadow-sm transition-shadow hover:shadow-md'
      : 'justify-center gap-2 rounded-lg font-semibold',
  );

  // Load saved searches + telegram status once the session is known; sign-out cache
  // reset is owned centrally by +layout.svelte, so it isn't duplicated here.
  $effect(() => {
    if (isAuthenticated()) {
      void savedSearches.ensureLoaded();
      void notifications.ensureLoaded();
    }
  });

  const matched = $derived(matchedSavedSearch(query, savedSearches.items));
  const subscription = $derived(matched ? notifications.forSavedSearch(matched.id) : undefined);
  const tg = $derived(notifications.telegram);
  const alertState = $derived(
    alertStateFor({
      authed: isAuthenticated(),
      saved: matched !== undefined,
      telegramEnabled: tg.enabled,
      subscription,
      connecting,
    }),
  );

  // Step 1 — save the current filters (auth-gated). Standalone: no Telegram involved.
  async function save() {
    if (busy) return;
    if (!isAuthenticated()) {
      setPendingAlert(query); // resume the save after sign-in (incl. an OAuth redirect)
      openAuthDialog();
      return;
    }
    busy = true;
    error = null;
    try {
      await ensureSaved(query, savedSearches);
    } catch (e) {
      error = e instanceof ApiError ? e.message : 'Could not save. Please try again.';
    } finally {
      busy = false;
    }
  }

  // Step 2 — turn on the Telegram alert for the (already saved) search. Links the bot
  // first if needed (the "I've connected" recheck then finishes the subscribe).
  async function subscribe() {
    if (!matched || busy) return;
    busy = true;
    error = null;
    try {
      await notifications.ensureLoaded();
      if (!notifications.telegram.linked) {
        const url = await notifications.link();
        window.open(url, '_blank', 'noopener');
        connecting = true;
        return;
      }
      if (notifications.forSavedSearch(matched.id)) return; // already subscribed
      await notifications.subscribe(matched.id);
    } catch (e) {
      if (!(e instanceof ApiError) || e.status !== 409) {
        error = e instanceof ApiError ? e.message : 'Could not set up the alert. Please try again.';
      }
    } finally {
      busy = false;
    }
  }

  async function recheck() {
    if (busy) return;
    busy = true;
    error = null;
    try {
      await notifications.refreshTelegram();
      if (notifications.telegram.linked) connecting = false;
      else error = 'Not connected yet — tap “Start” in Telegram, then retry.';
    } catch {
      error = 'Could not check the connection. Please try again.';
    } finally {
      busy = false;
    }
    if (notifications.telegram.linked) await subscribe(); // linked now → subscribe
  }

  async function turnOff() {
    if (!subscription || busy) return;
    busy = true;
    error = null;
    try {
      await notifications.unsubscribe(subscription.id);
    } catch (e) {
      error = e instanceof ApiError ? e.message : 'Could not turn off the alert. Please try again.';
    } finally {
      busy = false;
    }
  }

  // Resume a pending save (after sign-in): run it exactly once. onMount (not a $effect)
  // so it can't self-invalidate through save()'s reactive reads and loop.
  onMount(() => {
    if (autostart) void save();
  });
</script>

<div class="flex flex-col gap-1.5">
  {#if alertState === 'signed-out'}
    <Button variant="primary" size={saveBig ? 'lg' : 'md'} onclick={save} class={saveBtnClass}>
      <Bookmark class={saveBig ? 'size-[1.15rem]' : 'size-4'} aria-hidden="true" />
      Save this search
    </Button>
  {:else if alertState === 'unsaved'}
    <Button variant="primary" size={saveBig ? 'lg' : 'md'} onclick={save} disabled={busy} class={saveBtnClass}>
      <Bookmark class={saveBig ? 'size-[1.15rem]' : 'size-4'} aria-hidden="true" />
      {busy ? 'Saving…' : 'Save filter'}
    </Button>
  {:else if alertState === 'saved'}
    <p class="flex items-center gap-1.5 text-sm text-muted-foreground">
      <Check class="size-4 text-primary" aria-hidden="true" /> Saved
    </p>
  {:else if alertState === 'idle'}
    <Button variant="secondary" size="sm" onclick={subscribe} disabled={busy} class="justify-center gap-1.5">
      <Bell class="size-4" aria-hidden="true" />
      {busy ? 'Setting up…' : 'Get new jobs in Telegram'}
    </Button>
  {:else if alertState === 'connecting'}
    <p class="text-sm text-muted-foreground">Opened Telegram — tap “Start”, then:</p>
    <Button variant="secondary" size="sm" onclick={recheck} disabled={busy} class="justify-center">
      {busy ? 'Checking…' : 'I’ve connected'}
    </Button>
  {:else if alertState === 'subscribed'}
    <p class="flex items-center gap-1.5 text-sm">
      <Check class="size-4 text-primary" aria-hidden="true" />
      <span>You’ll get new jobs in Telegram</span>
    </p>
    {#if variant === 'full'}
      <button
        type="button"
        onclick={turnOff}
        disabled={busy}
        class="self-start text-xs font-medium text-muted-foreground transition-colors hover:text-foreground disabled:opacity-50"
      >
        Turn off
      </button>
    {/if}
  {/if}

  {#if error}
    <p class="text-sm text-destructive">{error}</p>
  {/if}
</div>
