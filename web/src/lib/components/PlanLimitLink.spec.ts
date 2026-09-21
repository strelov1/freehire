import { render, screen } from '@testing-library/svelte';
import { describe, expect, it, vi } from 'vitest';
import PlanLimitLink from './PlanLimitLink.svelte';
import type { PlansMatrix } from '$lib/types';

vi.mock('$app/paths', () => ({ resolve: (p: string) => p, base: '', assets: '' }));
vi.mock('$lib/i18n/currentLocale.svelte', () => ({ locale: () => 'en' }));

// Mutable, like AccountTimezone.spec.ts's `user` — a test flips the tier before
// rendering rather than reaching for a fixed object that could only prove one branch.
const { user, plans } = vi.hoisted(() => ({
  user: { current: null as { tier: 'free' | 'pro' | 'ultra' } | null },
  plans: vi.fn(),
}));
vi.mock('$lib/auth.svelte', () => ({ currentUser: () => user.current }));
vi.mock('$lib/api', () => ({ api: { plans } }));

const proMonthlyPrice = (amount_cents: number): PlansMatrix => ({
  features: [],
  enforced: [],
  prices: [{ id: 'price_1', amount_cents, currency: 'usd', interval: 'month', default: true, tier: 'pro' }],
});

describe('PlanLimitLink', () => {
  it('shows the priced Upgrade CTA to a free reader, read from the same source /pricing uses', async () => {
    plans.mockResolvedValue(proMonthlyPrice(500));
    user.current = { tier: 'free' };
    render(PlanLimitLink);
    // findByRole waits for the async price fetch to land — a hardcoded price here would
    // be a second copy of the money rule money.ts warns against.
    const link = await screen.findByRole('link', { name: 'Upgrade to Pro — $5.00/mo' });
    expect(link.getAttribute('href')).toBe('/my/plan');
  });

  it('still upgrades, just without a price in the label, when the price fetch fails', async () => {
    plans.mockRejectedValue(new Error('network'));
    user.current = { tier: 'free' };
    render(PlanLimitLink);
    screen.getByRole('link', { name: 'Upgrade to Pro' });
  });

  it('shows the plain link to an already-paying reader — "Upgrade to Pro" would be wrong to say', () => {
    plans.mockResolvedValue(proMonthlyPrice(500));
    user.current = { tier: 'pro' };
    render(PlanLimitLink);
    expect(screen.getByRole('link', { name: 'See your plan' }).getAttribute('href')).toBe('/my/plan');
    expect(screen.queryByText(/Upgrade to Pro/)).toBeNull();
  });

  it('defaults an unresolved reader to the free CTA rather than assuming they already paid', () => {
    plans.mockResolvedValue(proMonthlyPrice(500));
    user.current = null;
    render(PlanLimitLink);
    screen.getByRole('link', { name: 'Upgrade to Pro' });
  });

  it('honors a caller-supplied variant, so a destructive-styled host (AssistantChat) is not fought by a filled brand button', () => {
    plans.mockResolvedValue(proMonthlyPrice(500));
    user.current = { tier: 'free' };
    render(PlanLimitLink, { variant: 'outline' });
    const link = screen.getByRole('link', { name: 'Upgrade to Pro' });
    expect(link.className).not.toContain('bg-brand');
  });
});
