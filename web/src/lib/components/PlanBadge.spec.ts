import { render, screen } from '@testing-library/svelte';
import { describe, expect, it, vi } from 'vitest';
import PlanBadge from './PlanBadge.svelte';

vi.mock('$app/paths', () => ({ resolve: (path: string) => path }));
vi.mock('$app/state', () => ({ page: { data: {}, url: new URL('http://localhost/') } }));

describe('PlanBadge', () => {
  it('sends a known Free account to the pricing benefits', () => {
    render(PlanBadge, { tier: 'free' });
    const link = screen.getByRole('link', { name: /free plan/i });
    expect(link.getAttribute('href')).toBe('/pricing');
    expect(link.textContent).toContain('Free');
  });

  it.each([
    ['pro', 'Pro'],
    ['ultra', 'Ultra'],
  ] as const)('names the real %s tier and links to the account plan', (tier, label) => {
    render(PlanBadge, { tier });
    const link = screen.getByRole('link', { name: new RegExp(`${label} plan`, 'i') });
    expect(link.getAttribute('href')).toBe('/my/plan');
    expect(link.textContent).toContain(label);
    expect(screen.queryByText('Free')).toBeNull();
  });

  it.each([[null], [undefined], ['']])('renders nothing for an unknown tier (%s)', (tier) => {
    const { container } = render(PlanBadge, { tier: tier as never });
    expect(container.querySelector('a')).toBeNull();
    expect(container.textContent?.trim()).toBe('');
  });
});
