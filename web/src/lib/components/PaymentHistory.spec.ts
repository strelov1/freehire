import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { SvelteMap } from 'svelte/reactivity';

const mocks = vi.hoisted(() => ({ attempts: vi.fn() }));
vi.mock('#lib/api.js', async (importOriginal) => ({
  ...(await importOriginal<typeof import('#lib/api.js')>()),
  api: { billingAttempts: mocks.attempts },
}));
vi.mock('#lib/auth.svelte.js', () => ({ currentUser: () => ({ id: owner.get('id') }) }));
vi.mock('#lib/i18n/currentLocale.svelte.js', () => ({ locale: () => 'en' }));

import PaymentHistory from './PaymentHistory.svelte';

const owner = new SvelteMap([['id', 1]]);

const failed = {
  id: 'ch_retry',
  date: '2026-11-14T13:24:38Z',
  amount_cents: 290,
  currency: 'brl',
  status: 'failed',
  reason: 'insufficient_funds',
  card_brand: 'mastercard',
  card_last4: '4242',
  card_funding: 'credit',
  wallet: 'apple_pay',
  amount_refunded_cents: 0,
};

beforeEach(() => {
  vi.resetAllMocks();
  owner.set('id', 1);
});
afterEach(cleanup);

describe('payment history', () => {
  it('shows separate failures and success, with bank-attributed help and no repayment action', async () => {
    mocks.attempts.mockResolvedValue([{ ...failed, id: 'ch_success', status: 'succeeded', reason: '' }, failed]);
    render(PaymentHistory);
    await screen.findByText('Paid');
    expect(screen.getByText('Payment failed')).toBeTruthy();
    expect(screen.getByText(/The bank reported insufficient funds/)).toBeTruthy();
    expect(screen.getByText(/available credit limit/)).toBeTruthy();
    expect(screen.getAllByText(/4242/)).toHaveLength(2);
    expect(screen.queryByRole('button', { name: /pay|purchase/i })).toBeNull();
  });

  it('distinguishes provider errors from empty history and can refresh', async () => {
    mocks.attempts.mockRejectedValueOnce(new Error('offline')).mockResolvedValueOnce([]);
    render(PaymentHistory);
    await screen.findByText("Couldn't load payment history. Try refreshing in a moment.");
    await fireEvent.click(screen.getByRole('button', { name: 'Refresh' }));
    await screen.findByText('No payment attempts yet.');
    expect(mocks.attempts).toHaveBeenCalledTimes(2);
  });

  it('uses safe generic copy for unknown failure reasons', async () => {
    mocks.attempts.mockResolvedValue([{ ...failed, reason: 'private_provider_reason' }]);
    render(PaymentHistory);
    await screen.findByText('The payment was declined.');
    expect(screen.queryByText('private_provider_reason')).toBeNull();
    await waitFor(() => expect(screen.getByRole('button', { name: 'Refresh' }).hasAttribute('disabled')).toBe(false));
  });

  it('drops a late response from the previous account', async () => {
    let finish: (value: (typeof failed)[]) => void = () => {};
    mocks.attempts
      .mockReturnValueOnce(
        new Promise<(typeof failed)[]>((resolve) => {
          finish = resolve;
        }),
      )
      .mockResolvedValueOnce([]);
    render(PaymentHistory);
    await waitFor(() => expect(mocks.attempts).toHaveBeenCalledTimes(1));
    owner.set('id', 2);
    await screen.findByText('No payment attempts yet.');
    finish([failed]);
    await new Promise((resolve) => setTimeout(resolve, 0));
    expect(screen.queryByText(/4242/)).toBeNull();
    expect(screen.getByText('No payment attempts yet.')).toBeTruthy();
  });
});
