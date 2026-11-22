import { render } from '@testing-library/svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import FeedToast from './FeedToast.svelte';

afterEach(() => vi.useRealTimers());

function setHidden(v: boolean) {
  Object.defineProperty(document, 'hidden', { configurable: true, value: v });
  document.dispatchEvent(new Event('visibilitychange'));
}

describe('FeedToast', () => {
  it('stays while the tab is hidden and expires after the time left', async () => {
    vi.useFakeTimers();
    const onClose = vi.fn();
    render(FeedToast, { message: 'x', onClose });
    await vi.advanceTimersByTimeAsync(2000);
    setHidden(true);
    await vi.advanceTimersByTimeAsync(6000);
    expect(onClose).not.toHaveBeenCalled();
    setHidden(false);
    await vi.advanceTimersByTimeAsync(2900);
    expect(onClose).not.toHaveBeenCalled();
    await vi.advanceTimersByTimeAsync(200);
    expect(onClose).toHaveBeenCalledOnce();
  });
});
