import { fireEvent, render, screen, waitFor, within } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { User } from '$lib/types';
import DeleteAccountButton from './DeleteAccountButton.svelte';

// The one irreversible action on the site. These cases re-assert its existing guarantees
// BEFORE its confirmation block is replaced by the shared component, then pin the new
// behaviour: the dialog survives a provider round trip, and the typed address does not.

const {
  deleteAccount,
  reauthenticatePassword,
  connectedIdentities,
  billingManageUrl,
  begin,
  expiry,
  forget,
  goto,
  invalidateAll,
  user,
} = vi.hoisted(() => ({
  deleteAccount: vi.fn(),
  reauthenticatePassword: vi.fn(),
  connectedIdentities: vi.fn(),
  billingManageUrl: vi.fn(),
  begin: vi.fn(),
  expiry: vi.fn(),
  forget: vi.fn(),
  goto: vi.fn(),
  invalidateAll: vi.fn(),
  user: { current: null as User | null },
}));

const draft = vi.hoisted(() => ({ next: null as unknown }));

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
vi.mock('$app/navigation', () => ({ goto, invalidateAll }));
vi.mock('$lib/api', () => ({
  api: { deleteAccount, reauthenticatePassword, connectedIdentities, billingManageUrl },
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

const EMAIL = 'a@b.test';

function signedIn(hasPassword: boolean): void {
  user.current = { id: 1, email: EMAIL, has_password: hasPassword } as User;
}

function dialog(): HTMLElement {
  const open = screen
    .getAllByRole('dialog', { hidden: true })
    .find((d) => (d as HTMLDialogElement).open);
  if (!open) throw new Error('no dialog is open');
  return open;
}

// jest-dom's matchers are not installed in this project, so `disabled` is read off the
// element directly.
const deleteButton = () =>
  within(dialog()).getByRole<HTMLButtonElement>('button', { name: 'Delete account permanently' });

async function openDialog(): Promise<void> {
  await fireEvent.click(screen.getByRole('button', { name: 'Delete account' }));
}

beforeEach(() => {
  draft.next = null;
  begin.mockReset();
  forget.mockReset();
  goto.mockReset();
  invalidateAll.mockReset();
  expiry.mockReset().mockReturnValue(null);
  deleteAccount.mockReset().mockResolvedValue(undefined);
  reauthenticatePassword.mockReset().mockResolvedValue('2099-01-01T00:00:00Z');
  billingManageUrl.mockReset().mockResolvedValue({ url: null });
  connectedIdentities.mockReset().mockResolvedValue({
    has_password: false,
    identities: [{ provider: 'google', status: 'active', linked_at: '', can_unlink: true }],
  });
});

describe('DeleteAccountButton — guarantees that must not move', () => {
  it('keeps the destructive action disabled until the typed address matches', async () => {
    signedIn(true);
    render(DeleteAccountButton);
    await openDialog();

    expect(deleteButton().disabled).toBe(true);
    await fireEvent.input(within(dialog()).getByPlaceholderText(EMAIL), {
      target: { value: EMAIL },
    });

    expect(deleteButton().disabled).toBe(false);
  });

  it('will not delete on a near-miss of the typed address', async () => {
    signedIn(true);
    render(DeleteAccountButton);
    await openDialog();

    await fireEvent.input(within(dialog()).getByPlaceholderText(EMAIL), {
      target: { value: 'a@b.tes' },
    });
    await fireEvent.click(deleteButton());

    expect(deleteAccount).not.toHaveBeenCalled();
  });

  it('clears the session and leaves for a public page once deletion succeeds', async () => {
    signedIn(true);
    render(DeleteAccountButton);
    await openDialog();
    await fireEvent.input(within(dialog()).getByPlaceholderText(EMAIL), {
      target: { value: EMAIL },
    });
    await fireEvent.input(within(dialog()).getByLabelText('Password'), {
      target: { value: 'hunter2' },
    });

    await fireEvent.click(deleteButton());

    // `waitFor`: deletion is a chain of four awaits, and a single event flush lands in
    // the middle of it.
    await waitFor(() => expect(goto).toHaveBeenCalledWith('/'));
    expect(reauthenticatePassword).toHaveBeenCalledWith('hunter2');
    expect(deleteAccount).toHaveBeenCalledWith(EMAIL);
    expect(invalidateAll).toHaveBeenCalled();
  });
});

describe('DeleteAccountButton — surviving the provider round trip', () => {
  it('offers the confirmation inside the dialog', async () => {
    signedIn(false);
    render(DeleteAccountButton);

    await openDialog();

    expect(
      await within(dialog()).findByRole('button', { name: 'Confirm with google' }),
    ).toBeTruthy();
  });

  it('carries no typed address across the trip, only the instruction to reopen', async () => {
    signedIn(false);
    render(DeleteAccountButton);
    await openDialog();
    await fireEvent.input(within(dialog()).getByPlaceholderText(EMAIL), {
      target: { value: EMAIL },
    });

    await fireEvent.click(await within(dialog()).findByRole('button', { name: 'Confirm with google' }));

    expect(begin).toHaveBeenCalledWith('google', '/my/security', { surface: 'delete-account' });
  });

  it('reopens on return with the barrier re-armed rather than pre-cleared', async () => {
    signedIn(false);
    draft.next = { surface: 'delete-account' };
    expiry.mockReturnValue(new Date(Date.now() + 9 * 60_000));

    render(DeleteAccountButton);

    expect(await within(dialog()).findByText(/Identity confirmed/)).toBeTruthy();
    expect(within(dialog()).getByPlaceholderText<HTMLInputElement>(EMAIL).value).toBe('');
    expect(deleteButton().disabled).toBe(true);
  });

  it('does not spend an empty password when a proof is already held', async () => {
    signedIn(false);
    draft.next = { surface: 'delete-account' };
    expiry.mockReturnValue(new Date(Date.now() + 9 * 60_000));
    render(DeleteAccountButton);
    await within(dialog()).findByText(/Identity confirmed/);

    await fireEvent.input(within(dialog()).getByPlaceholderText(EMAIL), {
      target: { value: EMAIL },
    });
    await fireEvent.click(deleteButton());

    expect(reauthenticatePassword).not.toHaveBeenCalled();
    expect(deleteAccount).toHaveBeenCalledWith(EMAIL);
  });

  it('asks again and deletes nothing when the server refuses a held proof', async () => {
    signedIn(false);
    expiry.mockReturnValue(new Date(Date.now() + 9 * 60_000));
    deleteAccount.mockRejectedValue(new StubApiError(428));
    render(DeleteAccountButton);
    await openDialog();
    await fireEvent.input(within(dialog()).getByPlaceholderText(EMAIL), {
      target: { value: EMAIL },
    });

    await fireEvent.click(deleteButton());

    expect(forget).toHaveBeenCalled();
    expect(goto).not.toHaveBeenCalled();
    expect(
      await within(dialog()).findByRole('button', { name: 'Confirm with google' }),
    ).toBeTruthy();
  });
});
