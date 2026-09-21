<script lang="ts">
  import { onMount } from 'svelte';
  import { resolve } from '$app/paths';
  import { currentUser } from '$lib/auth.svelte';
  import { locale } from '$lib/i18n/currentLocale.svelte';
  import { format, t } from '$lib/i18n/t';
  import { api } from '$lib/api';
  import { formatMinorUnits } from '$lib/money';
  import { Button, type ButtonVariant } from '$lib/ui';
  import { messages } from './PlanLimitLink.messages';

  // The one link every spent-daily-allowance message points to. `/my/plan` shows the
  // candidate exactly what they've used today and where to upgrade — see PlanView.svelte —
  // so every refusal banner sends them there instead of stating the limit and nothing else.
  //
  // A free reader is the one this refusal can actually convert, so they get the real CTA:
  // a button naming the price, not a line of underlined text easy to skim past. A paying
  // reader lands here too on rare occasion — Pro's own fair-use guard firing — and "Upgrade
  // to Pro" would be a wrong sentence to show somebody who already bought it, so they keep
  // the plain link instead.
  //
  // `variant` lets a caller whose surrounding chrome is already colored (AssistantChat's
  // destructive-styled plan-limit alert) ask for something that doesn't fight it, without
  // dulling the CTA everywhere else it sits on a neutral card.
  let { variant = 'primary' }: { variant?: ButtonVariant } = $props();

  const isFree = $derived((currentUser()?.tier ?? 'free') === 'free');
  const planHref = resolve('/my/plan');
  const s = $derived(t(messages, locale()));

  // The price this button quotes has to be the one thing every other money-rendering
  // surface reads (`/pricing`, `/my/plan`) — `GET /api/v1/plans`, formatted through the
  // same `formatMinorUnits` they use. A literal here would be a second copy of a money
  // rule, silently wrong the day the price changes or a self-hosted deployment prices Pro
  // in a different currency (see money.ts). Best-effort: a failed fetch still upgrades,
  // just without a price in the label.
  let proMonthlyLabel = $state<string | null>(null);
  onMount(async () => {
    try {
      const { prices } = await api.plans();
      const monthly = prices.find((p) => p.tier === 'pro' && p.interval === 'month');
      if (monthly) proMonthlyLabel = formatMinorUnits(monthly.amount_cents, monthly.currency);
    } catch {
      /* best-effort — see above */
    }
  });
</script>

{#if isFree}
  <Button href={planHref} {variant} size="sm">
    {proMonthlyLabel ? format(s.upgradeWithPrice, { price: `${proMonthlyLabel}/mo` }) : s.upgrade}
  </Button>
{:else}
  <a href={planHref} class="text-xs font-medium underline underline-offset-4">{s.seeYourPlan}</a>
{/if}
