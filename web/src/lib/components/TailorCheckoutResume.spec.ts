import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { PlanState } from '#lib/types.js';
import { tick } from 'svelte';

const { myPlan, plans, billingCheckout, track, goto, invalidateAll, account } = await vi.hoisted(async () => {
  // Reactive outside a component, so a test can switch accounts under a mounted one.
  const { SvelteMap } = await import('svelte/reactivity');
  return {
    myPlan: vi.fn(),
    plans: vi.fn(),
    billingCheckout: vi.fn(),
    track: vi.fn(),
    goto: vi.fn(),
    invalidateAll: vi.fn(),
    account: new SvelteMap<string, number | undefined>([['id', 7]]),
  };
});
vi.mock('$app/navigation', () => ({ goto, invalidateAll }));
vi.mock('$app/state', () => ({ page: { data: {}, url: new URL('http://localhost/') } }));
vi.mock('$app/paths', () => ({
  resolve: (p: string, params?: Record<string, string>) => p.replace('[slug]', params?.slug ?? ''),
}));
vi.mock('#lib/api.js', () => ({ api: { myPlan, plans, billingCheckout } }));
vi.mock('#lib/analytics.js', () => ({ track }));
vi.mock('#lib/auth.svelte.js', () => ({ currentUser: () => (account.get('id') ? { id: account.get('id') } : null) }));

import { saveTailorContext, readTailorContext } from '#lib/tailorCheckout.js';
import TailorCheckoutResume from './TailorCheckoutResume.svelte';

const free = { plan: 'free', allowances: [] } as unknown as PlanState;
const pro = { plan: 'pro', allowances: [] } as unknown as PlanState;
const ultra = { plan: 'ultra', allowances: [] } as unknown as PlanState;
const monthly = { id: 'price_m', tier: 'pro', interval: 'month', amount_cents: 500, currency: 'usd' };

let navigated = '';
function show(plan: PlanState | null, returned: boolean, onPlan = vi.fn()) {
  render(TailorCheckoutResume, { plan, returned, onPlan });
  return onPlan;
}

beforeEach(() => {
  for (const m of [myPlan, plans, billingCheckout, track, goto, invalidateAll]) m.mockReset();
  invalidateAll.mockResolvedValue(undefined);
  account.set('id', 7);
  sessionStorage.clear();
  navigated = '';
  saveTailorContext(7, { slug: 'senior-go-acme-1', baseCvId: 'base-b' }, sessionStorage);
  vi.stubGlobal('location', {
    get href() {
      return navigated;
    },
    set href(v: string) {
      navigated = v;
    },
  });
});

