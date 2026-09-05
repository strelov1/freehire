<script lang="ts">
  import { resolve } from '$app/paths';
  import { api } from '$lib/api';
  import { currentUser, isAuthenticated } from '$lib/auth.svelte';
  import { locale } from '$lib/i18n/currentLocale.svelte';
  import { plural, t, tokenLabel } from '$lib/i18n/t';
  import { formatMinorUnits } from '$lib/money';
  import type { AiUsage, BillingOverview, PlanState, UsageHistoryEntry } from '$lib/types';
  import { messages } from './PlanView.messages';
  import States from './States.svelte';

  const s = $derived(t(messages, locale()));

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

  // What is being paid and what has been taken. Null for a free account, and for a provider
  // we could not reach — in both cases the section is simply absent, because a billing
  // section that cannot show the money is worse than none.
  let billing = $state<BillingOverview | null>(null);

  // Both are asked for together, and only for a plan that could have a subscription behind
  // it: a free account has none by definition, and asking anyway spends two provider
  // round-trips per page view to be told 404 — the answer `plan` already gave us.
  //
  // They are cleared FIRST, on every run. What is on screen belongs to the plan we last
  // read, so when that changes — a subscription lapses, or another account signs in without
  // a reload — leaving it there shows one person's invoices and receipt links to the next.
  // `live` drops a slow response from a plan we have since moved off, which is the same
  // leak arriving late.
  $effect(() => {
    const pro = plan?.plan === 'pro';
    billing = null;
    manageUrl = null;
    if (!pro) return;

    let live = true;
    api
      .billingSubscription()
      .then((b) => live && (billing = b))
      .catch(() => {});
    api
      .billingManageUrl()
      .then(({ url }) => live && (manageUrl = url))
      .catch(() => {});
    return () => {
      live = false;
    };
  });

  const money = formatMinorUnits;

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

  const fmtDate = (iso: string) =>
    new Date(iso).toLocaleDateString(undefined, { month: 'short', day: 'numeric', year: 'numeric' });
  // When today's allowance starts over, in the reader's own clock rather than in UTC — the
  // day is keyed in UTC, but "resets at 3am" is what actually happens to them.
  const fmtTime = (iso: string) =>
    new Date(iso).toLocaleTimeString(undefined, { hour: 'numeric', minute: '2-digit' });
</script>

