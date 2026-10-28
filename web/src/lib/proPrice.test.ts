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
  // The server renders the label and the browser hydrates it; a visitor whose browser
  // speaks German must get the same "$5" the server wrote, not "5 $".
  it('reads the same whatever the runtime locale', () => {
    const original = Intl.NumberFormat;
    const german = function (_locales?: string | string[], options?: Intl.NumberFormatOptions) {
      return new original(_locales ?? 'de-DE', options);
    } as unknown as typeof Intl.NumberFormat;
    Intl.NumberFormat = german;
    try {
      expect(priceLabel(price({ amount_cents: 500 }))).toBe('$5');
    } finally {
      Intl.NumberFormat = original;
    }
  });
});
