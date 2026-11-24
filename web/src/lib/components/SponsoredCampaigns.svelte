<script lang="ts">
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import { onMount } from 'svelte';
  import type { SponsoredCampaign, SponsoredCampaignDetail } from '#lib/api-methods/sponsored.js';
  import { ApiError, api } from '#lib/api.js';
  import { AsyncData } from '#lib/asyncData.svelte.js';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { t, tokenLabel } from '#lib/i18n/t.js';
  import { dollarsToCents, usd } from '#lib/sponsored.js';
  import { Button, ConfirmDialog } from '#lib/ui/index.js';
  import { messages } from './Sponsored.messages.js';
  import States from './States.svelte';

  const s = $derived(t(messages, locale()));
  const c = $derived(s.campaigns);
  const money = (cents: number) => usd(cents, locale());

  const campaigns = new AsyncData<SponsoredCampaign[]>([]);
  onMount(() => void campaigns.run(() => api.listSponsoredCampaigns()));

  // Stripe returns the browser here with ?topup=success|cancel.
  const topup = $derived(page.url.searchParams.get('topup'));

  let openId = $state<number | null>(null);
  let detail = $state<SponsoredCampaignDetail | null>(null);
  let busy = $state(false);
  let error = $state<string | null>(null);
  let topupAmount = $state('29');
  let endTarget = $state<SponsoredCampaign | null>(null);
  let endOpen = $state(false);

  const failure = (e: unknown) => (e instanceof ApiError ? e.message : c.actionFailed);

  async function toggle(id: number) {
    error = null;
    detail = null;
    if (openId === id) {
      openId = null;
      return;
    }
    openId = id;
    try {
      detail = await api.getSponsoredCampaign(id);
    } catch (e) {
      error = failure(e);
    }
  }

  async function act(run: () => Promise<SponsoredCampaign>) {
    if (busy) return;
    busy = true;
    error = null;
    try {
      const next = await run();
      campaigns.value = campaigns.value.map((x) => (x.id === next.id ? next : x));
    } catch (e) {
      error = failure(e);
    } finally {
      busy = false;
    }
  }

  async function pay(id: number) {
    const cents = dollarsToCents(topupAmount);
    if (busy || cents === null) {
      error = c.actionFailed;
      return;
    }
    busy = true;
    error = null;
    try {
      const { url } = await api.topupSponsoredCampaign(id, cents);
      window.location.assign(url);
    } catch (e) {
      error = failure(e);
      busy = false;
    }
  }
</script>

