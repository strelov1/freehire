import { afterEach, describe, expect, it } from 'vitest';
import { CLI_BANNER_DISMISSED_KEY, readDismissed, writeDismissed } from './cliPromo';

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

  it('does not inherit the retired Product Hunt strip’s dismissal', () => {
    installStorage(new Map([['hire.ph-banner-dismissed', '1']]));

    expect(readDismissed()).toBe(false);
  });

  it('writes the key the app.html no-flash script reads', () => {
    const store = new Map<string, string>();
    installStorage(store);

    writeDismissed();

    expect(store.get(CLI_BANNER_DISMISSED_KEY)).toBe('1');
    expect(CLI_BANNER_DISMISSED_KEY).toBe('hire.cli-banner-dismissed');
  });

  it('reads as not dismissed when storage is unavailable', () => {
    installHostileStorage();

    expect(readDismissed()).toBe(false);
  });

  it('does not throw when the dismissal cannot be stored', () => {
    installHostileStorage();

    expect(() => writeDismissed()).not.toThrow();
  });
});
