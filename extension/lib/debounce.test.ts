import { describe, it, expect, vi } from 'vitest';
import { debounce } from './debounce';

describe('debounce', () => {
  it('calls once for a burst, after the quiet period', async () => {
    vi.useFakeTimers();
    const calls: number[] = [];
    const fn = debounce(() => calls.push(1), 400);

    fn();
    fn();
    fn();
    expect(calls).toHaveLength(0);

    vi.advanceTimersByTime(399);
    expect(calls).toHaveLength(0);
    vi.advanceTimersByTime(1);
    expect(calls).toHaveLength(1);

    vi.useRealTimers();
  });

  // An ATS form fires input events for as long as the user types. Each keystroke
  // must push the call out, or a slow typist gets one re-read mid-word and none
  // at the end.
  it('restarts the wait on every call', () => {
    vi.useFakeTimers();
    const calls: number[] = [];
    const fn = debounce(() => calls.push(1), 400);

    fn();
    vi.advanceTimersByTime(300);
    fn();
    vi.advanceTimersByTime(300);
    expect(calls).toHaveLength(0);

    vi.advanceTimersByTime(100);
    expect(calls).toHaveLength(1);

    vi.useRealTimers();
  });

  it('runs again for a later burst', () => {
    vi.useFakeTimers();
    const calls: number[] = [];
    const fn = debounce(() => calls.push(1), 100);

    fn();
    vi.advanceTimersByTime(100);
    fn();
    vi.advanceTimersByTime(100);

    expect(calls).toHaveLength(2);
    vi.useRealTimers();
  });
});