<div class="flex flex-col gap-6">
  <div>
    <h1 class="text-2xl font-semibold tracking-tight">{c.title}</h1>
    <p class="mt-1 text-sm text-muted-foreground">{c.description}</p>
  </div>

  {#if topup === 'success'}
    <p role="status" class="rounded-lg border border-border bg-secondary px-4 py-3 text-sm">{c.topupSuccess}</p>
  {:else if topup === 'cancel'}
    <p role="status" class="rounded-lg border border-border bg-secondary px-4 py-3 text-sm">{c.topupCancel}</p>
  {/if}

  {#if error}<p role="alert" class="text-sm text-destructive">{error}</p>{/if}

  {#if campaigns.status === 'loading'}
    <States state="loading" />
  {:else if campaigns.status === 'error'}
    <States state="error" message={c.loadFailed} />
  {:else if campaigns.value.length === 0}
    <States state="empty" message={c.empty} />
  {:else}
    <ul class="flex flex-col divide-y divide-border rounded-lg border border-border">
      {#each campaigns.value as k (k.id)}
        <li class="flex flex-col gap-3 px-4 py-3">
          <div class="flex flex-wrap items-start justify-between gap-2">
            <div class="min-w-0">
              <a href={resolve('/jobs/[slug]', { slug: k.job_slug })} class="text-sm font-medium hover:underline">
                {k.job_title}
              </a>
              <p class="text-xs text-muted-foreground">{k.job_company}</p>
            </div>
            <span class="rounded-md border border-border px-2 py-0.5 text-xs font-medium">
              {tokenLabel(s.status, k.status)}
            </span>
          </div>
          <dl class="grid grid-cols-2 gap-x-4 gap-y-1 text-xs sm:grid-cols-4">
            <div>
              <dt class="text-muted-foreground">{c.cpc}</dt>
              <dd>{money(k.cpc_cents)}</dd>
            </div>
            <div>
              <dt class="text-muted-foreground">{c.budget}</dt>
              <dd>{money(k.budget_cents)}</dd>
            </div>
            <div>
              <dt class="text-muted-foreground">{c.spent}</dt>
              <dd>{money(k.spent_cents)}</dd>
            </div>
            <div>
              <dt class="text-muted-foreground">{c.remaining}</dt>
              <dd>{money(k.remaining_cents)}</dd>
            </div>
          </dl>
          {#if k.reject_reason}
            <p class="text-sm text-destructive">{c.rejectedWhy}{k.reject_reason}</p>
          {/if}
          <div class="flex flex-wrap gap-2">
            <Button variant="outline" size="sm" aria-expanded={openId === k.id} onclick={() => toggle(k.id)}>
              {openId === k.id ? c.hideDetails : c.details}
            </Button>
            {#if k.status === 'active'}
              <Button
                variant="ghost"
                size="sm"
                disabled={busy}
                onclick={() => act(() => api.pauseSponsoredCampaign(k.id))}
              >
                {c.pause}
              </Button>
            {:else if k.status === 'paused'}
              <Button
                variant="ghost"
                size="sm"
                disabled={busy}
                onclick={() => act(() => api.resumeSponsoredCampaign(k.id))}
              >
                {c.resume}
              </Button>
            {/if}
            {#if k.status !== 'ended' && k.status !== 'rejected'}
              <Button
                variant="ghost"
                size="sm"
                disabled={busy}
                onclick={() => {
                  endTarget = k;
                  endOpen = true;
                }}
              >
                {c.end}
              </Button>
            {/if}
          </div>

          {#if openId === k.id}
            <div class="flex flex-col gap-4 border-t border-border pt-3">
              {#if k.status !== 'ended' && k.status !== 'rejected'}
                <form
                  class="flex flex-wrap items-end gap-2"
                  onsubmit={(e) => {
                    e.preventDefault();
                    void pay(k.id);
                  }}
                >
                  <label class="flex flex-col gap-1 text-xs">
                    {c.topupAmount}
                    <input
                      name="topup"
                      inputmode="decimal"
                      bind:value={topupAmount}
                      class="w-32 rounded-lg border border-input bg-transparent px-3 py-2 text-sm"
                    />
                  </label>
                  <Button type="submit" variant="primary" size="sm" disabled={busy}>{c.topupGo}</Button>
                </form>
              {/if}
              {#if detail && detail.campaign.id === k.id}
                <h2 class="text-sm font-semibold">{c.days}</h2>
                {#if detail.days.length === 0}
                  <p class="text-sm text-muted-foreground">{c.noDays}</p>
                {:else}
                  <div class="overflow-x-auto">
                    <table class="w-full text-left text-xs">
                      <thead class="text-muted-foreground">
                        <tr>
                          <th class="py-1 pr-3 font-medium">{c.day}</th>
                          <th class="py-1 pr-3 font-medium">{c.impressions}</th>
                          <th class="py-1 pr-3 font-medium">{c.validClicks}</th>
                          <th class="py-1 pr-3 font-medium">{c.invalidClicks}</th>
                          <th class="py-1 font-medium">{c.spend}</th>
                        </tr>
                      </thead>
                      <tbody>
                        {#each detail.days as d (d.day)}
                          <tr class="border-t border-border">
                            <td class="py-1 pr-3">{d.day}</td>
                            <td class="py-1 pr-3">{d.impressions}</td>
                            <td class="py-1 pr-3">{d.valid_clicks}</td>
                            <td class="py-1 pr-3">{d.invalid_clicks}</td>
                            <td class="py-1">{money(d.spent_cents)}</td>
                          </tr>
                        {/each}
                      </tbody>
                    </table>
                  </div>
                {/if}
                {#if detail.invalid_clicks_by_reason.length > 0}
                  <h2 class="text-sm font-semibold">{c.invalidTitle}</h2>
                  <ul class="text-sm">
                    {#each detail.invalid_clicks_by_reason as r (r.reason)}
                      <li>{tokenLabel(s.reason, r.reason)}: {r.clicks}</li>
                    {/each}
                  </ul>
                {/if}
              {/if}
            </div>
          {/if}
        </li>
      {/each}
    </ul>
  {/if}
</div>

<ConfirmDialog
  bind:open={endOpen}
  title={c.endTitle}
  description={c.endBody}
  confirmLabel={c.end}
  variant="destructive"
  onConfirm={() => {
    const target = endTarget;
    return target ? act(() => api.endSponsoredCampaign(target.id)) : undefined;
  }}
/>
