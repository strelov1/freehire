import { afterEach, describe, expect, it } from 'vitest';
import {
  ownsMobileStickyCta,
  readDismissed,
  suppressesToast,
  writeDismissed,
} from './supportToast';

describe('route rules', () => {
  it('suppresses the toast on the open-source page', () => {
    expect(suppressesToast('/open')).toBe(true);
  });

  it('suppresses the toast across the account area', () => {
    expect(suppressesToast('/my')).toBe(true);
    expect(suppressesToast('/my/profile')).toBe(true);
    expect(suppressesToast('/my/assistant/0f0b1e3a')).toBe(true);
  });

  it('allows the toast elsewhere', () => {
    expect(suppressesToast('/')).toBe(false);
    expect(suppressesToast('/jobs')).toBe(false);
    expect(suppressesToast('/openings')).toBe(false);
    expect(suppressesToast('/mycompany')).toBe(false);
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

  it('reads as unanswered when storage is unavailable', () => {
    installHostileStorage();

    expect(readDismissed()).toBe(false);
  });

  it('does not throw when the dismissal cannot be stored', () => {
    installHostileStorage();

    expect(() => writeDismissed()).not.toThrow();
  });

});
