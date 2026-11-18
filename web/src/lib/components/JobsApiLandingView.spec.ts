import { render, screen } from '@testing-library/svelte';
import { describe, expect, it, vi } from 'vitest';
import JobsApiLandingView from './JobsApiLandingView.svelte';
import type { PlansMatrix } from '#lib/types.js';

vi.mock('$app/paths', () => ({ resolve: (p: string) => p, base: '', assets: '' }));
vi.mock('#lib/auth.svelte.js', () => ({ isAuthenticated: () => false }));
vi.mock('#lib/signin.js', () => ({ promptSignIn: vi.fn(), promptLogin: vi.fn() }));
vi.mock('#lib/api.js', () => ({ api: { billingCheckout: vi.fn() } }));

const plans: PlansMatrix = {
  features: [],
  enforced: [],
  prices: [
    { id: 'price_api', tier: 'api', interval: 'month', amount_cents: 2900, currency: 'usd', default: true },
    { id: 'price_data', tier: 'api_data', interval: 'month', amount_cents: 9900, currency: 'usd', default: true },
  ],
};

describe('JobsApiLandingView', () => {
  it('quotes the live catalogue figures and both plans', () => {
    render(JobsApiLandingView, { props: { stats: { jobs: 3405639, companies: 481352, atsPlatforms: 124 }, plans } });
    expect(screen.getByText('3.4M')).toBeTruthy();
    expect(screen.getByText('481K')).toBeTruthy();
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

  it('shows the largest sources linked to their searches, and a link to all of them', () => {
    render(JobsApiLandingView, {
      props: {
        stats: { jobs: null, companies: null, atsPlatforms: null },
        plans: null,
        sources: { total: 214, top: [{ source: 'greenhouse', kind: 'ats', jobs: 412000 }] },
      },
    });
    expect(screen.getByRole('heading', { name: '214 sources, one schema.' })).toBeTruthy();
    expect(screen.getByRole('link', { name: /Greenhouse/ }).getAttribute('href')).toBe('/jobs?source=greenhouse');
    expect(screen.getByText(/ATS · 412K/)).toBeTruthy();
    expect(screen.getByRole('link', { name: 'All sources' }).getAttribute('href')).toBe('/sources');
  });

  it('drops the source strip when the catalogue could not be read', () => {
    render(JobsApiLandingView, {
      props: { stats: { jobs: null, companies: null, atsPlatforms: null }, plans: null, sources: null },
    });
    expect(screen.queryByRole('link', { name: 'All sources' })).toBeNull();
  });

  it('compares the flat plan with metered APIs and a home-built crawler', () => {
    render(JobsApiLandingView, { props: { stats: { jobs: null, companies: null, atsPlatforms: null }, plans: null } });
    expect(screen.getByRole('columnheader', { name: 'Record-metered API' })).toBeTruthy();
    expect(screen.getByRole('columnheader', { name: 'Your own crawler' })).toBeTruthy();
    expect(screen.getByRole('rowheader', { name: 'What you pay for' })).toBeTruthy();
  });
});
