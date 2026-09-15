import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
  beginProviderReauthentication,
  completeProviderReauthentication,
  consumeReauthDraft,
  forgetRecentAuthExpiry,
  recentAuthExpiry,
} from './recentAuth';

// Confirming through a provider is a full-page navigation, so whatever the member had
// typed is gone unless it is carried across. These tests pin what is carried, what is
// deliberately NOT carried, and that it is consumed exactly once — a draft that outlived
// its trip would reopen a dialog on an unrelated later visit.

const { exchange } = vi.hoisted(() => ({ exchange: vi.fn() }));

vi.mock('$lib/api', () => ({
  api: { exchangeOAuthReauthentication: exchange },
}));

// Mirrors autoApplyPauseStorage.test.ts: the plain-Node vitest env has no browser
// storage, so a minimal in-memory Map stands in for it.
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
  exchange.mockReset();
  exchange.mockResolvedValue('2026-09-15T00:10:00Z');
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

const DRAFT_KEY = 'freehire.reauth.draft';
const VERIFIER_KEY = 'freehire.reauth.verifier';

describe('beginProviderReauthentication', () => {
  it('stores the draft beside the verifier so the pending action survives the trip', async () => {
    await beginProviderReauthentication('google', '/my/api-keys', {
      surface: 'create-api-key',
      name: 'CI bot',
      days: 30,
    });

    expect(storage.getItem(VERIFIER_KEY)).toBeTruthy();
    expect(JSON.parse(storage.getItem(DRAFT_KEY) as string)).toEqual({
      surface: 'create-api-key',
      name: 'CI bot',
      days: 30,
    });
    expect(assign).toHaveBeenCalledOnce();
  });

  it('clears a draft left by an earlier trip when this one carries none', async () => {
    // Otherwise an abandoned key-creation trip would reopen that dialog the next time
    // the member confirms from an unrelated surface.
    storage.setItem(DRAFT_KEY, JSON.stringify({ surface: 'create-api-key', name: 'old', days: 0 }));

    await beginProviderReauthentication('google', '/my/security');

    expect(storage.getItem(DRAFT_KEY)).toBeNull();
  });
});

describe('consumeReauthDraft', () => {
  it('returns the stored draft and removes it', async () => {
    await beginProviderReauthentication('google', '/my/api-keys', {
      surface: 'create-api-key',
      name: 'CI bot',
      days: 0,
    });

    expect(consumeReauthDraft('create-api-key')).toEqual({
      surface: 'create-api-key',
      name: 'CI bot',
      days: 0,
    });
    expect(storage.getItem(DRAFT_KEY)).toBeNull();
  });

  it('returns null on a second call, so a later visit does not reopen the dialog', async () => {
    await beginProviderReauthentication('google', '/my/api-keys', { surface: 'delete-account' });
    consumeReauthDraft('delete-account');

    expect(consumeReauthDraft('delete-account')).toBeNull();
  });

  it('carries which key a revocation was about', async () => {
    // Without this the member confirms, comes back to a closed dialog, and has to find the
    // key again — the silent loss the transport exists to prevent, one surface short.
    await beginProviderReauthentication('google', '/my/api-keys', {
      surface: 'revoke-api-key',
      keyId: 7,
    });

    expect(consumeReauthDraft('revoke-api-key')).toEqual({ surface: 'revoke-api-key', keyId: 7 });
  });

  it('refuses a revocation draft whose key id is not a number', () => {
    storage.setItem(DRAFT_KEY, JSON.stringify({ surface: 'revoke-api-key', keyId: 'seven' }));

    expect(consumeReauthDraft('revoke-api-key')).toBeNull();
  });

  it('leaves another surface’s draft alone instead of eating it', async () => {
    // Two surfaces on one route both consume on mount. Without the tag the first to run
    // swallows a draft it does not recognise and the one it belongs to never sees it —
    // a silent loss, which is the failure this whole change exists to end.
    await beginProviderReauthentication('google', '/my/security', { surface: 'delete-account' });

    expect(consumeReauthDraft('create-api-key')).toBeNull();
    expect(consumeReauthDraft('delete-account')).toEqual({ surface: 'delete-account' });
  });

  it('returns null rather than throwing when the stored value is not readable', () => {
    storage.setItem(DRAFT_KEY, '{not json');

    expect(consumeReauthDraft('create-api-key')).toBeNull();
    expect(storage.getItem(DRAFT_KEY)).toBeNull();
  });

  it('refuses readable JSON that is not the shape it claims to be', () => {
    // `JSON.parse(raw) as ReauthDraft` is a promise the runtime does not keep; the
    // restored value goes straight into an input, so its fields have to be checked.
    storage.setItem(DRAFT_KEY, JSON.stringify({ surface: 'create-api-key', name: {}, days: 'x' }));

    expect(consumeReauthDraft('create-api-key')).toBeNull();
    expect(storage.getItem(DRAFT_KEY)).toBeNull();
  });

  it('returns null when nothing was stored', () => {
    expect(consumeReauthDraft('create-api-key')).toBeNull();
  });
});

