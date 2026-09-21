import { render, screen } from '@testing-library/svelte';
import { describe, expect, it, vi } from 'vitest';
import PlanLimitLink from './PlanLimitLink.svelte';

vi.mock('$app/paths', () => ({ resolve: (p: string) => p, base: '', assets: '' }));

// Mutable, like AccountTimezone.spec.ts's `user` — a test flips the tier before
// rendering rather than reaching for a fixed object that could only prove one branch.
const { user } = vi.hoisted(() => ({
  user: { current: null as { tier: 'free' | 'pro' | 'ultra' } | null },
}));
vi.mock('$lib/auth.svelte', () => ({ currentUser: () => user.current }));

describe('PlanLimitLink', () => {
  it('shows the priced Upgrade CTA to a free reader — the one this refusal can convert', () => {
    user.current = { tier: 'free' };
    render(PlanLimitLink);
    const link = screen.getByRole('link', { name: 'Upgrade to Pro — $5/mo' });
    expect(link.getAttribute('href')).toBe('/my/plan');
  });

  it('shows the plain link to an already-paying reader — "Upgrade to Pro" would be wrong to say', () => {
    user.current = { tier: 'pro' };
    render(PlanLimitLink);
    expect(screen.getByRole('link', { name: 'See your plan' }).getAttribute('href')).toBe('/my/plan');
    expect(screen.queryByText(/Upgrade to Pro/)).toBeNull();
  });

  it('defaults an unresolved reader to the free CTA rather than assuming they already paid', () => {
    user.current = null;
    render(PlanLimitLink);
    expect(screen.queryByRole('link', { name: 'Upgrade to Pro — $5/mo' })).not.toBeNull();
  });
});
