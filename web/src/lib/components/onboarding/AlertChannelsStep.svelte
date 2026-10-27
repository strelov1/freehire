<script lang="ts">
  // The tour's alert step: the "jobs matching my profile" alert on email, Telegram or the
  // app, and a glimpse of the email it sends.
  //
  // Email and Telegram write straight away (through #lib/profileAlert, the same path the
  // Search alerts tab's toggle uses), so leaving the step saves nothing more. Push has no
  // switch: only the mobile app registers a device, so all the web can do is send people
  // to it.
  import { resolve } from '$app/paths';
  import { ArrowRight, Check, LoaderCircle, Mail, Smartphone } from '@lucide/svelte';
  import { ApiError } from '#lib/api.js';
  import { APP_STORE_URL } from '#lib/appLinks.js';
  import { notifications } from '#lib/notifications.svelte.js';
  import { enableProfileAlert, profileAlertSearch } from '#lib/profileAlert.js';
  import { savedSearches } from '#lib/savedSearches.svelte.js';
  import { Button, ProviderIcon } from '#lib/ui/index.js';
  import JobAlertDigestPreview from '#lib/components/JobAlertDigestPreview.svelte';
  import type { UserProfile } from '#lib/types.js';

  interface Props {
    /** The saved search profile; null when the feed step was skipped, and then there is
     *  nothing for an alert to follow. */
    profile: UserProfile | null;
  }

  let { profile }: Props = $props();

  $effect(() => {
    void savedSearches.ensureLoaded();
    void notifications.ensureLoaded();
  });

  const search = $derived(profileAlertSearch());
  const on = (channel: string) => search !== null && notifications.forSavedSearch(search.id, channel) !== undefined;
  const emailOn = $derived(on('email'));
  const telegramOn = $derived(on('telegram'));

  let busy = $state<'email' | 'telegram' | null>(null);
  let error = $state<string | null>(null);
  // The bot link is open in another tab and we are waiting for the candidate to press Start.
  let telegramPending = $state(false);

  async function turnOn(channel: 'email' | 'telegram') {
    if (!profile || busy) return;
    busy = channel;
    error = null;
    try {
      await enableProfileAlert(profile, channel);
    } catch (e) {
      error = e instanceof ApiError ? e.message : 'Could not turn the alert on. Please try again.';
    } finally {
      busy = null;
    }
  }

  async function turnEmailOff() {
    const sub = search ? notifications.forSavedSearch(search.id, 'email') : undefined;
    if (!sub || busy) return;
    busy = 'email';
    error = null;
    try {
      await notifications.unsubscribe(sub.id);
    } catch {
      error = 'Could not turn email off. Please try again.';
    } finally {
      busy = null;
    }
  }

  async function connectTelegram() {
    if (notifications.telegram.linked) return turnOn('telegram');
    busy = 'telegram';
    error = null;
    try {
      window.open(await notifications.link(), '_blank', 'noopener');
      telegramPending = true;
    } catch (e) {
      error = e instanceof ApiError ? e.message : 'Could not start the Telegram connection. Please try again.';
    } finally {
      busy = null;
    }
  }

  async function recheckTelegram() {
    busy = 'telegram';
    error = null;
    try {
      await notifications.refreshTelegram();
    } catch {
      error = 'Could not check the connection. Please try again.';
    } finally {
      busy = null;
    }
    if (!notifications.telegram.linked) {
      error ??= 'Not connected yet — press “Start” in Telegram, then check again.';
      return;
    }
    telegramPending = false;
    await turnOn('telegram');
  }
</script>

<h2 class="text-xl font-semibold tracking-tight text-balance">Get new jobs without checking back</h2>
<p class="mt-1 text-sm text-muted-foreground text-pretty">
  When a new job matches your feed, we send it to you — the moment it appears, or once a day.
</p>

