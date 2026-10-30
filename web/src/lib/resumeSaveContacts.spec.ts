import { beforeEach, describe, expect, it, vi } from 'vitest';

// Three cards save the same contact block, and each save replaces the whole block. Two
// saves started a moment apart must not both be built from the same copy, or the second
// puts back what the first replaced.

const { getResume, putResumeContacts } = vi.hoisted(() => ({ getResume: vi.fn(), putResumeContacts: vi.fn() }));
vi.mock('#lib/api.js', () => ({ api: { getResume, putResumeContacts } }));

const { resumeStore } = await import('./resume.svelte');

let stored: Record<string, unknown>;

beforeEach(async () => {
  stored = { full_name: 'Ada', headline: 'Engineer' };
  getResume.mockReset().mockImplementation(() => Promise.resolve({ enabled: true, present: true, contacts: stored }));
  putResumeContacts.mockReset().mockImplementation(async (body: Record<string, unknown>) => {
    stored = body;
    return body;
  });
  resumeStore.reset();
  await resumeStore.refresh();
});

describe('resumeStore.saveContacts', () => {
  it('builds each save from the block the one before it saved', async () => {
    const first = resumeStore.saveContacts({ full_name: 'Ada Lovelace' });
    const second = resumeStore.saveContacts({ headline: 'Mathematician' });
    await Promise.all([first, second]);

    expect(putResumeContacts).toHaveBeenCalledTimes(2);
    expect(putResumeContacts).toHaveBeenNthCalledWith(2, { full_name: 'Ada Lovelace', headline: 'Mathematician' });
    expect(resumeStore.meta?.contacts).toEqual({ full_name: 'Ada Lovelace', headline: 'Mathematician' });
  });

  it('refuses a save still queued when the account signs out', async () => {
    let release!: () => void;
    putResumeContacts.mockImplementationOnce(
      (body: Record<string, unknown>) => new Promise((r) => (release = () => r(body))),
    );
    const first = resumeStore.saveContacts({ full_name: 'X' });
    const second = resumeStore.saveContacts({ headline: 'Y' });
    await Promise.resolve();
    resumeStore.reset();
    release();
    await first;

    await expect(second).rejects.toThrow('signed out');
    expect(putResumeContacts).toHaveBeenCalledTimes(1);
  });

  it('lets a failed save reject its caller without blocking the next', async () => {
    putResumeContacts.mockRejectedValueOnce(new Error('offline'));

    await expect(resumeStore.saveContacts({ full_name: 'X' })).rejects.toThrow('offline');
    await expect(resumeStore.saveContacts({ headline: 'Y' })).resolves.toEqual({ full_name: 'Ada', headline: 'Y' });
  });
});
