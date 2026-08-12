import { describe, it, expect } from 'vitest';
import { serialQueue } from './serialQueue';

/** A promise the test settles by hand, so a second job can be queued while the first
 *  is still running. */
function deferred<T = void>() {
  let resolve!: (v: T) => void;
  let reject!: (e: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

/** Let every already-resolved promise deliver. The queue starts a job in a microtask
 *  rather than synchronously, so nothing has run right after queueing. */
const flush = () => new Promise((resolve) => setTimeout(resolve, 0));

describe('serialQueue', () => {
  it('does not start a job while its predecessor is still running', async () => {
    const queue = serialQueue();
    const first = deferred<string>();
    const started: string[] = [];

    const a = queue(() => {
      started.push('a');
      return first.promise;
    });
    const b = queue(() => {
      started.push('b');
      return Promise.resolve('b');
    });

    await flush();
    expect(started).toEqual(['a']);
    first.resolve('a');
    await a;
    await b;
    expect(started).toEqual(['a', 'b']);
  });

  it('lets a job read state its predecessor wrote', async () => {
    const queue = serialQueue();
    const first = deferred();
    let state = 'initial';

    const a = queue(async () => {
      await first.promise;
      state = 'written by a';
    });
    const b = queue(async () => `b saw: ${state}`);

    first.resolve();
    await a;
    expect(await b).toBe('b saw: written by a');
  });

  it('returns each job its own result', async () => {
    const queue = serialQueue();
    expect(await Promise.all([queue(async () => 1), queue(async () => 2)])).toEqual([1, 2]);
  });

  it('rejects the failing job without wedging the ones behind it', async () => {
    const queue = serialQueue();
    const failing = queue(async () => {
      throw new Error('write failed');
    });
    const next = queue(async () => 'ran anyway');

    await expect(failing).rejects.toThrow('write failed');
    expect(await next).toBe('ran anyway');
  });
});
