import { describe, expect, it } from 'vitest';
import { formatMinorUnits } from './money';

// The expected strings are built with Intl rather than written out, because the separators
// and symbol placement are the RUNTIME's business and vary with its locale. What is being
// asserted here is only the divisor — which unit amount the minor amount stands for — and
// that is the whole of what this function decides.
const asCurrency = (amount: number, code: string) =>
  new Intl.NumberFormat(undefined, { style: 'currency', currency: code }).format(amount);

describe('formatMinorUnits', () => {
  it('divides by a hundred where the currency has two decimals', () => {
    expect(formatMinorUnits(500, 'usd')).toBe(asCurrency(5, 'USD'));
    expect(formatMinorUnits(4900, 'eur')).toBe(asCurrency(49, 'EUR'));
  });

  it('divides by nothing where the currency has none', () => {
    expect(formatMinorUnits(1000, 'jpy')).toBe(asCurrency(1000, 'JPY'));
  });

  it('divides by a thousand where the currency has three decimals', () => {
    expect(formatMinorUnits(5000, 'kwd')).toBe(asCurrency(5, 'KWD'));
  });

  // Stripe's exceptions: no decimals, but the amount still arrives multiplied by a hundred.
  // Taking Intl's exponent at face value here would print 500 for a charge of 5.
  it('divides by a hundred for the currencies Stripe encodes that way anyway', () => {
    expect(formatMinorUnits(500, 'isk')).toBe(asCurrency(5, 'ISK'));
    expect(formatMinorUnits(1800000, 'ugx')).toBe(asCurrency(18000, 'UGX'));
  });

  it('falls back to dollars when the provider sent no currency', () => {
    expect(formatMinorUnits(500, '')).toBe(asCurrency(5, 'USD'));
  });
});
