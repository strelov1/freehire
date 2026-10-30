import { describe, expect, it, vi } from 'vitest';
import { SaveStatus } from './saveStatus.svelte';
import { TextAutosave } from './textAutosave';

vi.mock('$app/state', () => ({ page: { data: { locale: 'en' }, url: new URL('http://localhost/') } }));

function deferred() {
  let resolve!: () => void;
  const promise = new Promise<void>((r) => (resolve = r));
  return { promise, resolve };
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
});
