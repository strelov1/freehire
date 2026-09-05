<script lang="ts">
  import { page } from '$app/state';
  import { api } from '$lib/api';
  import { promptSignIn } from '$lib/signin';
  import { formatMinorUnits } from '$lib/money';
  import { isAuthenticated } from '$lib/auth.svelte';
  import Seo from '$lib/components/Seo.svelte';
  import type { PublicPrice } from '$lib/types';
  import type { PageData } from './$types';

  const { data }: { data: PageData } = $props();

  const origin = $derived(page.url.origin);
  const plans = $derived(data.plans);

  // Monthly and annual, whichever the backend actually offers. Nothing here assumes both
  // exist: a deployment selling only one shows no toggle rather than a dead tab.
  const monthly = $derived(plans?.prices.find((p) => p.interval === 'month') ?? null);
  const annual = $derived(plans?.prices.find((p) => p.interval === 'year') ?? null);

  let interval = $state<'month' | 'year'>('month');
  const chosen = $derived(interval === 'year' ? annual : monthly);

  const money = (p: PublicPrice) => formatMinorUnits(p.amount_cents, p.currency);

  // What the annual price saves against twelve monthly ones, as a whole percentage. Shown
  // only when both prices exist and the saving is real — a "save 0%" badge is worse than
  // none.
  const saving = $derived.by(() => {
    if (!monthly || !annual) return 0;
    const full = monthly.amount_cents * 12;
    if (full <= annual.amount_cents) return 0;
    return Math.round(((full - annual.amount_cents) / full) * 100);
  });

  // What each metered feature is called here. The API's names are stable identifiers; these
  // are what a candidate would call the thing they just did.
  const FEATURE_LABELS: Record<string, string> = {
    tailor: 'CV editing sessions',
    match: 'Job fit analyses',
    assistant: 'Assistant messages',
    dictation: 'Voice dictation',
    'cover-letter': 'Cover letters',
  };
  const label = (id: string) => FEATURE_LABELS[id] ?? id;

  // Every plan offers every feature; only the daily amount differs. That is the product
  // decision this page has to communicate, and it is why there are no ticks and crosses:
  // a cross would say "you cannot", and nothing here is withheld.
  const rows = $derived(plans?.features ?? []);

  let busy = $state(false);
  let error = $state<string | null>(null);

  // Prefilled from a code that arrived in a link. Nothing is spent by filling the field —
  // the code is only redeemed when the buy button is pressed, because a redemption spends
  // the account's one lifetime code and must not happen to somebody who merely looked.
  let code = $state(data.promo ?? '');
  let checking = $state(false);
  let checked = $state<number | null>(null);
  let codeError = $state<string | null>(null);

  // What the code is worth, checked without spending it. Deliberately a button rather than
  // something that fires as you type: the route is rate limited server-side, and firing on
  // every keystroke would spend a real caller's budget rendering "not available" over and
  // over while they are still typing.
  async function checkCode() {
    const entered = code.trim();
    if (!entered || checking) return;
    checking = true;
    checked = null;
    codeError = null;
    try {
      const { percent_off } = await api.promoPreview(entered);
      checked = percent_off;
    } catch (e) {
      // Every refusal about the code itself reads the same on the server, so there is one
      // message here too. 409 is the one exception: it is about this account, not the code.
      codeError =
        e instanceof Error && e.message.includes('already')
          ? 'You have already used a promo code.'
          : 'That code is not available.';
    } finally {
      checking = false;
    }
  }

  async function buy() {
    if (!chosen || busy) return;
    busy = true;
    error = null;

    // Redeem first, as its own POST. The checkout call is a GET and must stay read-only —
    // `SameSite=Lax` sends the session cookie on a cross-site navigation, so a GET that
    // spent a code would let any page burn a visitor's one redemption.
    //
    // A refusal here stops the purchase and SAYS so. Charging somebody full price after
    // they entered an offer, without telling them, is the version of this they would
    // rightly call a bug. Redemption is durable, so if the checkout below then fails, the
    // discount still applies on their next attempt.
    const entered = code.trim();
    if (entered) {
      try {
        checked = (await api.promoRedeem(entered)).percent_off;
        codeError = null;
      } catch (e) {
        // "Already redeemed" does NOT stop the purchase. The discount this account holds is
        // durable and the checkout below picks it up whatever happens here — refusing to
        // sell to somebody because they entered a code they had already used would turn a
        // note into a dead end.
        const alreadyHeld = e instanceof Error && e.message.includes('already');
        codeError = alreadyHeld
          ? 'You have already used a promo code — your existing discount still applies.'
          : 'That code is not available. Clear it to continue at the usual price.';
        if (!alreadyHeld) {
          busy = false;
          return;
        }
      }
    }

    try {
      const { url } = await api.billingCheckout(chosen.id);
      window.location.href = url;
    } catch {
      error = 'Checkout is not available right now. Please try again in a moment.';
      busy = false;
    }
  }
</script>

<Seo
  title="Pricing — freehire"
  description="Every AI feature is on every plan. Free gives you a trial-sized amount each day; Pro removes the ceilings. $5 a month, cancel any time."
  canonical={`${origin}/pricing`}
/>

