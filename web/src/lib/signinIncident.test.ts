import { afterEach, describe, expect, it, vi } from 'vitest';
import { markSigninIncident, takeSigninIncident } from './signinIncident';

function stubTab() {
  const values = new Map<string, string>();
  vi.stubGlobal('window', {
    crypto: { randomUUID: () => crypto.randomUUID() },
    sessionStorage: {
      getItem: (key: string) => values.get(key) ?? null,
      setItem: (key: string, value: string) => values.set(key, value),
      removeItem: (key: string) => values.delete(key),
    },
  });
  return values;
}

afterEach(() => {
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

describe('sign-in exception correlation', () => {
  it('reuses one incident and consumes it when an authenticated session is observed', () => {
    const tab = stubTab();
    const incident = markSigninIncident();
    expect(incident).toMatch(/^[a-f0-9-]{36}$/);
    expect(markSigninIncident()).toBe(incident);
    expect(takeSigninIncident()).toBe(incident);
    expect(takeSigninIncident()).toBeUndefined();
    expect(tab.size).toBe(0);
  });

  it('discards expired or malformed incident records', () => {
    vi.useFakeTimers();
    const tab = stubTab();
    const incident = markSigninIncident();
    vi.advanceTimersByTime(30 * 60_000);
    expect(takeSigninIncident()).toBeUndefined();
    expect(tab.size).toBe(0);
    expect(markSigninIncident()).not.toBe(incident);
    for (const key of tab.keys()) tab.set(key, 'arbitrary text');
    expect(takeSigninIncident()).toBeUndefined();
    expect(tab.size).toBe(0);
  });

  it('keeps recording optional when browser storage is blocked', () => {
    vi.stubGlobal('window', {
      get sessionStorage() {
        throw new DOMException('Storage blocked', 'SecurityError');
      },
    });
    expect(markSigninIncident()).toBeUndefined();
    expect(takeSigninIncident()).toBeUndefined();
  });
});
