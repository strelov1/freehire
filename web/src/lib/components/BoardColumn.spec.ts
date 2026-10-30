import { fireEvent, render, screen } from '@testing-library/svelte';
import { describe, expect, it, vi } from 'vitest';
import type { BoardItem } from '#lib/board.js';
import BoardColumn from './BoardColumn.svelte';

vi.mock('$app/state', () => ({
  page: { data: { locale: 'en' }, url: new URL('http://localhost/my/tracking'), state: {} },
}));
vi.mock('$app/paths', () => ({ resolve: (p: string) => p, base: '', assets: '' }));

const card = (n: number): BoardItem => ({
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

const items = Array.from({ length: 70 }, (_, i) => card(i));

function renderColumn(onfinalize = vi.fn(), cards = items) {
  return {
    onfinalize,
    ...render(BoardColumn, {
      props: { id: 'applied', label: 'Applied', items: cards, onconsider: vi.fn(), onfinalize, onopen: vi.fn() },
    }),
  };
}

const drawn = () => screen.queryAllByText(/^Role \d+$/).length;

// The zone is the scroll container: the one holding the cards' wrappers.
const zone = () => must(screen.getByText('Role 0').closest('section')?.querySelector('.overflow-y-auto'));
function must<T>(v: T | null | undefined): T {
  if (!v) throw new Error('missing');
  return v;
}

describe('BoardColumn', () => {
  it('draws the first page of a long column, and its full count', () => {
    // jsdom lays nothing out (clientHeight 0), which is also the guard that keeps a
    // column with no layout from drawing everything.
    renderColumn();

    expect(drawn()).toBe(30);
    expect(screen.getByText('70')).toBeTruthy();
  });

  it('draws the next page when the column is scrolled near its end', async () => {
    renderColumn();
    const el = zone() as HTMLElement;
    Object.defineProperties(el, {
      clientHeight: { value: 600, configurable: true },
      scrollHeight: { value: 3000, configurable: true },
      scrollTop: { value: 2100, configurable: true },
    });

    await fireEvent.scroll(el);

    // The stub's height does not grow with the cards, so every check after a page lands
    // still reads "near the end" and the column fills; a real one stops once it reaches
    // past the viewport.
    await vi.waitFor(() => expect(drawn()).toBe(70));
  });

  it('keeps the undrawn cards when a drop reorders the drawn ones', async () => {
    const { onfinalize } = renderColumn();
    const reordered = [items[1], items[0], ...items.slice(2, 30)];

    await fireEvent(zone(), new CustomEvent('finalize', { detail: { items: reordered, info: {} } }));

    const written = must(onfinalize.mock.calls[0])[1] as BoardItem[];
    expect(written).toHaveLength(70);
    expect(written.slice(0, 2).map((i) => i.id)).toEqual(['app-1', 'app-0']);
    expect(written.slice(30).map((i) => i.id)).toEqual(items.slice(30).map((i) => i.id));
  });

  it('draws a card dropped onto a column whose first page is exactly full', async () => {
    const page = items.slice(0, 30);
    const dropped = card(99);
    const { onfinalize, rerender } = renderColumn(vi.fn(), page);

    await fireEvent(zone(), new CustomEvent('finalize', { detail: { items: [...page, dropped], info: {} } }));
    const written = must(onfinalize.mock.calls[0])[1] as BoardItem[];
    await rerender({ items: written });

    expect(written).toHaveLength(31);
    expect(screen.getByText('Role 99')).toBeTruthy();
  });

  it('resumes drawing once a dragged card leaves the column or the drag is cancelled', async () => {
    renderColumn();
    const el = zone() as HTMLElement;
    const consider = (trigger: string) =>
      fireEvent(el, new CustomEvent('consider', { detail: { items: items.slice(0, 30), info: { trigger } } }));
    Object.defineProperties(el, {
      clientHeight: { value: 600, configurable: true },
      scrollHeight: { value: 3000, configurable: true },
      scrollTop: { value: 2100, configurable: true },
    });

    for (const trigger of ['draggedLeft', 'dragStopped']) {
      await consider('dragStarted');
      await consider(trigger);
    }
    await fireEvent.scroll(el);

    await vi.waitFor(() => expect(drawn()).toBeGreaterThan(30));
  });
});
