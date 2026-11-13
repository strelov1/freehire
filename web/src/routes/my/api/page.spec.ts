import { render, screen } from '@testing-library/svelte';
import { describe, expect, it, vi } from 'vitest';
import Page from './+page.svelte';

vi.mock('$app/paths', () => ({ resolve: (p: string) => p, base: '', assets: '' }));
vi.mock('#lib/i18n/currentLocale.svelte', () => ({ locale: () => 'en' }));
vi.mock('#lib/auth.svelte.js', () => ({ isAuthenticated: () => true }));
const { getApiUsage } = vi.hoisted(() => ({ getApiUsage: vi.fn() }));
vi.mock('#lib/api.js', () => ({ api: { getApiUsage } }));

describe('API usage page', () => {
  it('shows unknown figures, not zeros, when the first read fails', async () => {
    getApiUsage.mockRejectedValue(new Error('down'));
    render(Page);
    expect(await screen.findByText('Could not load usage.')).toBeTruthy();
    expect(screen.getAllByText('—')).toHaveLength(3);
    expect(screen.queryByText('0')).toBeNull();
  });

  it('shows the counts once a read succeeds', async () => {
    const today = new Date().toISOString().slice(0, 10);
    getApiUsage.mockResolvedValue([{ day: today, requests: 12, errors: 2 }]);
    render(Page);
    expect((await screen.findAllByText('12')).length).toBeGreaterThan(0);
    expect(screen.getByText('2')).toBeTruthy();
  });
});
