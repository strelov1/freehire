import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const { getJobMatch, myPlan, listBaseCvs, plans, billingCheckout, track } = vi.hoisted(() => ({
  getJobMatch: vi.fn(),
  myPlan: vi.fn(),
  listBaseCvs: vi.fn(),
  plans: vi.fn(),
  billingCheckout: vi.fn(),
  track: vi.fn(),
}));
vi.mock('$app/paths', () => ({ resolve: (p: string) => p }));
vi.mock('#lib/api.js', () => ({ api: { getJobMatch, myPlan, listBaseCvs, plans, billingCheckout } }));
vi.mock('#lib/analytics.js', () => ({ track }));
vi.mock('#lib/auth.svelte.js', () => ({ currentUser: () => ({ id: 7 }), isAuthenticated: () => true }));
vi.mock('#lib/i18n/currentLocale.svelte.js', () => ({ locale: () => 'en' }));

import { readTailorContext } from '#lib/tailorCheckout.js';
import { askConfirmTailor, settleConfirmTailorDialog } from '#lib/confirmTailorDialog.svelte.js';
import ConfirmTailorDialog from './ConfirmTailorDialog.svelte';

const monthly = { id: 'price_m', tier: 'pro', interval: 'month', amount_cents: 500, currency: 'usd' };
const yearly = { ...monthly, id: 'price_y', interval: 'year', amount_cents: 4800 };
const freeNoTailor = { plan: 'free', allowances: [{ feature: 'tailor', used: 0, unlimited: false, enforced: true }] };
const base = (id: string, is_default = false) => ({ id, title: id, is_default });

let navigated = '';
beforeEach(() => {
  for (const m of [getJobMatch, myPlan, listBaseCvs, plans, billingCheckout, track]) m.mockReset();
  sessionStorage.clear();
  navigated = '';
  getJobMatch.mockResolvedValue({});
  plans.mockResolvedValue({ prices: [yearly, monthly] });
  listBaseCvs.mockResolvedValue([base('base-a', true), base('base-b')]);
  vi.stubGlobal('location', {
    get href() {
      return navigated;
    },
    set href(v: string) {
      navigated = v;
    },
  });
});

// Not awaited by callers: the promise settles only when the dialog does.
function open(plan: unknown = freeNoTailor) {
  myPlan.mockResolvedValue(plan);
  render(ConfirmTailorDialog);
  return askConfirmTailor('senior-go-acme-1', 'Go dev at Acme');
}

describe('ConfirmTailorDialog contextual Pro checkout', () => {
  it('states a monthly subscription at the real price and links to yearly choices and terms', async () => {
    const answer = open();
    const cta = await screen.findByRole('button', { name: 'Upgrade to Pro — $5/month' });
    expect(cta).toBeTruthy();
    expect(screen.getByText(/Monthly subscription/)).toBeTruthy();
    expect(screen.getByRole('link', { name: 'Yearly plans' }).getAttribute('href')).toBe('/pricing');
    expect(screen.getByRole('link', { name: 'Terms' }).getAttribute('href')).toBe('/terms');
    expect(track).toHaveBeenCalledWith('contextual_pro_offer_view', { surface: 'confirm_tailor' });
    expect(track.mock.calls.filter((c) => c[0] === 'contextual_pro_offer_view')).toHaveLength(1);
    settleConfirmTailorDialog(false);
    await answer;
  });

  it('goes straight to checkout, remembering the vacancy and the exact base', async () => {
    billingCheckout.mockResolvedValue({ url: 'https://checkout.example/s' });
    const answer = open();
    await fireEvent.click(await screen.findByRole('button', { name: 'Upgrade to Pro — $5/month' }));
    await waitFor(() => expect(navigated).toBe('https://checkout.example/s'));
    expect(billingCheckout).toHaveBeenCalledWith('price_m');
    expect(readTailorContext(7, sessionStorage)).toEqual({ slug: 'senior-go-acme-1', baseCvId: 'base-a' });
    settleConfirmTailorDialog(false);
    await answer;
  });

  it('keeps the base when it is the only one', async () => {
    listBaseCvs.mockResolvedValue([base('only', true)]);
    billingCheckout.mockResolvedValue({ url: 'https://checkout.example/s' });
    const answer = open();
    await fireEvent.click(await screen.findByRole('button', { name: 'Upgrade to Pro — $5/month' }));
    await waitFor(() => expect(navigated).not.toBe(''));
    expect(readTailorContext(7, sessionStorage)).toEqual({ slug: 'senior-go-acme-1', baseCvId: 'only' });
    settleConfirmTailorDialog(false);
    await answer;
  });

  it('says so and allows a retry when the checkout session cannot be created', async () => {
    billingCheckout
      .mockRejectedValueOnce(new Error('down'))
      .mockResolvedValueOnce({ url: 'https://checkout.example/s' });
    const answer = open();
    await fireEvent.click(await screen.findByRole('button', { name: 'Upgrade to Pro — $5/month' }));
    expect((await screen.findByRole('alert')).textContent).toMatch(/Checkout is not available/);
    expect(navigated).toBe('');
    await fireEvent.click(screen.getByRole('button', { name: 'Upgrade to Pro — $5/month' }));
    await waitFor(() => expect(navigated).toBe('https://checkout.example/s'));
    settleConfirmTailorDialog(false);
    await answer;
  });

  it('ignores a checkout answer that lands after the dialog was closed', async () => {
    let finish!: (v: { url: string }) => void;
    let fail!: (e: Error) => void;
    billingCheckout
      .mockReturnValueOnce(new Promise((r) => (finish = r)))
      .mockReturnValueOnce(new Promise((_, j) => (fail = j)));
    const first = open();
    await fireEvent.click(await screen.findByRole('button', { name: 'Upgrade to Pro — $5/month' }));
    settleConfirmTailorDialog(false);
    await first;
    finish({ url: 'https://checkout.example/late' });

    const second = askConfirmTailor('senior-go-acme-1', 'Go dev at Acme');
    await fireEvent.click(await screen.findByRole('button', { name: 'Upgrade to Pro — $5/month' }));
    settleConfirmTailorDialog(false);
    await second;
    fail(new Error('late'));
    await new Promise((r) => setTimeout(r, 0));

    expect(navigated).toBe('');
    const third = askConfirmTailor('senior-go-acme-1', 'Go dev at Acme');
    await screen.findByRole('button', { name: 'Upgrade to Pro — $5/month' });
    expect(screen.queryByRole('alert')).toBeNull();
    settleConfirmTailorDialog(false);
    await third;
  });

  it('offers nothing to an account that already has Pro', async () => {
    const answer = open({ plan: 'pro', allowances: [{ feature: 'tailor', used: 0, limit: 5, unlimited: false }] });
    await screen.findByText(/CV editing session/);
    expect(screen.queryByRole('button', { name: /Upgrade to Pro/ })).toBeNull();
    expect(track).not.toHaveBeenCalledWith('contextual_pro_offer_view', expect.anything());
    settleConfirmTailorDialog(false);
    await answer;
  });

  it('falls back to the plan page link when no price is on sale', async () => {
    plans.mockResolvedValue({ prices: [] });
    const answer = open();
    await screen.findByText("Your plan doesn't include CV editing sessions.");
    await waitFor(() => expect(plans).toHaveBeenCalled());
    expect(screen.queryByRole('button', { name: /Upgrade to Pro/ })).toBeNull();
    settleConfirmTailorDialog(false);
    await answer;
  });
});
