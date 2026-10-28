import { render, screen } from '@testing-library/svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { flushSync } from 'svelte';
vi.mock('$app/paths', () => ({ resolve: (p: string) => p, base: '', assets: '' }));

import BuddiesHeroDemo from './BuddiesHeroDemo.svelte';

function mockMotion(reduce: boolean) {
  vi.stubGlobal('matchMedia', (q: string) => ({ matches: reduce && q.includes('reduce'), media: q }));
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

  it('starts from one entry once visible, then adds the next', () => {
    vi.useFakeTimers();
    mockMotion(false);
    render(BuddiesHeroDemo);
    observed?.([{ isIntersecting: true }]);
    flushSync();
    expect(entry('amber-heron-42').className).toContain('opacity-100');
    expect(entry('tidy-kestrel-318').className).toContain('opacity-0');
    expect(slot('invite').className).toContain('invisible');
    expect(slot('reply').className).toContain('invisible');
    vi.advanceTimersByTime(1800); // one step, matching STEP_MS in the component
    flushSync();
    expect(entry('tidy-kestrel-318').className).toContain('opacity-100');
  });
});