<div class="mx-auto flex w-full max-w-5xl flex-col gap-10 px-4 py-12 sm:py-16">
  <header class="flex flex-col items-center gap-3 text-center">
    <h1 class="text-3xl font-semibold tracking-tight sm:text-4xl">Simple plans</h1>
    <p class="max-w-2xl text-balance text-muted-foreground">
      Every feature is on every plan. What changes is how much of each you can do in a day —
      and it starts over every night.
    </p>
  </header>

  {#if monthly && annual}
    <div class="flex justify-center">
      <div class="inline-flex rounded-lg border border-border p-1" role="tablist">
        <button
          type="button"
          role="tab"
          aria-selected={interval === 'month'}
          class="rounded-md px-4 py-1.5 text-sm font-medium {interval === 'month'
            ? 'bg-primary text-primary-foreground'
            : 'text-muted-foreground'}"
          onclick={() => (interval = 'month')}
        >
          Monthly
        </button>
        <button
          type="button"
          role="tab"
          aria-selected={interval === 'year'}
          class="rounded-md px-4 py-1.5 text-sm font-medium {interval === 'year'
            ? 'bg-primary text-primary-foreground'
            : 'text-muted-foreground'}"
          onclick={() => (interval = 'year')}
        >
          Yearly
          {#if saving > 0}
            <span class="ml-1 text-xs opacity-80">−{saving}%</span>
          {/if}
        </button>
      </div>
    </div>
  {/if}

  <div class="grid gap-4 sm:grid-cols-2">
    <!-- Free -->
    <section class="flex flex-col gap-4 rounded-xl border border-border p-6">
      <div class="flex flex-col gap-1">
        <h2 class="text-lg font-semibold">Free</h2>
        <p class="text-3xl font-semibold tracking-tight">$0</p>
        <p class="text-sm text-muted-foreground">Everything, in trial-sized daily amounts.</p>
      </div>
      <ul class="flex flex-col gap-2 text-sm">
        {#each rows as f (f.feature)}
          <li class="flex items-baseline justify-between gap-3 border-b border-border/60 pb-2">
            <span class="text-muted-foreground">{label(f.feature)}</span>
            <span class="shrink-0 font-medium tabular-nums">{f.free_daily} / day</span>
          </li>
        {/each}
      </ul>
    </section>

    <!-- Pro -->
    <section class="flex flex-col gap-4 rounded-xl border-2 border-primary p-6">
      <div class="flex flex-col gap-1">
        <h2 class="text-lg font-semibold">Pro</h2>
        {#if chosen}
          <p class="text-3xl font-semibold tracking-tight">
            {money(chosen)}
            <span class="text-base font-normal text-muted-foreground"
              >/ {chosen.interval === 'year' ? 'year' : 'month'}</span
            >
          </p>
        {:else}
          <p class="text-3xl font-semibold tracking-tight text-muted-foreground">—</p>
        {/if}
        <p class="text-sm text-muted-foreground">The same product, without the ceilings.</p>
      </div>
      <ul class="flex flex-col gap-2 text-sm">
        {#each rows as f (f.feature)}
          <li class="flex items-baseline justify-between gap-3 border-b border-border/60 pb-2">
            <span class="text-muted-foreground">{label(f.feature)}</span>
            <span class="shrink-0 font-medium">
              {f.pro_unlimited ? 'Unlimited' : `${f.free_daily} / day`}
            </span>
          </li>
        {/each}
      </ul>

      {#if chosen}
        {#if isAuthenticated()}
          <div class="mt-auto flex flex-col gap-2">
            <label class="text-sm text-muted-foreground" for="promo-code">
              Have a promo code?
            </label>
            <div class="flex gap-2">
              <!-- The placeholder is deliberately not shaped like a code. A public
                   repository showing an example that looks redeemable is showing a code,
                   as far as anyone reading it is concerned. -->
              <input
                id="promo-code"
                type="text"
                bind:value={code}
                placeholder="Enter your code"
                autocomplete="off"
                spellcheck="false"
                class="min-w-0 flex-1 rounded-md border border-border bg-background px-3 py-2 text-sm uppercase"
              />
              <button
                type="button"
                class="shrink-0 rounded-md border border-border px-3 py-2 text-sm disabled:opacity-60"
                disabled={checking || !code.trim()}
                onclick={checkCode}
              >
                {checking ? 'Checking…' : 'Check'}
              </button>
            </div>
            {#if checked !== null}
              <p class="text-sm text-muted-foreground">
                {checked}% off your first month. It is applied when you continue.
              </p>
            {/if}
            {#if codeError}
              <p class="text-sm text-destructive">{codeError}</p>
            {/if}
          </div>
          <button
            type="button"
            class="rounded-md bg-primary px-4 py-2.5 text-sm font-medium text-primary-foreground disabled:opacity-60"
            disabled={busy}
            onclick={buy}
          >
            {busy ? 'Opening checkout…' : 'Upgrade to Pro'}
          </button>
        {:else}
          <!-- Signing in first is not a hurdle we invented: the purchase is attached to an
               account, and a payment made before there is one lands on nobody. The dialog
               rather than a route, because that is what signing in is on this site. -->
          <button
            type="button"
            class="mt-auto rounded-md bg-primary px-4 py-2.5 text-sm font-medium text-primary-foreground"
            onclick={promptSignIn}>Sign in to upgrade</button
          >
        {/if}
        {#if error}
          <p class="text-sm text-destructive">{error}</p>
        {/if}
      {/if}
    </section>
  </div>

  <!-- Said plainly rather than buried: what a subscription does and does not commit anyone
       to is the question people actually hesitate on. -->
  <p class="text-center text-sm text-muted-foreground">
    Cancel any time — you keep Pro until the end of the period you have paid for. Nothing you
    have created is ever withdrawn when a plan lapses.
  </p>
</div>