{#if !profile}
  <p class="mt-4 rounded-lg bg-secondary px-3 py-2 text-sm text-muted-foreground">
    Alerts follow your feed. Go back a step and pick a specialization and a skill to turn them on.
  </p>
{/if}

<!-- Each channel: what it is on top, its one action pinned to the bottom so the three line
     up whatever their copy wraps to. -->
<div class="mt-6 grid gap-3 sm:grid-cols-3">
  <div class={['flex flex-col rounded-xl border p-4 transition-colors', emailOn ? 'border-brand bg-brand-muted/40' : 'border-border bg-card']}>
    <p class="flex items-center gap-2 text-sm font-semibold">
      <Mail class="size-5" aria-hidden="true" /> Email
    </p>
    <p class="mt-1.5 text-xs text-muted-foreground">To the address you signed up with</p>
    <div class="mt-auto flex items-center justify-between gap-2 pt-4 text-sm font-medium">
      <span aria-hidden="true">{emailOn ? 'On' : 'Off'}</span>
      <button
        type="button"
        role="switch"
        aria-checked={emailOn}
        aria-label="Email me new jobs matching my feed"
        disabled={!profile || busy !== null}
        onclick={() => void (emailOn ? turnEmailOff() : turnOn('email'))}
        class={[
          'relative inline-flex h-6 w-11 shrink-0 items-center rounded-full transition-colors disabled:opacity-50',
          'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-card',
          emailOn ? 'bg-brand' : 'bg-muted',
        ]}
      >
        <span
          class={['inline-block size-5 rounded-full bg-white shadow-sm transition-transform', emailOn ? 'translate-x-5.5' : 'translate-x-0.5']}
        ></span>
      </button>
    </div>
  </div>

  <div class={['flex flex-col rounded-xl border p-4 transition-colors', telegramOn ? 'border-brand bg-brand-muted/40' : 'border-border bg-card']}>
    <p class="flex items-center gap-2 text-sm font-semibold">
      <ProviderIcon provider="telegram" class="size-5" /> Telegram
    </p>
    <p class="mt-1.5 text-xs text-muted-foreground">Messages from our bot</p>
    <div class="mt-auto pt-4">
      {#if telegramOn}
        <p class="flex h-8 items-center gap-1 text-sm font-medium text-brand-strong"><Check class="size-4" aria-hidden="true" /> Connected</p>
      {:else if telegramPending}
        <Button variant="outline" size="sm" class="w-full" disabled={busy !== null} onclick={() => void recheckTelegram()}>
          {#if busy === 'telegram'}<LoaderCircle class="size-4 animate-spin" aria-hidden="true" />{/if}
          I pressed Start
        </Button>
      {:else}
        <Button variant="outline" size="sm" class="w-full" disabled={!profile || busy !== null} onclick={() => void connectTelegram()}>
          {#if busy === 'telegram'}<LoaderCircle class="size-4 animate-spin" aria-hidden="true" />{/if}
          {notifications.telegram.linked ? 'Turn on' : 'Connect'}
        </Button>
      {/if}
    </div>
  </div>

  <div class="flex flex-col rounded-xl border border-border bg-card p-4">
    <p class="flex items-center gap-2 text-sm font-semibold">
      <Smartphone class="size-5" aria-hidden="true" /> Push
    </p>
    <p class="mt-1.5 text-xs text-muted-foreground">In the freehire iPhone app</p>
    <div class="mt-auto pt-4">
      <Button variant="outline" size="sm" class="w-full" href={APP_STORE_URL} target="_blank">Get the app</Button>
    </div>
  </div>
</div>

{#if error}
  <p class="mt-3 text-sm text-destructive" role="alert">{error}</p>
{:else if emailOn || telegramOn}
  <p class="mt-3 flex items-center gap-1.5 text-sm text-brand-strong">
    <Check class="size-4" aria-hidden="true" /> Done. You'll hear about the next match.
  </p>
{/if}

<p class="mb-2 mt-7 text-sm font-medium">What you'll get</p>
<div aria-hidden="true" inert>
  <JobAlertDigestPreview search="My profile" bare compact />
</div>

<p class="mt-4 text-sm text-muted-foreground text-pretty">
  Instant or once a day, and quiet hours, are in notification settings.
  <a
    href={resolve('/features/notifications')}
    target="_blank"
    class="inline-flex items-center gap-1 font-medium text-brand-strong hover:underline"
  >
    How alerts work <ArrowRight class="size-3.5" aria-hidden="true" />
  </a>
</p>
