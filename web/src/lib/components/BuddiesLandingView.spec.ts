import { render, screen } from '@testing-library/svelte';
import { describe, expect, it, vi } from 'vitest';
import BuddiesLandingView from './BuddiesLandingView.svelte';
import { BUDDIES_FAQ } from '#lib/buddiesFaq.js';

vi.mock('$app/paths', () => ({ resolve: (p: string) => p }));
vi.stubGlobal('matchMedia', (q: string) => ({
  matches: q.includes('reduce'),
  media: q,
  addEventListener() {},
  removeEventListener() {},
}));
vi.stubGlobal('IntersectionObserver', class { observe() {} disconnect() {} });

describe('BuddiesLandingView', () => {
  it('leads with the headline and sends every join button to /buddies', () => {
    render(BuddiesLandingView);
    expect(screen.getByRole('heading', { level: 1 }).textContent).toContain('Find people interviewing where you are.');
    const joins = screen.getAllByRole('link', { name: 'Join interview buddies' });
    expect(joins.length).toBeGreaterThan(0);
    for (const a of joins) expect(a.getAttribute('href')).toBe('/buddies');
  });

  it('joins in place when given onJoin, for a signed-in non-member at /buddies', async () => {
    const onJoin = vi.fn();
    render(BuddiesLandingView, { onJoin });
    expect(screen.queryByRole('link', { name: 'Join interview buddies' })).toBeNull();
    const joins = screen.getAllByRole('button', { name: 'Join interview buddies' });
    expect(joins.length).toBeGreaterThan(0);
    for (const b of joins) b.click();
    expect(onJoin).toHaveBeenCalledTimes(joins.length);
  });

  it('renders every FAQ question', () => {
    render(BuddiesLandingView);
    for (const f of BUDDIES_FAQ) expect(screen.getByText(f.question)).toBeTruthy();
  });
  // Screen readers navigate by the outline; the how-it-works steps (h3) need an h2 above them.
  it('never skips a heading level', () => {
    const { container } = render(BuddiesLandingView);
    let previous = 1;
    for (const h of container.querySelectorAll('h1, h2, h3, h4')) {
      const level = Number(h.tagName[1]);
      expect(level - previous, `${h.tagName} "${h.textContent?.trim()}"`).toBeLessThanOrEqual(1);
      previous = level;
    }
    expect(screen.getByRole('heading', { level: 2, name: 'From a handle to a conversation.' })).toBeTruthy();
  });
});
