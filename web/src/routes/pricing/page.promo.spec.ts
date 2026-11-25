import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';

// Signed IN on purpose: page.spec.ts stays signed out, because the headline and the free
// tier's warnings are what a first-time visitor reads. The promo box only exists for an
// authenticated caller on a sold Pro plan, so that is the state this file renders.
vi.mock('#lib/auth.svelte.js', () => ({ isAuthenticated: () => true }));
vi.mock('#lib/signin.js', () => ({ promptSignIn: vi.fn() }));
const { promoPreview, promoRedeem, billingCheckout, myPlan, getApiPlan } = vi.hoisted(() => ({
  promoPreview: vi.fn<() => Promise<{ percent_off: number }>>(),
  promoRedeem: vi.fn<() => Promise<{ percent_off: number }>>(),
  billingCheckout: vi.fn<() => Promise<{ url: string }>>(),
  myPlan: vi.fn<() => Promise<{ plan: string }>>(),
  getApiPlan: vi.fn<() => Promise<{ tier: string }>>(),
}));
// The real ApiError class: the page reads `status` off it, so a stand-in shape would let
// the test pass against a class the page never sees.
vi.mock('#lib/api.js', async () => ({
  ApiError: (await import('#lib/api-core.js')).ApiError,
  api: { promoPreview, promoRedeem, billingCheckout, myPlan, getApiPlan },
}));

import { ApiError } from '#lib/api-core.js';
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

const plans: PlansMatrix = {
  prices: [price('pro', 'price_pro')],
  features: [],
  enforced: [],
};

function renderPricing() {
  return render(Page, {
    props: { data: { user: null, locale: 'en', plans, promo: '' } as unknown as PageData },
  });
}

const codeInput = () => screen.getByLabelText('Have a promo code?');
const notAvailable = 'That code is not available.';

beforeEach(() => {
  myPlan.mockReset().mockResolvedValue({ plan: 'free' });
  // ApiPlans asks which API tier the caller holds whenever it renders for a signed-in
  // visitor; a refusal there is swallowed by the component, so it is the quiet default.
  getApiPlan.mockReset().mockRejectedValue(new ApiError(404, 'no plan'));
  billingCheckout.mockReset().mockResolvedValue({ url: 'https://checkout.example.test/session' });
});

describe('/pricing — the promo code box', () => {
  it('tells an account that already used a code by the 409, not by the error wording', async () => {
    // The message deliberately does NOT contain "already": a regression to matching on the
    // wording as well as the status would still pass a fixture that did.
    promoPreview.mockRejectedValue(new ApiError(409, 'redemption exists'));
    renderPricing();

    await fireEvent.input(codeInput(), { target: { value: 'SPRING' } });
    await fireEvent.click(screen.getByRole('button', { name: 'Check' }));

    expect(await screen.findByText('You have already used a promo code.')).toBeTruthy();
  });

  it('reads any other refusal as a code that is not available, whatever it says', async () => {
    // The word the page used to branch on, arriving on a status that is not the 409: the
    // account is told the CODE is refused, which is what the server answered.
    promoPreview.mockRejectedValue(new ApiError(404, 'that code is not available (already used by someone)'));
    renderPricing();

    await fireEvent.input(codeInput(), { target: { value: 'SPRING' } });
    await fireEvent.click(screen.getByRole('button', { name: 'Check' }));

    expect(await screen.findByText(notAvailable)).toBeTruthy();
    expect(screen.queryByText('You have already used a promo code.')).toBeNull();
  });

  it('keeps selling to an account that already holds a code, and still opens checkout', async () => {
    promoRedeem.mockRejectedValue(new ApiError(409, 'redemption exists'));
    renderPricing();

    await fireEvent.input(codeInput(), { target: { value: 'SPRING' } });
    await fireEvent.click(screen.getByRole('button', { name: 'Upgrade to Pro' }));

    expect(
      await screen.findByText('You have already used a promo code — your existing discount still applies.'),
    ).toBeTruthy();
    await waitFor(() => expect(billingCheckout).toHaveBeenCalledWith('price_pro'));
  });

  it('stops the purchase on any other refusal and never opens checkout', async () => {
    // The old string match would have read this 404 as "already held" and charged full price.
    promoRedeem.mockRejectedValue(new ApiError(404, 'that code is not available (already used by someone)'));
    renderPricing();

    await fireEvent.input(codeInput(), { target: { value: 'SPRING' } });
    await fireEvent.click(screen.getByRole('button', { name: 'Upgrade to Pro' }));

    expect(await screen.findByText(`${notAvailable} Clear it to continue at the usual price.`)).toBeTruthy();
    expect(billingCheckout).not.toHaveBeenCalled();
  });
});
