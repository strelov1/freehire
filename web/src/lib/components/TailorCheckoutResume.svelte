<script lang="ts">
  // Picks a tailoring task back up on the plan page after the candidate left it to buy Pro
  // (see tailorCheckout.ts). Three honest states, none of which trusts the address or
  // browser storage as proof of anything:
  //   - Pro is verified on a fresh read of the server's plan → an explicit "Continue
  //     tailoring" to the same vacancy and base. Nothing is started on the way: the
  //     workspace asks again before any model call.
  //   - The candidate came back from checkout but Pro has not shown up (a slow webhook) →
  //     keep checking for a short while, then say so and offer another check.
  //   - They came back without buying (cancelled, or just walked away) → the task is still
  //     here to retry or to leave, and nothing suggests a payment happened.
  import { goto, invalidateAll } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { track } from '#lib/analytics.js';
  import { api } from '#lib/api.js';
  import { currentUser } from '#lib/auth.svelte.js';
  import { proMonthlyPrice } from '#lib/proPrice.js';
  import {
    browserStore,
    clearTailorContext,
    readTailorContext,
    startTailorCheckout,
    type TailorContext,
  } from '#lib/tailorCheckout.js';
  import { tailorHref } from '#lib/tailorHref.js';
  import type { PlanState } from '#lib/types.js';
  import { Button } from '#lib/ui/index.js';

  const {
    plan,
    returned,
    onPlan,
  }: {
    /** The page's own plan read: null while unknown. */
    plan: PlanState | null;
    /** The address says a checkout just finished. A hint to look harder, never a verdict. */
    returned: boolean;
    /** A fresher plan than the page holds, so the strip above stops saying Free. */
    onPlan: (p: PlanState) => void;
  } = $props();

  const POLLS = 6;
  const POLL_MS = 2000;
  type Tier = 'pro' | 'ultra';
  const paidTier = (p: PlanState | null): Tier | null => (p?.plan === 'pro' || p?.plan === 'ultra' ? p.plan : null);
  const sameTask = (a: TailorContext | null, b: TailorContext) => a?.slug === b.slug && a?.baseCvId === b.baseCvId;

  // A $derived, so a refreshed session for the SAME account (invalidateAll swaps the user
  // object) does not read as a change of account and wipe the verdict mid-continue.
  const userId = $derived(currentUser()?.id);
  let ctx = $state.raw<TailorContext | null>(null);
  // Latches on the first sight of the marker: the page strips it once the purchase is
  // recorded, and checking must not stop (or flip to "you did not buy") when it does.
  let waiting = $state(false);
  // What a fresh read of the server's plan said; kept even if an older read lands after it.
  let verifiedTier = $state<Tier | null>(null);
  let giveUp = $state(false);
  let attempt = $state(0);
  let busy = $state(false);
  let failed = $state<'checkout' | 'workspace' | null>(null);
  let announced = false;

  // Everything below belongs to one account. When the account changes the task, the
  // verdicts and the progress all start over, and (through the cleanup in the effect below)
  // any answer still in flight for the previous account is dropped on arrival.
  $effect(() => {
    const store = browserStore();
    ctx = store ? readTailorContext(userId, store) : null;
    waiting = false;
    verifiedTier = null;
    giveUp = false;
    busy = false;
    failed = null;
    announced = false;
  });
  $effect(() => {
    if (returned && ctx) waiting = true;
  });

  $effect(() => {
    void attempt;
    if (!waiting || verifiedTier || userId === undefined) return;
    let live = true;
    let timer: ReturnType<typeof setTimeout> | undefined;
    giveUp = false;
    const check = async (left: number) => {
      try {
        const fresh = await api.myPlan();
        if (!live) return;
        onPlan(fresh);
        const tier = paidTier(fresh);
        if (tier) {
          verifiedTier = tier;
          return;
        }
      } catch {
        // Not readable yet; the next try may be.
      }
      if (!live) return;
      if (left > 0) timer = setTimeout(() => void check(left - 1), POLL_MS);
      else giveUp = true;
    };
    void check(POLLS);
    return () => {
      live = false;
      clearTimeout(timer);
    };
  });

  const tier = $derived(verifiedTier ?? paidTier(plan));
  const checking = $derived(waiting && !tier && !giveUp);

  $effect(() => {
    if (ctx && tier && !announced) {
      announced = true;
      track('contextual_pro_continue_view', { surface: 'plan' });
    }
  });

  async function resume() {
    const target = ctx;
    const owner = userId;
    if (!target || busy) return;
    busy = true;
    failed = null;
    try {
      // The tier in the session snapshot predates the purchase; reading it again first means
      // the workspace opens already knowing the account is paid.
      await invalidateAll().catch(() => {});
      if (userId !== owner) return;
      // eslint-disable-next-line svelte/no-navigation-without-resolve -- tailorHref() wraps resolve('/tailor/[slug]'); the rule can't see through the appended ?base= query
      await goto(tailorHref(target.slug, target.baseCvId));
      if (userId !== owner) return;
      track('contextual_pro_continue', { surface: 'plan' });
      // Only now is the task done with, and only if it is still the one that was resumed:
      // a newer one saved meanwhile must survive.
      const store = browserStore();
      if (store && sameTask(readTailorContext(owner, store), target)) clearTailorContext(store);
    } catch {
      if (userId === owner) failed = 'workspace';
    } finally {
      if (userId === owner) busy = false;
    }
  }

  async function retry() {
    const target = ctx;
    const owner = userId;
    if (!target || busy) return;
    busy = true;
    failed = null;
    try {
      const price = proMonthlyPrice(await api.plans());
      if (userId !== owner) return;
      if (!price) throw new Error('no price');
      const url = await startTailorCheckout(owner, target, price);
      if (userId === owner) window.location.href = url;
    } catch {
      if (userId === owner) failed = 'checkout';
    } finally {
      if (userId === owner) busy = false;
    }
  }

  function dismiss() {
    const store = browserStore();
    if (store) clearTailorContext(store);
    ctx = null;
  }
