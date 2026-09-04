<script lang="ts">
  import { onMount } from 'svelte';
  import { page } from '$app/state';
  import { replaceState } from '$app/navigation';
  import { api, ApiError } from '$lib/api';
  import type { GmailStatus } from '$lib/api';
  import { notifications } from '$lib/notifications.svelte';
  import { Badge, Button, ConfirmDialog, ProviderIcon } from '$lib/ui';
  import { Mail, CalendarDays } from '@lucide/svelte';
  import { errorMessage } from '$lib/utils';
  import GmailConnectDialog from './GmailConnectDialog.svelte';

  // The account-level home for every third-party connection: Google (Gmail +
  // Calendar, two separate consents on the same grant) and Telegram (the alert
  // bot, connected here independently of any one saved search). Each connection's
  // own page (Inbox, Tracking → Calendar) keeps only a short status line pointing
  // back here — this is the one place that owns connect/disconnect.

  // --- Google: Mail ---
  let gmail = $state<GmailStatus | null>(null);
  let gmailLoading = $state(true);
  let gmailError = $state<string | null>(null);
  let syncing = $state(false);
  let syncStarted = $state(false);
  let showConnectDialog = $state(false);
  let confirmDisconnectOpen = $state(false);

  const hasGmail = $derived(!!gmail?.connected);
  const hasCalendar = $derived(gmail?.calendar_connected === true);

  async function loadGmail() {
    gmailLoading = true;
    try {
      gmail = await api.gmailStatus();
    } catch (e) {
      gmailError = errorMessage(e, 'Failed to load Gmail status.');
    } finally {
      gmailLoading = false;
    }
  }

  function connectGmail() {
    window.location.href = '/api/v1/me/gmail/connect';
  }

  async function sync() {
    if (syncing) return;
    syncing = true;
    syncStarted = false;
    gmailError = null;
    try {
      await api.syncGmail();
      syncStarted = true;
    } catch (e) {
      gmailError = errorMessage(e, 'Sync failed.');
    } finally {
      syncing = false;
    }
  }

  async function disconnectGmail() {
    try {
      await api.disconnectGmail();
      gmail = { connected: false, available: gmail?.available, calendar_connected: false };
    } catch (e) {
      gmailError = errorMessage(e, 'Failed to disconnect.');
    }
  }

  // Both the mail and calendar connect flows end as a top-level browser navigation
  // back here, carrying their verdict in the URL — the same convention the inbox
  // used to read for the mail flow alone. Only the `auth` message names the product;
  // `state` and `exchange` are the same regardless of which consent failed.
  const connectErrors = (product: string): Record<string, string> => ({
    auth: `Your session ended before ${product} finished connecting. Sign in, then try again.`,
    state: 'That connect link expired or was opened out of order. Start the connection again.',
    exchange: 'Google did not finish handing over access. Try connecting again.',
  });
  const GMAIL_CONNECT_ERRORS = connectErrors('Gmail');
  const CALENDAR_CONNECT_ERRORS = connectErrors('Calendar');
  let googleNotice = $state<{ ok: boolean; text: string } | null>(null);

  function readGoogleVerdict() {
    const params = page.url.searchParams;
    const gmailFailed = params.get('gmail_error');
    const calendarFailed = params.get('calendar_error');
    if (gmailFailed) {
      googleNotice = { ok: false, text: GMAIL_CONNECT_ERRORS[gmailFailed] ?? 'Connecting Gmail failed. Try again.' };
    } else if (calendarFailed) {
      googleNotice = {
        ok: false,
        text: CALENDAR_CONNECT_ERRORS[calendarFailed] ?? 'Connecting Calendar failed. Try again.',
      };
    } else if (params.get('gmail') === 'connected') {
      googleNotice = { ok: true, text: 'Gmail connected — your ATS mail will show up in the Inbox shortly.' };
    } else if (params.get('calendar') === 'connected') {
      googleNotice = { ok: true, text: 'Calendar connected — accepted interviews will show up on the Tracking calendar.' };
    } else {
      return;
    }
    // eslint-disable-next-line svelte/no-navigation-without-resolve -- shallow same-page URL clean-up to the current pathname; nothing to resolve
    replaceState(page.url.pathname, {});
  }

  // --- Telegram ---
  const tg = $derived(notifications.telegram);
  let tgBusy = $state(false);
  let tgConnecting = $state(false);
  let tgError = $state<string | null>(null);

  async function connectTelegram() {
    if (tgBusy) return;
    tgBusy = true;
    tgError = null;
    try {
      const url = await notifications.link();
      window.open(url, '_blank', 'noopener');
      tgConnecting = true;
    } catch (e) {
      tgError = e instanceof ApiError ? e.message : 'Could not start the Telegram connection. Please try again.';
    } finally {
      tgBusy = false;
    }
  }

  async function recheckTelegram() {
    if (tgBusy) return;
    tgBusy = true;
    tgError = null;
    try {
      await notifications.refreshTelegram();
      if (notifications.telegram.linked) tgConnecting = false;
      else tgError = 'Not connected yet — tap “Start” in Telegram, then retry.';
    } catch {
      tgError = 'Could not check the connection. Please try again.';
    } finally {
      tgBusy = false;
    }
  }

  async function disconnectTelegram() {
    if (tgBusy) return;
    tgBusy = true;
    tgError = null;
    try {
      await notifications.unlink();
    } catch (e) {
      tgError = e instanceof ApiError ? e.message : 'Could not disconnect Telegram. Please try again.';
    } finally {
      tgBusy = false;
    }
  }

  onMount(() => {
    readGoogleVerdict();
    void loadGmail();
    void notifications.ensureLoaded();
  });

  // Shared by every "already connected" badge below (Mail, Calendar, Telegram).
  const CONNECTED_BADGE_CLASS = 'border-brand-ring/40 text-brand-strong';
