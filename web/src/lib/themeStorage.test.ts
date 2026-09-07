import { afterEach, describe, expect, it } from 'vitest';
import { THEME_KEY, readStoredTheme, writeStoredTheme } from './themeStorage';

// The node env has no localStorage at all, so each test installs the one it needs and
// removes it afterwards (same shape as cliPromo.test.ts / filterStorage.test.ts).
function installStorage(store: Map<string, string>) {
  Object.defineProperty(globalThis, 'localStorage', {
    configurable: true,
    value: {
      getItem: (k: string) => store.get(k) ?? null,
      setItem: (k: string, v: string) => void store.set(k, v),
    },
  });
}

/** A document denied storage — a sandboxed iframe, or a browser set to block site
 *  data. The throw comes from READING the `localStorage` property, which is what makes
 *  a `typeof` guard in front of the try useless. */
function installHostileStorage() {
  Object.defineProperty(globalThis, 'localStorage', {
    configurable: true,
    get() {
      throw new Error('Access is denied for this document.');
    },
  });
}

afterEach(() => {
  Reflect.deleteProperty(globalThis, 'localStorage');
});

describe('readStoredTheme', () => {
  it('defaults to light with nothing stored', () => {
    installStorage(new Map());
    expect(readStoredTheme()).toBe('light');
  });

  it('reads a stored dark mode', () => {
    installStorage(new Map([[THEME_KEY, 'dark']]));
    expect(readStoredTheme()).toBe('dark');
  });

  it('treats an unrecognized value as light', () => {
    installStorage(new Map([[THEME_KEY, 'system']]));
    expect(readStoredTheme()).toBe('light');
  });

  // The regression this module exists for. Unguarded, the throw escaped a module-scope
  // constructor and took the whole theme module — and with it the root layout that
  // imports it — down for anyone browsing with site data blocked.
  it('falls back to light when the document is denied storage', () => {
    installHostileStorage();
    expect(() => readStoredTheme()).not.toThrow();
    expect(readStoredTheme()).toBe('light');
  });

  it('falls back to light when there is no storage at all (SSR)', () => {
    expect(readStoredTheme()).toBe('light');
  });
});

describe('writeStoredTheme', () => {
  it('round-trips through storage', () => {
    installStorage(new Map());
    writeStoredTheme('dark');
    expect(readStoredTheme()).toBe('dark');
  });

  it('does not throw when the document is denied storage', () => {
    installHostileStorage();
    expect(() => writeStoredTheme('dark')).not.toThrow();
  });

  it('does not throw when there is no storage at all (SSR)', () => {
    expect(() => writeStoredTheme('dark')).not.toThrow();
  });
});
