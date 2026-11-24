<script lang="ts">
  import { ApiError, api } from '#lib/api.js';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { t } from '#lib/i18n/t.js';
  import { dollarsToCents } from '#lib/sponsored.js';
  import type { Job } from '#lib/types.js';
  import { Button, FormField } from '#lib/ui/index.js';
  import { messages } from './Sponsored.messages.js';

  // The buyer's first step: price, optional cap and the initial budget. Creates the campaign,
  // then starts the Stripe top-up and sends the browser there. The server owns the minimums,
  // so its 422 text is shown as-is.
  let { slug }: { slug: string | null } = $props();
  const s = $derived(t(messages, locale()));

  let job = $state<Job | null>(null);
  let loadError = $state<string | null>(null);
  let cpc = $state('0.50');
  let cap = $state('');
  let budget = $state('29');
  let busy = $state(false);
  let error = $state<string | null>(null);

  $effect(() => {
    job = null;
    loadError = null;
    if (!slug) return;
    api
      .getJob(slug)
      .then((j) => {
        job = j;
        if (j.closed_at) loadError = s.promoteForm.closed;
      })
      .catch(() => (loadError = s.promoteForm.notFound));
  });

  const input =
    'w-full rounded-lg border border-input bg-transparent px-3 py-2 text-sm focus-visible:border-ring focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring';

  async function submit(e: SubmitEvent) {
    e.preventDefault();
    if (busy || !job) return;
    const cpcCents = dollarsToCents(cpc);
    const budgetCents = dollarsToCents(budget);
    const capCents = cap.trim() ? dollarsToCents(cap) : undefined;
    if (cpcCents === null || budgetCents === null || capCents === null) {
      error = s.promoteForm.invalid;
      return;
    }
    busy = true;
    error = null;
    let created: Awaited<ReturnType<typeof api.createSponsoredCampaign>>;
    try {
      created = await api.createSponsoredCampaign({
        job_slug: job.public_slug,
        cpc_cents: cpcCents,
        daily_cap_cents: capCents,
      });
    } catch (err) {
      error = err instanceof ApiError ? err.message : s.promoteForm.failed;
      busy = false;
      return;
    }
    try {
      const { url } = await api.topupSponsoredCampaign(created.id, budgetCents);
      // Stripe Checkout is another origin: a full navigation, not a router move.
      window.location.assign(url);
    } catch (err) {
      error = `${err instanceof ApiError ? err.message : ''} ${s.promoteForm.paymentFailed}`.trim();
      busy = false;
    }
  }
</script>

<div class="flex max-w-xl flex-col gap-4">
  <h1 class="text-2xl font-semibold tracking-tight">{s.promoteForm.title}</h1>
  {#if !slug}
    <p class="text-sm text-muted-foreground">{s.promoteForm.jobMissing}</p>
  {:else if loadError}
    <p role="alert" class="text-sm text-destructive">{loadError}</p>
  {:else if job}
    <p class="text-sm font-medium">{job.title} · {job.company}</p>
    <p class="text-sm text-muted-foreground">{s.promoteForm.intro}</p>
    <form class="flex flex-col gap-4" onsubmit={submit}>
      <FormField label={s.promoteForm.cpc} hint={s.promoteForm.cpcHint} required>
        {#snippet children({ id, describedBy })}
          <input {id} name="cpc" inputmode="decimal" aria-describedby={describedBy} bind:value={cpc} class={input} />
        {/snippet}
      </FormField>
      <FormField label={s.promoteForm.cap} hint={s.promoteForm.capHint}>
        {#snippet children({ id, describedBy })}
          <input {id} name="cap" inputmode="decimal" aria-describedby={describedBy} bind:value={cap} class={input} />
        {/snippet}
      </FormField>
      <FormField label={s.promoteForm.budget} hint={s.promoteForm.budgetHint} required>
        {#snippet children({ id, describedBy })}
          <input
            {id}
            name="budget"
            inputmode="decimal"
            aria-describedby={describedBy}
            bind:value={budget}
            class={input}
          />
        {/snippet}
      </FormField>
      {#if error}<p role="alert" class="text-sm text-destructive">{error}</p>{/if}
      <div>
        <Button type="submit" variant="primary" disabled={busy}>
          {busy ? s.promoteForm.submitting : s.promoteForm.submit}
        </Button>
      </div>
    </form>
  {/if}
</div>
