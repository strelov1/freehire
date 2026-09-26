import { describe, expect, it } from 'vitest';

import { cardsUnsupportedFrom } from './billingGeo';

describe('cardsUnsupportedFrom', () => {
  it('refuses the country our own Stripe account is registered in', () => {
    expect(cardsUnsupportedFrom('br')).toBe(true);
  });

  it('accepts the edge’s upper case, which is how it actually arrives', () => {
    expect(cardsUnsupportedFrom('BR')).toBe(true);
    expect(cardsUnsupportedFrom(' Br ')).toBe(true);
  });

  it('permits every other country', () => {
    for (const country of ['us', 'pt', 'de', 'kg', 'rs', 'gb']) {
      expect(cardsUnsupportedFrom(country)).toBe(false);
    }
  });

  // An unplaced visitor must keep the button's promise intact. Warning somebody we
  // could not locate would spend a real reader's attention on a guess we did not make.
  it('permits a visitor the edge could not place', () => {
    expect(cardsUnsupportedFrom(null)).toBe(false);
    expect(cardsUnsupportedFrom(undefined)).toBe(false);
    expect(cardsUnsupportedFrom('')).toBe(false);
    expect(cardsUnsupportedFrom('   ')).toBe(false);
  });

  // Cloudflare's two reserved values are shaped exactly like country codes and are
  // not countries. Neither is Brazil, so both fall out of the check on their own —
  // named here so a later reader knows that is intended rather than incidental.
  it('permits Cloudflare’s reserved codes', () => {
    expect(cardsUnsupportedFrom('XX')).toBe(false);
    expect(cardsUnsupportedFrom('T1')).toBe(false);
  });
});
