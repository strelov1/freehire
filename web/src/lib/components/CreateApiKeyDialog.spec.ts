import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { CreatedApiKey, User } from '$lib/types';
import CreateApiKeyDialog from './CreateApiKeyDialog.svelte';

// Creating a key is the action the whole change started from: a member reported being
// unable to do it at all. These cases pin the two halves of that — the confirmation is
// reachable from inside the dialog, and leaving for a provider does not cost the member
// what they had already typed.

const { createApiKey, reauthenticatePassword, connectedIdentities, begin, expiry, forget, user } =
  vi.hoisted(() => ({
    createApiKey: vi.fn(),
    reauthenticatePassword: vi.fn(),
    connectedIdentities: vi.fn(),
    begin: vi.fn(),
    expiry: vi.fn(),
    forget: vi.fn(),
    user: { current: null as User | null },
  }));

const draft = vi.hoisted(() => ({ next: null as unknown }));

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
vi.mock('$lib/api', () => ({
  api: { createApiKey, reauthenticatePassword, connectedIdentities },
  ApiError: StubApiError,
}));
vi.mock('$lib/auth.svelte', () => ({ currentUser: () => user.current }));
vi.mock('$lib/recentAuth', () => ({
  beginProviderReauthentication: begin,
  recentAuthExpiry: expiry,
  forgetRecentAuthExpiry: forget,
  consumeReauthDraft: () => {
    const d = draft.next;
    draft.next = null;
    return d;
  },
}));

const created: CreatedApiKey = {
  id: 7,
  name: 'CI bot',
  token_prefix: 'fh_ab12',
  created_at: '2026-09-15T00:00:00Z',
  last_used_at: null,
  expires_at: null,
  // Deliberately not shaped like a credential: a realistic-looking fixture trips the
  // gitleaks pre-commit hook, and a scanner taught to ignore this file would stop guarding
  // the real thing.
  token: 'plaintext-token-shown-once',
};

function signedIn(hasPassword: boolean): void {
  user.current = { id: 1, email: 'a@b.test', has_password: hasPassword } as User;
}

beforeEach(() => {
  draft.next = null;
  begin.mockReset();
  forget.mockReset();
  expiry.mockReset().mockReturnValue(null);
  createApiKey.mockReset().mockResolvedValue(created);
  reauthenticatePassword.mockReset().mockResolvedValue('2099-01-01T00:00:00Z');
  connectedIdentities.mockReset().mockResolvedValue({
    has_password: false,
    identities: [{ provider: 'google', status: 'active', linked_at: '', can_unlink: true }],
  });
});

describe('CreateApiKeyDialog', () => {
  it('confirms with the password and only then creates the key', async () => {
    signedIn(true);
    const onCreated = vi.fn();
    render(CreateApiKeyDialog, { props: { open: true, onCreated } });

    await fireEvent.input(screen.getByLabelText('Name'), { target: { value: 'CI bot' } });
    await fireEvent.input(await screen.findByLabelText('Password'), {
      target: { value: 'hunter2' },
    });
    await fireEvent.click(screen.getByRole('button', { name: 'Create key' }));

    // `waitFor`: submitting is a chain of awaits and a single event flush lands mid-way.
    await waitFor(() => expect(onCreated).toHaveBeenCalledWith(created));
    expect(reauthenticatePassword).toHaveBeenCalledWith('hunter2');
    expect(createApiKey).toHaveBeenCalledWith('CI bot', undefined);
  });

  it('does not spend an empty password when a proof is already held', async () => {
    // The member confirmed through a provider; no password was ever asked for. Sending ''
    // would 401 and be rendered to a confirmed member as "wrong password".
    signedIn(false);
    expiry.mockReturnValue(new Date(Date.now() + 9 * 60_000));
    render(CreateApiKeyDialog, { props: { open: true, onCreated: vi.fn() } });

    await fireEvent.input(screen.getByLabelText('Name'), { target: { value: 'CI bot' } });
    await fireEvent.click(screen.getByRole('button', { name: 'Create key' }));

    expect(reauthenticatePassword).not.toHaveBeenCalled();
    expect(createApiKey).toHaveBeenCalledOnce();
  });

  it('carries the name and expiry the member had typed when they leave for a provider', async () => {
    signedIn(false);
    render(CreateApiKeyDialog, { props: { open: true, onCreated: vi.fn() } });
    await fireEvent.input(screen.getByLabelText('Name'), { target: { value: 'CI bot' } });

    await fireEvent.click(await screen.findByRole('button', { name: 'Confirm with google' }));

    expect(begin).toHaveBeenCalledWith('google', '/my/api-keys', {
      surface: 'create-api-key',
      name: 'CI bot',
      days: 0,
    });
  });

  it('reopens itself with the typed name restored when the member returns', async () => {
    signedIn(false);
    draft.next = { surface: 'create-api-key', name: 'CI bot', days: 30 };
    expiry.mockReturnValue(new Date(Date.now() + 9 * 60_000));

    render(CreateApiKeyDialog, { props: { open: false, onCreated: vi.fn() } });

    expect((await screen.findByLabelText<HTMLInputElement>('Name')).value).toBe('CI bot');
    expect(await screen.findByText(/Identity confirmed/)).toBeTruthy();
  });

  it('does not create the key merely because the member came back confirmed', async () => {
    signedIn(false);
    draft.next = { surface: 'create-api-key', name: 'CI bot', days: 30 };
    expiry.mockReturnValue(new Date(Date.now() + 9 * 60_000));

    render(CreateApiKeyDialog, { props: { open: false, onCreated: vi.fn() } });
    await screen.findByLabelText('Name');

    expect(createApiKey).not.toHaveBeenCalled();
  });

  it('asks for confirmation again when the server refuses a held proof', async () => {
    signedIn(false);
    expiry.mockReturnValue(new Date(Date.now() + 9 * 60_000));
    createApiKey.mockRejectedValue(new StubApiError(428));
    render(CreateApiKeyDialog, { props: { open: true, onCreated: vi.fn() } });

    await fireEvent.input(screen.getByLabelText('Name'), { target: { value: 'CI bot' } });
    await fireEvent.click(screen.getByRole('button', { name: 'Create key' }));

    expect(forget).toHaveBeenCalled();
    expect(await screen.findByRole('button', { name: 'Confirm with google' })).toBeTruthy();
  });

  it('refuses to submit without a name', async () => {
    signedIn(true);
    render(CreateApiKeyDialog, { props: { open: true, onCreated: vi.fn() } });

    await fireEvent.click(screen.getByRole('button', { name: 'Create key' }));

    expect(createApiKey).not.toHaveBeenCalled();
  });
});
