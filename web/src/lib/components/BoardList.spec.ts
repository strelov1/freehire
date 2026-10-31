import { render, screen } from '@testing-library/svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { flushSync } from 'svelte';
import type { BoardItem } from '#lib/board.js';
import BoardList from './BoardList.svelte';

vi.mock('$app/state', () => ({
  page: { data: { locale: 'en' }, url: new URL('http://localhost/my/tracking/list'), state: {} },
}));

// The sentinel's observer, captured so a test can scroll it into view. A real observer
// also reports once when it starts observing; `inView` makes the stub do the same.
let reveal: (() => void) | undefined;
let inView = false;
vi.stubGlobal(
  'IntersectionObserver',
  class {
    cb: (e: { isIntersecting: boolean }[]) => void;
    constructor(cb: (e: { isIntersecting: boolean }[]) => void) {
      this.cb = cb;
    }
    observe() {
      reveal = () => this.cb([{ isIntersecting: true }]);
      if (inView) queueMicrotask(reveal);
    }
    disconnect() {}
  },
);
afterEach(() => {
  reveal = undefined;
  inView = false;
});

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

  it('keeps drawing while the end stays in view on a tall viewport', async () => {
    // The sentinel never leaves the viewport, so no observer sees it ENTER; only one
    // started after each page reports it.
    inView = true;
    render(BoardList, { props: { items: Array.from({ length: 120 }, (_, i) => row(i)), onopen: vi.fn(), onsetstage: vi.fn() } });

    await vi.waitFor(() => expect(drawn()).toBe(120));
  });

  it('offers a load-more control for keyboard and screen-reader users', async () => {
    render(BoardList, { props: { items: Array.from({ length: 60 }, (_, i) => row(i)), onopen: vi.fn(), onsetstage: vi.fn() } });

    screen.getByRole('button', { name: 'Load more' }).click();
    flushSync();

    expect(drawn()).toBe(60);
    expect(screen.queryByRole('button', { name: 'Load more' })).toBeNull();
  });
});
