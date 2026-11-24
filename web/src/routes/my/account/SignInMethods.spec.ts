import { fireEvent, render, screen } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { User } from '#lib/types.js';
import SignInMethods from './SignInMethods.svelte';

const { connectedIdentities, reauthenticatePassword, beginProviderLink, replaceState, pageState, user } = vi.hoisted(
  () => ({
    connectedIdentities: vi.fn(),
    reauthenticatePassword: vi.fn(),
    beginProviderLink: vi.fn(),
    replaceState: vi.fn(),
    pageState: { url: new URL('http://localhost/my/account') },
    user: { current: null as User | null },
  }),
);

vi.mock('$app/state', () => ({
  page: {
    data: {},
    state: {},
    get url() {
      return pageState.url;
    },
  },
}));
vi.mock('$app/paths', () => ({ resolve: (p: string) => p, base: '', assets: '' }));
vi.mock('$app/navigation', () => ({ replaceState, goto: vi.fn(), invalidateAll: vi.fn() }));
vi.mock('#lib/api.js', () => ({
  api: { connectedIdentities, reauthenticatePassword },
  ApiError: class extends Error {},
}));
vi.mock('#lib/auth.svelte.js', () => ({ currentUser: () => user.current }));
vi.mock('#lib/oauthProviders.js', () => ({
  PROVIDER_LABELS: { google: 'Google', github: 'GitHub', linkedin: 'LinkedIn', apple: 'Apple' },
  loadOAuthProviders: vi.fn().mockResolvedValue(['apple', 'github', 'google']),
}));
vi.mock('#lib/recentAuth.js', async (importOriginal) => ({
  ...(await importOriginal<typeof import('#lib/recentAuth.js')>()),
  beginProviderLink,
}));

beforeEach(() => {
  user.current = { id: 1, email: 'a@b.test', has_password: true } as User;
  pageState.url = new URL('http://localhost/my/account');
  connectedIdentities.mockReset().mockResolvedValue({
    has_password: true,
    identities: [{ provider: 'google', linked_at: '2026-01-02T00:00:00Z', status: 'active', can_unlink: true }],
  });
  reauthenticatePassword.mockReset().mockResolvedValue('2099-01-01T00:00:00Z');
  beginProviderLink.mockReset().mockResolvedValue(undefined);
  replaceState.mockReset();
});

describe('SignInMethods', () => {
  it('lists connected providers and offers the others, never Apple on the web', async () => {
    render(SignInMethods);
    expect(await screen.findByText('GitHub')).toBeTruthy();
    expect(screen.getByText('Google')).toBeTruthy();
    expect(screen.queryByText('Apple')).toBeNull();
    // Only GitHub is not connected yet, so there is exactly one Connect button.
    expect(screen.getAllByRole('button', { name: 'Connect' })).toHaveLength(1);
  });

  it('confirms with the password, then leaves for the provider', async () => {
    render(SignInMethods);
    await fireEvent.click(await screen.findByRole('button', { name: 'Connect' }));
    await fireEvent.input(screen.getByLabelText('Password'), { target: { value: 'hunter2' } });
    await fireEvent.click(screen.getByRole('button', { name: 'Continue to GitHub' }));
    await vi.waitFor(() => expect(beginProviderLink).toHaveBeenCalledWith('github'));
    expect(reauthenticatePassword).toHaveBeenCalledWith('hunter2');
  });

  it('does not leave without a password', async () => {
    render(SignInMethods);
    await fireEvent.click(await screen.findByRole('button', { name: 'Connect' }));
    await fireEvent.click(screen.getByRole('button', { name: 'Continue to GitHub' }));
    expect(beginProviderLink).not.toHaveBeenCalled();
  });

  it('announces a completed link once and clears it from the address', async () => {
    pageState.url = new URL('http://localhost/my/account?linked=github');
    render(SignInMethods);
    expect(await screen.findByText('GitHub is connected. You can now sign in with it.')).toBeTruthy();
    expect(replaceState).toHaveBeenCalledOnce();
    expect(replaceState.mock.calls[0]?.[0]).toBe('/my/account');
  });

  it('words a refusal from the server', async () => {
    pageState.url = new URL('http://localhost/my/account?link_error=identity_in_use');
    render(SignInMethods);
    expect(await screen.findByText(/already signs in to a different freehire account/)).toBeTruthy();
  });
});
