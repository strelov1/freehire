import { render, screen } from '@testing-library/svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { flushSync } from 'svelte';
import TailorHeroDemo from './TailorHeroDemo.svelte';

// One tick of the loop, as in the component. The finished state stays up for its own tick
// plus the component's two hold ticks — at the end of every loop, and so also on arrival.
const STEP_MS = 1800;
const FINISHED_TICKS = 3;

let setReduce: (reduce: boolean) => void = () => {};

function mockMotion(reduce: boolean) {
  const listeners = new Set<(e: Event) => void>();
  const mql = {
    matches: reduce,
    media: '',
    addEventListener: (_: string, fn: (e: Event) => void) => listeners.add(fn),
    removeEventListener: (_: string, fn: (e: Event) => void) => listeners.delete(fn),
  };
  setReduce = (next) => {
    mql.matches = next;
    listeners.forEach((fn) => fn(new Event('change')));
  };
  vi.stubGlobal('matchMedia', () => mql);
}

let observed: ((entries: { isIntersecting: boolean }[]) => void) | null = null;
vi.stubGlobal(
  'IntersectionObserver',
  class {
    constructor(cb: (entries: { isIntersecting: boolean }[]) => void) {
      observed = cb;
    }
    observe() {}
    disconnect() {}
  },
);

afterEach(() => {
  vi.useRealTimers();
  observed = null;
});

describe('TailorHeroDemo', () => {
  it('shows the finished example when motion is reduced', () => {
    mockMotion(true);
    render(TailorHeroDemo);
    expect(screen.getByText('86')).toBeTruthy();
    expect(screen.getByText('Migrated the payment ledger to Postgres.').className).toContain('opacity-100');
    expect(screen.getByText(/asked you first/).className).not.toContain('invisible');
  });

  // The server renders the finished state; dropping straight to the first score the moment
  // the demo scrolls into view is a visible flash above the fold.
  it('keeps the finished state once visible, then restarts from the first score and climbs', () => {
    vi.useFakeTimers();
    mockMotion(false);
    render(TailorHeroDemo);
    observed?.([{ isIntersecting: true }]);
    flushSync();
    expect(screen.getByText('86')).toBeTruthy();
    vi.advanceTimersByTime(STEP_MS * FINISHED_TICKS);
    flushSync();
    expect(screen.getByText('54')).toBeTruthy();
    vi.advanceTimersByTime(STEP_MS);
    flushSync();
    expect(screen.getByText('68')).toBeTruthy();
  });

  it('shows the finished state when motion becomes reduced mid-loop', () => {
    vi.useFakeTimers();
    mockMotion(false);
    render(TailorHeroDemo);
    observed?.([{ isIntersecting: true }]);
    vi.advanceTimersByTime(STEP_MS * (FINISHED_TICKS + 1));
    flushSync();
    expect(screen.getByText('68')).toBeTruthy();
    setReduce(true);
    flushSync();
    expect(screen.getByText('86')).toBeTruthy();
    vi.advanceTimersByTime(STEP_MS * 3);
    flushSync();
    expect(screen.getByText('86')).toBeTruthy();
  });
});
