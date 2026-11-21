import type { PublicPrice } from '#lib/types.js';

// shortcut: the coming prices are written here, not in Stripe, until they go live on
// 1 December; delete this file once the new Stripe prices are the default ones.
export const PRICE_INCREASE_LABEL = 'December 1';
const PRICE_INCREASE_AT = new Date('2026-12-01T00:00:00Z');
// Keyed `tier/interval`; a price not listed here is not changing.
const UPCOMING_CENTS: Record<string, number> = {
  'pro/month': 900,
  'pro/year': 7900,
  'api/month': 3900,
  'api_data/month': 12900,
};

// The price a plan moves to, or null when nothing is coming: past the date, or the
// live price already caught up — so a forgotten file never shows a fake "was" price.
export function upcomingCents(p: PublicPrice, now = new Date()): number | null {
  const next = UPCOMING_CENTS[`${p.tier}/${p.interval}`];
  if (!next || now >= PRICE_INCREASE_AT) return null;
  return next > p.amount_cents ? next : null;
}
