import type { BillingPaymentAttempt } from './types';
import { minorUnitsValue } from './money';

type CheckoutStore = Pick<Storage, 'getItem' | 'setItem' | 'removeItem'>;
const key = 'fh:checkout';
// Browser and provider timestamps need not share a clock; known charge IDs still exclude prior payments.
const clockSkewMs = 10 * 60 * 1000;

export function beginCheckoutTracking(
  userId: number,
  chargeIds: string[],
  store: CheckoutStore,
  now = Date.now(),
): void {
  try {
    store.setItem(key, JSON.stringify({ userId, chargeIds, started: Math.floor(now / 1000) * 1000 }));
  } catch {
    // Analytics must not prevent checkout when storage is unavailable.
  }
}

export function claimCheckoutPurchase(
  userId: number,
  attempts: BillingPaymentAttempt[],
  store: CheckoutStore,
  now = Date.now(),
) {
  try {
    const pending: unknown = JSON.parse(store.getItem(key) ?? 'null');
    if (
      !pending ||
      typeof pending !== 'object' ||
      !('userId' in pending) ||
      pending.userId !== userId ||
      !('started' in pending) ||
      typeof pending.started !== 'number' ||
      !('chargeIds' in pending) ||
      !Array.isArray(pending.chargeIds) ||
      now < pending.started ||
      now - pending.started > 86400000
    )
      return null;
    const started = pending.started;
    const chargeIds = pending.chargeIds;
    const charge = attempts.find(
      (a) =>
        a.status === 'succeeded' &&
        a.amount_cents > 0 &&
        a.id &&
        a.currency &&
        !chargeIds.includes(a.id) &&
        Date.parse(a.date) >= started - clockSkewMs &&
        Date.parse(a.date) <= now + clockSkewMs,
    );
    if (!charge) return null;
    const value = minorUnitsValue(charge.amount_cents, charge.currency);
    store.removeItem(key);
    return { transaction_id: charge.id, value, currency: charge.currency.toUpperCase() };
  } catch {
    return null;
  }
}
