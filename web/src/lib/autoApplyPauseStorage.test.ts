import { describe, it, expect, afterEach } from 'vitest';
import { isAutoApplyPaused, setAutoApplyPaused } from './autoApplyPauseStorage';

// Mirrors filterStorage.test.ts's stand-in: there is no browser storage in the plain-Node
// vitest env, so a minimal in-memory Map fills in for it.
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

describe('autoApplyPauseStorage', () => {
  afterEach(() => {
    // @ts-expect-error - clean up the global we install per test
    delete globalThis.localStorage;
  });

  it('defaults to false for a queue id that was never paused', () => {
    // @ts-expect-error - install the stand-in
    globalThis.localStorage = new MemoryStorage();

    expect(isAutoApplyPaused(123)).toBe(false);
  });

  it('reports true after being set paused', () => {
    // @ts-expect-error - install the stand-in
    globalThis.localStorage = new MemoryStorage();

    setAutoApplyPaused(123, true);

    expect(isAutoApplyPaused(123)).toBe(true);
  });

  it('reports false again after being un-paused', () => {
    // @ts-expect-error - install the stand-in
    globalThis.localStorage = new MemoryStorage();

    setAutoApplyPaused(123, true);
    setAutoApplyPaused(123, false);

    expect(isAutoApplyPaused(123)).toBe(false);
  });

  it('tracks two queue ids independently', () => {
    // @ts-expect-error - install the stand-in
    globalThis.localStorage = new MemoryStorage();

    setAutoApplyPaused(1, true);

    expect(isAutoApplyPaused(1)).toBe(true);
    expect(isAutoApplyPaused(2)).toBe(false);
  });

  it('defaults to false when storage is unavailable (SSR)', () => {
    // No globalThis.localStorage installed.
    expect(isAutoApplyPaused(123)).toBe(false);
    expect(() => setAutoApplyPaused(123, true)).not.toThrow();
  });

  it('swallows storage access errors (private mode / quota)', () => {
    // @ts-expect-error - install a throwing stand-in
    globalThis.localStorage = {
      getItem() {
        throw new Error('denied');
      },
      setItem() {
        throw new Error('quota');
      },
      removeItem() {
        throw new Error('denied');
      },
    };

    expect(isAutoApplyPaused(123)).toBe(false);
    expect(() => setAutoApplyPaused(123, true)).not.toThrow();
  });
});
