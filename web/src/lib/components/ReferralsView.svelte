<script lang="ts">
  import { onMount } from 'svelte';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { replaceState } from '$app/navigation';
  import { FileText } from '@lucide/svelte';
  import { api, ApiError } from '$lib/api';
  import { tablist } from '$lib/actions/tablist';
  import { AsyncData } from '$lib/asyncData.svelte';
  import type {
    IncomingReferralRequest,
    ReferralOffer,
    ReferralRequestStatus,
    SeekerReferralRequest,
  } from '$lib/types';
  import { locale } from '$lib/i18n/currentLocale.svelte';
  import { t, tokenLabel } from '$lib/i18n/t';
  import { companyLogoUrl } from '$lib/logo';
  import { Button, ConfirmDialog, EntityLogo, FormField, Table } from '$lib/ui';
  import { isLinkedInUrl, timeAgo } from '$lib/utils';
  import CompanyPicker from './CompanyPicker.svelte';
  import { messages } from './ReferralsView.messages';
  import States from './States.svelte';

  const s = $derived(t(messages, locale()));

  type Tab = 'requests' | 'offers' | 'incoming';
  const tabs: Tab[] = ['requests', 'offers', 'incoming'];

  // Open on the tab named in `?tab=` so deep-links land right — notably the
  // "new referral request" ping, which links approved referrers to `?tab=incoming`.
  function readTab(): Tab {
    // Not `t` — that name is the catalog resolver imported above.
    const requested = page.url.searchParams.get('tab');
    return tabs.includes(requested as Tab) ? (requested as Tab) : 'requests';
  }
  let tab = $state<Tab>(readTab());
  function selectTab(next: Tab) {
    if (next === tab) return;
    tab = next;
    // eslint-disable-next-line svelte/no-navigation-without-resolve -- in-place query write to the current pathname; there is no route to resolve
    replaceState(`${page.url.pathname}?tab=${next}`, {});
  }

  const requests = new AsyncData<SeekerReferralRequest[]>([]);
  const offers = new AsyncData<ReferralOffer[]>([]);
  const incoming = new AsyncData<IncomingReferralRequest[]>([]);

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
  const offerPill: Record<string, string> = {
    approved: 'border-brand/30 bg-brand-muted text-brand-strong',
    pending: 'bg-muted text-muted-foreground',
    rejected: 'bg-muted text-muted-foreground line-through',
  };

  // ── Offer to refer ──────────────────────────────────────────────────────
  let offerOpen = $state(false);
  let offerSlug = $state('');
  let offerLinkedin = $state('');
  let offerFile = $state<FileList | null>(null);
  let offerBusy = $state(false);
  let offerError = $state<string | null>(null);

  const offerLinkedinValid = $derived(isLinkedInUrl(offerLinkedin));
  const canSubmitOffer = $derived(
    offerSlug.trim() !== '' && offerLinkedinValid && !!offerFile?.[0],
  );

  function offerErrorMessage(err: unknown): string {
    if (err instanceof ApiError) {
      if (err.status === 409) return s.offers.errors.duplicate;
      if (err.status === 404) return s.offers.errors.unknownCompany;
      if (err.status === 422) return s.offers.errors.badLinkedin;
      if (err.status === 503) return s.offers.errors.uploadUnavailable;
    }
    return s.offers.errors.generic;
  }

  async function submitOffer(e: SubmitEvent) {
    e.preventDefault();
    const file = offerFile?.[0];
    if (!canSubmitOffer || !file) return;
    offerError = null;
    offerBusy = true;
    try {
      await api.submitReferralOffer(offerSlug.trim(), offerLinkedin.trim(), file);
      offerOpen = false;
      offerSlug = '';
      offerLinkedin = '';
      offerFile = null;
      await offers.run(() => api.listMyReferralOffers());
    } catch (err) {
      offerError = offerErrorMessage(err);
    } finally {
      offerBusy = false;
    }
  }

  // Stop being a referrer: delete the offer after a confirm, then drop it optimistically
  // (reloading on failure to resurface it). `withdrawing` disables the acting row's button.
  let withdrawing = $state<string | null>(null);
  let withdrawTarget = $state<ReferralOffer | null>(null);
  let confirmWithdrawOpen = $state(false);

  function requestWithdraw(o: ReferralOffer) {
    if (withdrawing !== null) return;
    withdrawTarget = o;
    confirmWithdrawOpen = true;
  }

  async function withdrawOffer() {
    const o = withdrawTarget;
    if (!o) return;
    withdrawing = o.id;
    try {
      await api.withdrawReferralOffer(o.id);
      offers.value = offers.value.filter((x) => x.id !== o.id);
    } catch {
      await offers.run(() => api.listMyReferralOffers());
    } finally {
      withdrawing = null;
    }
  }

  // ── Incoming: mark contacted / declined ─────────────────────────────────
  async function resolveRequest(req: IncomingReferralRequest, status: 'contacted' | 'declined') {
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
  <!-- use:tablist is what makes role="tablist" true. Without it the group announces
       itself as one widget and then cannot be stepped through: every tab stays in the
       Tab sequence and the arrow keys do nothing — the promise without the behaviour. -->
  <div class="flex gap-1 border-b border-border" role="tablist" use:tablist={tab}>
    {#each tabs as id (id)}
      <button
        type="button"
        role="tab"
        aria-selected={tab === id}
        onclick={() => selectTab(id)}
        class={[
          '-mb-px border-b-2 px-3 py-2.5 text-sm font-semibold',
          tab === id ? 'border-brand text-foreground' : 'border-transparent text-muted-foreground hover:text-foreground',
        ]}
      >
        {s.tabs[id]}
        {#if id === 'incoming' && incoming.value.length > 0}
          <span class="ml-1.5 rounded-full bg-brand px-1.5 py-0.5 text-xs font-bold text-brand-foreground">
            {incoming.value.length}
          </span>
        {/if}
      </button>
    {/each}
  </div>
</div>

<!-- ── My requests ── -->
{#if tab === 'requests'}
  {#if requests.status === 'loading'}
    <States state="loading" />
  {:else if requests.status === 'error'}
    <States state="error" />
  {:else if requests.value.length === 0}
    <States state="empty" message={s.requests.empty} />
  {:else}
    <Table class="mt-4">
      {#snippet header()}
        <tr class="text-xs uppercase tracking-wide text-muted-foreground">
          <th class="pb-2 pr-4 text-left font-semibold">{s.requests.columns.company}</th>
          <th class="pb-2 pr-4 text-left font-semibold">{s.requests.columns.cvShared}</th>
          <th class="pb-2 pr-4 text-left font-semibold">{s.requests.columns.status}</th>
          <th class="pb-2 text-left font-semibold">{s.requests.columns.sent}</th>
        </tr>
      {/snippet}
      {#each requests.value as r (r.id)}
        <tr class="border-t border-border">
          <td class="py-3 pr-4 font-medium">
            <a href={resolve('/companies/[slug]', { slug: r.company_slug })} class="flex items-center gap-2 hover:underline">
              <EntityLogo
                name={r.company_name || r.company_slug}
                src={companyLogoUrl(r.company_name || r.company_slug) ?? undefined}
                shape="square"
                size="xs"
              />
              <span class="min-w-0 truncate">{r.company_name || r.company_slug}</span>
            </a>
          </td>
          <td class="py-3 pr-4 text-muted-foreground">
            {r.cv_kind === 'built' ? s.requests.cvBuilt : s.requests.cvUploaded}
          </td>
          <td class="py-3 pr-4">
            <span class="inline-flex rounded-full px-2.5 py-0.5 text-xs font-semibold {pillClass[r.status]}">
              {tokenLabel(s.requests.status, r.status)}
            </span>
          </td>
          <td class="py-3 text-muted-foreground">{r.created_at ? timeAgo(r.created_at) : ''}</td>
        </tr>
      {/each}
    </Table>
    <p class="mt-4 text-xs text-muted-foreground">{s.requests.footnote}</p>
  {/if}

<!-- ── Offers to refer ── -->
{:else if tab === 'offers'}
  <div class="mt-4 flex items-center justify-between">
    <p class="text-sm text-muted-foreground">{s.offers.lead}</p>
    <Button variant="primary" size="sm" onclick={() => (offerOpen = !offerOpen)}>
      {s.offers.openForm}
    </Button>
  </div>

  {#if offerOpen}
    <form onsubmit={submitOffer} class="mt-3 flex flex-col gap-3 rounded-lg border border-border p-4">
      <div class="flex flex-col gap-1.5 text-sm">
        <span class="font-medium">{s.offers.companyLabel}</span>
        <CompanyPicker onSelect={(c) => (offerSlug = c?.slug ?? '')} />
        <span class="text-xs text-muted-foreground">{s.offers.companyHint}</span>
      </div>
      <FormField
        label={s.offers.linkedinLabel}
        error={offerLinkedin.trim() !== '' && !offerLinkedinValid
          ? s.offers.linkedinInvalid
          : undefined}
        hint={s.offers.linkedinHint}
      >
        {#snippet children({ id, describedBy, invalid })}
          <input
            {id}
            type="url"
            bind:value={offerLinkedin}
            placeholder="https://linkedin.com/in/your-handle"
            aria-invalid={invalid}
            aria-describedby={describedBy}
            class="rounded-md border border-border bg-background px-3 py-2 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring aria-[invalid=true]:border-destructive"
          />
        {/snippet}
      </FormField>
      <label class="flex flex-col gap-1.5 text-sm">
        <span class="font-medium">{s.offers.proofLabel}</span>
        <input type="file" accept="application/pdf" bind:files={offerFile} class="text-sm" />
        <span class="text-xs text-muted-foreground">{s.offers.proofHint}</span>
      </label>
      {#if offerError}<p class="text-sm text-destructive">{offerError}</p>{/if}
      <div class="flex justify-end">
        <Button type="submit" variant="primary" size="sm" disabled={offerBusy || !canSubmitOffer}>
          {offerBusy ? s.offers.submitting : s.offers.submit}
        </Button>
      </div>
    </form>
  {/if}

  {#if offers.status === 'loading'}
    <States state="loading" />
  {:else if offers.status === 'error'}
    <States state="error" />
  {:else if offers.value.length === 0}
    <States state="empty" message={s.offers.empty} />
  {:else}
    <ul class="mt-3">
      {#each offers.value as o (o.id)}
        <li class="flex flex-wrap items-center gap-x-3 gap-y-2 border-t border-border py-3 text-sm">
          <EntityLogo
            name={o.company_name || o.company_slug}
            src={companyLogoUrl(o.company_name || o.company_slug) ?? undefined}
            shape="square"
            size="xs"
          />
          <span class="min-w-0 truncate font-medium">{o.company_name || o.company_slug}</span>
          <span class="inline-flex rounded-full px-2.5 py-0.5 text-xs font-semibold {offerPill[o.status] ?? 'bg-muted text-muted-foreground'}">
            {tokenLabel(s.offers.status, o.status)}
          </span>
          <Button
            variant="ghost"
            size="sm"
            class="ml-auto text-muted-foreground hover:text-destructive"
            disabled={withdrawing === o.id}
            onclick={() => requestWithdraw(o)}
          >
            {withdrawing === o.id ? s.offers.withdrawing : s.offers.withdraw}
          </Button>
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
    <States state="empty" message={s.incoming.empty} />
  {:else}
    <div class="mt-4 flex flex-col gap-3">
      {#each incoming.value as req (req.id)}
        <div class="rounded-lg border border-border p-4">
          <div class="flex items-center justify-between gap-4">
            <b class="flex min-w-0 items-center gap-2 text-sm">
              <EntityLogo
                name={req.company_name || req.company_slug}
                src={companyLogoUrl(req.company_name || req.company_slug) ?? undefined}
                shape="square"
                size="xs"
              />
              <span class="min-w-0 truncate">
                {s.incoming.wantsReferralPrefix}
                {req.company_name || req.company_slug}
              </span>
            </b>
            <span class="shrink-0 text-xs text-muted-foreground">{req.created_at ? timeAgo(req.created_at) : ''}</span>
          </div>
          <div class="mt-1.5 flex flex-wrap items-center gap-2 text-sm">
            <span>{s.incoming.contactLabel}</span>
            {#if req.contact_telegram}<code class="rounded bg-muted px-1.5 py-0.5 text-xs">{req.contact_telegram}</code>{/if}
            {#if req.contact_email}<code class="rounded bg-muted px-1.5 py-0.5 text-xs">{req.contact_email}</code>{/if}
            {#if req.linkedin_url}
              <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- external LinkedIn URL, not an internal route -->
              <a href={req.linkedin_url} target="_blank" rel="noopener" class="text-brand-strong hover:underline">{s.incoming.linkedin}</a>
            {/if}
          </div>
          {#if req.note}<p class="mt-1 text-sm italic text-muted-foreground">“{req.note}”</p>{/if}
          <div class="mt-3 flex items-center gap-2">
            <Button variant="outline" size="sm" href={api.referralCvUrl(req.id)} target="_blank" rel="noopener">
              <FileText class="size-4" />
              {s.incoming.viewCv}
            </Button>
            <span class="flex-1"></span>
            <Button variant="primary" size="sm" onclick={() => resolveRequest(req, 'contacted')}>
              {s.incoming.markContacted}
            </Button>
            <Button variant="outline" size="sm" onclick={() => resolveRequest(req, 'declined')}>
              {s.incoming.decline}
            </Button>
          </div>
        </div>
      {/each}
    </div>
    <p class="mt-4 text-xs text-muted-foreground">{s.incoming.footnote}</p>
  {/if}
{/if}

<ConfirmDialog
  bind:open={confirmWithdrawOpen}
  title={`${s.offers.withdrawDialog.titlePrefix} ${withdrawTarget?.company_name || withdrawTarget?.company_slug || ''}${s.offers.withdrawDialog.titleSuffix}`}
  description={s.offers.withdrawDialog.description}
  confirmLabel={s.offers.withdrawDialog.confirmLabel}
  variant="destructive"
  onConfirm={withdrawOffer}
/>
