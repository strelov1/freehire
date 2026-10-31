import { render, screen, waitFor } from '@testing-library/svelte';
import { createRawSnippet } from 'svelte';
import { describe, expect, it, vi } from 'vitest';
import MentorBookingView from './MentorBookingView.svelte';
import type { MentorSlot, MentorSlots } from '#lib/types.js';

const url = vi.hoisted(() => ({ current: new URL('http://localhost/buddies/x?tab=mock') }));
vi.mock('$app/state', () => ({
  page: {
    get url() {
      return url.current;
    },
  },
}));
vi.mock('$app/navigation', () => ({ replaceState: vi.fn() }));

// Frozen at the last day of a month whose remaining hours are all inside a host's notice
// period: the first free hour is next month, which is what a visitor arriving now must see.
vi.useFakeTimers({ now: new Date('2026-10-31T12:00:00Z'), toFake: ['Date'] });

const november: MentorSlot = {
  starts_at: '2026-11-02T18:00:00Z',
  ends_at: '2026-11-02T19:00:00Z',
  local_start: '2026-11-02T18:00:00Z',
  local_end: '2026-11-02T19:00:00Z',
  utc_offset: '+00:00',
};

function loader(slots: MentorSlot[]) {
  return vi.fn(async (): Promise<MentorSlots> => ({ slots, timezone: 'UTC' }));
}

const booker = createRawSnippet(() => ({ render: () => '<span></span>' }));

function view(loadSlots: ReturnType<typeof loader>) {
  return render(MentorBookingView, { loadSlots, sessionMinutes: 60, counterpart: 'calm-otter-11', booker });
}

describe('MentorBookingView', () => {
  it('opens on the first month with an open hour when the current month has none', async () => {
    url.current = new URL('http://localhost/buddies/x?tab=mock');
    const loadSlots = loader([november]);
    view(loadSlots);

    await waitFor(() => expect(screen.getByText('November 2026')).toBeTruthy());
    // The opening read already spans both months, so the jump is one extra read at most.
    expect(loadSlots.mock.calls[0]?.slice(0, 2)).toEqual(['2026-09-30T00:00:00Z', '2026-12-01T00:00:00Z']);
    expect(loadSlots).toHaveBeenCalledTimes(2);
  });

  it('keeps a month the address names, even an empty one', async () => {
    url.current = new URL('http://localhost/mentors/x?month=2026-10');
    const loadSlots = loader([november]);
    view(loadSlots);

    await waitFor(() => expect(loadSlots).toHaveBeenCalled());
    expect(screen.getByText('October 2026')).toBeTruthy();
    expect(loadSlots.mock.calls[0]?.slice(0, 2)).toEqual(['2026-09-30T00:00:00Z', '2026-11-01T00:00:00Z']);
  });

  it('stays on the current month when nothing is open anywhere', async () => {
    url.current = new URL('http://localhost/buddies/x?tab=mock');
    const loadSlots = loader([]);
    view(loadSlots);

    await waitFor(() => expect(loadSlots).toHaveBeenCalled());
    expect(screen.getByText('October 2026')).toBeTruthy();
    expect(loadSlots).toHaveBeenCalledTimes(1);
  });
});