{#if !isAuthenticated()}
  <p class="py-12 text-center text-sm text-muted-foreground">{s.signedOut}</p>
{:else}
  <div class="flex flex-col gap-6">
    <div class="flex flex-col gap-1">
      <h1 class="text-2xl font-semibold tracking-tight">{s.title}</h1>
      <p class="text-sm text-muted-foreground">{s.description}</p>
    </div>

    {#if plan}
      <!-- The plan strip. On Pro it states when the subscription runs to; on Free it offers
           the upgrade — but only once we know there is somewhere to send them. -->
      <div
        class="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-border px-4 py-3"
      >
        <div class="flex flex-col gap-0.5">
          <span class="text-sm font-semibold">
            {plan.plan === 'pro' ? s.planStrip.pro : s.planStrip.free}
          </span>
          {#if plan.plan === 'pro' && plan.pro_until}
            <span class="text-xs text-muted-foreground">
              {s.planStrip.runsUntilPrefix}
              {fmtDate(plan.pro_until)}
            </span>
          {:else if plan.plan === 'free'}
            <span class="text-xs text-muted-foreground">{s.planStrip.freeSubtitle}</span>
          {/if}
        </div>
        {#if plan.plan === 'free'}
          <!-- To /pricing rather than straight to checkout: the choice between monthly and
               annual belongs on a page that can explain it, and sending someone to a payment
               form without it silently sells them the monthly one. -->
          <a
            class="shrink-0 rounded-md bg-primary px-3 py-1.5 text-sm font-medium text-primary-foreground"
            href={resolve('/pricing')}>{s.planStrip.upgrade}</a
          >
          <!-- eslint-disable svelte/no-navigation-without-resolve -- the payment provider's own page, not a SvelteKit route -->
        {:else if manageUrl}
          <a
            class="shrink-0 rounded-md border border-border px-3 py-1.5 text-sm font-medium"
            href={manageUrl}
            target="_blank"
            rel="noopener noreferrer">{s.planStrip.manageSubscription}</a
          ><!-- eslint-enable svelte/no-navigation-without-resolve -->
        {/if}
      </div>

      {#if billing}
        <div class="flex flex-col gap-3">
          <h2 class="text-sm font-medium text-muted-foreground">{s.subscription.heading}</h2>
          <div class="flex flex-col gap-3 rounded-lg border border-border px-4 py-3">
            <div class="flex flex-wrap items-baseline justify-between gap-2">
              <span class="text-sm font-medium">
                {money(billing.amount_cents, billing.currency)} / {tokenLabel(s.subscription.interval, billing.interval)}
              </span>
              <span class="text-xs text-muted-foreground">
                {tokenLabel(s.subscription.status, billing.status)}
              </span>
            </div>
            <!-- One date or the other, never both: a renewal date beside a cancellation is
                 the contradiction that generates support mail. -->
            {#if billing.ends_at}
              <p class="text-xs text-muted-foreground">
                {s.subscription.cancelledPrefix}
                {fmtDate(billing.ends_at)}
              </p>
            {:else if billing.renews_at}
              <p class="text-xs text-muted-foreground">
                {s.subscription.nextChargePrefix}
                {fmtDate(billing.renews_at)}
              </p>
            {/if}

            {#if billing.invoices.length > 0}
              <ul class="flex flex-col divide-y divide-border/60 border-t border-border/60">
                {#each billing.invoices as inv (inv.id)}
                  <li class="flex items-center justify-between gap-3 py-2 text-sm">
                    <span class="text-muted-foreground">{fmtDate(inv.date)}</span>
                    <span class="flex items-center gap-3">
                      <span class="tabular-nums"
                        >{money(inv.amount_cents, inv.currency)}</span
                      >
                      {#if inv.status !== 'paid'}
                        <span class="text-xs text-destructive">{inv.status}</span>
                      {/if}
                      {#if inv.receipt_url}
                        <!-- eslint-disable svelte/no-navigation-without-resolve -- the payment provider's hosted invoice, not a SvelteKit route -->
                        <a
                          class="text-xs underline"
                          href={inv.receipt_url}
                          target="_blank"
                          rel="noopener noreferrer">{s.subscription.receipt}</a
                        ><!-- eslint-enable svelte/no-navigation-without-resolve -->
                      {/if}
                    </span>
                  </li>
                {/each}
              </ul>
            {/if}
          </div>
        </div>
      {/if}

      <div class="flex flex-col gap-3">
        <div class="flex items-baseline justify-between gap-3">
          <h2 class="text-sm font-medium text-muted-foreground">{s.today.heading}</h2>
          <span class="text-xs text-muted-foreground">
            {s.today.resetsAtPrefix}
            {fmtTime(plan.resets_at)}
          </span>
        </div>
        <ul class="flex flex-col divide-y divide-border rounded-lg border border-border">
          {#each plan.allowances as a (a.feature)}
            <li class="flex items-center justify-between gap-3 px-4 py-3">
              <span class="truncate text-sm font-medium">{tokenLabel(s.today.features, a.feature)}</span>
              {#if a.unlimited}
                <span class="shrink-0 text-sm text-muted-foreground">{s.today.unlimited}</span>
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
        <h2 class="text-sm font-medium text-muted-foreground">{s.usage.heading}</h2>
        <div class="rounded-lg border border-border px-5 py-4">
          <div class="flex items-baseline gap-2">
            <span class="text-2xl font-semibold tabular-nums">{usage.requests}</span>
            <span class="text-sm text-muted-foreground">
              {plural(locale(), usage.requests, s.usage.modelCalls)}
            </span>
          </div>
          <p class="mt-1 text-xs text-muted-foreground">
            {usage.tokens.toLocaleString()}
            {plural(locale(), usage.tokens, s.usage.tokens)}{usage.failed > 0
              ? ` · ${usage.failed} ${s.usage.failedSuffix}`
              : ''} · {s.usage.resetsPrefix}
            {fmtDate(usage.resets_at)}
          </p>
          <p class="mt-2 text-xs text-muted-foreground">{s.usage.explanation}</p>
        </div>
      </div>
    {/if}

    <div class="flex flex-col gap-3">
      <h2 class="text-sm font-medium text-muted-foreground">{s.history.heading}</h2>
      {#if status === 'loading'}
        <States state="loading" />
      {:else if status === 'error'}
        <States state="error" message={s.history.loadError} />
      {:else if history.length === 0}
        <States state="empty" message={s.history.empty} />
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
