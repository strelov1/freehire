import { afterEach, describe, expect, it } from 'vitest';
import { readAlertsDismissed, writeAlertsDismissed } from './accountSetupDismiss';

// The node environment has no localStorage at all, so each test installs the one it
// needs and removes it afterwards. Mirrors supportToast.test.ts's fixtures.
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

describe('alerts step dismissal', () => {
  it('round-trips through storage for a given account', () => {
    installStorage(new Map());

    expect(readAlertsDismissed(42)).toBe(false);
    writeAlertsDismissed(42);
    expect(readAlertsDismissed(42)).toBe(true);
  });

  // A shared computer must not carry one account's dismissal into another's session.
  it('keeps one account’s dismissal from leaking into another’s', () => {
    installStorage(new Map());

    writeAlertsDismissed(1);

    expect(readAlertsDismissed(1)).toBe(true);
    expect(readAlertsDismissed(2)).toBe(false);
  });

  it('reads as not dismissed when there is no signed-in account yet', () => {
    installStorage(new Map());

    expect(readAlertsDismissed(undefined)).toBe(false);
    expect(() => writeAlertsDismissed(undefined)).not.toThrow();
  });

  it('reads as unanswered when storage is unavailable', () => {
    installHostileStorage();

    expect(readAlertsDismissed(42)).toBe(false);
  });

  it('does not throw when the dismissal cannot be stored', () => {
    installHostileStorage();

    expect(() => writeAlertsDismissed(42)).not.toThrow();
  });
});
