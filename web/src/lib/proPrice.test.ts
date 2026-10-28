import { describe, expect, it } from 'vitest';
import type { PlansMatrix, PublicPrice } from './types';
import { priceLabel, proMonthlyPrice } from './proPrice';

const price = (over: Partial<PublicPrice>): PublicPrice =>
  ({ id: 'p', amount_cents: 500, currency: 'usd', interval: 'month', default: true, tier: 'pro', ...over }) as PublicPrice;

describe('proMonthlyPrice', () => {
  it('picks the Pro monthly price', () => {
    const plans = { prices: [price({ id: 'u', tier: 'ultra' }), price({ id: 'y', interval: 'year' }), price({ id: 'm' })] } as PlansMatrix;
    expect(proMonthlyPrice(plans)?.id).toBe('m');
  });
  it('is null without a matrix or a monthly Pro price', () => {
    expect(proMonthlyPrice(null)).toBeNull();
    expect(proMonthlyPrice({ prices: [price({ interval: 'year' })] } as PlansMatrix)).toBeNull();
  });
});

describe('priceLabel', () => {
  it('drops zero cents', () => {
    expect(priceLabel(price({ amount_cents: 500 }))).toBe('$5');
  });
  it('keeps real cents', () => {
    expect(priceLabel(price({ amount_cents: 499 }))).toBe('$4.99');
  });
});
