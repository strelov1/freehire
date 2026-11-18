import { render, screen } from '@testing-library/svelte';
import { describe, expect, it, vi } from 'vitest';
import ApiPlanPanel from './ApiPlanPanel.svelte';

vi.mock('$app/paths', () => ({ resolve: (p: string) => p, base: '', assets: '' }));
vi.mock('#lib/i18n/currentLocale.svelte', () => ({ locale: () => 'en' }));
vi.mock('#lib/auth.svelte.js', () => ({ isAuthenticated: () => true }));
const { getApiPlan } = vi.hoisted(() => ({ getApiPlan: vi.fn() }));
vi.mock('#lib/api.js', () => ({ api: { getApiPlan } }));

describe('ApiPlanPanel', () => {
  it('names the product and its expiry for a subscriber', async () => {
    getApiPlan.mockResolvedValue({ tier: 'api', until: '2026-11-19T12:00:00Z' });
    render(ApiPlanPanel);
    expect(await screen.findByText('API plan active until November 19, 2026.')).toBeTruthy();
  });

  it('drops the date when the server sent none it can read', async () => {
    getApiPlan.mockResolvedValue({ tier: 'api_data', until: 'not-a-date' });
    render(ApiPlanPanel);
    expect(await screen.findByText('Data plan active.')).toBeTruthy();
  });

  it('points an account without the product to the plans', async () => {
    getApiPlan.mockResolvedValue({ tier: '' });
    render(ApiPlanPanel);
    const link = await screen.findByRole('link', { name: 'See API plans' });
    expect(link.getAttribute('href')).toBe('/pricing#api');
  });
});
