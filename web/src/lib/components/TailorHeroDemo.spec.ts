import { render, screen } from '@testing-library/svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { flushSync } from 'svelte';
import TailorHeroDemo from './TailorHeroDemo.svelte';

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

describe('TailorHeroDemo', () => {
  it('shows the finished example when motion is reduced', () => {
    mockMotion(true);
    render(TailorHeroDemo);
    expect(screen.getByText('86')).toBeTruthy();
    expect(screen.getByText('Migrated the payment ledger to Postgres.').className).toContain('opacity-100');
    expect(screen.getByText(/asked you first/).className).not.toContain('invisible');
  });

  it('restarts from the first score once visible, then climbs', () => {
    vi.useFakeTimers();
    mockMotion(false);
    render(TailorHeroDemo);
    observed?.([{ isIntersecting: true }]);
    flushSync();
    expect(screen.getByText('54')).toBeTruthy();
    vi.advanceTimersByTime(1800); // one step — keep in step with STEP_MS in the component
    flushSync();
    expect(screen.getByText('68')).toBeTruthy();
  });
});
