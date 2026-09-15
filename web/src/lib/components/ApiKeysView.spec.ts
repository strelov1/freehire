import { fireEvent, render, screen, within } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { ApiKey, User } from '$lib/types';
import ApiKeysView from './ApiKeysView.svelte';

// The bug this whole change started from lives here: the revoke dialog demanded a password
// whose only input was in the create form on the page BEHIND the modal backdrop, so the
// action could not be completed at all. These cases hold that shut.

const {
  listApiKeys,
  revokeApiKey,
  createApiKey,
  reauthenticatePassword,
  connectedIdentities,
  begin,
  expiry,
  forget,
  user,
} = vi.hoisted(() => ({
  listApiKeys: vi.fn(),
  revokeApiKey: vi.fn(),
  createApiKey: vi.fn(),
  reauthenticatePassword: vi.fn(),
  connectedIdentities: vi.fn(),
  begin: vi.fn(),
  expiry: vi.fn(),
  forget: vi.fn(),
  user: { current: null as User | null },
}));

// Hoisted with the mocks: `vi.mock` is lifted above ordinary declarations, so a class
// declared normally here is still in its temporal dead zone when the factory runs.
const { StubApiError } = vi.hoisted(() => ({
  StubApiError: class StubApiError extends Error {
    constructor(
      public status: number,
      message = 'failed',
    ) {
      super(message);
    }
  },
}));

vi.mock('$app/state', () => ({ page: { data: {}, url: new URL('http://localhost/') } }));
vi.mock('$app/paths', () => ({ resolve: (p: string) => p, base: '', assets: '' }));
vi.mock('$lib/api', () => ({
  api: { listApiKeys, revokeApiKey, createApiKey, reauthenticatePassword, connectedIdentities },
  ApiError: StubApiError,
}));
vi.mock('$lib/auth.svelte', () => ({
  currentUser: () => user.current,
  isAuthenticated: () => user.current !== null,
}));
vi.mock('$lib/recentAuth', () => ({
  beginProviderReauthentication: begin,
  recentAuthExpiry: expiry,
  forgetRecentAuthExpiry: forget,
  consumeReauthDraft: (surface: string) => {
    const d = draft.next as { surface?: string } | null;
    if (!d || d.surface !== surface) return null;
    draft.next = null;
    return d;
  },
}));

const draft = vi.hoisted(() => ({ next: null as unknown }));

const key: ApiKey = {
  id: 7,
  name: 'CI bot',
  token_prefix: 'fh_ab12',
  created_at: '2026-09-15T00:00:00Z',
  last_used_at: null,
  expires_at: null,
};

function signedIn(hasPassword: boolean): void {
  user.current = { id: 1, email: 'a@b.test', has_password: hasPassword } as User;
}

/** Whichever dialog is currently open, as the member sees it — everything outside it is
 *  behind a modal backdrop and unreachable, which is the whole point of these cases. */
function openDialog(): HTMLElement {
  const dialogs = screen.getAllByRole('dialog', { hidden: true });
  const open = dialogs.find((d) => (d as HTMLDialogElement).open);
  if (!open) throw new Error('no dialog is open');
  return open;
}

beforeEach(() => {
  draft.next = null;
  begin.mockReset();
  forget.mockReset();
  expiry.mockReset().mockReturnValue(null);
  listApiKeys.mockReset().mockResolvedValue([key]);
  revokeApiKey.mockReset().mockResolvedValue(undefined);
  createApiKey.mockReset();
  reauthenticatePassword.mockReset().mockResolvedValue('2099-01-01T00:00:00Z');
  connectedIdentities.mockReset().mockResolvedValue({
    has_password: false,
    identities: [{ provider: 'google', status: 'active', linked_at: '', can_unlink: true }],
  });
});

