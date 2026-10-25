import { fireEvent, render, screen, within } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { OAuthGrant, User } from '#lib/types.js';
import ConnectedDevicesView from './ConnectedDevicesView.svelte';

const {
  listOAuthGrants,
  revokeOAuthGrant,
  reauthenticatePassword,
  connectedIdentities,
  getAgentPermissions,
  setAgentPermissions,
  user,
} = vi.hoisted(() => ({
  getAgentPermissions: vi.fn(),
  setAgentPermissions: vi.fn(),
  listOAuthGrants: vi.fn(),
  revokeOAuthGrant: vi.fn(),
  reauthenticatePassword: vi.fn(),
  connectedIdentities: vi.fn(),
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
vi.mock('#lib/api.js', () => ({
  api: {
    listOAuthGrants,
    revokeOAuthGrant,
    reauthenticatePassword,
    connectedIdentities,
    getAgentPermissions,
    setAgentPermissions,
  },
  ApiError: StubApiError,
}));
vi.mock('#lib/auth.svelte.js', () => ({
  currentUser: () => user.current,
  isAuthenticated: () => user.current !== null,
}));
vi.mock('#lib/recentAuth.js', () => ({
  beginProviderReauthentication: vi.fn(),
  recentAuthExpiry: () => null,
  forgetRecentAuthExpiry: vi.fn(),
  consumeReauthDraft: () => null,
}));

const grant: OAuthGrant = {
  id: 9,
  client_name: 'Claude Desktop',
  created_at: '2026-09-15T00:00:00Z',
  last_used_at: null,
  expires_at: null,
};

function signedIn(hasPassword: boolean): void {
  user.current = { id: 1, email: 'a@b.test', has_password: hasPassword } as User;
}

function openDialog(): HTMLElement {
  const dialogs = screen.getAllByRole('dialog', { hidden: true });
  const open = dialogs.find((d) => (d as HTMLDialogElement).open);
  if (!open) throw new Error('no dialog is open');
  return open;
}

beforeEach(() => {
  listOAuthGrants.mockReset().mockResolvedValue([grant]);
  revokeOAuthGrant.mockReset().mockResolvedValue(undefined);
  reauthenticatePassword.mockReset().mockResolvedValue('2099-01-01T00:00:00Z');
  connectedIdentities.mockReset().mockResolvedValue({ has_password: false, identities: [] });
  getAgentPermissions.mockReset().mockResolvedValue({ agents_may_apply: false });
  setAgentPermissions.mockReset().mockImplementation(async (p) => p);
});

describe('ConnectedDevicesView', () => {
  it('turns on letting connected agents apply, starting from the stored setting', async () => {
    signedIn(true);
    render(ConnectedDevicesView);
    const toggle = await screen.findByRole('switch', { name: /let connected agents apply/i });
    await vi.waitFor(() => expect((toggle as HTMLButtonElement).disabled).toBe(false));
    expect(toggle.getAttribute('aria-checked')).toBe('false');

    await fireEvent.click(toggle);

    expect(setAgentPermissions).toHaveBeenCalledWith({ agents_may_apply: true });
    await vi.waitFor(() => expect(toggle.getAttribute('aria-checked')).toBe('true'));
  });

  it('renders an empty state when there are no grants', async () => {
    signedIn(true);
    listOAuthGrants.mockResolvedValue([]);
    render(ConnectedDevicesView);
    await screen.findByText(/no connected/i);
  });

  it('lists a grant by its client name', async () => {
    signedIn(true);
    render(ConnectedDevicesView);
    await screen.findByText('Claude Desktop');
  });

  it('revokes a grant using only controls inside the dialog', async () => {
    signedIn(true);
    render(ConnectedDevicesView);
    await screen.findByText('Claude Desktop');

    await fireEvent.click(screen.getByRole('button', { name: 'Revoke' }));
    const dialog = openDialog();
    await fireEvent.input(within(dialog).getByLabelText('Password'), {
      target: { value: 'hunter2' },
    });
    await fireEvent.click(within(dialog).getByRole('button', { name: 'Revoke' }));

    expect(reauthenticatePassword).toHaveBeenCalledWith('hunter2');
    expect(revokeOAuthGrant).toHaveBeenCalledWith(9);
  });
});
