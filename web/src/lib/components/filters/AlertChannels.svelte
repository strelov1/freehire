<script lang="ts">
  import { Bell, Check, Mail } from '@lucide/svelte';
  import { ApiError } from '$lib/api';
  import { notifications } from '$lib/notifications.svelte';
  import ProviderIcon from '../ProviderIcon.svelte';

  // The unified per-search alert control: one toggle chip per delivery channel
  // (Telegram, Email) for an already-saved search. This is the single home of channel
  // subscribe/unsubscribe logic — Telegram carries a connect step (link the bot, then
  // recheck once), email delivers to the account address with no linking. Shared by the
  // account page (/my/searches), the filter modal, and the sidebar's post-save state.
  //
  // A chip is "on" when a subscription exists for that channel; tapping an on chip turns
  // it off. Telegram hides itself when the feature isn't configured server-side.
  let { savedSearchId }: { savedSearchId: number } = $props();

  let busy = $state<'telegram' | 'email' | null>(null);
  // The Telegram deep link was opened; we await the user's "I've connected" recheck.
  let connecting = $state(false);
  let error = $state<string | null>(null);

  const tg = $derived(notifications.telegram);
  const tgSub = $derived(notifications.forSavedSearch(savedSearchId, 'telegram'));
  const emailSub = $derived(notifications.forSavedSearch(savedSearchId, 'email'));

  // Telegram: unsubscribe if on; else subscribe — linking the bot first when the chat
  // isn't connected yet (the "I've connected" recheck then finishes the subscribe).
  async function toggleTelegram() {
    if (busy) return;
    busy = 'telegram';
    error = null;
    try {
      if (tgSub) {
        await notifications.unsubscribe(tgSub.id);
        return;
      }
      await notifications.ensureLoaded();
      if (!notifications.telegram.linked) {
        const url = await notifications.link();
        window.open(url, '_blank', 'noopener');
        connecting = true;
        return;
      }
      if (!notifications.forSavedSearch(savedSearchId, 'telegram')) {
        await notifications.subscribe(savedSearchId, 'telegram');
      }
    } catch (e) {
      if (!(e instanceof ApiError) || e.status !== 409) {
        error = e instanceof ApiError ? e.message : 'Could not update the Telegram alert. Please try again.';
      }
    } finally {
      busy = null;
    }
  }

  async function recheck() {
    if (busy) return;
    busy = 'telegram';
    error = null;
    try {
      await notifications.refreshTelegram();
      if (notifications.telegram.linked) connecting = false;
      else error = 'Not connected yet — tap “Start” in Telegram, then retry.';
    } catch {
      error = 'Could not check the connection. Please try again.';
    } finally {
      busy = null;
    }
    if (notifications.telegram.linked) await toggleTelegram(); // linked now → subscribe
  }

  // Email: plain subscribe/unsubscribe — no linking, delivery goes to the account address.
  async function toggleEmail() {
    if (busy) return;
    busy = 'email';
    error = null;
    try {
      if (emailSub) await notifications.unsubscribe(emailSub.id);
      else await notifications.subscribe(savedSearchId, 'email');
    } catch (e) {
      error = e instanceof ApiError ? e.message : 'Could not update the email alert. Please try again.';
    } finally {
      busy = null;
    }
  }

  // A chip: filled brand tint when on, neutral outline when off; the leading glyph is a
  // check once subscribed, otherwise the channel mark.
  const chipClass = (on: boolean) =>
    [
      'inline-flex items-center gap-1.5 rounded-full border px-3 py-1.5 text-xs font-semibold transition-colors disabled:opacity-50',
      on
        ? 'border-transparent bg-brand-muted text-brand-strong'
        : 'border-border bg-background text-muted-foreground hover:border-muted-foreground/40',
    ];
</script>

<div class="flex flex-col gap-1.5">
  <div class="flex flex-wrap items-center gap-2">
    <span class="flex items-center gap-1 text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">
      <Bell class="size-3.5" aria-hidden="true" /> Alerts
    </span>

    {#if tg.enabled}
      <button type="button" onclick={toggleTelegram} disabled={busy !== null} aria-pressed={tgSub != null} class={chipClass(tgSub != null)}>
        {#if tgSub}
          <Check class="size-3.5" aria-hidden="true" />
        {:else}
          <ProviderIcon provider="telegram" class="size-3.5" />
        {/if}
        Telegram
      </button>
    {/if}

    <button type="button" onclick={toggleEmail} disabled={busy !== null} aria-pressed={emailSub != null} class={chipClass(emailSub != null)}>
      {#if emailSub}
        <Check class="size-3.5" aria-hidden="true" />
      {:else}
        <Mail class="size-3.5" aria-hidden="true" />
      {/if}
      Email
    </button>
  </div>

  {#if connecting}
    <p class="text-xs text-muted-foreground">
      Opened Telegram — tap “Start”, then
      <button type="button" onclick={recheck} disabled={busy !== null} class="font-medium text-foreground underline underline-offset-2 hover:opacity-80">
        I’ve connected
      </button>.
    </p>
  {/if}

  {#if error}
    <p class="text-xs text-destructive">{error}</p>
  {/if}
</div>
