import { describe, expect, it } from 'vitest';
import { beginCheckoutTracking, claimCheckoutPurchase } from './purchaseTracking';
import type { BillingPaymentAttempt } from './types';

function storage() {
  const values = new Map<string, string>();
  return {
    getItem: (key: string) => values.get(key) ?? null,
    setItem: (key: string, value: string) => {
      values.set(key, value);
    },
    removeItem: (key: string) => {
      values.delete(key);
    },
  };
}
const now = Date.parse('2026-11-15T12:00:00Z');
const charge: BillingPaymentAttempt = {
  id: 'ch_new',
  date: new Date(now + 1000).toISOString(),
  amount_cents: 1234,
  amount_refunded_cents: 0,
  currency: 'usd',
  status: 'succeeded',
};
describe('checkout purchase attribution', () => {
  it('requires a checkout and a new confirmed charge, then claims it once', () => {
    const store = storage();
    expect(claimCheckoutPurchase(7, [charge], store, now + 2000)).toBeNull();
    beginCheckoutTracking(7, ['ch_old'], store, now);
    expect(
      claimCheckoutPurchase(
        7,
        [
          { ...charge, id: 'ch_old' },
          { ...charge, status: 'failed' },
        ],
        store,
        now + 2000,
      ),
    ).toBeNull();
    expect(claimCheckoutPurchase(7, [charge], store, now + 2000)).toEqual({
      transaction_id: 'ch_new',
      value: 12.34,
      currency: 'USD',
    });
    expect(claimCheckoutPurchase(7, [charge], store, now + 2000)).toBeNull();
  });
  it('rejects other accounts, old charges and expired checkouts', () => {
    const store = storage();
    beginCheckoutTracking(7, [], store, now);
    expect(claimCheckoutPurchase(8, [charge], store, now + 2000)).toBeNull();
    expect(
      claimCheckoutPurchase(7, [{ ...charge, date: new Date(now - 1000).toISOString() }], store, now + 2000),
    ).toBeNull();
    expect(claimCheckoutPurchase(7, [charge], store, now + 86400001)).toBeNull();
  });
  it('converts zero-decimal and provider-specific currencies', () => {
    for (const [currency, value] of [
      ['jpy', 1234],
      ['isk', 12.34],
    ] as const) {
      const store = storage();
      beginCheckoutTracking(7, [], store, now);
      expect(claimCheckoutPurchase(7, [{ ...charge, currency }], store, now + 2000)?.value).toBe(value);
    }
  });
  it('ignores corrupt or unavailable storage', () => {
    const store = storage();
    store.setItem('fh:checkout', '{');
    expect(claimCheckoutPurchase(7, [charge], store, now)).toBeNull();
  });
});
