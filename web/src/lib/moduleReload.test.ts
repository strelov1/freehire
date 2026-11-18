import { describe, expect, it } from 'vitest';
import { claimModuleReload } from './moduleReload';

function memoryStore() {
  const data = new Map<string, string>();
  return { getItem: (k: string) => data.get(k) ?? null, setItem: (k: string, v: string) => void data.set(k, v) };
}

describe('claimModuleReload', () => {
  it('allows one reload per URL, then holds the error page', () => {
    const store = memoryStore();
    expect(claimModuleReload(store, 'https://x/jobs/swipe?a=1', 0)).toBe(true);
    expect(claimModuleReload(store, 'https://x/jobs/swipe?a=1', 1_000)).toBe(false);
  });

  it('allows it again after the window, or for another URL', () => {
    const store = memoryStore();
    claimModuleReload(store, 'https://x/a', 0);
    expect(claimModuleReload(store, 'https://x/b', 1_000)).toBe(true);
    expect(claimModuleReload(store, 'https://x/b', 1_000 + 5 * 60_000)).toBe(true);
  });

  it('never reloads without working storage', () => {
    expect(claimModuleReload(undefined, 'https://x/a', 0)).toBe(false);
    const broken = {
      getItem: () => null,
      setItem: () => {
        throw new Error('QuotaExceededError');
      },
    };
    expect(claimModuleReload(broken, 'https://x/a', 0)).toBe(false);
  });

  it('treats a corrupted record as none', () => {
    const store = memoryStore();
    store.setItem('freehire:module-reload', '{not json');
    expect(claimModuleReload(store, 'https://x/a', 0)).toBe(true);
  });
});