</script>

{#if ctx && (plan || waiting)}
  <section
    class="flex flex-col gap-3 rounded-lg border border-brand/30 bg-brand/[0.03] px-4 py-3"
    aria-labelledby="tailor-resume-title"
  >
    {#if tier}
      <div class="flex flex-col gap-0.5">
        <h2 id="tailor-resume-title" class="text-sm font-semibold">
          {tier === 'ultra' ? 'Ultra' : 'Pro'} is active on your account
        </h2>
        <p class="text-xs text-muted-foreground">
          Pick up where you left off: the same role and the same CV. Nothing runs until you confirm in the workspace.
        </p>
      </div>
      <div class="flex flex-wrap items-center gap-2">
        <Button variant="cta" size="sm" disabled={busy} onclick={resume}>Continue tailoring</Button>
        <Button variant="ghost" size="sm" onclick={dismiss}>Not now</Button>
      </div>
      {#if failed === 'workspace'}
        <p class="text-xs text-destructive" role="alert">
          Couldn't open the workspace. Your task is saved — try again.
        </p>
      {/if}
    {:else if checking}
      <div class="flex flex-col gap-0.5" role="status">
        <h2 id="tailor-resume-title" class="text-sm font-semibold">Checking your plan…</h2>
        <p class="text-xs text-muted-foreground">This can take a minute after you pay. Your tailoring task is saved.</p>
      </div>
    {:else}
      <div class="flex flex-col gap-0.5">
        <h2 id="tailor-resume-title" class="text-sm font-semibold">Your tailoring task is saved</h2>
        <p class="text-xs text-muted-foreground">
          {waiting
            ? "Pro isn't showing on your account yet. If you just paid, check again in a moment."
            : 'No Pro plan is active on your account. You can try again or go back to the role.'}
        </p>
      </div>
      <div class="flex flex-wrap items-center gap-2">
        {#if waiting}
          <Button variant="cta" size="sm" onclick={() => attempt++}>Check again</Button>
        {:else}
          <Button variant="cta" size="sm" disabled={busy} onclick={retry}>Try Pro again</Button>
        {/if}
        <Button href={resolve('/jobs/[slug]', { slug: ctx.slug })} variant="outline" size="sm">Back to the role</Button>
        <Button variant="ghost" size="sm" onclick={dismiss}>Dismiss</Button>
      </div>
      {#if failed === 'checkout'}
        <p class="text-xs text-destructive" role="alert">
          Checkout is not available right now. Please try again in a moment.
        </p>
      {/if}
    {/if}
  </section>
{/if}
