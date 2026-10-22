import { render, screen } from '@testing-library/svelte';
import { describe, expect, it, vi } from 'vitest';
import JobsApiLandingView from './JobsApiLandingView.svelte';
import type { PlansMatrix } from '#lib/types.js';

vi.mock('$app/paths', () => ({ resolve: (p: string) => p, base: '', assets: '' }));
vi.mock('#lib/auth.svelte.js', () => ({ isAuthenticated: () => false }));
vi.mock('#lib/signin.js', () => ({ promptSignIn: vi.fn() }));
vi.mock('#lib/api.js', () => ({ api: { billingCheckout: vi.fn() } }));

const plans: PlansMatrix = {
  features: [],
  enforced: [],
  api_limits: { anonymous_per_minute: 600, anonymous_per_day: 0, api_per_minute: 1200, data_per_minute: 6000 },
  prices: [
    { id: 'price_api', tier: 'api', interval: 'month', amount_cents: 2900, currency: 'usd', default: true },
    { id: 'price_data', tier: 'api_data', interval: 'month', amount_cents: 9900, currency: 'usd', default: true },
  ],
};

describe('JobsApiLandingView', () => {
  it('quotes the live catalogue figures and both plans', () => {
    render(JobsApiLandingView, { props: { stats: { jobs: 3405639, companies: 481352, atsPlatforms: 124 }, plans } });
    expect(screen.getByText('3.4M')).toBeTruthy();
    expect(screen.getByText('481.4K')).toBeTruthy();
    expect(screen.getByText('124')).toBeTruthy();
    expect(screen.getByRole('heading', { name: 'API', level: 3 })).toBeTruthy();
    expect(screen.getByRole('heading', { name: 'Data', level: 3 })).toBeTruthy();
  });

  it('renders the pitch without figures or plans rather than guessing them', () => {
    render(JobsApiLandingView, { props: { stats: { jobs: null, companies: null, atsPlatforms: null }, plans: null } });
    expect(screen.getByRole('heading', { level: 1 })).toBeTruthy();
    expect(screen.queryByText(/open postings, de-duplicated/)).toBeNull();
    expect(screen.getByRole('link', { name: 'pricing page' }).getAttribute('href')).toBe('/pricing');
  });
});
