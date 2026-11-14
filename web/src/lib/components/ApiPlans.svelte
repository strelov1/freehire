<script lang="ts">
  import { buttonVariants, cn, type ButtonVariant } from '#lib/ui/index.js';
  import { resolve } from '$app/paths';
  import { api } from '#lib/api.js';
  import { isAuthenticated } from '#lib/auth.svelte.js';
  import { formatMinorUnits } from '#lib/money.js';
  import { promptSignIn } from '#lib/signin.js';
  import type { ApiPlan, PublicPrice } from '#lib/types.js';

  // `buttonVariant`: the API's own page sells the API, so its buttons are the page's CTA;
  // under the Pro plan on /pricing they step down to outline so Pro keeps the one fill.
  const { prices, buttonVariant = 'cta' }: { prices: PublicPrice[]; buttonVariant?: ButtonVariant } = $props();

  // One card per tier, and only for a tier that is actually sold: the monthly price when there
  // is one, otherwise whatever interval the deployment offers.
  const pick = (tier: PublicPrice['tier']) => {
    const own = prices.filter((p) => p.tier === tier);
    return own.find((p) => p.interval === 'month') ?? own[0] ?? null;
  };
  const apiPrice = $derived(pick('api'));
  const dataPrice = $derived(pick('api_data'));

  const money = (p: PublicPrice) => formatMinorUnits(p.amount_cents, p.currency, undefined, { trimWhole: true });

  // The tier the caller already holds, so its card says so instead of selling it again.
  let held = $state<ApiPlan['tier']>('');
  // Cleared first: a sign-out or account switch without a reload must not keep the last tier.
  $effect(() => {
    held = '';
    if (!isAuthenticated()) return;
    let live = true;
    api
      .getApiPlan()
      .then((p) => live && (held = p.tier))
      .catch(() => {});
    return () => {
      live = false;
    };
  });

  // Which plan's checkout is opening, so only its own button says so.
  let busyId = $state<string | null>(null);
  let error = $state<string | null>(null);

  async function buy(p: PublicPrice) {
    if (busyId) return;
    busyId = p.id;
    error = null;
    try {
      const { url } = await api.billingCheckout(p.id);
      window.location.href = url;
    } catch {
      error = 'Checkout is not available right now. Please try again in a moment.';
      busyId = null;
    }
  }

  const cards = $derived.by(() => {
    return [
      apiPrice && {
        price: apiPrice,
        name: 'API',
        blurb: 'For a job board, a bot or an agent that reads the catalogue.',
        perks: [
          'Every search filter, every field, company data',
          'Store and show the postings in your product, under the data licence',
        ],
      },
      dataPrice && {
        price: dataPrice,
        name: 'Data',
        blurb: 'For mirroring the catalogue at volume.',
        perks: ['Everything in API', 'Bulk catalogue files — coming soon'],
      },
    ].filter((c) => !!c);
  });
</script>

{#if cards.length > 0}
  <section id="api" class="flex scroll-mt-20 flex-col gap-6">
    <header class="flex flex-col items-center gap-2 text-center">
      <h2 class="text-2xl font-semibold tracking-tight">Jobs API</h2>
      <p class="max-w-2xl text-balance text-muted-foreground">
        The read API is free for personal and non-commercial use. An API plan licenses the data for use in your product.
        Send a full-access key as
        <code class="text-sm">Authorization: Bearer</code>.
      </p>
      <p class="max-w-2xl text-balance text-sm text-muted-foreground">
        API plans are billed separately from Pro. You do not need Pro to use the API, and an API plan does not include
        Pro's AI job-search features.
      </p>
    </header>

    <div class="grid gap-4 sm:grid-cols-2">
      {#each cards as card (card.price.id)}
        <section class="flex flex-col gap-4 rounded-xl border border-border p-6">
          <div class="flex flex-col gap-1">
            <h3 class="text-lg font-semibold">{card.name}</h3>
            <p class="text-3xl font-semibold tracking-tight">
              {money(card.price)}
              <span class="text-base font-normal text-muted-foreground"
                >/ {card.price.interval === 'year' ? 'year' : 'month'}</span
              >
            </p>
            <p class="text-sm text-muted-foreground">{card.blurb}</p>
          </div>
          <ul class="flex flex-col gap-2 text-sm">
            {#each card.perks as perk (perk)}
              <li class="border-b border-border/60 pb-2 text-muted-foreground">{perk}</li>
            {/each}
          </ul>
          {#if held === card.price.tier}
            <a href={resolve('/my/api')} class={cn(buttonVariants({ variant: 'outline', size: 'lg' }), 'mt-auto')}
              >Your current plan · Open API workspace</a
            >
          {:else if isAuthenticated()}
            <button
              type="button"
              class={cn(buttonVariants({ variant: buttonVariant, size: 'lg' }), 'mt-auto disabled:opacity-60')}
              disabled={busyId !== null}
              onclick={() => buy(card.price)}
            >
              {busyId === card.price.id ? 'Opening checkout…' : `Get ${card.name}`}
            </button>
          {:else}
            <button
              type="button"
              class={cn(buttonVariants({ variant: buttonVariant, size: 'lg' }), 'mt-auto')}
              onclick={promptSignIn}>Sign in to subscribe</button
            >
          {/if}
        </section>
      {/each}
    </div>
    {#if error}
      <p class="text-center text-sm text-destructive" role="alert">{error}</p>
    {/if}
    <p class="text-center text-sm text-muted-foreground">
      What the API carries and who it is for is on the <a
        href={resolve('/jobs-api')}
        class="font-medium text-foreground underline-offset-4 hover:underline">Jobs API</a
      >
      page. Keys live on your
      <a
        href={resolve('/my/integrations/api-keys')}
        class="font-medium text-foreground underline-offset-4 hover:underline">API keys</a
      >
      page. What you may do with the data is in the
      <!-- eslint-disable svelte/no-navigation-without-resolve -- resolve() is applied; the rule cannot see through the appended section anchor -->
      <a
        href={resolve('/terms') + '#data-licence'}
        class="font-medium text-foreground underline-offset-4 hover:underline">data licence</a
      ><!-- eslint-enable svelte/no-navigation-without-resolve -->.
    </p>
  </section>
{/if}
