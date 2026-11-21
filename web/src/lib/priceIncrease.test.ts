import { describe, expect, it } from 'vitest';
import type { PublicPrice } from '#lib/types.js';
import { upcomingCents } from './priceIncrease.js';

const price = (tier: PublicPrice['tier'], amount_cents: number, interval = 'month'): PublicPrice =>
  ({ id: 'p', tier, amount_cents, interval, currency: 'usd', default: true }) as PublicPrice;
const before = new Date('2026-11-25T00:00:00Z');

describe('upcomingCents', () => {
  it('shows the coming price for Pro monthly and yearly, and API', () => {
    expect(upcomingCents(price('pro', 5000, 'year'), before)).toBe(7900);
    expect(upcomingCents(price('pro', 500), before)).toBe(900);
    expect(upcomingCents(price('api', 2900), before)).toBe(3900);
  });
  it('shows nothing for weekly, after the date, or once the price is live', () => {
    expect(upcomingCents(price('pro', 200, 'week'), before)).toBeNull();
    expect(upcomingCents(price('api_data', 9900), before)).toBe(12900);
    expect(upcomingCents(price('pro', 500), new Date('2026-12-01T00:00:00Z'))).toBeNull();
    expect(upcomingCents(price('pro', 900), before)).toBeNull();
  });
});
