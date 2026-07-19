<script lang="ts">
  import { onMount } from 'svelte';
  import { FileText } from '@lucide/svelte';
  import { api, ApiError } from '$lib/api';
  import { AsyncData } from '$lib/asyncData.svelte';
  import { currentUser } from '$lib/auth.svelte';
  import type {
    IncomingReferralRequest,
    ReferralOffer,
    ReferralRequestStatus,
    SeekerReferralRequest,
  } from '$lib/types';
  import { Button } from '$lib/ui';
  import { timeAgo } from '$lib/utils';
  import CompanyPicker from './CompanyPicker.svelte';
  import States from './States.svelte';

  type Tab = 'requests' | 'offers' | 'incoming';
  let tab = $state<Tab>('requests');

  const requests = new AsyncData<SeekerReferralRequest[]>([]);
  const offers = new AsyncData<ReferralOffer[]>([]);
  const incoming = new AsyncData<IncomingReferralRequest[]>([]);

  const isModerator = $derived(['moderator', 'admin'].includes(currentUser()?.role ?? ''));

  onMount(() => {
    void requests.run(() => api.listMyReferralRequests());
    void offers.run(() => api.listMyReferralOffers());
    void incoming.run(() => api.listIncomingReferrals());
  });

  // Status pill treatment shared by request rows and inbox cards.
  const pillClass: Record<ReferralRequestStatus, string> = {
    sent: 'bg-muted text-muted-foreground',
    contacted: 'border-brand/30 bg-brand-muted text-brand-strong',
    declined: 'bg-muted text-muted-foreground line-through',
  };
  const pillLabel: Record<ReferralRequestStatus, string> = {
    sent: 'Sent',
    contacted: 'Contacted',
    declined: 'Declined',
  };
  const offerPill: Record<string, string> = {
    approved: 'border-brand/30 bg-brand-muted text-brand-strong',
    pending: 'bg-muted text-muted-foreground',
    rejected: 'bg-muted text-muted-foreground line-through',
  };

  // ── Offer to refer ──────────────────────────────────────────────────────
  let offerOpen = $state(false);
  let offerSlug = $state('');
  let offerFile = $state<FileList | null>(null);
  let offerBusy = $state(false);
  let offerError = $state<string | null>(null);

  function offerErrorMessage(err: unknown): string {
    if (err instanceof ApiError) {
      if (err.status === 409) return 'You already offered to refer for this company.';
      if (err.status === 404) return "We don't have that company — check the slug in its page URL.";
      if (err.status === 503) return 'File upload is unavailable right now.';
    }
    return 'Could not submit the offer. Please try again.';
  }

  async function submitOffer(e: SubmitEvent) {
    e.preventDefault();
    const file = offerFile?.[0];
    if (!offerSlug.trim() || !file) return;
    offerError = null;
    offerBusy = true;
    try {
      await api.submitReferralOffer(offerSlug.trim(), file);
      offerOpen = false;
      offerSlug = '';
      offerFile = null;
      await offers.run(() => api.listMyReferralOffers());
    } catch (err) {
      offerError = offerErrorMessage(err);
    } finally {
      offerBusy = false;
    }
  }

  // ── Incoming: mark contacted / declined ─────────────────────────────────
  async function resolve(req: IncomingReferralRequest, status: 'contacted' | 'declined') {
    try {
      await api.resolveReferral(req.id, status);
      // Drop it from the open inbox — resolved requests leave the pool.
      incoming.value = incoming.value.filter((r) => r.id !== req.id);
    } catch {
      // Best-effort UI; a reload reflects the true state.
      await incoming.run(() => api.listIncomingReferrals());
    }
  }
</script>

