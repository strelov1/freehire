import { render, screen } from '@testing-library/svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';

// Signed out on purpose: the warning has to reach somebody who has not signed in yet,
// because that is the state a first-time visitor reads the page in. If it were nested
// under the authenticated branch it would only ever appear to people who had already
// got past the decision it is meant to inform.
vi.mock('$lib/auth.svelte', () => ({ isAuthenticated: () => false }));
vi.mock('$lib/signin', () => ({ promptSignIn: vi.fn() }));
vi.mock('$lib/api', () => ({ api: { promoPreview: vi.fn(), promoRedeem: vi.fn(), billingCheckout: vi.fn() } }));

import Page from './+page.svelte';

const price = (tier: string, id: string) => ({
  id,
  tier,
  interval: 'month',
  amount_cents: 500,
  currency: 'usd',
});

const data = {
  plans: {
    prices: [price('pro', 'price_pro'), price('ultra', 'price_ultra')],
    features: [],
  },
  promo: null,
};

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
    render(Page, { props: { data } });
    // Both paid cards carry it: somebody comparing Pro against Ultra must not have to
    // pick the right column to be told their card cannot pay for either.
    await expect(screen.findAllByText(WARNING)).resolves.toHaveLength(2);
  });

  it('says nothing to a visitor anywhere else', async () => {
    edgePlaces('PT');
    render(Page, { props: { data } });
    await screen.findByText('Upgrade to Pro', { exact: false }).catch(() => null);
    expect(screen.queryByText(WARNING)).toBeNull();
  });

  it('says nothing when the edge could not place the visitor', async () => {
    edgePlaces(null);
    render(Page, { props: { data } });
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
    render(Page, { props: { data } });
    await screen.findByText('Upgrade to Pro', { exact: false }).catch(() => null);
    expect(screen.queryByText(WARNING)).toBeNull();
  });
});
