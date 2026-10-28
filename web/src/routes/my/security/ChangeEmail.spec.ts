import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { User } from '#lib/types.js';
import ChangeEmail from './ChangeEmail.svelte';

const { requestEmailChange, confirmEmailChange, reauthenticatePassword, invalidateAll, user } =
  vi.hoisted(() => ({
    requestEmailChange: vi.fn(),
    confirmEmailChange: vi.fn(),
    reauthenticatePassword: vi.fn(),
    invalidateAll: vi.fn(),
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
vi.mock('$app/navigation', () => ({ goto: vi.fn(), invalidateAll }));
vi.mock('#lib/api.js', () => ({
  api: { requestEmailChange, confirmEmailChange, reauthenticatePassword },
  ApiError: StubApiError,
}));
vi.mock('#lib/auth.svelte.js', () => ({ currentUser: () => user.current }));

beforeEach(() => {
  sessionStorage.clear();
  user.current = { id: 1, email: 'old@example.test', has_password: true } as User;
  requestEmailChange.mockReset().mockResolvedValue(undefined);
  confirmEmailChange.mockReset().mockResolvedValue(user.current);
  reauthenticatePassword.mockReset().mockResolvedValue('2099-01-01T00:00:00Z');
  invalidateAll.mockReset().mockResolvedValue(undefined);
});

async function requestCode(address: string): Promise<void> {
  await fireEvent.input(screen.getByLabelText('New email'), { target: { value: address } });
  await fireEvent.input(screen.getByLabelText('Password'), { target: { value: 'hunter2' } });
  await fireEvent.click(screen.getByRole('button', { name: 'Send confirmation code' }));
}

describe('security page — email change', () => {
  it('confirms identity, mails the new address, then applies the code', async () => {
    render(ChangeEmail);
    expect(screen.getByText('old@example.test')).toBeTruthy();

    await requestCode(' new@example.test ');

    await waitFor(() => expect(screen.getByLabelText('Code')).toBeTruthy());
    expect(reauthenticatePassword).toHaveBeenCalledWith('hunter2');
    expect(requestEmailChange).toHaveBeenCalledWith('new@example.test');

    await fireEvent.input(screen.getByLabelText('Code'), { target: { value: '123456' } });
    await fireEvent.click(screen.getByRole('button', { name: 'Change email' }));

    await waitFor(() => expect(screen.getByText(/Email changed/)).toBeTruthy());
    expect(confirmEmailChange).toHaveBeenCalledWith('123456');
    expect(invalidateAll).toHaveBeenCalled();
  });

  it('says so when another account holds the address', async () => {
    requestEmailChange.mockRejectedValue(new StubApiError(409));
    render(ChangeEmail);

    await requestCode('taken@example.test');

    await waitFor(() =>
      expect(screen.getByText('Another freehire account already uses this email.')).toBeTruthy(),
    );
    expect(screen.queryByLabelText('Code')).toBeNull();
  });

  it('refuses the current address without asking the server', async () => {
    render(ChangeEmail);

    await requestCode('OLD@example.test');

    await waitFor(() => expect(screen.getByText('That is already your email.')).toBeTruthy());
    expect(requestEmailChange).not.toHaveBeenCalled();
  });

  it('keeps the member on the code step when the code is wrong', async () => {
    confirmEmailChange.mockRejectedValue(new StubApiError(400));
    render(ChangeEmail);

    await requestCode('new@example.test');
    await waitFor(() => expect(screen.getByLabelText('Code')).toBeTruthy());
    await fireEvent.input(screen.getByLabelText('Code'), { target: { value: '000000' } });
    await fireEvent.click(screen.getByRole('button', { name: 'Change email' }));

    await waitFor(() => expect(screen.getByText(/That code is wrong or has expired/)).toBeTruthy());
    expect(screen.getByLabelText('Code')).toBeTruthy();
    expect(invalidateAll).not.toHaveBeenCalled();
  });
});