<div class="flex items-center justify-between gap-4">
  <div class="flex gap-1 border-b border-border" role="tablist">
    {#each [['requests', 'My requests'], ['offers', 'Offers to refer'], ['incoming', 'Incoming']] as [id, label] (id)}
      <button
        type="button"
        role="tab"
        aria-selected={tab === id}
        onclick={() => (tab = id as Tab)}
        class={[
          '-mb-px border-b-2 px-3 py-2.5 text-sm font-semibold',
          tab === id ? 'border-brand text-foreground' : 'border-transparent text-muted-foreground hover:text-foreground',
        ]}
      >
        {label}
        {#if id === 'incoming' && incoming.value.length > 0}
          <span class="ml-1.5 rounded-full bg-brand px-1.5 py-0.5 text-xs font-bold text-brand-foreground">
            {incoming.value.length}
          </span>
        {/if}
      </button>
    {/each}
  </div>
  {#if isModerator}
    <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- query-bearing link to a static route; resolve() can't carry the ?tab param -->
    <a href="/moderation?tab=referrals" class="text-sm font-medium text-brand-strong hover:underline">Review offers →</a>
  {/if}
</div>

<!-- ── My requests ── -->
{#if tab === 'requests'}
  {#if requests.status === 'loading'}
    <States state="loading" />
  {:else if requests.status === 'error'}
    <States state="error" />
  {:else if requests.value.length === 0}
    <States state="empty" message="You haven't requested any referrals yet." />
  {:else}
    <table class="mt-4 w-full text-sm">
      <thead>
        <tr class="text-xs uppercase tracking-wide text-muted-foreground">
          <th class="pb-2 pr-4 text-left font-semibold">Company</th>
          <th class="pb-2 pr-4 text-left font-semibold">CV shared</th>
          <th class="pb-2 pr-4 text-left font-semibold">Status</th>
          <th class="pb-2 text-left font-semibold">Sent</th>
        </tr>
      </thead>
      <tbody>
        {#each requests.value as r (r.id)}
          <tr class="border-t border-border">
            <td class="py-3 pr-4 font-medium">
              <a href="/companies/{r.company_slug}" class="hover:underline">{r.company_slug}</a>
            </td>
            <td class="py-3 pr-4 text-muted-foreground">
              {r.cv_kind === 'built' ? 'Tailored CV' : 'Uploaded CV'}
            </td>
            <td class="py-3 pr-4">
              <span class="inline-flex rounded-full px-2.5 py-0.5 text-xs font-semibold {pillClass[r.status]}">
                {pillLabel[r.status]}
              </span>
            </td>
            <td class="py-3 text-muted-foreground">{r.created_at ? timeAgo(r.created_at) : ''}</td>
          </tr>
        {/each}
      </tbody>
    </table>
    <p class="mt-4 text-xs text-muted-foreground">
      No notifications here — the referrer contacts you over the channel you left.
    </p>
  {/if}

<!-- ── Offers to refer ── -->
{:else if tab === 'offers'}
  <div class="mt-4 flex items-center justify-between">
    <p class="text-sm text-muted-foreground">Companies you can refer into.</p>
    <Button variant="primary" size="sm" onclick={() => (offerOpen = !offerOpen)}>+ Offer to refer</Button>
  </div>

  {#if offerOpen}
    <form onsubmit={submitOffer} class="mt-3 flex flex-col gap-3 rounded-lg border border-border p-4">
      <div class="flex flex-col gap-1.5 text-sm">
        <span class="font-medium">Company</span>
        <CompanyPicker onSelect={(c) => (offerSlug = c?.slug ?? '')} />
        <span class="text-xs text-muted-foreground">Search and pick the company you work at.</span>
      </div>
      <label class="flex flex-col gap-1.5 text-sm">
        <span class="font-medium">Proof of employment (PDF)</span>
        <input type="file" accept="application/pdf" bind:files={offerFile} class="text-sm" />
        <span class="text-xs text-muted-foreground">A CV or letter showing you work there. A moderator reviews it.</span>
      </label>
      {#if offerError}<p class="text-sm text-destructive">{offerError}</p>{/if}
      <div class="flex justify-end">
        <Button type="submit" variant="primary" size="sm" disabled={offerBusy || !offerSlug.trim() || !offerFile?.[0]}>
          {offerBusy ? 'Submitting…' : 'Submit for review'}
        </Button>
      </div>
    </form>
  {/if}

  {#if offers.status === 'loading'}
    <States state="loading" />
  {:else if offers.status === 'error'}
    <States state="error" />
  {:else if offers.value.length === 0}
    <States state="empty" message="You haven't offered to refer anywhere yet." />
  {:else}
    <ul class="mt-3">
      {#each offers.value as o (o.id)}
        <li class="flex items-center justify-between border-t border-border py-3 text-sm">
          <span class="font-medium">{o.company_slug}</span>
          <span class="inline-flex rounded-full px-2.5 py-0.5 text-xs font-semibold {offerPill[o.status] ?? 'bg-muted text-muted-foreground'}">
            {o.status}
          </span>
        </li>
      {/each}
    </ul>
  {/if}

<!-- ── Incoming ── -->
{:else}
  {#if incoming.status === 'loading'}
    <States state="loading" />
  {:else if incoming.status === 'error'}
    <States state="error" />
  {:else if incoming.value.length === 0}
    <States state="empty" message="No incoming referral requests." />
  {:else}
    <div class="mt-4 flex flex-col gap-3">
      {#each incoming.value as req (req.id)}
        <div class="rounded-lg border border-border p-4">
          <div class="flex items-center justify-between gap-4">
            <b class="text-sm">Someone wants a referral into {req.company_slug}</b>
            <span class="shrink-0 text-xs text-muted-foreground">{req.created_at ? timeAgo(req.created_at) : ''}</span>
          </div>
          <div class="mt-1.5 text-sm">
            Contact:
            {#if req.contact_telegram}<code class="rounded bg-muted px-1.5 py-0.5 text-xs">{req.contact_telegram}</code>{/if}
            {#if req.contact_email}<code class="rounded bg-muted px-1.5 py-0.5 text-xs">{req.contact_email}</code>{/if}
          </div>
          {#if req.note}<p class="mt-1 text-sm italic text-muted-foreground">“{req.note}”</p>{/if}
          <div class="mt-3 flex items-center gap-2">
            <Button variant="outline" size="sm" href={api.referralCvUrl(req.id)} target="_blank" rel="noopener">
              <FileText class="size-4" /> View CV
            </Button>
            <span class="flex-1"></span>
            <Button variant="primary" size="sm" onclick={() => resolve(req, 'contacted')}>Mark contacted</Button>
            <Button variant="outline" size="sm" onclick={() => resolve(req, 'declined')}>Decline</Button>
          </div>
        </div>
      {/each}
    </div>
    <p class="mt-4 text-xs text-muted-foreground">
      The seeker's identity is never shown — only the contact they chose to share. You reach out directly.
    </p>
  {/if}
{/if}
