<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '$lib/api';
  import { formatMinorUnits } from '$lib/money';
  import type { InviteSummary } from '$lib/types';

  let summary = $state<InviteSummary | null>(null);
  let error = $state(false);
  let copied = $state(false);

  onMount(async () => {
    try {
      summary = await api.myInvite();
    } catch {
      error = true;
    }
  });

  // The link is minted server-side on first read, so there is nothing to do here but show
  // it. Clipboard access can be refused (an insecure context, a permission prompt declined)
  // and the failure is silent by design: the link is on the page and selectable either way.
  async function copy() {
    if (!summary) return;
    try {
      await navigator.clipboard.writeText(summary.link);
      copied = true;
      setTimeout(() => (copied = false), 2000);
    } catch {
      copied = false;
    }
  }
</script>

<svelte:head>
  <title>Invite a friend — freehire</title>
</svelte:head>

<div class="flex max-w-3xl flex-col gap-6">
  <header class="flex flex-col gap-2">
    <h1 class="text-2xl font-semibold tracking-tight">Invite a friend</h1>
    {#if summary}
      <p class="text-muted-foreground">
        They get {summary.percent_off}% off their first month. You get {summary.percent_off}%
        off your next one, once they have actually paid.
      </p>
    {/if}
  </header>

  {#if error}
    <p class="text-sm text-destructive">
      Your invite link could not be loaded. Please try again in a moment.
    </p>
  {:else if summary}
    <section class="flex flex-col gap-2">
      <span class="text-sm text-muted-foreground">Your link</span>
      <div class="flex gap-2">
        <input
          type="text"
          readonly
          value={summary.link}
          class="min-w-0 flex-1 rounded-md border border-border bg-muted px-3 py-2 font-mono text-sm"
        />
        <button
          type="button"
          class="shrink-0 rounded-md border border-border px-3 py-2 text-sm"
          onclick={copy}
        >
          {copied ? 'Copied' : 'Copy'}
        </button>
      </div>
    </section>

    <!-- Counts and a total, naming nobody. Telling somebody which of their contacts signed
         up would disclose that a particular person is looking for work, and that is not the
         referrer's to know — the API has no field that could carry it. -->
    <section class="grid grid-cols-3 gap-4">
      <div class="flex flex-col gap-1 rounded-md border border-border p-4">
        <span class="text-2xl font-semibold">{summary.invitees}</span>
        <span class="text-sm text-muted-foreground">Signed up</span>
      </div>
      <div class="flex flex-col gap-1 rounded-md border border-border p-4">
        <span class="text-2xl font-semibold">{summary.rewarded}</span>
        <span class="text-sm text-muted-foreground">Subscribed</span>
      </div>
      <div class="flex flex-col gap-1 rounded-md border border-border p-4">
        <span class="text-2xl font-semibold">
          {formatMinorUnits(summary.credit_cents, 'usd')}
        </span>
        <span class="text-sm text-muted-foreground">Credit earned</span>
      </div>
    </section>

    <p class="text-sm text-muted-foreground">
      Credit is applied to your next invoice automatically. If you are not on Pro yet, it
      waits until you are.
    </p>
  {/if}
</div>
