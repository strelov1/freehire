import { render, screen } from '@testing-library/svelte';
import { describe, expect, it, vi } from 'vitest';
import BuddiesLandingView from './BuddiesLandingView.svelte';
import { BUDDIES_FAQ } from '#lib/buddiesFaq.js';

vi.mock('$app/paths', () => ({ resolve: (p: string) => p }));
vi.stubGlobal('matchMedia', (q: string) => ({ matches: q.includes('reduce'), media: q }));
vi.stubGlobal('IntersectionObserver', class { observe() {} disconnect() {} });

describe('BuddiesLandingView', () => {
  it('leads with the headline and sends every join button to /my/buddies', () => {
    render(BuddiesLandingView);
    expect(screen.getByRole('heading', { level: 1 }).textContent).toContain('Find people interviewing where you are.');
    const joins = screen.getAllByRole('link', { name: 'Join interview buddies' });
    expect(joins.length).toBeGreaterThan(0);
    for (const a of joins) expect(a.getAttribute('href')).toBe('/my/buddies');
  });

  it('renders every FAQ question', () => {
    render(BuddiesLandingView);
    for (const f of BUDDIES_FAQ) expect(screen.getByText(f.question)).toBeTruthy();
  });
});
