import { fireEvent, render, screen } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import Page from './+page.svelte';

vi.mock('$app/paths', () => ({ resolve: (p: string) => p, base: '', assets: '' }));
vi.mock('$app/navigation', () => ({ goto: vi.fn() }));
vi.mock('$app/state', () => ({
  page: { data: { locale: 'en' }, url: new URL('http://localhost/my/employer/join?token=abc') },
}));
vi.mock('#lib/i18n/currentLocale.svelte.js', () => ({ locale: () => 'en' }));
vi.mock('#lib/auth.svelte.js', () => ({ isAuthenticated: () => true }));
const { previewEmployerTeamInvitation, joinEmployerTeam, ApiError } = vi.hoisted(() => ({
  previewEmployerTeamInvitation: vi.fn(),
  joinEmployerTeam: vi.fn(),
  ApiError: class extends Error {
    constructor(
      public status: number,
      message: string,
    ) {
      super(message);
    }
  },
}));
vi.mock('#lib/api.js', () => ({ api: { previewEmployerTeamInvitation, joinEmployerTeam }, ApiError }));

describe('team invitation page', () => {
  beforeEach(() => vi.clearAllMocks());

  it('offers a retry when the preview could not be read, not "the link is dead"', async () => {
    previewEmployerTeamInvitation
      .mockRejectedValueOnce(new ApiError(503, 'unavailable'))
      .mockResolvedValueOnce({ company_name: 'Acme', email: 'dev@acme.test' });
    render(Page);
    expect(await screen.findByText(/Could not load this invitation/)).toBeTruthy();
    expect(screen.queryByText(/invalid or has expired/)).toBeNull();
    await fireEvent.click(screen.getByRole('button', { name: 'Try again' }));
    expect(await screen.findByRole('button', { name: 'Join the team' })).toBeTruthy();
    expect(previewEmployerTeamInvitation).toHaveBeenCalledTimes(2);
  });

  it('says the link is dead when the server confirms it', async () => {
    previewEmployerTeamInvitation.mockRejectedValue(new ApiError(404, 'not found'));
    render(Page);
    expect(await screen.findByText(/invalid or has expired/)).toBeTruthy();
    expect(screen.queryByRole('button', { name: 'Try again' })).toBeNull();
  });
});
