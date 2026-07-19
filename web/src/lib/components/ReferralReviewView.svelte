<script lang="ts">
  import { onMount } from 'svelte';
  import { FileText } from '@lucide/svelte';
  import { api } from '$lib/api';
  import { AsyncData } from '$lib/asyncData.svelte';
  import type { ReferralOffer } from '$lib/types';
  import { Button } from '$lib/ui';
  import { timeAgo } from '$lib/utils';
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
  <table class="w-full text-sm">
    <thead>
      <tr class="text-xs uppercase tracking-wide text-muted-foreground">
        <th class="pb-2 pr-4 text-left font-semibold">Company</th>
        <th class="pb-2 pr-4 text-left font-semibold">Submitted</th>
        <th class="pb-2 pr-4 text-left font-semibold">Proof</th>
        <th class="pb-2 text-right font-semibold">Decision</th>
      </tr>
    </thead>
    <tbody>
      {#each pending.value as o (o.id)}
        <tr class="border-t border-border">
          <td class="py-3 pr-4 font-medium">{o.company_slug}</td>
          <td class="py-3 pr-4 text-muted-foreground">{o.created_at ? timeAgo(o.created_at) : ''}</td>
          <td class="py-3 pr-4">
            <Button variant="outline" size="sm" href={api.referralProofUrl(o.id)} target="_blank" rel="noopener">
              <FileText class="size-4" /> View proof
            </Button>
          </td>
          <td class="py-3 text-right">
            <div class="inline-flex gap-2">
              <Button variant="primary" size="sm" disabled={busy === o.id} onclick={() => decide(o, true)}>Approve</Button>
              <Button variant="outline" size="sm" disabled={busy === o.id} onclick={() => decide(o, false)}>Reject</Button>
            </div>
          </td>
        </tr>
      {/each}
    </tbody>
  </table>
  <p class="mt-4 text-xs text-muted-foreground">
    Verify the proof shows the person works there — the only trust gate. No automated verification.
  </p>
{/if}