describe('completeProviderReauthentication', () => {
  it('leaves the draft for the returning surface to consume', async () => {
    await beginProviderReauthentication('google', '/my/api-keys', {
      surface: 'create-api-key',
      name: 'CI bot',
      days: 90,
    });

    await expect(completeProviderReauthentication('code-from-provider')).resolves.toBe(
      '/my/api-keys',
    );
    expect(consumeReauthDraft('create-api-key')).toEqual({ surface: 'create-api-key', name: 'CI bot', days: 90 });
  });

  it('discards the draft when the exchange fails, leaving nothing to reopen later', async () => {
    await beginProviderReauthentication('google', '/my/api-keys', {
      surface: 'create-api-key',
      name: 'CI bot',
      days: 0,
    });
    exchange.mockRejectedValue(new Error('attempt already used'));

    await expect(completeProviderReauthentication('code-from-provider')).rejects.toThrow();
    expect(storage.getItem(DRAFT_KEY)).toBeNull();
  });

  it('discards the draft when the attempt expired before the return', async () => {
    storage.setItem(DRAFT_KEY, JSON.stringify({ surface: 'delete-account' }));
    // No verifier: the attempt never started in this tab, or was already consumed.

    await expect(completeProviderReauthentication('code-from-provider')).rejects.toThrow();
    expect(storage.getItem(DRAFT_KEY)).toBeNull();
  });
});

// `recentAuthExpiry()` is read at COMPONENT INIT, which runs on the server. The jsdom
// project always provides `sessionStorage`, so no component test can ever reach this —
// these cases are the only thing standing between the change and a 500 on every signed-in
// `/my/*` page load.
describe('without usable storage', () => {
  it('reports no proof during SSR instead of throwing', () => {
    // @ts-expect-error - the server has no browser storage at all
    delete globalThis.sessionStorage;

    expect(() => recentAuthExpiry()).not.toThrow();
    expect(recentAuthExpiry()).toBeNull();
  });

  it('carries no draft during SSR instead of throwing', () => {
    // @ts-expect-error - the server has no browser storage at all
    delete globalThis.sessionStorage;

    expect(consumeReauthDraft('create-api-key')).toBeNull();
  });

  it('forgets the expiry during SSR instead of throwing', () => {
    // @ts-expect-error - the server has no browser storage at all
    delete globalThis.sessionStorage;

    expect(() => forgetRecentAuthExpiry()).not.toThrow();
  });

  it('reports no proof when the browser refuses storage access', () => {
    // Safari with site data blocked throws on access rather than answering null, so
    // `typeof` alone is not the guard — it is reading it at all that fails.
    // @ts-expect-error - install a throwing stand-in
    globalThis.sessionStorage = {
      getItem() {
        throw new Error('SecurityError');
      },
      setItem() {
        throw new Error('SecurityError');
      },
      removeItem() {
        throw new Error('SecurityError');
      },
    };

    expect(recentAuthExpiry()).toBeNull();
    expect(consumeReauthDraft('create-api-key')).toBeNull();
    expect(() => forgetRecentAuthExpiry()).not.toThrow();
  });

  it('refuses to start a round trip it cannot possibly finish', async () => {
    // Unlike a read, this one must NOT degrade quietly: without a stored verifier the
    // return leg can never complete, so sending the member to their provider would spend
    // a full-page navigation to arrive at "attempt expired".
    // @ts-expect-error - install a throwing stand-in
    globalThis.sessionStorage = {
      getItem: () => null,
      setItem() {
        throw new Error('SecurityError');
      },
      removeItem() {},
    };

    await expect(beginProviderReauthentication('google', '/my/api-keys')).rejects.toThrow();
    expect(assign).not.toHaveBeenCalled();
  });
});

// The proof lives in a cookie the client cannot read, but it does not have to guess when it
// dies: both issuing routes answer with the expiry, and every caller used to discard it.
describe('recentAuthExpiry', () => {
  it("records the server's own expiry rather than a locally guessed lifetime", async () => {
    exchange.mockResolvedValue('2099-01-01T00:10:00.000Z');
    await beginProviderReauthentication('google', '/my/api-keys');

    await completeProviderReauthentication('code-from-provider');

    expect(recentAuthExpiry()).toEqual(new Date('2099-01-01T00:10:00.000Z'));
  });

  it('reports nothing before any confirmation', () => {
    expect(recentAuthExpiry()).toBeNull();
  });

  it('reports nothing once the recorded expiry has passed', async () => {
    exchange.mockResolvedValue('2000-01-01T00:00:00.000Z');
    await beginProviderReauthentication('google', '/my/api-keys');
    await completeProviderReauthentication('code-from-provider');

    expect(recentAuthExpiry()).toBeNull();
  });

  it('forgets the expiry when the server overrules the hint with a 428', async () => {
    exchange.mockResolvedValue('2099-01-01T00:10:00.000Z');
    await beginProviderReauthentication('google', '/my/api-keys');
    await completeProviderReauthentication('code-from-provider');

    forgetRecentAuthExpiry();

    expect(recentAuthExpiry()).toBeNull();
  });

  it('reports nothing when the recorded value is not a readable date', async () => {
    exchange.mockResolvedValue('not a timestamp');
    await beginProviderReauthentication('google', '/my/api-keys');
    await completeProviderReauthentication('code-from-provider');

    expect(recentAuthExpiry()).toBeNull();
  });
});
