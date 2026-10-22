import { render, screen } from '@testing-library/svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';

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
  api_limits: { anonymous_per_minute: 120, anonymous_per_day: 2000, api_per_minute: 1200, data_per_minute: 6000 },
};

// Typed by hand rather than as `PageData`, which declares `plans` as `null`: the loader's
// success branch does return a matrix, but the two branches collapse in the generated
// `$types`, and the page only ever reaches it through `plans?.`. A populated matrix is
// what production renders, so that is what the spec renders, and the mismatch is narrowed
// to this one boundary instead of being spread across four calls.
function renderPricing(matrix: PlansMatrix = plans) {
  return render(Page, { props: { data: { user: null, locale: 'en', plans: matrix, promo: '' } as unknown as PageData } });
}

/** Stands in for the edge, answering `/geo/region` with one country. */
function edgePlaces(country: string | null) {
  vi.stubGlobal(
    'fetch',
    vi.fn(async () => new Response(JSON.stringify({ region: null, country }), { status: 200 })),
  );
}

// The page asks the edge on arrival and renders the answer a tick later, so every
// assertion here waits rather than reading the first paint.
const WARNING = /Cards issued in Brazil can.t be charged here/;

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('/pricing — the local-card warning', () => {
  it('warns a visitor the edge places in the account’s own country', async () => {
    edgePlaces('BR');
    renderPricing();
    // Both paid cards carry it: somebody comparing Pro against Ultra must not have to
    // pick the right column to be told their card cannot pay for either.
    await expect(screen.findAllByText(WARNING)).resolves.toHaveLength(2);
  });

  it('says nothing to a visitor anywhere else', async () => {
    edgePlaces('PT');
    renderPricing();
    await screen.findByText('Upgrade to Pro', { exact: false }).catch(() => null);
    expect(screen.queryByText(WARNING)).toBeNull();
  });

  it('says nothing when the edge could not place the visitor', async () => {
    edgePlaces(null);
    renderPricing();
    await screen.findByText('Upgrade to Pro', { exact: false }).catch(() => null);
    expect(screen.queryByText(WARNING)).toBeNull();
  });

  // The warning may only ever add a sentence. An edge that cannot be reached must
  // leave the page exactly as it would have been for anyone else — never blank, and
  // never warning everybody just because one request failed.
  it('says nothing when the edge cannot be reached', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => {
        throw new Error('offline');
      }),
    );
    renderPricing();
    await screen.findByText('Upgrade to Pro', { exact: false }).catch(() => null);
    expect(screen.queryByText(WARNING)).toBeNull();
  });
});

describe('/pricing — the API plans', () => {
  it('lists both API plans with the limits the server enforces', async () => {
    edgePlaces(null);
    renderPricing({
      ...plans,
      prices: [...plans.prices, price('api', 'price_api'), price('api_data', 'price_data')],
    });
    expect(await screen.findByRole('heading', { name: 'API', level: 3 })).toBeTruthy();
    expect(screen.getByRole('heading', { name: 'Data', level: 3 })).toBeTruthy();
    expect(screen.getByText('1,200 requests a minute, no daily cap')).toBeTruthy();
    expect(screen.getByText('6,000 requests a minute, no daily cap')).toBeTruthy();
    expect(screen.getByText(/up to 120 requests a minute/)).toBeTruthy();
  });

  it('shows no API section when the product is not sold', async () => {
    edgePlaces(null);
    renderPricing();
    await Promise.resolve();
    expect(screen.queryByRole('heading', { name: 'API', level: 2 })).toBeNull();
  });
});
