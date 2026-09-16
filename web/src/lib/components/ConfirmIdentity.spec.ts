import { fireEvent, render, screen } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { User } from '$lib/types';
import ConfirmIdentity from './ConfirmIdentity.svelte';

// `ConnectedIdentity` is deliberately not exported from `$lib/types` — only the envelope
// around it is — so the shape is restated here rather than widening the module's public
// surface for a test.
type Identity = { provider: string; linked_at: string; status: string; can_unlink: boolean };

// The one surface every gated action borrows. What it must never do is state that
// confirmation is required and leave the member nothing to press — which is exactly what
// the API-keys revoke dialog did.

const { connectedIdentities, reauthenticatePassword, begin, expiry, forget, user } = vi.hoisted(() => ({
  connectedIdentities: vi.fn(),
  reauthenticatePassword: vi.fn(),
  begin: vi.fn(),
  expiry: vi.fn(),
  forget: vi.fn(),
  user: { current: null as User | null },
}));

// `locale()` reads `page.data.locale`; the shared stub carries only `url`, and an absent
// locale is what selects the English source catalogue.
vi.mock('$app/state', () => ({ page: { data: {}, url: new URL('http://localhost/') } }));
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

vi.mock('$lib/api', () => ({
  api: { connectedIdentities, reauthenticatePassword },
  ApiError: StubApiError,
}));
vi.mock('$lib/auth.svelte', () => ({ currentUser: () => user.current }));
vi.mock('$lib/recentAuth', () => ({
  beginProviderReauthentication: begin,
  recentAuthExpiry: expiry,
  forgetRecentAuthExpiry: forget,
}));

const identity = (provider: string, status: 'active' | 'revocation_pending'): Identity => ({
  provider,
  status,
  linked_at: '2026-01-01T00:00:00Z',
  can_unlink: true,
});

function signedIn(hasPassword: boolean): void {
  user.current = { id: 1, email: 'a@b.test', has_password: hasPassword } as User;
}

beforeEach(() => {
  begin.mockReset();
  forget.mockReset();
  expiry.mockReset().mockReturnValue(null);
  reauthenticatePassword.mockReset().mockResolvedValue('2099-01-01T00:00:00Z');
  connectedIdentities.mockReset().mockResolvedValue({
    has_password: false,
    identities: [identity('google', 'active')],
  });
});

const props = { returnTo: '/my/api-keys' as const, prompt: 'A key is permanent access.' };

