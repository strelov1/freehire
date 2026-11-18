import { render, screen } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('$app/state', () => ({
  page: { data: { locale: 'en' }, url: new URL('http://localhost/my/getting-started') },
}));
vi.mock('$app/paths', () => ({ resolve: (p: string) => p, base: '', assets: '' }));

const h = vi.hoisted(() => ({ getSurvey: vi.fn() }));
vi.mock('#lib/api.js', async (importOriginal) => ({
  ...(await importOriginal<typeof import('#lib/api.js')>()),
  api: { getSurvey: h.getSurvey },
}));

import Page from './+page.svelte';

beforeEach(() => {
  h.getSurvey.mockReset();
});

describe('/my/getting-started', () => {
  it('introduces the AI connector, CV tailoring and the extension', () => {
    h.getSurvey.mockResolvedValue({});
    render(Page);
    expect(screen.getByRole('heading', { name: 'Search jobs from your AI assistant with MCP' })).toBeTruthy();
    expect(screen.getByRole('heading', { name: /Tailoring CV/ })).toBeTruthy();
    expect(screen.getByRole('heading', { name: 'Fill any application from the side panel' })).toBeTruthy();
  });

  it("shows what helps with the account's stored answers", async () => {
    h.getSurvey.mockResolvedValue({ biggest_challenges: ['visa_sponsorship'] });
    render(Page);
    expect(await screen.findByRole('heading', { name: 'What helps' })).toBeTruthy();
    expect(screen.getByRole('link', { name: /Jobs that sponsor a visa/ })).toBeTruthy();
  });

  it('shows no tips for an account that stated nothing', async () => {
    h.getSurvey.mockResolvedValue({});
    render(Page);
    await Promise.resolve();
    expect(screen.queryByRole('heading', { name: 'What helps' })).toBeNull();
  });
});
