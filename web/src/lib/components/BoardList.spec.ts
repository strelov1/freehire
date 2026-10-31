import { render, screen } from '@testing-library/svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { flushSync } from 'svelte';
import type { BoardItem } from '#lib/board.js';
import BoardList from './BoardList.svelte';

vi.mock('$app/state', () => ({
  page: { data: { locale: 'en' }, url: new URL('http://localhost/my/tracking/list'), state: {} },
}));

// The sentinel's observer, captured so a test can scroll it into view.
let reveal: (() => void) | undefined;
vi.stubGlobal(
  'IntersectionObserver',
  class {
    constructor(private cb: (e: { isIntersecting: boolean }[]) => void) {}
    observe() {
      reveal = () => this.cb([{ isIntersecting: true }]);
    }
    disconnect() {}
  },
);
afterEach(() => (reveal = undefined));

const row = (n: number): BoardItem => ({
  id: `app-${n}`,
  company_slug: 'acme',
  role_title: `Role ${n}`,
  job: null,
  viewed_at: '2026-10-01T00:00:00Z',
  saved_at: null,
  applied_at: '2026-10-01T00:00:00Z',
  stage: 'applied',
  notes: null,
  email_count: 0,
  last_activity_at: null,
  days_silent: null,
  silence_state: null,
  followed_up_at: null,
  cv_opened_at: null,
});

const drawn = () => screen.queryAllByText(/^Role \d+$/).length;

describe('BoardList', () => {
  it('draws the first page, then the next when the end scrolls into view', () => {
    render(BoardList, { props: { items: Array.from({ length: 120 }, (_, i) => row(i)), onopen: vi.fn(), onsetstage: vi.fn() } });
    expect(drawn()).toBe(50);

    reveal?.();
    flushSync();

    expect(drawn()).toBe(100);
  });

  it('draws a short list whole, with nothing left to load', () => {
    render(BoardList, { props: { items: [row(1), row(2)], onopen: vi.fn(), onsetstage: vi.fn() } });

    expect(drawn()).toBe(2);
    expect(reveal).toBeUndefined();
  });
});
