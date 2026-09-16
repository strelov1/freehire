import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { User } from '$lib/types';
import SecurityPage from './+page.svelte';

// `keyFor` here knew 401 and 400 and nothing else, so a 428 — the one refusal this surface
// can actually meet — came out as "Something went wrong. Please try again.", which is both
// untrue and unactionable.

const { changePassword, reauthenticatePassword, logoutEverywhere, user } = vi.hoisted(() => ({
  changePassword: vi.fn(),
  reauthenticatePassword: vi.fn(),
  logoutEverywhere: vi.fn(),
  user: { current: null as User | null },
}));

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
vi.mock('$app/navigation', () => ({ goto: vi.fn(), invalidateAll: vi.fn() }));
vi.mock('$lib/api', () => ({
  api: { changePassword, reauthenticatePassword, logoutEverywhere },
  ApiError: StubApiError,
}));
vi.mock('$lib/auth.svelte', () => ({ currentUser: () => user.current }));
// The delete-account dialog lives on this page too and has its own spec; it is stubbed out
// so these cases exercise only the password form.
vi.mock('$lib/components/DeleteAccountButton.svelte', async () => ({
  default: (await import('./DeleteAccountButtonStub.svelte')).default,
}));

beforeEach(() => {
  user.current = { id: 1, email: 'a@b.test', has_password: true } as User;
  changePassword.mockReset().mockResolvedValue(undefined);
  reauthenticatePassword.mockReset().mockResolvedValue('2099-01-01T00:00:00Z');
  logoutEverywhere.mockReset().mockResolvedValue(undefined);
});

async function fillAndSubmit(): Promise<void> {
  await fireEvent.input(screen.getByLabelText('Current password'), {
    target: { value: 'hunter2' },
  });
  await fireEvent.input(screen.getByLabelText('New password'), {
    target: { value: 'correct horse battery' },
  });
  await fireEvent.input(screen.getByLabelText('Repeat new password'), {
    target: { value: 'correct horse battery' },
  });
  await fireEvent.click(screen.getByRole('button', { name: 'Change password' }));
}

describe('security page — password change', () => {
  it('names the real remedy when the session cannot prove recent control', async () => {
    // A 428 here cannot mean "you have not confirmed": the current password IS the
    // confirmation and it was just accepted. It means the proof would not bind to this
    // session, and the only cure is signing in again.
    changePassword.mockRejectedValue(new StubApiError(428));
    render(SecurityPage);

    await fillAndSubmit();

    await waitFor(() => expect(screen.getByText(/sign out and sign back in/i)).toBeTruthy());
    expect(screen.queryByText('Something went wrong. Please try again.')).toBeNull();
  });

  it('still distinguishes a wrong current password', async () => {
    reauthenticatePassword.mockRejectedValue(new StubApiError(401));
    render(SecurityPage);

    await fillAndSubmit();

    await waitFor(() =>
      expect(screen.getByText('That current password is not right.')).toBeTruthy(),
    );
    expect(changePassword).not.toHaveBeenCalled();
  });

  it('confirms with the current password before changing it', async () => {
    render(SecurityPage);

    await fillAndSubmit();

    await waitFor(() => expect(changePassword).toHaveBeenCalled());
    expect(reauthenticatePassword).toHaveBeenCalledWith('hunter2');
    expect(changePassword).toHaveBeenCalledWith('hunter2', 'correct horse battery');
  });
});
