<script lang="ts">
  import { resolve } from '$app/paths';
  import { api } from '$lib/api';
  import { currentUser, isAuthenticated } from '$lib/auth.svelte';
  import type { AiUsage, PlanState, UsageHistoryEntry } from '$lib/types';
  import States from './States.svelte';

  // The plan page: which plan the caller is on, what each metered feature allows today and
  // how much of it they have used, plus the history of what they spent it on. Read-only —
  // never triggers the LLM.
  //
  // It replaces the credits page, and the difference is the noun rather than the numbers: a
  // balance said "you own 12 of something", this says "1 of today's 3 analyses". Nothing
  // here accumulates and nothing is a currency.
  let plan = $state<PlanState | null>(null);
  let history = $state<UsageHistoryEntry[]>([]);
  let status = $state<'loading' | 'error' | 'ready'>('loading');

  // Gateway activity is beta-only for now. It answers a different question from the
  // allowances — what the account DID, not what it may still do — and the two want
  // watching together for a while before everyone is shown a second number.
  const showUsage = $derived(currentUser()?.beta_tester ?? false);
  let usage = $state<AiUsage | null>(null);

  $effect(() => {
    if (!isAuthenticated() || !showUsage) return;
    // Its own request, and its own failure: the endpoint answers zeroes rather than
    // erroring, so a rejection here means the network went away — which must not take
    // the plan and the history down with it.
    api
      .myUsage()
      .then((u) => (usage = u))
      .catch(() => (usage = null));
  });

  // Where a subscriber changes their card or cancels — the provider's own page. Null when
  // there is no subscription to manage, which is the ordinary state for a free account.
  let manageUrl = $state<string | null>(null);

  $effect(() => {
    if (!isAuthenticated()) return;
    api
      .billingManageUrl()
      .then(({ url }) => (manageUrl = url))
      .catch(() => (manageUrl = null));
  });

  $effect(() => {
    if (!isAuthenticated()) return;
    status = 'loading';
    Promise.all([api.myPlan(), api.myPlanHistory()])
      .then(([p, h]) => {
        plan = p;
        history = h;
        status = 'ready';
      })
      .catch(() => {
        status = 'error';
      });
  });

  // What each metered feature is called on the page. The API's names are stable
  // identifiers; these are what a candidate would call the thing they just did.
  const FEATURE_LABELS: Record<string, string> = {
    tailor: 'CV editing sessions',
    match: 'Job analyses',
    assistant: 'Assistant messages',
    dictation: 'Dictation',
  };
  const featureLabel = (id: string) => FEATURE_LABELS[id] ?? id;

  const fmtDate = (iso: string) =>
    new Date(iso).toLocaleDateString(undefined, { month: 'short', day: 'numeric', year: 'numeric' });
  // When today's allowance starts over, in the reader's own clock rather than in UTC — the
  // day is keyed in UTC, but "resets at 3am" is what actually happens to them.
  const fmtTime = (iso: string) =>
    new Date(iso).toLocaleTimeString(undefined, { hour: 'numeric', minute: '2-digit' });
</script>

