import { render, screen } from '@testing-library/svelte';
import { describe, expect, it, vi } from 'vitest';

// Signed out on purpose: the warning has to reach somebody who has not signed in yet,
// because that is the state a first-time visitor reads the page in. If it were nested
// under the authenticated branch it would only ever appear to people who had already
// got past the decision it is meant to inform.
vi.mock('#lib/auth.svelte.js', () => ({ isAuthenticated: () => false }));
vi.mock('#lib/signin.js', () => ({ promptSignIn: vi.fn() }));
vi.mock('#lib/api.js', () => ({ api: { promoPreview: vi.fn(), promoRedeem: vi.fn(), billingCheckout: vi.fn() } }));

import type { PlansMatrix, PublicPrice } from '#lib/types.js';

import Page from './+page.svelte';
import type { PageData } from './$types';

const price = (tier: PublicPrice['tier'], id: string): PublicPrice => ({
  id,
  tier,
  interval: 'month',
  amount_cents: 500,
  currency: 'usd',
  default: true,
});

// Both tiers on sale, so the assertions can count the warning across two cards rather
// than assume which one is present. No features and nothing enforced: the allowance rows
// are another subject entirely, and empty lists render the cards just as well.
const plans: PlansMatrix = {
  prices: [price('pro', 'price_pro'), price('ultra', 'price_ultra')],
  features: [],
  enforced: [],
};

// Typed by hand rather than as `PageData`, which declares `plans` as `null`: the loader's
// success branch does return a matrix, but the two branches collapse in the generated
// `$types`, and the page only ever reaches it through `plans?.`. A populated matrix is
// what production renders, so that is what the spec renders, and the mismatch is narrowed
// to this one boundary instead of being spread across four calls.
function renderPricing(matrix: PlansMatrix = plans) {
  return render(Page, {
    props: { data: { user: null, locale: 'en', plans: matrix, promo: '' } as unknown as PageData },
  });
}

describe('/pricing — the API plans', () => {
  it('lists both API plans and their commercial-use licence', async () => {
    renderPricing({
      ...plans,
      prices: [...plans.prices, price('api', 'price_api'), price('api_data', 'price_data')],
    });
    expect(await screen.findByRole('heading', { name: 'API', level: 3 })).toBeTruthy();
    expect(screen.getByRole('heading', { name: 'Data', level: 3 })).toBeTruthy();
    expect(screen.getByText(/An API plan licenses the data for use in your product/)).toBeTruthy();
    expect(screen.getByText('For builders')).toBeTruthy();
  });

  it('shows no API section when the product is not sold', async () => {
    renderPricing();
    await Promise.resolve();
    expect(screen.queryByRole('heading', { name: 'Jobs API', level: 2 })).toBeNull();
    expect(screen.queryByText('For builders')).toBeNull();
  });
});
