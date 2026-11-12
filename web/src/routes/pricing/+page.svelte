<script lang="ts">
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { api } from '#lib/api.js';
  import { promptSignIn } from '#lib/signin.js';
  import { formatMinorUnits } from '#lib/money.js';
  import { isAuthenticated } from '#lib/auth.svelte.js';
  import { PRO_PERKS } from '#lib/proPerks.js';
  import Seo from '#lib/components/Seo.svelte';
  import { ProviderIcon, SectionLabel, buttonVariants, cn } from '#lib/ui/index.js';
  import type { PlanTierAllowance, PlansMatrix, PublicPrice } from '#lib/types.js';
  import { Check } from '@lucide/svelte';
  import type { PageData } from './$types';
  import ApiPlans from '#lib/components/ApiPlans.svelte';
  import { tablist } from '#lib/actions/tablist.js';

  const { data }: { data: PageData } = $props();

  const origin = $derived(page.url.origin);
  const plans = $derived(data.plans);

  // Monthly and annual, whichever the backend actually offers. Nothing here assumes both
  // exist: a deployment selling only one shows no toggle rather than a dead tab.
  // Prices carry the tier they buy, so a plan's own column can never offer another's price.
  const proPrices = $derived(plans?.prices.filter((p) => p.tier === 'pro') ?? []);
  const ultraPrices = $derived(plans?.prices.filter((p) => p.tier === 'ultra') ?? []);

  const monthly = $derived(proPrices.find((p) => p.interval === 'month') ?? null);
  const annual = $derived(proPrices.find((p) => p.interval === 'year') ?? null);
  const ultraMonthly = $derived(ultraPrices.find((p) => p.interval === 'month') ?? null);
  const ultraAnnual = $derived(ultraPrices.find((p) => p.interval === 'year') ?? null);

  // One toggle drives both plans, so the two columns can never show different intervals.
  let interval = $state<'month' | 'year'>('month');
  const chosen = $derived(interval === 'year' ? annual : monthly);
  // Falls back to whichever Ultra price exists, because the column's presence must depend on
  // whether Ultra is SOLD and not on which toggle is showing. Without it, a deployment
  // offering only a monthly Ultra loses the whole column the moment a reader flips to
  // Yearly — the plan vanishing as a side effect of looking at the other one. The interval
  // shown is read off the price itself, so a fallback says "$19 / month" rather than
  // implying an annual price that does not exist.
  const ultraChosen = $derived((interval === 'year' ? ultraAnnual : ultraMonthly) ?? ultraMonthly ?? ultraAnnual);

  const money = (p: PublicPrice) => formatMinorUnits(p.amount_cents, p.currency, undefined, { trimWhole: true });

  // What one tier's allowance reads as. `unlimited` makes the number a fair-use guard rather
  // than a ceiling, so it is not shown — quoting a guard as a limit would promise less than
  // the plan gives. Zero is "not on this plan" rather than "nothing today".
  const allowanceText = (a: PlanTierAllowance) => (a.unlimited ? 'Unlimited' : a.limit > 0 ? `${a.limit} / day` : '—');

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
    assistant: 'Assistant messages',
    'cover-letter': 'Cover letters',
    'auto-apply': 'Background auto-apply',
  };
  const label = (id: string) => FEATURE_LABELS[id] ?? id;

  // Metered features that stay metered but are not worth a line on this page: dictation is
  // an input method, and the fit analysis is part of tailoring rather than a thing bought.
  const UNLISTED = new Set(['dictation', 'match']);
  const rows = $derived((plans?.features ?? []).filter((f) => !UNLISTED.has(f.feature)));
  // A metered feature reaches the free card only when free actually gets some of it. Today
  // every free allowance is zero, so the card is the unmetered product alone; reopening one
  // with `PLAN_FREE_DAILY_<F>` brings its row back without touching this file.
  const freeRows = $derived(rows.filter((f) => f.free.unlimited || f.free.limit > 0));

  // What costs nothing at all, on any plan. These are not metered, so the plan matrix knows
  // nothing about them and they are written here. Each links to the page that explains it.
  type Perk = { text: string; href: string };
  const FREE_FEATURES: Perk[] = [
    { text: 'Every job, every filter, and AI search', href: resolve('/features/advanced-search') },
    { text: 'Alerts by email, Telegram and push', href: resolve('/features/notifications') },
    { text: 'Application tracking board', href: resolve('/features/tracking') },
    { text: 'Inbox that sorts employer replies into your applications', href: resolve('/features/inbox') },
    { text: 'Ghost-job warnings on every posting', href: resolve('/features/ghost-jobs') },
    { text: 'Search from ChatGPT, Claude or any agent over MCP, and the CLI', href: resolve('/mcp') },
    { text: 'Anonymous employee referrals', href: resolve('/features/referrals') },
  ];

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
  // something that fires as you type: firing on
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

  // Ultra carries no promo field of its own. A code is redeemed once for the account, not
  // per plan, so offering the box twice would suggest two separate offers.
  async function buyUltra() {
    if (!ultraChosen || busy) return;
    busy = true;
    error = null;
    try {
      const { url } = await api.billingCheckout(ultraChosen.id);
      window.location.href = url;
    } catch {
      error = 'Checkout is not available right now. Please try again in a moment.';
      busy = false;
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
  description="For job seekers: search, alerts, tracking, the inbox and agent access over MCP are free; Pro adds CV tailoring, the AI assistant, the extension that fills applications and background auto-apply for $5 a month. For builders: the Jobs API licenses the catalogue for your product at a flat monthly price."
  canonical={`${origin}/pricing`}
/>

<div class="mx-auto flex w-full max-w-5xl flex-col gap-10 px-4 py-12 sm:py-16">
  <header class="flex flex-col items-center gap-3 text-center">
    <h1 class="text-3xl font-semibold tracking-tight sm:text-4xl">Simple plans</h1>
    <p class="max-w-2xl text-balance text-muted-foreground">
      Finding and tracking jobs is free, for good — on the site, in your AI agent and from the CLI. Pro adds the AI that
      does the applying with you; the Jobs API is for building on the catalogue.
    </p>
  </header>

  <SectionLabel text="For job seekers" class="-mb-6 text-center" />

  {#if monthly && annual}
    <div class="flex justify-center">
      <div
        class="inline-flex rounded-lg border border-border p-1"
        role="tablist"
        aria-label="Billing interval"
        use:tablist={interval}
      >
        <button
          type="button"
          role="tab"
          id="pricing-tab-month"
          aria-selected={interval === 'month'}
          aria-controls="pricing-plans"
          class="rounded-md px-4 py-1.5 text-sm font-medium {interval === 'month'
            ? 'bg-secondary text-foreground shadow-xs'
            : 'text-muted-foreground'}"
          onclick={() => (interval = 'month')}
        >
          Monthly
        </button>
        <button
          type="button"
          role="tab"
          id="pricing-tab-year"
          aria-selected={interval === 'year'}
          aria-controls="pricing-plans"
          class="rounded-md px-4 py-1.5 text-sm font-medium {interval === 'year'
            ? 'bg-secondary text-foreground shadow-xs'
            : 'text-muted-foreground'}"
          onclick={() => (interval = 'year')}
        >
          Yearly
          {#if saving > 0}
            <!-- The saving is the reason to pick this tab, so it gets the accent, not a fade. -->
            <span class="ml-1 rounded-full bg-brand-muted px-1.5 py-0.5 text-xs font-semibold text-brand-strong"
              >−{saving}%</span
            >
          {/if}
        </button>
      </div>
    </div>
  {/if}

  <!-- What a plan includes, each line linking to the page that explains it. -->
  {#snippet perkList(items: Perk[])}
    <ul class="flex flex-col gap-2 text-sm">
      {#each items as item (item.text)}
        <li class="flex items-baseline gap-2">
          <Check class="size-4 shrink-0 translate-y-0.5 text-primary" aria-hidden="true" />
          <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- every href is built with resolve() in FREE_FEATURES / PRO_PERKS; the linter can't trace it via the variable -->
          <a href={item.href} class="hover:underline">{item.text}</a>
        </li>
      {/each}
    </ul>
  {/snippet}

  <!-- The daily amounts of the metered features, as the backend states them. -->
  {#snippet allowanceList(items: PlansMatrix['features'], tier: 'free' | 'pro' | 'ultra')}
    <div class="flex flex-col gap-2">
      <p class="text-xs font-medium text-muted-foreground">Daily allowance</p>
      <ul class="flex flex-col gap-2 text-sm">
        {#each items as f (f.feature)}
          <li class="flex items-baseline justify-between gap-3 border-b border-border/60 pb-2">
            <span class="text-muted-foreground">{label(f.feature)}</span>
            <span class="shrink-0 font-medium tabular-nums">{allowanceText(f[tier])}</span>
          </li>
        {/each}
      </ul>
    </div>
  {/snippet}

  <!-- The one thing on this page that is NOT a daily amount, so it cannot be a row in the
     allowance lists: free does not get a smaller share of it, it gets none. It is rendered on
     the paid cards and simply absent from the free one, because a crossed-out row would say
     "you cannot" — the thing this page is built to never say. A snippet, so the two paid
     cards cannot drift apart about how the one perk they share is worded. -->
  {#snippet discordPerk()}
    <p class="flex items-baseline gap-2 text-sm">
      <ProviderIcon provider="discord" class="size-4 shrink-0 translate-y-0.5" />
      <span class="text-muted-foreground">Members-only channels on the community Discord.</span>
    </p>
  {/snippet}

  <!-- Three tracks only when there is a third card to fill one: a grid fixed at
     lg:grid-cols-3 with two children leaves the third an empty gap on wide screens instead
     of the two cards sharing the row the way they already do at the sm breakpoint.
     The billing-interval tabs above (rendered only when both intervals are sold) control
     the prices on these cards, so the grid is their tabpanel. -->
  <div
    id="pricing-plans"
    role={monthly && annual ? 'tabpanel' : undefined}
    aria-labelledby={monthly && annual ? `pricing-tab-${interval}` : undefined}
    class="grid gap-4 sm:grid-cols-2 {ultraChosen ? 'lg:grid-cols-3' : ''}"
  >
    <!-- Free. As tall as its own content (`self-start`): stretched to Pro's height it opened a
         hollow band between its list and its button. -->
    <section class="flex flex-col gap-4 self-start rounded-xl border border-border p-6">
      <div class="flex flex-col gap-1">
        <h2 class="text-lg font-semibold">Free</h2>
        <p class="text-3xl font-semibold tracking-tight">$0</p>
        <p class="text-sm text-muted-foreground">Everything you need to find and track a job.</p>
      </div>
      {@render perkList(FREE_FEATURES)}
      {#if freeRows.length > 0}
        {@render allowanceList(freeRows, 'free')}
      {/if}
      {#if !isAuthenticated()}
        <button
          type="button"
          class="mt-auto rounded-md border border-border px-4 py-2.5 text-sm font-medium"
          onclick={promptSignIn}>Sign up free</button
        >
      {/if}
    </section>

    <!-- Pro -->
    <section class="flex flex-col gap-4 rounded-xl border border-brand/40 bg-brand/[0.03] p-6 shadow-sm">
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
        <p class="text-sm text-muted-foreground">Everything in Free, plus:</p>
      </div>
      {@render perkList(PRO_PERKS)}
      {@render allowanceList(rows, 'pro')}

      {@render discordPerk()}

      {#if chosen}
        {#if isAuthenticated()}
          <div class="mt-auto flex flex-col gap-2">
            <label class="text-sm text-muted-foreground" for="promo-code"> Have a promo code? </label>
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
            class={cn(buttonVariants({ variant: 'cta', size: 'lg' }), 'disabled:opacity-60')}
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
            class={cn(buttonVariants({ variant: 'cta', size: 'lg' }), 'mt-auto')}
            onclick={promptSignIn}>Sign in to upgrade</button
          >
        {/if}
        {#if error}
          <p class="text-sm text-destructive">{error}</p>
        {/if}
      {/if}
    </section>

    <!-- Ultra. Rendered only when the deployment actually sells it: a column with no price
         behind it is a plan nobody can buy, which is worse than two plans. -->
    {#if ultraChosen}
      <section class="flex flex-col gap-4 rounded-xl border border-border p-6">
        <div class="flex flex-col gap-1">
          <h2 class="text-lg font-semibold">Ultra</h2>
          <p class="text-3xl font-semibold tracking-tight">
            {money(ultraChosen)}
            <span class="text-base font-normal text-muted-foreground"
              >/ {ultraChosen.interval === 'year' ? 'year' : 'month'}</span
            >
          </p>
          <p class="text-sm text-muted-foreground">
            Everything in Pro, and apply to as many jobs as you like, unattended.
          </p>
        </div>
        {@render allowanceList(rows, 'ultra')}

        {@render discordPerk()}

        {#if isAuthenticated()}
          <button
            type="button"
            class="mt-auto rounded-md border border-primary px-4 py-2.5 text-sm font-medium disabled:opacity-60"
            disabled={busy}
            onclick={buyUltra}
          >
            {busy ? 'Opening checkout…' : 'Upgrade to Ultra'}
          </button>
        {:else}
          <button
            type="button"
            class="mt-auto rounded-md border border-primary px-4 py-2.5 text-sm font-medium"
            onclick={promptSignIn}>Sign in to upgrade</button
          >
        {/if}
      </section>
    {/if}
  </div>

  {#if plans}
    {#if plans.prices.some((p) => p.tier === 'api' || p.tier === 'api_data')}
      <SectionLabel text="For builders" class="-mb-6 text-center" />
    {/if}
    <ApiPlans prices={plans.prices} buttonVariant="outline" />
  {/if}

  <!-- Said plainly rather than buried: what a subscription does and does not commit anyone
       to is the question people actually hesitate on. -->
  <p class="text-center text-sm text-muted-foreground">
    Cancel any time — you keep your plan until the end of the period you have paid for. Nothing you have created is ever
    withdrawn when a plan lapses. Not what you expected? Email us within 14 days and we'll refund it, no questions
    asked.
  </p>
</div>
