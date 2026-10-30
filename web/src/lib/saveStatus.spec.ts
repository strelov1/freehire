import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { SaveStatus } from './saveStatus.svelte.js';

const { announce } = vi.hoisted(() => ({ announce: vi.fn() }));
vi.mock('#lib/announce.svelte.js', () => ({ announce }));
vi.mock('#lib/i18n/currentLocale.svelte.js', () => ({ locale: () => 'en' }));

function deferred() {
  let resolve!: () => void;
  let reject!: (e: unknown) => void;
  const promise = new Promise<void>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

beforeEach(() => {
  vi.useFakeTimers();
  announce.mockReset();
});

afterEach(() => {
  vi.useRealTimers();
});

describe('SaveStatus', () => {
  it('shows saving, then saved for a moment, then nothing', async () => {
    const status = new SaveStatus();
    const write = deferred();
    const done = status.run(() => write.promise);
    expect(status.state).toBe('saving');

    write.resolve();
    expect(await done).toBe(true);
    expect(status.state).toBe('saved');
    expect(announce).toHaveBeenCalledWith('Saved');

    vi.advanceTimersByTime(1500);
    expect(status.state).toBe('idle');
  });

  it('reports a failure in the caller’s words, and Retry runs the write again', async () => {
    const status = new SaveStatus();
    const write = vi.fn().mockRejectedValueOnce(new Error('offline')).mockResolvedValueOnce(undefined);

    expect(await status.run(write, { failed: "Couldn't save Go." })).toBe(false);
    expect(status.state).toBe('error');
    expect(status.failure).toBe("Couldn't save Go.");
    expect(announce).toHaveBeenCalledWith("Couldn't save Go.", 'assertive');

    expect(await status.retry()).toBe(true);
    expect(write).toHaveBeenCalledTimes(2);
    expect(status.state).toBe('saved');
  });

  it('lets only the newest of two overlapping writes report', async () => {
    const status = new SaveStatus();
    const older = deferred();
    const newer = deferred();
    const first = status.run(() => older.promise);
    const second = status.run(() => newer.promise);

    newer.reject(new Error('refused'));
    await second;
    expect(status.state).toBe('error');

    // The older write answering late must not paint "Saved" over the newer failure.
    older.resolve();
    await first;
    expect(status.state).toBe('error');
  });

  it('offers an Undo that runs the reversing write, then no Undo of its own', async () => {
    const status = new SaveStatus();
    const undo = vi.fn().mockResolvedValue(undefined);

    await status.run(() => Promise.resolve(), { undo });
    expect(status.canUndo).toBe(true);

    expect(await status.undo()).toBe(true);
    expect(undo).toHaveBeenCalledTimes(1);
    expect(status.state).toBe('saved');
    expect(status.canUndo).toBe(false);
  });

  it('keeps an Undo on screen longer than a plain Saved', async () => {
    const status = new SaveStatus();
    await status.run(() => Promise.resolve(), { undo: () => Promise.resolve() });

    vi.advanceTimersByTime(1500);
    expect(status.canUndo).toBe(true);
    vi.advanceTimersByTime(4500);
    expect(status.state).toBe('idle');
    expect(status.canUndo).toBe(false);
  });

  it('saves quietly without showing or saying anything', async () => {
    const status = new SaveStatus();
    expect(await status.run(() => Promise.resolve(), {}, true)).toBe(true);
    expect(status.state).toBe('idle');
    expect(announce).not.toHaveBeenCalled();
  });

  // A failure the screen cannot show — superseded by a newer write, or flushed as the
  // control went away — must still leave a trace.
  it('logs every failed write, including the ones it does not show', async () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {});
    const status = new SaveStatus();

    expect(await status.run(() => Promise.reject(new Error('shown')))).toBe(false);
    expect(await status.run(() => Promise.reject(new Error('quiet')), {}, true)).toBe(false);

    const older = deferred();
    const superseded = status.run(() => older.promise);
    void status.run(() => Promise.resolve());
    older.reject(new Error('superseded'));
    expect(await superseded).toBe(false);

    const logged = warn.mock.calls.map((call) => (call[1] as Error).message);
    expect(logged).toEqual(['shown', 'quiet', 'superseded']);
    warn.mockRestore();
  });
});
