<script lang="ts">
  import { buttonVariants, cn } from '#lib/ui/index.js';
  import { resolve } from '$app/paths';
  import { api } from '#lib/api.js';
  import { isAuthenticated } from '#lib/auth.svelte.js';
  import { formatMinorUnits } from '#lib/money.js';
  import { promptSignIn } from '#lib/signin.js';
  import type { PlansMatrix, PublicPrice } from '#lib/types.js';

  // The API product: access to the public read API for software, sold apart from the plans
  // above. Its figures come from the same response as theirs, so the page cannot quote a
  // limit the rate limiter does not enforce.
  // `limits` is optional: a backend that predates the API product sends none, and the section
  // then stays out of the page rather than taking the plans above it down.
  const { prices, limits }: { prices: PublicPrice[]; limits?: PlansMatrix['api_limits'] } =
    $props();

  // One card per tier, and only for a tier that is actually sold: the monthly price when there
  // is one, otherwise whatever interval the deployment offers.
  const pick = (tier: PublicPrice['tier']) => {
    const own = prices.filter((p) => p.tier === tier);
    return own.find((p) => p.interval === 'month') ?? own[0] ?? null;
  };
  const apiPrice = $derived(pick('api'));
  const dataPrice = $derived(pick('api_data'));

  const count = (n: number) => n.toLocaleString('en-US');
  const money = (p: PublicPrice) => formatMinorUnits(p.amount_cents, p.currency);

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
    if (!limits) return [];
    return [
      apiPrice && {
        price: apiPrice,
        name: 'API',
        blurb: 'For a job board, a bot or an agent that reads the catalogue.',
        perks: [
          `${count(limits.api_per_minute)} requests a minute, no daily cap`,
          'Every search filter, every field, company data',
          'Store and show the postings in your product, under the data licence',
        ],
      },
      dataPrice && {
        price: dataPrice,
        name: 'Data',
        blurb: 'For mirroring the catalogue at volume.',
        perks: [
          `${count(limits.data_per_minute)} requests a minute, no daily cap`,
          'Everything in API',
          'Bulk catalogue files — coming soon',
        ],
      },
    ].filter((c) => !!c);
  });
</script>

{#if limits && cards.length > 0}
  <section id="api" class="flex scroll-mt-20 flex-col gap-6">
    <header class="flex flex-col items-center gap-2 text-center">
      <h2 class="text-2xl font-semibold tracking-tight">API</h2>
      <p class="max-w-2xl text-balance text-muted-foreground">
        The read API is free without a key, up to {count(limits.anonymous_per_minute)} requests a
        minute{#if limits.anonymous_per_day > 0}&nbsp;and {count(limits.anonymous_per_day)} a day{/if}
        per address. For more, and for the right to use the data in a commercial product, pick a plan
        and send its key as <code class="text-sm">Authorization: Bearer</code>.
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
          {#if isAuthenticated()}
            <button
              type="button"
              class={cn(buttonVariants({ variant: 'cta', size: 'lg' }), 'mt-auto disabled:opacity-60')}
              disabled={busyId !== null}
              onclick={() => buy(card.price)}
            >
              {busyId === card.price.id ? 'Opening checkout…' : `Get ${card.name}`}
            </button>
          {:else}
            <button
              type="button"
              class={cn(buttonVariants({ variant: 'cta', size: 'lg' }), 'mt-auto')}
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
      > page. Keys live on your <a
        href={resolve('/my/api-keys')}
        class="font-medium text-foreground underline-offset-4 hover:underline">API keys</a
      > page. What you may do with the data is in the
      <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- resolve() is applied; the rule cannot see through the appended section anchor -->
      <a href={resolve('/terms') + '#data-licence'} class="font-medium text-foreground underline-offset-4 hover:underline"
        >data licence</a
      >.
    </p>
  </section>
{/if}
