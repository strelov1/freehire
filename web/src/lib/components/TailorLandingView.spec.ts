import { render, screen, within } from '@testing-library/svelte';
import { describe, expect, it, vi } from 'vitest';
import TailorLandingView from './TailorLandingView.svelte';
import type { PublicPrice } from '#lib/types.js';

vi.mock('$app/paths', () => ({ resolve: (p: string) => p }));
vi.mock('#lib/signin.js', () => ({ promptSignIn: vi.fn() }));
vi.mock('#lib/auth.svelte.js', () => ({ isAuthenticated: () => false, currentUser: () => null }));
vi.mock('#lib/api.js', () => ({ api: { billingCheckout: vi.fn() } }));

vi.stubGlobal('matchMedia', (q: string) => ({ matches: q.includes('reduce'), media: q }));
vi.stubGlobal(
  'IntersectionObserver',
  class {
    observe() {}
    disconnect() {}
  },
);
Element.prototype.animate = vi.fn(() => ({ cancel() {}, finished: Promise.resolve(), onfinish: null })) as never;

const price = { id: 'price_m', amount_cents: 500, currency: 'usd', interval: 'month', default: true, tier: 'pro' } as PublicPrice;

describe('TailorLandingView', () => {
  it('states the promise and carries the founder note', () => {
    render(TailorLandingView, { price });
    expect(screen.getByRole('heading', { level: 1 }).textContent?.trim()).toBe('Your CV, rewritten for this job. Nothing made up.');
    expect(screen.getByText('— Ilya Strelov, founder')).toBeTruthy();
  });

  it('quotes the price in the comparison table', () => {
    render(TailorLandingView, { price });
    expect(within(screen.getByRole('table')).getByText('$5 / month')).toBeTruthy();
  });

  it('falls back to the plan name and a pricing link without a price', () => {
    render(TailorLandingView, { price: null });
    expect(within(screen.getByRole('table')).getByText('Pro plan')).toBeTruthy();
    for (const link of screen.getAllByRole('link', { name: 'See Pro plans' })) expect(link.getAttribute('href')).toBe('/pricing');
  });
});
