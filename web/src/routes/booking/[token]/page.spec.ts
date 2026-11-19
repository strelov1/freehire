import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { MentorSlot } from '#lib/types.js';
import BookingPage from './+page.svelte';

const { getBookingSlots, bookBookingSlot, StubApiError } = vi.hoisted(() => ({
  getBookingSlots: vi.fn(),
  bookBookingSlot: vi.fn(),
  StubApiError: class extends Error {
    constructor(
      public status: number,
      message = 'failed',
    ) {
      super(message);
    }
  },
}));

vi.mock('$app/state', () => ({
  page: { url: new URL('http://localhost/booking/tok?month=2026-11&date=2026-11-02&slot=2026-11-02T10:00:00Z') },
}));
vi.mock('$app/navigation', () => ({ replaceState: vi.fn() }));
vi.mock('#lib/api.js', () => ({ api: { getBookingSlots, bookBookingSlot }, ApiError: StubApiError }));

vi.useFakeTimers({ now: new Date('2026-10-31T12:00:00Z'), toFake: ['Date'] });

const at = (hour: string): MentorSlot => ({
  starts_at: `2026-11-02T${hour}:00:00Z`,
  ends_at: `2026-11-02T${hour}:30:00Z`,
  local_start: `2026-11-02T${hour}:00:00Z`,
  local_end: `2026-11-02T${hour}:30:00Z`,
  utc_offset: '+00:00',
});
const ten = at('10');
const eleven = at('11');

async function submit(): Promise<void> {
  await fireEvent.input(screen.getByLabelText('Your name'), { target: { value: 'Ann' } });
  await fireEvent.input(screen.getByLabelText('Your work email'), { target: { value: 'ann@acme.test' } });
  await fireEvent.click(screen.getByRole('button', { name: 'Book this time' }));
}

const slotButton = (time: string) =>
  screen.getAllByTestId('mentor-slot').find((b) => b.textContent?.includes(time)) as HTMLElement;

describe('booking page', () => {
  beforeEach(() => {
    getBookingSlots.mockReset();
    bookBookingSlot.mockReset();
  });

  it('keeps a 409 notice after the taken slot leaves the list, and never shows it against another slot', async () => {
    getBookingSlots.mockResolvedValueOnce({ slots: [ten, eleven], timezone: 'UTC' });
    getBookingSlots.mockResolvedValue({ slots: [eleven], timezone: 'UTC' });
    bookBookingSlot.mockRejectedValue(new StubApiError(409));
    render(BookingPage, { data: { token: 'tok' } as never });

    await waitFor(() => screen.getByRole('button', { name: 'Book this time' }));
    await submit();

    await waitFor(() => expect(screen.getByRole('alert').textContent).toContain('was just taken'));
    await waitFor(() => expect(screen.queryByRole('button', { name: 'Book this time' })).toBeNull());

    await fireEvent.click(slotButton('11'));
    expect(screen.getByRole('alert').textContent).toContain('10');
    expect(screen.getByRole('button', { name: 'Book this time' })).toBeTruthy();
  });

  it('shows a non-409 error only against the slot it was about', async () => {
    getBookingSlots.mockResolvedValue({ slots: [ten, eleven], timezone: 'UTC' });
    bookBookingSlot.mockRejectedValue(new StubApiError(500, 'server exploded'));
    render(BookingPage, { data: { token: 'tok' } as never });

    await waitFor(() => screen.getByRole('button', { name: 'Book this time' }));
    await submit();
    await waitFor(() => expect(screen.getByText('server exploded')).toBeTruthy());

    await fireEvent.click(slotButton('11'));
    expect(screen.queryByText('server exploded')).toBeNull();
  });
});
