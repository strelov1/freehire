import { fireEvent, render, screen } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import ProCheckoutButton from './ProCheckoutButton.svelte';
import type { PublicPrice } from '#lib/types.js';

const { promptSignIn, billingCheckout, authed, user } = vi.hoisted(() => ({
  promptSignIn: vi.fn(),
  billingCheckout: vi.fn(),
  authed: { value: false },
  user: { value: null as { tier: string } | null },
}));
vi.mock('$app/paths', () => ({ resolve: (p: string) => p }));
vi.mock('#lib/signin.js', () => ({ promptSignIn }));
vi.mock('#lib/auth.svelte.js', () => ({ isAuthenticated: () => authed.value, currentUser: () => user.value }));
vi.mock('#lib/api.js', () => ({ api: { billingCheckout } }));

const price = { id: 'price_m', amount_cents: 500, currency: 'usd', interval: 'month', default: true, tier: 'pro' } as PublicPrice;

beforeEach(() => {
  promptSignIn.mockReset();
  billingCheckout.mockReset();
  authed.value = false;
  user.value = null;
});

describe('ProCheckoutButton', () => {
  it('links to /pricing when no price is known', () => {
    render(ProCheckoutButton, { price: null });
    expect(screen.getByRole('link', { name: 'See Pro plans' }).getAttribute('href')).toBe('/pricing');
  });

  it('quotes the price and asks a signed-out visitor to sign in', async () => {
    render(ProCheckoutButton, { price });
    await fireEvent.click(screen.getByRole('button', { name: 'Get Pro — $5/mo' }));
    expect(promptSignIn).toHaveBeenCalledOnce();
    expect(billingCheckout).not.toHaveBeenCalled();
  });

  it('opens checkout for a signed-in visitor', async () => {
    authed.value = true;
    billingCheckout.mockResolvedValue({ url: 'https://checkout.example/s' });
    render(ProCheckoutButton, { price });
    await fireEvent.click(screen.getByRole('button', { name: 'Get Pro — $5/mo' }));
    expect(billingCheckout).toHaveBeenCalledWith('price_m');
  });

  it('sends a paid account to its plan page, never to checkout', () => {
    authed.value = true;
    user.value = { tier: 'pro' };
    render(ProCheckoutButton, { price });
    expect(screen.getByRole('link', { name: 'Manage your plan' }).getAttribute('href')).toBe('/my/plan');
    expect(screen.queryByRole('button')).toBeNull();
    expect(billingCheckout).not.toHaveBeenCalled();
  });

  it('says so when checkout fails', async () => {
    authed.value = true;
    billingCheckout.mockRejectedValue(new Error('down'));
    render(ProCheckoutButton, { price });
    await fireEvent.click(screen.getByRole('button', { name: 'Get Pro — $5/mo' }));
    expect(await screen.findByText(/Checkout is not available/)).toBeTruthy();
  });
});