{#if !isAuthenticated()}
  <p class="py-12 text-center text-sm text-muted-foreground">Sign in to view your plan.</p>
{:else}
  <div class="flex flex-col gap-6">
    <div class="flex flex-col gap-1">
      <h1 class="text-2xl font-semibold tracking-tight">Your plan</h1>
      <p class="text-sm text-muted-foreground">
        Every AI feature is available on every plan. What changes is how much of each you can
        do in a day — and it starts over every night.
      </p>
    </div>

    {#if plan}
      <!-- The plan strip. On Pro it states when the subscription runs to; on Free it offers
           the upgrade — but only once we know there is somewhere to send them. -->
      <div
        class="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-border px-4 py-3"
      >
        <div class="flex flex-col gap-0.5">
          <span class="text-sm font-semibold">{plan.plan === 'pro' ? 'Pro' : 'Free'}</span>
          {#if plan.plan === 'pro' && plan.pro_until}
            <span class="text-xs text-muted-foreground">Runs until {fmtDate(plan.pro_until)}</span>
          {:else if plan.plan === 'free'}
            <span class="text-xs text-muted-foreground">Same features, daily limits</span>
          {/if}
        </div>
        {#if plan.plan === 'free'}
          <!-- To /pricing rather than straight to checkout: the choice between monthly and
               annual belongs on a page that can explain it, and sending someone to a payment
               form without it silently sells them the monthly one. -->
          <a
            class="shrink-0 rounded-md bg-primary px-3 py-1.5 text-sm font-medium text-primary-foreground"
            href={resolve('/pricing')}>Upgrade to Pro</a
          >
          <!-- eslint-disable svelte/no-navigation-without-resolve -- the payment provider's own page, not a SvelteKit route -->
        {:else if manageUrl}
          <a
            class="shrink-0 rounded-md border border-border px-3 py-1.5 text-sm font-medium"
            href={manageUrl}
            target="_blank"
            rel="noopener noreferrer">Manage subscription</a
          ><!-- eslint-enable svelte/no-navigation-without-resolve -->
        {/if}
      </div>

      <div class="flex flex-col gap-3">
        <div class="flex items-baseline justify-between gap-3">
          <h2 class="text-sm font-medium text-muted-foreground">Today</h2>
          <span class="text-xs text-muted-foreground">
            Resets at {fmtTime(plan.resets_at)}
          </span>
        </div>
        <ul class="flex flex-col divide-y divide-border rounded-lg border border-border">
          {#each plan.allowances as a (a.feature)}
            <li class="flex items-center justify-between gap-3 px-4 py-3">
              <span class="truncate text-sm font-medium">{featureLabel(a.feature)}</span>
              {#if a.unlimited}
                <span class="shrink-0 text-sm text-muted-foreground">Unlimited</span>
              {:else}
                <span
                  class="shrink-0 text-sm font-semibold tabular-nums {a.used >= (a.limit ?? 0)
                    ? 'text-muted-foreground'
                    : ''}"
                >
                  {a.used} / {a.limit}
                </span>
              {/if}
            </li>
          {/each}
        </ul>
      </div>
    {/if}

    {#if showUsage && usage}
      <div class="flex flex-col gap-2">
        <h2 class="text-sm font-medium text-muted-foreground">AI activity today</h2>
        <div class="rounded-lg border border-border px-5 py-4">
          <div class="flex items-baseline gap-2">
            <span class="text-2xl font-semibold tabular-nums">{usage.requests}</span>
            <span class="text-sm text-muted-foreground">
              model {usage.requests === 1 ? 'call' : 'calls'}
            </span>
          </div>
          <p class="mt-1 text-xs text-muted-foreground">
            {usage.tokens.toLocaleString()} tokens{usage.failed > 0
              ? ` · ${usage.failed} failed`
              : ''} · resets {fmtDate(usage.resets_at)}
          </p>
          <p class="mt-2 text-xs text-muted-foreground">
            One message takes several calls — the assistant works in rounds. This counts the
            work, not what it costs you; the allowances above are what you spend.
          </p>
        </div>
      </div>
    {/if}

    <div class="flex flex-col gap-3">
      <h2 class="text-sm font-medium text-muted-foreground">Recent activity</h2>
      {#if status === 'loading'}
        <States state="loading" />
      {:else if status === 'error'}
        <States state="error" message="Couldn't load your plan." />
      {:else if history.length === 0}
        <States
          state="empty"
          message="Nothing yet. What you do with the AI features will appear here."
        />
      {:else}
        <ul class="flex flex-col divide-y divide-border rounded-lg border border-border">
          {#each history as entry, i (i)}
            <li class="flex items-center justify-between gap-3 px-4 py-3">
              <div class="flex min-w-0 flex-col gap-0.5">
                <span class="truncate text-sm font-medium">{entry.label}</span>
                <span class="truncate text-xs text-muted-foreground">
                  {#if entry.subtitle}{entry.subtitle} · {/if}{fmtDate(entry.created_at)}
                </span>
              </div>
            </li>
          {/each}
        </ul>
      {/if}
    </div>
  </div>
{/if}
