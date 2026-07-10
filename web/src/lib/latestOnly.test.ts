import { describe, it, expect } from 'vitest';
import { latestOnly } from './latestOnly';

/** A promise plus its external resolver, so tests control resolution order. */
function deferred<T>() {
  let resolve!: (v: T) => void;
  let reject!: (e: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

describe('latestOnly', () => {
  it('applies only the newest call when an earlier fetch resolves last', async () => {
    const first = deferred<number>();
    const second = deferred<number>();
    let call = 0;
    let applied: number | undefined;
    const run = latestOnly<number>(
      () => (call++ === 0 ? first.promise : second.promise),
      (v) => (applied = v),
    );

    const a = run(); // generation 1 → first
    const b = run(); // generation 2 → second
    second.resolve(2); // newer resolves first…
    first.resolve(1); // …older resolves last and must be discarded
    await Promise.all([a, b]);

    expect(applied).toBe(2);
  });

  it('applies a lone call', async () => {
    let applied: number | undefined;
    const run = latestOnly<number>(
      () => Promise.resolve(7),
      (v) => (applied = v),
    );
    await run();
    expect(applied).toBe(7);
  });

  it('swallows a rejected fetch without applying', async () => {
    let applied = 'untouched';
    const run = latestOnly<string>(
      () => Promise.reject(new Error('boom')),
      (v) => (applied = v),
    );
    await run();
    expect(applied).toBe('untouched');
  });
});
