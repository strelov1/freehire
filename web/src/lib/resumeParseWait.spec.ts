import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

// A CV upload answers before its background parse has run, and the profile's cards and tab
// dots read what that parse writes. waitForParse follows it until it settles, so the
// caller knows when re-reading is worth it.

const { getResume } = vi.hoisted(() => ({ getResume: vi.fn() }));
vi.mock('#lib/api.js', () => ({ api: { getResume } }));

const { resumeStore } = await import('./resume.svelte');

const meta = (parse_status: string) => ({ enabled: true, present: true, parse_status });

beforeEach(() => {
  vi.useFakeTimers();
  getResume.mockReset();
  resumeStore.reset();
});

afterEach(() => {
  vi.useRealTimers();
});

describe('resumeStore.waitForParse', () => {
  it('keeps reading until the parse lands, then resolves true', async () => {
    getResume
      .mockResolvedValueOnce(meta('pending'))
      .mockResolvedValueOnce(meta('pending'))
      .mockResolvedValueOnce(meta('done'));

    const parsed = resumeStore.waitForParse();
    await vi.runAllTimersAsync();

    await expect(parsed).resolves.toBe(true);
    expect(getResume).toHaveBeenCalledTimes(3);
    expect(resumeStore.meta?.parse_status).toBe('done');
  });

  it('resolves false when the parse failed', async () => {
    getResume.mockResolvedValueOnce(meta('pending')).mockResolvedValueOnce(meta('failed'));

    const parsed = resumeStore.waitForParse();
    await vi.runAllTimersAsync();

    await expect(parsed).resolves.toBe(false);
  });

  it('stops, resolving false, once the user signs out', async () => {
    getResume.mockResolvedValue(meta('pending'));

    const parsed = resumeStore.waitForParse();
    await vi.advanceTimersByTimeAsync(0);
    resumeStore.reset();
    await vi.runAllTimersAsync();

    await expect(parsed).resolves.toBe(false);
    expect(getResume).toHaveBeenCalledTimes(1);
  });

  it('gives up, resolving false, when the parse outlasts the wait', async () => {
    getResume.mockResolvedValue(meta('pending'));

    const parsed = resumeStore.waitForParse();
    await vi.runAllTimersAsync();

    await expect(parsed).resolves.toBe(false);
  });
});
