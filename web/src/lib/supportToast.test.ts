import { afterEach, describe, expect, it } from 'vitest';
import { CLI_BANNER_DISMISSED_KEY } from './cliPromo';
import {
  ownsMobileStickyCta,
  readDismissed,
  SUPPORT_DISMISSED_KEY,
  suppressesToast,
  writeDismissed,
} from './supportToast';

describe('route rules', () => {
  it('suppresses the toast on the open-source page', () => {
    expect(suppressesToast('/open')).toBe(true);
  });

  it('allows the toast elsewhere', () => {
    expect(suppressesToast('/')).toBe(false);
    expect(suppressesToast('/jobs')).toBe(false);
    expect(suppressesToast('/openings')).toBe(false);
  });

  it('marks the job page as owning a sticky mobile call to action', () => {
    expect(ownsMobileStickyCta('/jobs/senior-go-engineer-acme')).toBe(true);
  });

  it('does not mark the listing or other sections', () => {
    expect(ownsMobileStickyCta('/jobs')).toBe(false);
    expect(ownsMobileStickyCta('/companies/acme')).toBe(false);
  });
});

// The node environment has no localStorage at all, so each test installs the one it
// needs and removes it afterwards.
function installStorage(store: Map<string, string>) {
  Object.defineProperty(globalThis, 'localStorage', {
    configurable: true,
    value: {
      getItem: (k: string) => store.get(k) ?? null,
      setItem: (k: string, v: string) => void store.set(k, v),
    },
  });
}

/** A blocked origin or Safari private mode: every access throws. */
function installHostileStorage() {
  Object.defineProperty(globalThis, 'localStorage', {
    configurable: true,
    get() {
      throw new Error('access denied');
    },
  });
}

afterEach(() => {
  Reflect.deleteProperty(globalThis, 'localStorage');
});

describe('dismissal', () => {
  it('round-trips through storage', () => {
    installStorage(new Map());

    expect(readDismissed()).toBe(false);
    writeDismissed();
    expect(readDismissed()).toBe(true);
  });

  it('leaves the CLI strip’s own key alone', () => {
    const store = new Map([[CLI_BANNER_DISMISSED_KEY, '1']]);
    installStorage(store);

    writeDismissed();

    expect(store.get(CLI_BANNER_DISMISSED_KEY)).toBe('1');
    expect(store.get(SUPPORT_DISMISSED_KEY)).toBe('1');
  });

  it('reads as unanswered when storage is unavailable', () => {
    installHostileStorage();

    expect(readDismissed()).toBe(false);
  });

  it('does not throw when the dismissal cannot be stored', () => {
    installHostileStorage();

    expect(() => writeDismissed()).not.toThrow();
  });

});
