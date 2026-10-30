import { beforeEach, describe, expect, it, vi } from 'vitest';

// Two dropdowns picked a moment apart send two saves; the last pick must be the one that
// stays. A save answering after sign-out must not leave the old account's answers behind.

const { getScreeningAnswers, updateScreeningAnswers } = vi.hoisted(() => ({
  getScreeningAnswers: vi.fn(),
  updateScreeningAnswers: vi.fn(),
}));
vi.mock('#lib/api.js', () => ({ api: { getScreeningAnswers, updateScreeningAnswers } }));

const { screeningStore } = await import('./screening.svelte');

function deferred<T>() {
  let resolve!: (v: T) => void;
  const promise = new Promise<T>((r) => (resolve = r));
  return { promise, resolve };
}

beforeEach(() => {
  getScreeningAnswers.mockReset().mockResolvedValue(null);
  updateScreeningAnswers.mockReset();
  screeningStore.reset();
});

describe('screeningStore.save', () => {
  it('sends saves one at a time, in the order they were made', async () => {
    const first = deferred<object>();
    updateScreeningAnswers.mockReturnValueOnce(first.promise).mockResolvedValueOnce({ willing_to_relocate: true });

    const a = screeningStore.save({ willing_to_relocate: false });
    const b = screeningStore.save({ willing_to_relocate: true });
    await Promise.resolve();
    expect(updateScreeningAnswers).toHaveBeenCalledTimes(1);

    first.resolve({ willing_to_relocate: false });
    await Promise.all([a, b]);
    expect(updateScreeningAnswers).toHaveBeenNthCalledWith(2, { willing_to_relocate: true });
    expect(screeningStore.answers).toEqual({ willing_to_relocate: true });
  });

  it('does not send a save still queued when the account signs out', async () => {
    const first = deferred<object>();
    updateScreeningAnswers.mockReturnValueOnce(first.promise);

    const a = screeningStore.save({ age_18_or_older: true });
    const b = screeningStore.save({ willing_to_relocate: true });
    await Promise.resolve();
    screeningStore.reset();
    first.resolve({ age_18_or_older: true });
    await Promise.all([a, b]);

    expect(updateScreeningAnswers).toHaveBeenCalledTimes(1);
  });

  it('drops an answer that lands after sign-out', async () => {
    const pending = deferred<object>();
    updateScreeningAnswers.mockReturnValueOnce(pending.promise);

    const saving = screeningStore.save({ age_18_or_older: true });
    await Promise.resolve();
    screeningStore.reset();
    pending.resolve({ age_18_or_older: true });
    await saving;

    expect(screeningStore.answers).toBeNull();
  });
});
