<script lang="ts">
  import { onMount } from 'svelte';
  import { FileText } from '@lucide/svelte';
  import { api } from '$lib/api';
  import { AsyncData } from '$lib/asyncData.svelte';
  import type { ReferralOffer } from '$lib/types';
  import { Button } from '$lib/ui';
  import { timeAgo } from '$lib/utils';
  import CompanyLogo from './CompanyLogo.svelte';
  import States from './States.svelte';

  const pending = new AsyncData<ReferralOffer[]>([]);
  let busy = $state<number | null>(null);

  onMount(() => void pending.run(() => api.listPendingReferralOffers()));

  async function decide(offer: ReferralOffer, approve: boolean) {
    busy = offer.id;
    try {
      await api.decideReferralOffer(offer.id, approve);
      pending.value = pending.value.filter((o) => o.id !== offer.id);
    } catch {
      await pending.run(() => api.listPendingReferralOffers());
    } finally {
      busy = null;
    }
  }
</script>

{#if pending.status === 'loading'}
  <States state="loading" />
{:else if pending.status === 'error'}
  <States state="error" />
{:else if pending.value.length === 0}
  <States state="empty" message="No referral offers awaiting review." />
{:else}
  <!-- Card rows (not a table) so the actions stack cleanly on mobile instead of
       overflowing, matching the moderation-queue list in the same console. -->
  <ul class="flex flex-col divide-y divide-border rounded-lg border border-border">
    {#each pending.value as o (o.id)}
      <li class="flex flex-col gap-3 px-4 py-3 sm:flex-row sm:items-center sm:justify-between">
        <div class="flex min-w-0 items-center gap-2">
          <CompanyLogo name={o.company_name || o.company_slug} size="size-6" />
          <div class="flex min-w-0 flex-col gap-0.5">
            <span class="truncate text-sm font-medium">{o.company_name || o.company_slug}</span>
            <span class="truncate text-xs text-muted-foreground">
              Submitted {o.created_at ? timeAgo(o.created_at) : ''}
            </span>
          </div>
        </div>
        <div class="flex shrink-0 flex-wrap gap-2">
          <Button variant="outline" size="sm" href={api.referralProofUrl(o.id)} target="_blank" rel="noopener">
            <FileText class="size-4" /> View proof
          </Button>
          <Button variant="primary" size="sm" disabled={busy === o.id} onclick={() => decide(o, true)}>Approve</Button>
          <Button variant="outline" size="sm" disabled={busy === o.id} onclick={() => decide(o, false)}>Reject</Button>
        </div>
      </li>
    {/each}
  </ul>
  <p class="mt-4 text-xs text-muted-foreground">
    Verify the proof shows the person works there — the only trust gate. No automated verification.
  </p>
{/if}
