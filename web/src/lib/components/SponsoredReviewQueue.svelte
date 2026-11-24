<script lang="ts">
  import { onMount } from 'svelte';
  import type { SponsoredReviewItem } from '#lib/api-methods/sponsored.js';
  import { ApiError, api } from '#lib/api.js';
  import { AsyncData } from '#lib/asyncData.svelte.js';
  import { usd } from '#lib/sponsored.js';
  import { Button } from '#lib/ui/index.js';
  import { resolve } from '$app/paths';
  import States from './States.svelte';

  // Rendered only inside the moderator-gated ModerationView; the API authorizes independently.
  const queue = new AsyncData<SponsoredReviewItem[]>([]);
  onMount(() => void queue.run(() => api.listSponsoredReview()));

  let busy = $state<number | null>(null);
  let error = $state<string | null>(null);
  let notice = $state<string | null>(null);
  let rejecting = $state<number | null>(null);
  let reason = $state('');

  const drop = (id: number) => (queue.value = queue.value.filter((x) => x.id !== id));
  const failure = (e: unknown) => (e instanceof ApiError ? e.message : 'That did not work.');

  async function decide(id: number, run: () => Promise<void>) {
    if (busy !== null) return;
    busy = id;
    error = notice = null;
    try {
      await run();
      drop(id);
    } catch (e) {
      error = failure(e);
    } finally {
      busy = null;
    }
  }

  const approve = (id: number) => decide(id, async () => void (await api.approveSponsoredCampaign(id)));
  const reject = (id: number) =>
    decide(id, async () => {
      const r = await api.rejectSponsoredCampaign(id, reason.trim());
      rejecting = null;
      reason = '';
      if (r.refund_pending) notice = 'Rejected. The refund is pending and will be retried automatically.';
    });
</script>

<div class="flex flex-col gap-6">
  <p class="text-sm text-muted-foreground">
    Sponsored campaigns awaiting review. Approving starts the campaign; rejecting refunds the budget and shows the
    reason to the buyer.
  </p>
  {#if error}<p role="alert" class="text-sm text-destructive">{error}</p>{/if}
  {#if notice}<p role="status" class="text-sm">{notice}</p>{/if}

  {#if queue.status === 'loading'}
    <States state="loading" />
  {:else if queue.status === 'error'}
    <States state="error" message="Couldn't load the queue." />
  {:else if queue.value.length === 0}
    <States state="empty" message="No campaigns awaiting review." />
  {:else}
    <ul class="flex flex-col divide-y divide-border rounded-lg border border-border">
      {#each queue.value as k (k.id)}
        <li class="flex flex-col gap-3 px-4 py-3">
          <div class="flex flex-col gap-0.5">
            <a href={resolve('/jobs/[slug]', { slug: k.job_slug })} class="text-sm font-medium hover:underline">
              {k.job_title}
            </a>
            <span class="text-xs text-muted-foreground">
              {k.job_company} · {usd(k.cpc_cents)} per click · budget {usd(k.budget_cents)}
              {#if k.daily_cap_cents}
                · cap {usd(k.daily_cap_cents)}/day{/if} · buyer #{k.owner_user_id}
            </span>
          </div>
          {#if rejecting === k.id}
            <form
              class="flex flex-col gap-2"
              onsubmit={(e) => {
                e.preventDefault();
                void reject(k.id);
              }}
            >
              <label class="flex flex-col gap-1 text-xs">
                Reason (required, shown to the buyer)
                <textarea
                  name="reason"
                  rows="2"
                  maxlength={1000}
                  bind:value={reason}
                  class="w-full resize-y rounded-lg border border-input bg-transparent px-3 py-2 text-sm"></textarea>
              </label>
              <div class="flex gap-2">
                <Button type="submit" variant="destructive" size="sm" disabled={busy !== null || !reason.trim()}>
                  Reject
                </Button>
                <Button variant="ghost" size="sm" onclick={() => (rejecting = null)}>Cancel</Button>
              </div>
            </form>
          {:else}
            <div class="flex gap-2">
              <Button variant="primary" size="sm" disabled={busy !== null} onclick={() => approve(k.id)}>Approve</Button
              >
              <Button
                variant="ghost"
                size="sm"
                disabled={busy !== null}
                onclick={() => {
                  rejecting = k.id;
                  reason = '';
                }}
              >
                Reject
              </Button>
            </div>
          {/if}
        </li>
      {/each}
    </ul>
  {/if}
</div>
