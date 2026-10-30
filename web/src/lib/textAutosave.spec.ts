import { describe, expect, it, vi } from 'vitest';
import { SaveStatus } from './saveStatus.svelte';
import { TextAutosave } from './textAutosave';

vi.mock('$app/state', () => ({ page: { data: { locale: 'en' }, url: new URL('http://localhost/') } }));

function deferred() {
  let resolve!: () => void;
  const promise = new Promise<void>((r) => (resolve = r));
  return { promise, resolve };
}

const settle = () => new Promise((resolve) => setTimeout(resolve, 0));

// One text box over a store. `holding` is what keeps a card from copying the saved value
// back over its boxes, so a card stuck holding stops following Undo and fresh reads.
function card(save: (value: string) => Promise<unknown>) {
  const box = { value: '' };
  const store = { value: '' };
  const status = new SaveStatus();
  const autosave = new TextAutosave<string>({
    status,
    read: () => box.value,
    saved: () => store.value,
    save: async (value) => {
      await save(value);
      store.value = value;
    },
  });
  return { box, status, autosave };
}

describe('TextAutosave', () => {
  // The store still holds the value from before the first save while that save is in
  // flight. Compared with it, a box reverted in the meantime looks unchanged — so it was
  // skipped, and the first save then landed the value the candidate had just removed.
  it('sends a revert made while the first save is still in flight', async () => {
    let box = '';
    let stored = '';
    const first = deferred();
    const save = vi
      .fn<(v: string) => Promise<void>>()
      .mockImplementationOnce(async (v) => {
        await first.promise;
        stored = v;
      })
      .mockImplementation(async (v) => {
        stored = v;
      });
    const autosave = new TextAutosave<string>({
      status: new SaveStatus(),
      read: () => box,
      saved: () => stored,
      save,
    });

    box = '120000';
    autosave.edited();
    autosave.commit();
    box = '';
    autosave.edited();
    autosave.commit();
    first.resolve();
    await vi.waitFor(() => expect(save).toHaveBeenCalledTimes(2));

    expect(save).toHaveBeenLastCalledWith('');
  });

  it('does not resend the value already on its way', async () => {
    let box = '';
    const first = deferred();
    const save = vi.fn<(v: string) => Promise<void>>().mockImplementation(() => first.promise);
    const autosave = new TextAutosave<string>({
      status: new SaveStatus(),
      read: () => box,
      saved: () => '',
      save,
    });

    box = 'a';
    autosave.edited();
    autosave.commit();
    autosave.edited();
    autosave.commit();

    expect(save).toHaveBeenCalledTimes(1);
    first.resolve();
  });

  // SaveStatus.retry() re-runs the stored write without passing through commit(), so the
  // write itself has to release the card. #3898 dropped that as a no-op; this is the path.
  it('stops holding once a failed save succeeds on Retry', async () => {
    const save = vi.fn().mockRejectedValueOnce(new Error('offline')).mockResolvedValue(undefined);
    const { box, status, autosave } = card(save);

    box.value = 'Ada';
    autosave.edited();
    autosave.commit();
    await settle();
    expect(status.state).toBe('error');
    expect(autosave.holding).toBe(true);

    expect(await status.retry()).toBe(true);
    expect(autosave.holding).toBe(false);
  });

  it('keeps holding typing that landed while the retry was out', async () => {
    let finish!: () => void;
    const save = vi
      .fn()
      .mockRejectedValueOnce(new Error('offline'))
      .mockImplementationOnce(() => new Promise<void>((resolve) => (finish = resolve)));
    const { box, status, autosave } = card(save);

    box.value = 'Ada';
    autosave.edited();
    autosave.commit();
    await settle();

    const retried = status.retry();
    box.value = 'Ada L';
    autosave.edited();
    finish();
    await retried;
    expect(autosave.holding).toBe(true);
  });
});
