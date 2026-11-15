<script lang="ts">
  import { api, ApiError } from '#lib/api.js';
  import { currentUser } from '#lib/auth.svelte.js';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { format, t } from '#lib/i18n/t.js';
  import { formatMinorUnits } from '#lib/money.js';
  import { Button } from '#lib/ui/index.js';
  import type { BillingPaymentAttempt } from '#lib/types.js';
  import { messages } from './PaymentHistory.messages';

  const s = $derived(t(messages, locale()));
  const userID = $derived(currentUser()?.id);
  let attempts = $state<BillingPaymentAttempt[]>([]);
  let status = $state<'loading' | 'ready' | 'error' | 'disabled'>('loading');
  let refresh = $state(0);

  $effect(() => {
    const owner = userID;
    void refresh;
    attempts = [];
    status = 'loading';
    if (!owner) return;
    let live = true;
    api
      .billingAttempts()
      .then((rows) => {
        if (!live) return;
        attempts = rows;
        status = 'ready';
      })
      .catch((error: unknown) => {
        if (!live) return;
        status = error instanceof ApiError && error.status === 404 ? 'disabled' : 'error';
      });
    return () => {
      live = false;
    };
  });

  const label = (labels: Record<string, string>, key: string | undefined, fallback = '') =>
    (key && labels[key]) || fallback;
  const dateTime = (iso: string) =>
    new Date(iso).toLocaleString(locale(), {
      year: 'numeric',
      month: 'short',
      day: 'numeric',
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
    });
</script>

{#if userID && status !== 'disabled'}
  <section class="flex flex-col gap-3" aria-labelledby="payment-history-heading">
    <div class="flex items-center justify-between gap-3">
      <h2 id="payment-history-heading" class="text-sm font-medium text-muted-foreground">{s.heading}</h2>
      <Button variant="outline" size="sm" disabled={status === 'loading'} onclick={() => refresh++}>{s.refresh}</Button>
    </div>
    <p class="text-xs text-muted-foreground">{s.description}</p>
    <div aria-live="polite" aria-busy={status === 'loading'}>
      {#if status === 'loading'}
        <p class="py-4 text-sm text-muted-foreground">{s.loading}</p>
      {:else if status === 'error'}
        <p role="alert" class="py-4 text-sm text-destructive">{s.loadError}</p>
      {:else if attempts.length === 0}
        <p class="py-4 text-sm text-muted-foreground">{s.empty}</p>
      {:else}
        <ul class="divide-y divide-border rounded-lg border border-border">
          {#each attempts as attempt (attempt.id)}
            <li class="flex flex-col gap-2 px-4 py-3">
              <div class="flex flex-wrap items-baseline justify-between gap-x-4 gap-y-1">
                <span class="text-sm font-medium tabular-nums"
                  >{formatMinorUnits(attempt.amount_cents, attempt.currency, locale())}</span
                >
                <span class="text-sm {attempt.status === 'failed' ? 'text-destructive' : 'text-foreground'}"
                  >{label(s.status, attempt.status, s.status.unknown)}</span
                >
              </div>
              <div class="flex flex-wrap gap-x-3 gap-y-1 text-xs text-muted-foreground">
                <time datetime={attempt.date}>{dateTime(attempt.date)}</time>
                {#if attempt.card_last4}
                  <span>{attempt.card_brand || s.card} ••{attempt.card_last4}</span>
                {/if}
                {#if label(s.funding, attempt.card_funding)}<span>{label(s.funding, attempt.card_funding)}</span>{/if}
                {#if label(s.wallet, attempt.wallet)}<span>{label(s.wallet, attempt.wallet)}</span>{/if}
              </div>
              {#if attempt.status === 'failed'}
                <p class="text-sm">{label(s.reasons, attempt.reason, s.reasons.declined)}</p>
                <p class="text-sm text-muted-foreground">{label(s.help, attempt.reason, s.help.declined)}</p>
              {:else if attempt.amount_refunded_cents > 0}
                <p class="text-sm text-muted-foreground">
                  {format(s.refundedAmount, {
                    amount: formatMinorUnits(attempt.amount_refunded_cents, attempt.currency, locale()),
                  })}
                </p>
              {/if}
            </li>
          {/each}
        </ul>
      {/if}
    </div>
  </section>
{/if}
