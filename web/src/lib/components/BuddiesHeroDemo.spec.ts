import { render, screen } from '@testing-library/svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { flushSync } from 'svelte';
vi.mock('$app/paths', () => ({ resolve: (p: string) => p, base: '', assets: '' }));

import BuddiesHeroDemo from './BuddiesHeroDemo.svelte';

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

const entry = (handle: string) => screen.getAllByText(handle).at(0)?.closest('li') as HTMLElement;
const slot = (id: string) => screen.getByTestId(id);

describe('BuddiesHeroDemo', () => {
  it('shows every entry and the invitation when motion is reduced', () => {
    mockMotion(true);
    render(BuddiesHeroDemo);
    expect(entry('amber-heron-42').className).toContain('opacity-100');
    expect(entry('kind-reef-2041').className).toContain('opacity-100');
    expect(slot('invite').className).not.toContain('invisible');
    expect(slot('reply').className).not.toContain('invisible');
    expect(slot('typing').className).toContain('invisible');
  });

  it('keeps the finished state once visible, then restarts from one entry and adds the next', () => {
    vi.useFakeTimers();
    mockMotion(false);
    render(BuddiesHeroDemo);
    observed?.([{ isIntersecting: true }]);
    flushSync();
    expect(slot('reply').className).not.toContain('invisible');
    vi.advanceTimersByTime(1800 * 2); // the hold ticks, matching STEP_MS and HOLD_TICKS
    flushSync();
    expect(entry('amber-heron-42').className).toContain('opacity-100');
    expect(entry('tidy-kestrel-318').className).toContain('opacity-0');
    expect(slot('invite').className).toContain('invisible');
    expect(slot('reply').className).toContain('invisible');
    vi.advanceTimersByTime(1800); // one step, matching STEP_MS in the component
    flushSync();
    expect(entry('tidy-kestrel-318').className).toContain('opacity-100');
  });

  it('shows the finished state when motion becomes reduced mid-loop', () => {
    vi.useFakeTimers();
    mockMotion(false);
    render(BuddiesHeroDemo);
    observed?.([{ isIntersecting: true }]);
    flushSync();
    vi.advanceTimersByTime(1800 * 2);
    flushSync();
    expect(slot('reply').className).toContain('invisible');
    setReduce(true);
    flushSync();
    expect(slot('reply').className).not.toContain('invisible');
    expect(entry('kind-reef-2041').className).toContain('opacity-100');
    vi.advanceTimersByTime(1800 * 3);
    flushSync();
    expect(slot('reply').className).not.toContain('invisible');
  });
});
