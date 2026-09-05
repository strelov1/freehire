<script lang="ts">
  import { Bell, Check, Mail, Smartphone, Webhook } from '@lucide/svelte';
  import { resolve } from '$app/paths';
  import { ApiError } from '$lib/api';
  import { notifications } from '$lib/notifications.svelte';
  import { ProviderIcon } from '$lib/ui';

  // The unified per-search alert control: one toggle chip per delivery channel
  // (Telegram, Email, Push) for an already-saved search. This is the single home of
  // channel subscribe/unsubscribe logic; email and push subscribe/unsubscribe
  // directly, with no linking step (push delivers to whatever device the mobile app
  // has registered). Connecting the Telegram bot itself lives on Integrations
  // (/my/integrations) — this control only subscribes/unsubscribes an already-linked
  // chat, and points to Integrations when it isn't linked yet. Shared by the account
  // page (/my/notifications/searches), the filter modal, and the sidebar's post-save
  // state.
  //
  // A chip is "on" when a subscription exists for that channel; tapping an on chip turns
  // it off. Telegram hides itself when the feature isn't configured server-side.
  let { savedSearchId, showLabel = true }: { savedSearchId: number; showLabel?: boolean } = $props();

  let busy = $state<'telegram' | 'email' | 'push' | 'webhook' | null>(null);
  let error = $state<string | null>(null);

  const tg = $derived(notifications.telegram);
  const tgSub = $derived(notifications.forSavedSearch(savedSearchId, 'telegram'));
  const emailSub = $derived(notifications.forSavedSearch(savedSearchId, 'email'));
  const pushSub = $derived(notifications.forSavedSearch(savedSearchId, 'push'));
  const webhook = $derived(notifications.webhook);
  const webhookSub = $derived(notifications.forSavedSearch(savedSearchId, 'webhook'));

  // Telegram: unsubscribe if on; else subscribe. Only ever called while linked — the
  // chip renders as a link to Integrations instead of this button while it isn't.
  async function toggleTelegram() {
    if (busy || !tg.linked) return;
    busy = 'telegram';
    error = null;
    try {
      if (tgSub) await notifications.unsubscribe(tgSub.id);
      else await notifications.subscribe(savedSearchId, 'telegram');
    } catch (e) {
      if (!(e instanceof ApiError) || e.status !== 409) {
        error = e instanceof ApiError ? e.message : 'Could not update the Telegram alert. Please try again.';
      }
    } finally {
      busy = null;
    }
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

  // Push: plain subscribe/unsubscribe — no linking here either; delivery reaches
  // whichever device(s) the mobile app has registered, or soft-skips until one is.
  async function togglePush() {
    if (busy) return;
    busy = 'push';
    error = null;
    try {
      if (pushSub) await notifications.unsubscribe(pushSub.id);
      else await notifications.subscribe(savedSearchId, 'push');
    } catch (e) {
      error = e instanceof ApiError ? e.message : 'Could not update the push alert. Please try again.';
    } finally {
      busy = null;
    }
  }

  // Webhook: like email/push, plain subscribe/unsubscribe — but only once a
  // destination is configured; otherwise the chip is a link to /my/webhook (the
  // same "not linked yet" treatment Telegram gets for Integrations).
  async function toggleWebhook() {
    if (busy || !webhook?.enabled) return;
    busy = 'webhook';
    error = null;
    try {
      if (webhookSub) await notifications.unsubscribe(webhookSub.id);
      else await notifications.subscribe(savedSearchId, 'webhook');
    } catch (e) {
      if (!(e instanceof ApiError) || e.status !== 409) {
        error = e instanceof ApiError ? e.message : 'Could not update the webhook alert. Please try again.';
      }
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
    {#if showLabel}
      <span class="flex items-center gap-1 text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">
        <Bell class="size-3.5" aria-hidden="true" /> Alerts
      </span>
    {/if}

    {#if tg.enabled}
      {#if tg.linked}
        <button type="button" onclick={toggleTelegram} disabled={busy !== null} aria-pressed={tgSub != null} class={chipClass(tgSub != null)}>
          {#if tgSub}
            <Check class="size-3.5" aria-hidden="true" />
          {:else}
            <ProviderIcon provider="telegram" class="size-3.5" />
          {/if}
          Telegram
        </button>
      {:else}
        <a href={resolve('/my/integrations')} class={chipClass(false)} title="Connect Telegram in Integrations first">
          <ProviderIcon provider="telegram" class="size-3.5" />
          Telegram
        </a>
      {/if}
    {/if}

    <button type="button" onclick={toggleEmail} disabled={busy !== null} aria-pressed={emailSub != null} class={chipClass(emailSub != null)}>
      {#if emailSub}
        <Check class="size-3.5" aria-hidden="true" />
      {:else}
        <Mail class="size-3.5" aria-hidden="true" />
      {/if}
      Email
    </button>

    <button type="button" onclick={togglePush} disabled={busy !== null} aria-pressed={pushSub != null} class={chipClass(pushSub != null)}>
      {#if pushSub}
        <Check class="size-3.5" aria-hidden="true" />
      {:else}
        <Smartphone class="size-3.5" aria-hidden="true" />
      {/if}
      Push
    </button>

    {#if webhook?.enabled}
      <button type="button" onclick={toggleWebhook} disabled={busy !== null} aria-pressed={webhookSub != null} class={chipClass(webhookSub != null)}>
        {#if webhookSub}
          <Check class="size-3.5" aria-hidden="true" />
        {:else}
          <Webhook class="size-3.5" aria-hidden="true" />
        {/if}
        Webhook
      </button>
    {:else}
      <a
        href={resolve('/my/webhook')}
        class={chipClass(false)}
        title={webhook ? 'Re-enable your webhook in settings first' : 'Set up a webhook destination first'}
      >
        <Webhook class="size-3.5" aria-hidden="true" />
        Webhook
      </a>
    {/if}
  </div>

  {#if error}
    <p class="text-xs text-destructive">{error}</p>
  {/if}
</div>