describe('ApiKeysView — revoking', () => {
  it('puts the password input inside the revoke dialog, not on the page behind it', async () => {
    signedIn(true);
    render(ApiKeysView);
    await screen.findByText('CI bot');

    await fireEvent.click(screen.getByRole('button', { name: 'Revoke' }));

    expect(within(openDialog()).getByLabelText('Password')).toBeTruthy();
  });

  it('completes a revocation using only controls inside the dialog', async () => {
    signedIn(true);
    render(ApiKeysView);
    await screen.findByText('CI bot');
    await fireEvent.click(screen.getByRole('button', { name: 'Revoke' }));
    const dialog = openDialog();

    await fireEvent.input(within(dialog).getByLabelText('Password'), {
      target: { value: 'hunter2' },
    });
    await fireEvent.click(within(dialog).getByRole('button', { name: 'Revoke' }));

    expect(reauthenticatePassword).toHaveBeenCalledWith('hunter2');
    expect(revokeApiKey).toHaveBeenCalledWith(7);
  });

  it('offers a provider confirmation inside the dialog to an account with no password', async () => {
    signedIn(false);
    render(ApiKeysView);
    await screen.findByText('CI bot');

    await fireEvent.click(screen.getByRole('button', { name: 'Revoke' }));

    expect(
      await within(openDialog()).findByRole('button', { name: 'Confirm with google' }),
    ).toBeTruthy();
  });

  it('carries which key was being revoked across a provider round trip', async () => {
    signedIn(false);
    render(ApiKeysView);
    await screen.findByText('CI bot');
    await fireEvent.click(screen.getByRole('button', { name: 'Revoke' }));

    await fireEvent.click(
      await within(openDialog()).findByRole('button', { name: 'Confirm with google' }),
    );

    expect(begin).toHaveBeenCalledWith('google', '/my/api-keys', {
      surface: 'revoke-api-key',
      keyId: 7,
    });
  });

  it('reopens the revoke dialog for the same key when the member returns', async () => {
    // Otherwise they come back to a closed dialog and have to find the key again — the
    // silent loss this transport exists to prevent, one surface short.
    signedIn(false);
    draft.next = { surface: 'revoke-api-key', keyId: 7 };
    expiry.mockReturnValue(new Date(Date.now() + 9 * 60_000));

    render(ApiKeysView);

    expect(await screen.findByText('Revoke "CI bot"?')).toBeTruthy();
    expect(revokeApiKey).not.toHaveBeenCalled();
  });

  it('keeps the key listed when the server refuses a held proof', async () => {
    signedIn(false);
    expiry.mockReturnValue(new Date(Date.now() + 9 * 60_000));
    revokeApiKey.mockRejectedValue(new StubApiError(428));
    render(ApiKeysView);
    await screen.findByText('CI bot');
    await fireEvent.click(screen.getByRole('button', { name: 'Revoke' }));

    await fireEvent.click(within(openDialog()).getByRole('button', { name: 'Revoke' }));

    expect(forget).toHaveBeenCalled();
    expect(screen.getByText('CI bot')).toBeTruthy();
  });
});

describe('ApiKeysView — creating', () => {
  it('opens the creation dialog from the page rather than holding a form open', async () => {
    signedIn(true);
    render(ApiKeysView);
    await screen.findByText('CI bot');

    // Nothing to type into until the member says they want to create one.
    expect(screen.queryByLabelText('Name')).toBeNull();
    await fireEvent.click(screen.getByRole('button', { name: 'Create key' }));

    expect(await screen.findByLabelText('Name')).toBeTruthy();
  });

  it('shows the plaintext token once, after a key is created', async () => {
    signedIn(true);
    createApiKey.mockResolvedValue({ ...key, id: 8, name: 'deploy', token: 'plaintext-token-shown-once' });
    render(ApiKeysView);
    await screen.findByText('CI bot');
    await fireEvent.click(screen.getByRole('button', { name: 'Create key' }));

    await fireEvent.input(await screen.findByLabelText('Name'), { target: { value: 'deploy' } });
    await fireEvent.input(await screen.findByLabelText('Password'), {
      target: { value: 'hunter2' },
    });
    // Scoped to the dialog: the page header carries a "Create key" button too, and it is
    // the one that OPENED this dialog.
    await fireEvent.click(within(openDialog()).getByRole('button', { name: 'Create key' }));

    expect(await screen.findByText('plaintext-token-shown-once')).toBeTruthy();
  });
});
