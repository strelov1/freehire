<script lang="ts">
  import { api } from '$lib/api';
  import { isAuthenticated } from '$lib/auth.svelte';
  import type { AiCredits, CreditHistoryEntry } from '$lib/types';
  import States from './States.svelte';

  // The Credits page: the caller's AI-credits balance headline plus the transaction history
  // (monthly grants, match/tailor debits, contribution rewards). Read-only — never triggers
  // the LLM. The balance funds the metered AI features (match analysis, CV tailoring).
  let credits = $state<AiCredits | null>(null);
  let history = $state<CreditHistoryEntry[]>([]);
  let status = $state<'loading' | 'error' | 'ready'>('loading');

  $effect(() => {
    if (!isAuthenticated()) return;
    status = 'loading';
    Promise.all([api.myCredits(), api.myCreditsHistory()])
      .then(([c, h]) => {
        credits = c;
        history = h;
        status = 'ready';
      })
      .catch(() => {
        status = 'error';
      });
  });

  const fmtDate = (iso: string) =>
    new Date(iso).toLocaleDateString(undefined, { month: 'short', day: 'numeric', year: 'numeric' });
  // A signed amount, e.g. "+20" / "−1" (a true minus sign, not a hyphen).
  const fmtDelta = (n: number) => (n > 0 ? `+${n}` : `−${Math.abs(n)}`);
</script>

{#if !isAuthenticated()}
  <p class="py-12 text-center text-sm text-muted-foreground">Sign in to view your credits.</p>
{:else}
  <div class="flex flex-col gap-6">
    <div class="flex flex-col gap-1">
      <h1 class="text-2xl font-semibold tracking-tight">Credits</h1>
      <p class="text-sm text-muted-foreground">
        Your AI credits fund match analysis and CV tailoring. Earn more by contributing new boards.
      </p>
    </div>

    {#if credits}
      <div class="rounded-lg border border-border bg-secondary/40 px-5 py-4">
        <div class="flex items-baseline gap-2">
          <span class="text-3xl font-semibold tabular-nums">{credits.remaining}</span>
          <span class="text-sm text-muted-foreground">AI credits left this month</span>
        </div>
        <p class="mt-1 text-xs text-muted-foreground">Renews {fmtDate(credits.resets_at)}</p>
      </div>
    {/if}

    <div class="flex flex-col gap-3">
      <h2 class="text-sm font-medium text-muted-foreground">Transaction history</h2>
      {#if status === 'loading'}
        <States state="loading" />
      {:else if status === 'error'}
        <States state="error" message="Couldn't load your credit history." />
      {:else if history.length === 0}
        <States state="empty" message="No transactions yet. Your monthly grant and any credits you spend or earn will appear here." />
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
              <span
                class="shrink-0 text-sm font-semibold tabular-nums {entry.delta > 0
                  ? 'text-brand-strong'
                  : 'text-muted-foreground'}"
              >
                {fmtDelta(entry.delta)}
              </span>
            </li>
          {/each}
        </ul>
      {/if}
    </div>
  </div>
{/if}