</script>

<div class="flex flex-col gap-4">
  <div class="flex flex-col gap-1">
    <h1 class="text-2xl font-semibold tracking-tight">Integrations</h1>
    <p class="text-sm text-muted-foreground">Connect the third-party accounts freehire can use on your behalf.</p>
  </div>

  {#if googleNotice}
    <p
      class="rounded-md border px-3 py-2 text-sm {googleNotice.ok
        ? 'border-brand/30 bg-brand/10 text-foreground'
        : 'border-destructive/30 bg-destructive/10 text-destructive'}"
    >
      {googleNotice.text}
    </p>
  {/if}

  <!-- Google -->
  <div class="rounded-xl border border-border bg-card p-4">
    <div class="flex items-center gap-2 text-sm font-medium">
      <ProviderIcon provider="google" class="h-4 w-4" /> Google
    </div>

    {#if gmailLoading}
      <p class="mt-3 text-xs text-muted-foreground">Loading…</p>
    {:else}
      <div class="mt-3 flex flex-col gap-3">
        <!-- Mail -->
        <div class="flex flex-wrap items-center justify-between gap-3">
          <div class="min-w-0">
            <div class="flex items-center gap-1.5 text-sm">
              <Mail class="h-3.5 w-3.5 text-muted-foreground" /> Mail
              {#if hasGmail}
                <Badge variant="outline" class={CONNECTED_BADGE_CLASS}>Connected</Badge>
                {#if gmail?.status === 'needs_reconsent'}
                  <Badge variant="outline" class="border-destructive/40 text-destructive">Reconnect needed</Badge>
                {/if}
              {/if}
            </div>
            {#if hasGmail}
              <p class="truncate text-xs text-muted-foreground">{gmail?.email}</p>
            {:else if gmail?.available}
              <p class="text-xs text-muted-foreground">Pull replies from your own Gmail into the Inbox.</p>
            {:else}
              <p class="text-xs text-muted-foreground">Not available yet.</p>
            {/if}
          </div>
          <div class="flex shrink-0 flex-wrap gap-2">
            {#if hasGmail}
              {#if gmail?.status === 'needs_reconsent'}
                <Button variant="secondary" size="sm" onclick={connectGmail}>Reconnect</Button>
              {/if}
              <Button variant="secondary" size="sm" disabled={syncing} onclick={sync}>
                {syncing ? 'Syncing…' : 'Sync'}
              </Button>
              <Button variant="outline" size="sm" onclick={() => (confirmDisconnectOpen = true)}>Disconnect</Button>
            {:else if gmail?.available}
              <Button variant="primary" size="sm" onclick={() => (showConnectDialog = true)}>
                Connect <Mail class="h-4 w-4" />
              </Button>
            {/if}
          </div>
        </div>
        {#if syncStarted}
          <p class="text-xs text-muted-foreground">Sync started — new mail will show up in the Inbox shortly.</p>
        {/if}

        <!-- Calendar: a separate consent, so it needs its own status and its own connect. -->
        <div class="flex flex-wrap items-center justify-between gap-3 border-t border-border pt-3">
          <div class="min-w-0">
            <div class="flex items-center gap-1.5 text-sm">
              <CalendarDays class="h-3.5 w-3.5 text-muted-foreground" /> Calendar
              {#if hasCalendar}
                <Badge variant="outline" class={CONNECTED_BADGE_CLASS}>Connected</Badge>
              {/if}
            </div>
            <p class="text-xs text-muted-foreground">
              {hasCalendar
                ? 'Accepted interviews appear on the Tracking calendar.'
                : 'Connecting Mail does not grant this — it asks separately.'}
            </p>
          </div>
          {#if !hasCalendar && gmail?.available}
            <!-- eslint-disable svelte/no-navigation-without-resolve -- an API route the browser must navigate to so Google can redirect it back, not a SvelteKit page to resolve -->
            <a
              href="/api/v1/me/calendar/connect"
              class="shrink-0 rounded-md border border-border px-3 py-1.5 text-sm hover:bg-accent hover:text-accent-foreground"
              >Connect</a
            >
            <!-- eslint-enable svelte/no-navigation-without-resolve -->
          {/if}
        </div>
      </div>
    {/if}

    {#if gmailError}
      <p class="mt-2 text-xs text-destructive">{gmailError}</p>
    {/if}
  </div>

  <!-- Telegram -->
  {#if tg.enabled}
    <div class="rounded-xl border border-border bg-card p-4">
      <div class="flex flex-wrap items-center justify-between gap-3">
        <div class="min-w-0">
          <div class="flex items-center gap-2 text-sm font-medium">
            <ProviderIcon provider="telegram" class="h-4 w-4" /> Telegram
            {#if tg.linked}
              <Badge variant="outline" class={CONNECTED_BADGE_CLASS}>Connected</Badge>
            {/if}
          </div>
          <p class="mt-1 text-xs text-muted-foreground">
            {tg.linked
              ? 'Turn on alerts for individual saved searches from the search itself.'
              : 'Connect the bot once, then turn on alerts from any saved search.'}
          </p>
        </div>
        <div class="shrink-0">
          {#if tg.linked}
            <Button variant="outline" size="sm" disabled={tgBusy} onclick={disconnectTelegram}>
              {tgBusy ? 'Disconnecting…' : 'Disconnect'}
            </Button>
          {:else}
            <Button variant="primary" size="sm" disabled={tgBusy} onclick={connectTelegram}>
              {tgBusy ? 'Starting…' : 'Connect'}
            </Button>
          {/if}
        </div>
      </div>

      {#if tgConnecting}
        <p class="mt-2 text-xs text-muted-foreground">
          Opened Telegram — tap “Start”, then
          <button
            type="button"
            onclick={recheckTelegram}
            disabled={tgBusy}
            class="font-medium text-foreground underline underline-offset-2 hover:opacity-80"
          >
            I’ve connected
          </button>.
        </p>
      {/if}
      {#if tgError}
        <p class="mt-2 text-xs text-destructive">{tgError}</p>
      {/if}
    </div>
  {/if}
</div>

{#if showConnectDialog}
  <GmailConnectDialog onClose={() => (showConnectDialog = false)} onConnect={connectGmail} />
{/if}

<ConfirmDialog
  bind:open={confirmDisconnectOpen}
  title="Disconnect Gmail?"
  description="Its synced mail is removed."
  confirmLabel="Disconnect"
  onConfirm={disconnectGmail}
/>
