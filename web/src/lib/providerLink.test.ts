import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { completeProviderLink, beginProviderLink } from './providerLink';
import { beginProviderReauthentication, pendingProviderLink } from './recentAuth';

// The provider-link trip shares the re-authentication transport and its /my/reauth return;
// these pin that the two never mistake each other and that a link trip leaves nothing behind.

const { exchangeLink, StubApiError } = vi.hoisted(() => ({
  exchangeLink: vi.fn(),
  StubApiError: class extends Error {
    constructor(
      public status: number,
      message: string,
      public body?: Record<string, unknown>,
    ) {
      super(message);
    }
  },
}));

vi.mock('#lib/api.js', () => ({
  api: { exchangeOAuthLink: exchangeLink },
  ApiError: StubApiError,
}));

// The plain-Node vitest env has no browser storage (see recentAuth.test.ts).
class MemoryStorage {
  #map = new Map<string, string>();
  getItem(k: string): string | null {
    return this.#map.has(k) ? (this.#map.get(k) as string) : null;
  }
  setItem(k: string, v: string): void {
    this.#map.set(k, v);
  }
  removeItem(k: string): void {
    this.#map.delete(k);
  }
}

let storage: MemoryStorage;
let assign: ReturnType<typeof vi.fn>;

beforeEach(() => {
  storage = new MemoryStorage();
  assign = vi.fn();
  exchangeLink.mockReset();
  // @ts-expect-error - install the stand-ins the module reaches for
  globalThis.sessionStorage = storage;
  // @ts-expect-error - the module navigates via window.location.assign
  globalThis.window = { location: { assign } };
});

afterEach(() => {
  // @ts-expect-error - clean up the globals installed per test
  delete globalThis.sessionStorage;
  // @ts-expect-error - clean up the globals installed per test
  delete globalThis.window;
});

const VERIFIER_KEY = 'freehire.reauth.verifier';

describe('provider link', () => {
  const EXPIRY_KEY = 'freehire.reauth.expires';

  it('marks the trip as a link and asks the server for the link purpose', async () => {
    await beginProviderLink('github');
    expect(pendingProviderLink()).toBe(true);
    const url = new URL(String(assign.mock.calls[0]?.[0]), 'http://x');
    expect(url.pathname).toBe('/api/v2/auth/oauth/github/start');
    expect(url.searchParams.get('purpose')).toBe('link');
  });

  it('a later re-authentication does not inherit an abandoned link marker', async () => {
    await beginProviderLink('github');
    await beginProviderReauthentication('google', '/my/account');
    expect(pendingProviderLink()).toBe(false);
    expect(new URL(String(assign.mock.calls[1]?.[0]), 'http://x').searchParams.get('purpose')).toBe('reauth');
  });

  it('exchanges the code, drops the spent proof hint, and clears the trip', async () => {
    await beginProviderLink('github');
    storage.setItem(EXPIRY_KEY, String(Date.now() + 60_000));
    exchangeLink.mockResolvedValue({ provider: 'github' });
    expect(await completeProviderLink('the-code')).toEqual({ linked: 'github' });
    expect(exchangeLink).toHaveBeenCalledWith('the-code', expect.any(String));
    expect(storage.getItem(EXPIRY_KEY)).toBeNull();
    expect(pendingProviderLink()).toBe(false);
    expect(storage.getItem(VERIFIER_KEY)).toBeNull();
  });

  it("returns the server's refusal code", async () => {
    await beginProviderLink('github');
    exchangeLink.mockRejectedValue(new StubApiError(409, 'in use', { code: 'identity_in_use' }));
    expect(await completeProviderLink('c')).toEqual({ error: 'identity_in_use' });
  });

  it('a 428 drops the proof hint too', async () => {
    await beginProviderLink('github');
    storage.setItem(EXPIRY_KEY, String(Date.now() + 60_000));
    exchangeLink.mockRejectedValue(new StubApiError(428, 'x', { code: 'recent_auth_required' }));
    expect(await completeProviderLink('c')).toEqual({ error: 'recent_auth_required' });
    expect(storage.getItem(EXPIRY_KEY)).toBeNull();
  });

  it('a return without a code is a cancelled link, not an exchange', async () => {
    await beginProviderLink('github');
    expect(await completeProviderLink(null)).toEqual({ error: 'cancelled' });
    expect(exchangeLink).not.toHaveBeenCalled();
    expect(pendingProviderLink()).toBe(false);
  });
});
