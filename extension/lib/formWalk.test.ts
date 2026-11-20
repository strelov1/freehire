import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

const sendMessage = vi.hoisted(() => vi.fn());
vi.mock('wxt/browser', () => ({ browser: { runtime: { sendMessage } } }));

import { walkFills, walkReported, type WalkHooks } from './formWalk';

function hooks(stopAfter: number): WalkHooks & { calls: (string | null)[] } {
  const calls: (string | null)[] = [];
  let answered = 0;
  return {
    calls,
    filling: (l) => void calls.push(l),
    stopped: () => answered >= stopAfter,
    wrote: () => {},
    answered: () => void answered++,
  };
}

describe('Stop during a walk', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    sendMessage.mockReset();
    sendMessage.mockResolvedValue({ kind: 'FILL_OUTCOMES', outcomes: [{ status: 'filled' }] });
  });
  afterEach(() => vi.useRealTimers());

  it('walkFills stops after the step in flight and clears the spinner', async () => {
    const h = hooks(1);
    const p = walkFills(
      [
        { label: 'A', value: '1' },
        { label: 'B', value: '2' },
      ],
      h,
    );
    const walk = await vi.runAllTimersAsync().then(() => p);
    expect(sendMessage.mock.calls.filter(([m]) => m.kind === 'FILL_BY_LABEL')).toHaveLength(1);
    expect(walk.stopped).toBe(true);
    expect(walk.done).toHaveLength(1);
    expect(h.calls.at(-1)).toBeNull();
  });

  it('walkReported stops replaying the report', async () => {
    const h = hooks(1);
    const p = walkReported(['A', 'B', 'C'], h);
    await vi.runAllTimersAsync();
    await p;
    expect(sendMessage.mock.calls.filter(([m]) => m.kind === 'REVEAL_FIELD')).toHaveLength(1);
    expect(h.calls.at(-1)).toBeNull();
  });
});