describe('TailorCheckoutResume', () => {
  it('does not create checkout for another account after a slow price lookup', async () => {
    const prices = Promise.withResolvers<{ prices: (typeof monthly)[] }>();
    plans.mockReturnValue(prices.promise);
    show(free, false);
    await fireEvent.click(await screen.findByRole('button', { name: 'Try Pro again' }));
    saveTailorContext(8, { slug: 'other-role-2' }, sessionStorage);
    account.set('id', 8);
    await tick();
    prices.resolve({ prices: [monthly] });
    await prices.promise;
    await tick();
    expect(billingCheckout).not.toHaveBeenCalled();
    expect(readTailorContext(8, sessionStorage)).toEqual({ slug: 'other-role-2' });
  });

  it('does not redirect another account when checkout creation finishes late', async () => {
    const checkout = Promise.withResolvers<{ url: string }>();
    plans.mockResolvedValue({ prices: [monthly] });
    billingCheckout.mockReturnValue(checkout.promise);
    show(free, false);
    await fireEvent.click(await screen.findByRole('button', { name: 'Try Pro again' }));
    await waitFor(() => expect(billingCheckout).toHaveBeenCalled());
    saveTailorContext(8, { slug: 'other-role-2' }, sessionStorage);
    account.set('id', 8);
    await tick();
    checkout.resolve({ url: 'https://checkout.example/old-account' });
    await checkout.promise;
    await tick();
    expect(navigated).toBe('');
    expect(readTailorContext(8, sessionStorage)).toEqual({ slug: 'other-role-2' });
  });

  it('does not clear another account task when navigation finishes late', async () => {
    const navigation = Promise.withResolvers<undefined>();
    goto.mockReturnValue(navigation.promise);
    show(pro, false);
    await fireEvent.click(await screen.findByRole('button', { name: 'Continue tailoring' }));
    await waitFor(() => expect(goto).toHaveBeenCalled());
    saveTailorContext(8, { slug: 'other-role-2' }, sessionStorage);
    account.set('id', 8);
    await tick();
    navigation.resolve(undefined);
    await navigation.promise;
    await tick();
    expect(readTailorContext(8, sessionStorage)).toEqual({ slug: 'other-role-2' });
    expect(track).not.toHaveBeenCalledWith('contextual_pro_continue', expect.anything());
  });

  it('renders nothing when no task was saved', () => {
    sessionStorage.clear();
    show(free, false);
    expect(screen.queryByRole('region')).toBeNull();
    expect(screen.queryByRole('button')).toBeNull();
  });

  it('after a cancelled checkout keeps the task, says no plan is active and never claims payment', async () => {
    show(free, false);
    expect(await screen.findByText('Your tailoring task is saved')).toBeTruthy();
    expect(screen.getByText(/No Pro plan is active/)).toBeTruthy();
    expect(screen.queryByRole('button', { name: 'Continue tailoring' })).toBeNull();
    expect(screen.getByRole('link', { name: 'Back to the role' }).getAttribute('href')).toBe('/jobs/senior-go-acme-1');
    expect(myPlan).not.toHaveBeenCalled();
    expect(readTailorContext(7, sessionStorage)).not.toBeNull();
  });

  it('retries checkout from the saved task at the monthly price', async () => {
    plans.mockResolvedValue({ prices: [monthly] });
    billingCheckout.mockResolvedValue({ url: 'https://checkout.example/s' });
    show(free, false);
    await fireEvent.click(await screen.findByRole('button', { name: 'Try Pro again' }));
    await waitFor(() => expect(navigated).toBe('https://checkout.example/s'));
    expect(billingCheckout).toHaveBeenCalledWith('price_m');
    expect(readTailorContext(7, sessionStorage)).toEqual({ slug: 'senior-go-acme-1', baseCvId: 'base-b' });
  });

  it('says so when retrying cannot reach checkout, and keeps the task', async () => {
    plans.mockResolvedValue({ prices: [monthly] });
    billingCheckout.mockRejectedValue(new Error('down'));
    show(free, false);
    await fireEvent.click(await screen.findByRole('button', { name: 'Try Pro again' }));
    expect((await screen.findByRole('alert')).textContent).toMatch(/Checkout is not available/);
    expect(readTailorContext(7, sessionStorage)).not.toBeNull();
  });

  it('after paying, verifies on a FRESH plan read, then offers an explicit continue to the exact vacancy and base', async () => {
    myPlan.mockResolvedValue(pro);
    const onPlan = show(free, true);
    const go = await screen.findByRole('button', { name: 'Continue tailoring' });
    expect(myPlan).toHaveBeenCalled();
    expect(onPlan).toHaveBeenCalledWith(pro);
    expect(goto).not.toHaveBeenCalled();
    await fireEvent.click(go);
    await waitFor(() => expect(goto).toHaveBeenCalledWith('/tailor/senior-go-acme-1?base=base-b'));
    expect(invalidateAll).toHaveBeenCalled();
    expect(invalidateAll.mock.invocationCallOrder[0] ?? 0).toBeLessThan(goto.mock.invocationCallOrder[0] ?? 0);
    expect(readTailorContext(7, sessionStorage)).toBeNull();
    expect(track).toHaveBeenCalledWith('contextual_pro_continue', { surface: 'plan' });
  });

  it('waits for a delayed entitlement instead of claiming success', async () => {
    myPlan.mockResolvedValueOnce(free).mockResolvedValue(pro);
    show(free, true);
    expect(await screen.findByText('Checking your plan…')).toBeTruthy();
    expect(screen.queryByRole('button', { name: 'Continue tailoring' })).toBeNull();
    await screen.findByRole('button', { name: 'Continue tailoring' }, { timeout: 4000 });
    expect(myPlan).toHaveBeenCalledTimes(2);
  });

  it('does not offer continuation for a return marker without Pro, and lets the user check again', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    myPlan.mockResolvedValue(free);
    show(free, true);
    await vi.advanceTimersByTimeAsync(2000 * 7);
    expect(await screen.findByText(/Pro isn't showing on your account yet/)).toBeTruthy();
    expect(screen.queryByRole('button', { name: 'Continue tailoring' })).toBeNull();
    const calls = myPlan.mock.calls.length;
    expect(calls).toBe(7);
    myPlan.mockResolvedValue(pro);
    await fireEvent.click(screen.getByRole('button', { name: 'Check again' }));
    await screen.findByRole('button', { name: 'Continue tailoring' });
    vi.useRealTimers();
  });

  it('is not paid by an API-only plan', async () => {
    // plan stays free even though the account holds an API licence
    show({ plan: 'free', allowances: [] } as unknown as PlanState, false);
    expect(await screen.findByText('Your tailoring task is saved')).toBeTruthy();
    expect(screen.queryByRole('button', { name: 'Continue tailoring' })).toBeNull();
  });

  it('lets an existing paid account with a saved task continue without any polling', async () => {
    show(pro, false);
    expect(await screen.findByRole('button', { name: 'Continue tailoring' })).toBeTruthy();
    expect(myPlan).not.toHaveBeenCalled();
  });

  it('ignores a task saved by another account and removes it', async () => {
    account.set('id', 8);
    show(pro, true);
    expect(screen.queryByRole('button')).toBeNull();
    expect(myPlan).not.toHaveBeenCalled();
    expect(sessionStorage.length).toBe(0);
  });

  it('ignores a stale task', async () => {
    sessionStorage.clear();
    saveTailorContext(7, { slug: 'a-1' }, sessionStorage, Date.now() - 3 * 60 * 60 * 1000);
    show(pro, true);
    expect(screen.queryByRole('button')).toBeNull();
    expect(myPlan).not.toHaveBeenCalled();
  });

  it('dismisses the task', async () => {
    show(free, false);
    await fireEvent.click(await screen.findByRole('button', { name: 'Dismiss' }));
    expect(screen.queryByText('Your tailoring task is saved')).toBeNull();
    expect(readTailorContext(7, sessionStorage)).toBeNull();
  });

  it('names the tier that was actually verified', async () => {
    show(ultra, false);
    expect(await screen.findByText('Ultra is active on your account')).toBeTruthy();
  });

  it('ignores a slow answer for the previous account after the account changes', async () => {
    let late!: (p: PlanState) => void;
    myPlan.mockImplementationOnce(() => new Promise<PlanState>((r) => (late = r))).mockResolvedValue(free);
    const onPlan = show(free, true);
    await waitFor(() => expect(myPlan).toHaveBeenCalledTimes(1));
    // The tab now belongs to account 8, who saved a task of their own.
    saveTailorContext(8, { slug: 'other-role-2' }, sessionStorage);
    account.set('id', 8);
    await waitFor(() => expect(myPlan).toHaveBeenCalledTimes(2));
    late(pro);
    await new Promise((r) => setTimeout(r, 0));
    expect(onPlan).not.toHaveBeenCalledWith(pro);
    expect(screen.queryByRole('button', { name: 'Continue tailoring' })).toBeNull();
    expect(readTailorContext(8, sessionStorage)).toEqual({ slug: 'other-role-2' });
  });

  it('keeps its verdict when the session refresh swaps the user object for the same account', async () => {
    myPlan.mockResolvedValue(pro);
    invalidateAll.mockImplementation(async () => {
      account.set('id', 7); // same id, as invalidateAll() re-resolving the user
    });
    show(free, true);
    await fireEvent.click(await screen.findByRole('button', { name: 'Continue tailoring' }));
    await waitFor(() => expect(goto).toHaveBeenCalledWith('/tailor/senior-go-acme-1?base=base-b'));
  });

  it('keeps the task and offers a retry when the workspace cannot be opened', async () => {
    goto.mockRejectedValueOnce(new Error('navigation failed')).mockResolvedValue(undefined);
    show(pro, false);
    await fireEvent.click(await screen.findByRole('button', { name: 'Continue tailoring' }));
    expect((await screen.findByRole('alert')).textContent).toMatch(/Couldn't open the workspace/);
    expect(readTailorContext(7, sessionStorage)).toEqual({ slug: 'senior-go-acme-1', baseCvId: 'base-b' });
    expect(track).not.toHaveBeenCalledWith('contextual_pro_continue', expect.anything());
    await fireEvent.click(screen.getByRole('button', { name: 'Continue tailoring' }));
    await waitFor(() => expect(goto).toHaveBeenCalledTimes(2));
    await waitFor(() => expect(readTailorContext(7, sessionStorage)).toBeNull());
  });

  it('does not delete a newer task when a finished continuation clears its own', async () => {
    let finish!: () => void;
    goto.mockImplementationOnce(() => new Promise<void>((r) => (finish = r)));
    show(pro, false);
    await fireEvent.click(await screen.findByRole('button', { name: 'Continue tailoring' }));
    await waitFor(() => expect(goto).toHaveBeenCalled());
    saveTailorContext(7, { slug: 'newer-role-9', baseCvId: 'base-z' }, sessionStorage);
    finish();
    await waitFor(() => expect(track).toHaveBeenCalledWith('contextual_pro_continue', { surface: 'plan' }));
    expect(readTailorContext(7, sessionStorage)).toEqual({ slug: 'newer-role-9', baseCvId: 'base-z' });
  });
});
