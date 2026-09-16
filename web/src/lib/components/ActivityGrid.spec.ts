import { fireEvent, render, screen } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import type { TimelineEvent } from '$lib/types';
import ActivityGrid from './ActivityGrid.svelte';

// The model's own arithmetic is covered in activityGrid.test.ts, in two timezones. What
// is covered HERE is the contract between the model and the screen — the three things a
// reader would notice broken and the unit tests could never see:
//
//  - a day whose only event was an employer's reply is an EMPTY square whose panel still
//    lists that reply (the counting rule reaching the pixels, not just the number);
//  - selecting a day issues no request, because the whole window is already in hand;
//  - a server payload paints on first render, without a fetch on mount.

const { myTimeline } = vi.hoisted(() => ({ myTimeline: vi.fn() }));

// `locale()` reads page.data, so the page has to exist even though nothing here asserts on
// it — without this the component throws inside its first $derived and every case fails with
// a message about `locale` rather than about the grid.
vi.mock('$app/state', () => ({ page: { data: {}, url: new URL('http://localhost/my/tracking/activity') } }));
vi.mock('$app/paths', () => ({ resolve: (p: string) => p, base: '', assets: '' }));
vi.mock('$lib/api', () => ({ api: { myTimeline } }));

/** An event at local midday on a calendar date, so no assertion here depends on the zone the
 *  suite happens to run in. */
const on = (day: string, kind: string, source: string, over: Partial<TimelineEvent> = {}): TimelineEvent => {
  const [y = 0, m = 1, d = 1] = day.split('-').map(Number);
  return {
    id: 1,
    kind,
    source,
    observed: true,
    occurred_at: new Date(y, m - 1, d, 12, 0, 0).toISOString(),
    company_slug: 'acme',
    ...over,
  } as TimelineEvent;
};

/** A local calendar date n days before today, so the fixtures always land inside the window
 *  however long this test lives. */
const daysAgo = (n: number): string => {
  const now = new Date();
  const d = new Date(now.getFullYear(), now.getMonth(), now.getDate() - n);
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
};

/** The squares. Every one carries its own date in its accessible name, and only real days
 *  are buttons — the padding cells are inert. */
const squareFor = (day: string): HTMLElement => {
  const label = new Date(`${day}T12:00:00`).toLocaleDateString(undefined, {
    weekday: 'long',
    day: 'numeric',
    month: 'long',
    year: 'numeric',
  });
  const square = screen.getAllByRole('button').find((b) => b.getAttribute('aria-label')?.endsWith(label));
  if (!square) throw new Error(`no square for ${day} — the window should always contain it`);
  return square;
};

beforeEach(() => {
  myTimeline.mockReset();
});

describe('ActivityGrid', () => {
  it('paints the server payload without fetching', () => {
    render(ActivityGrid, { prefetched: [on(daysAgo(1), 'applied', 'user')] });

    expect(myTimeline).not.toHaveBeenCalled();
    expect(screen.getByText('Your last year')).toBeTruthy();
    expect(screen.getByText('1 action')).toBeTruthy();
  });

  // The whole feature's premise on screen: effort is shaded, everything else is history.
  it('leaves a day of employer replies unshaded and still lists them', async () => {
    const day = daysAgo(3);
    render(ActivityGrid, {
      prefetched: [on(day, 'employer_reply', 'mail_gmail', { email_subject: 'Thanks for applying' })],
    });

    const square = squareFor(day);
    expect(square.getAttribute('aria-label')).toContain('No actions');

    await fireEvent.click(square);

    expect(screen.getByText('Employer replied')).toBeTruthy();
    // …and selecting it asked nobody anything.
    expect(myTimeline).not.toHaveBeenCalled();
  });

  it('closes the panel when the open day is selected again', async () => {
    const day = daysAgo(2);
    render(ActivityGrid, { prefetched: [on(day, 'applied', 'user')] });

    const square = squareFor(day);
    await fireEvent.click(square);
    expect(screen.getByText('Applied')).toBeTruthy();

    await fireEvent.click(square);
    expect(screen.queryByText('Applied')).toBeNull();
  });

  it('fetches the window itself when the server load did not answer', async () => {
    myTimeline.mockResolvedValue([on(daysAgo(1), 'applied', 'user')]);

    render(ActivityGrid, { prefetched: undefined });
    await vi.waitFor(() => expect(screen.getByText('1 action')).toBeTruthy());

    expect(myTimeline).toHaveBeenCalledTimes(1);
    // The span it asks for has to be one the endpoint will answer: over
    // apptimeline.MaxRangeDays it refuses outright, and every reader sees the error state.
    const [from, to] = myTimeline.mock.calls[0] as [string, string];
    expect((Date.parse(to) - Date.parse(from)) / 86_400_000).toBeLessThanOrEqual(366);
  });

  it('says so rather than showing a blank grid when nothing was fetched', async () => {
    myTimeline.mockRejectedValue(new Error('down'));

    render(ActivityGrid, { prefetched: undefined });

    await vi.waitFor(() => expect(screen.getByText("Couldn't load your activity.")).toBeTruthy());
  });

  it('invites a caller with an empty year rather than scolding them', () => {
    render(ActivityGrid, { prefetched: [] });

    expect(screen.getByText(/Nothing here yet/)).toBeTruthy();
    // Both streak figures, current and longest — an empty year has neither.
    expect(screen.getAllByText('0 days')).toHaveLength(2);
  });
});