describe('ConfirmIdentity', () => {
  it('offers a password input to an account that has a password', async () => {
    signedIn(true);

    render(ConfirmIdentity, { props });

    expect(await screen.findByLabelText('Password')).toBeTruthy();
    expect(screen.queryByRole('button', { name: /Confirm with/ })).toBeNull();
  });

  it('does not ask the server for providers an account with a password will never use', async () => {
    signedIn(true);

    render(ConfirmIdentity, { props });
    await screen.findByLabelText('Password');

    expect(connectedIdentities).not.toHaveBeenCalled();
  });

  it('offers one control per active provider to an account with no password', async () => {
    signedIn(false);

    render(ConfirmIdentity, { props });

    expect(await screen.findByRole('button', { name: 'Confirm with google' })).toBeTruthy();
    expect(screen.queryByLabelText('Password')).toBeNull();
  });

  it('does not offer a provider whose identity is no longer active', async () => {
    signedIn(false);
    connectedIdentities.mockResolvedValue({
      has_password: false,
      identities: [identity('google', 'active'), identity('github', 'revocation_pending')],
    });

    render(ConfirmIdentity, { props });

    await screen.findByRole('button', { name: 'Confirm with google' });
    expect(screen.queryByRole('button', { name: 'Confirm with github' })).toBeNull();
  });

  it('carries the draft as it stands when the member leaves, not as it stood at mount', async () => {
    signedIn(false);
    let name = 'typed before mount';
    render(ConfirmIdentity, {
      props: { ...props, draft: () => ({ surface: 'create-api-key' as const, name, days: 30 }) },
    });
    name = 'typed just before leaving';

    await fireEvent.click(await screen.findByRole('button', { name: 'Confirm with google' }));

    expect(begin).toHaveBeenCalledWith('google', '/my/api-keys', {
      surface: 'create-api-key',
      name: 'typed just before leaving',
      days: 30,
    });
  });

  it('reports the confirmation once a proof is held, and stops asking for one', async () => {
    signedIn(false);
    expiry.mockReturnValue(new Date(Date.now() + 9 * 60_000));

    render(ConfirmIdentity, { props });

    expect(await screen.findByText(/Identity confirmed/)).toBeTruthy();
    expect(screen.queryByRole('button', { name: /Confirm with/ })).toBeNull();
  });

  it('asks again once the server overrules the held proof', async () => {
    signedIn(true);
    expiry.mockReturnValue(new Date(Date.now() + 9 * 60_000));
    const { component } = render(ConfirmIdentity, { props });
    await screen.findByText(/Identity confirmed/);

    (component as unknown as { refused: () => void }).refused();

    expect(forget).toHaveBeenCalledOnce();
    expect(await screen.findByLabelText('Password')).toBeTruthy();
  });

  it('says so plainly when the account has no way to confirm at all', async () => {
    // Not reachable by design — an account keeps at least one sign-in method — but a
    // surface that renders an empty box while claiming confirmation is required is the
    // defect this component exists to end.
    signedIn(false);
    connectedIdentities.mockResolvedValue({ has_password: false, identities: [] });

    render(ConfirmIdentity, { props });

    expect(await screen.findByText(/no way to confirm/i)).toBeTruthy();
  });

  it('reports a failure to load providers instead of rendering nothing', async () => {
    signedIn(false);
    connectedIdentities.mockRejectedValue(new Error('offline'));

    render(ConfirmIdentity, { props });

    expect(await screen.findByText(/Could not load/i)).toBeTruthy();
  });

  it('offers a way out of a failed provider load rather than a dead end', async () => {
    // The error state is terminal — the effect has no reason to re-run — so without a
    // retry a member whose network blipped is stuck until they reload the page. That is
    // the same shape as the revoke dialog this component was written to abolish.
    signedIn(false);
    connectedIdentities.mockRejectedValueOnce(new Error('offline'));
    render(ConfirmIdentity, { props });
    await screen.findByText(/Could not load/i);

    await fireEvent.click(screen.getByRole('button', { name: 'Try again' }));

    expect(await screen.findByRole('button', { name: 'Confirm with google' })).toBeTruthy();
  });

  it('renders at least one control in every state that demands confirmation', async () => {
    // Task 2.4's requirement, asserted across the states rather than at one of them:
    // "A surface SHALL NOT state a requirement it gives the member no way to satisfy."
    signedIn(false);
    for (const setUp of [
      () => connectedIdentities.mockResolvedValue({ has_password: false, identities: [] }),
      () => connectedIdentities.mockRejectedValue(new Error('offline')),
      () =>
        connectedIdentities.mockResolvedValue({
          has_password: false,
          identities: [identity('google', 'active')],
        }),
    ]) {
      setUp();
      const { unmount } = render(ConfirmIdentity, { props });

      // Waits for the state to settle: "Confirm it is you" is already on screen while
      // the providers are still loading, so asserting on it would check the wrong moment.
      // A state that never grows a control fails here by timing out, which is the point.
      expect(await screen.findAllByRole('button')).not.toHaveLength(0);
      unmount();
    }
  });

  it('tells its caller whether a proof is already held', async () => {
    // Without this a caller cannot tell "the member left the password blank" from "no
    // password was asked for because one is held", and would spend an empty password on
    // `reauthenticatePassword` — a 401 rendered to a confirmed member as "wrong password".
    signedIn(false);
    expiry.mockReturnValue(new Date(Date.now() + 9 * 60_000));
    const { component } = render(ConfirmIdentity, { props });
    const c = component as unknown as { isConfirmed: () => boolean; refused: () => void };
    await screen.findByText(/Identity confirmed/);

    expect(c.isConfirmed()).toBe(true);
    c.refused();
    expect(c.isConfirmed()).toBe(false);
  });

  it('stops claiming confirmation once the proof it was told about has expired', async () => {
    // A stale green tick on an auth surface is worse than no tick: it invites the member
    // to press a button that is about to 428.
    vi.useFakeTimers();
    try {
      signedIn(false);
      expiry.mockReturnValue(new Date(Date.now() + 2000));
      render(ConfirmIdentity, { props });
      expect(screen.getByText(/Identity confirmed/)).toBeTruthy();

      await vi.advanceTimersByTimeAsync(3000);

      expect(screen.queryByText(/Identity confirmed/)).toBeNull();
    } finally {
      vi.useRealTimers();
    }
  });

  it('refuses to spend a password the member never typed', async () => {
    // Sending '' earns a 401, which every caller words as "that password is not right" —
    // told to somebody who has not typed one yet. The old per-caller `passwordRequired`
    // message was lost when the check moved in here; this is it, beside its own input.
    signedIn(true);
    const { component } = render(ConfirmIdentity, { props });
    const c = component as unknown as { prove: () => Promise<boolean> };
    await screen.findByLabelText('Password');

    await expect(c.prove()).resolves.toBe(false);

    expect(reauthenticatePassword).not.toHaveBeenCalled();
    expect(await screen.findByText(/Enter your password/i)).toBeTruthy();
  });

  it('spends a typed password and reports that it may proceed', async () => {
    signedIn(true);
    const { component } = render(ConfirmIdentity, { props: { ...props, password: 'hunter2' } });
    const c = component as unknown as { prove: () => Promise<boolean> };

    await expect(c.prove()).resolves.toBe(true);

    expect(reauthenticatePassword).toHaveBeenCalledWith('hunter2');
  });

  it('needs no password at all when a proof is held', async () => {
    signedIn(false);
    expiry.mockReturnValue(new Date(Date.now() + 9 * 60_000));
    const { component } = render(ConfirmIdentity, { props });
    const c = component as unknown as { prove: () => Promise<boolean> };

    await expect(c.prove()).resolves.toBe(true);

    expect(reauthenticatePassword).not.toHaveBeenCalled();
  });

  it('owns the wording for the refusals that are about identity, and only those', async () => {
    // Three callers had three copies of this cascade — the same shape the component was
    // written to abolish. Anything not about identity is handed back for the caller to
    // word on its own surface.
    signedIn(true);
    expiry.mockReturnValue(new Date(Date.now() + 9 * 60_000));
    const { component } = render(ConfirmIdentity, { props });
    const c = component as unknown as { handleRefusal: (e: unknown) => string | null };

    expect(c.handleRefusal(new StubApiError(428))).toMatch(/confirm/i);
    expect(forget).toHaveBeenCalled();
    expect(c.handleRefusal(new StubApiError(401))).toMatch(/not right/i);
    expect(c.handleRefusal(new StubApiError(500))).toBeNull();
    expect(c.handleRefusal(new Error('offline'))).toBeNull();
  });

  it('does not call a 401 a wrong password when no password was asked for', async () => {
    signedIn(false);
    expiry.mockReturnValue(new Date(Date.now() + 9 * 60_000));
    const { component } = render(ConfirmIdentity, { props });
    const c = component as unknown as { handleRefusal: (e: unknown) => string | null };

    expect(c.handleRefusal(new StubApiError(401))).toBeNull();
  });

  it('does not demand confirmation while it is still working out how', async () => {
    // The loading state says what it is doing and asks for nothing. A heading demanding
    // confirmation above an empty box is the shape the spec forbids, and it is reachable
    // for as long as the providers request takes.
    signedIn(false);
    connectedIdentities.mockReturnValue(new Promise(() => {}));

    render(ConfirmIdentity, { props });

    expect(await screen.findByText(/Loading your sign-in providers/i)).toBeTruthy();
    expect(screen.queryByText('Confirm it is you')).toBeNull();
  });

  it('renders nothing and fetches nothing while its surface is closed', async () => {
    // Dialog renders its children whether or not it is open, so an always-active instance
    // would ask the server about a member who never opened the dialog.
    signedIn(false);

    render(ConfirmIdentity, { props: { ...props, active: false } });

    expect(screen.queryByText('Confirm it is you')).toBeNull();
    expect(connectedIdentities).not.toHaveBeenCalled();
  });

  it('counts down in minutes and seconds', async () => {
    signedIn(false);
    expiry.mockReturnValue(new Date(Date.now() + 9 * 60_000 + 41_000));

    render(ConfirmIdentity, { props });

    expect(await screen.findByText(/9:4[01]/)).toBeTruthy();
  });
});
