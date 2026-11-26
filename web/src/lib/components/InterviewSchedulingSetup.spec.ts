import { fireEvent, render, screen } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import InterviewSchedulingSetup from './InterviewSchedulingSetup.svelte';

const api = vi.hoisted(() => ({
  myAvailability: vi.fn(),
  saveAvailabilityBooking: vi.fn(),
  replaceAvailabilityWeek: vi.fn(),
  addAvailabilityDated: vi.fn(),
  deleteAvailabilityEntry: vi.fn(),
}));
vi.mock('#lib/api.js', () => ({ api }));

beforeEach(() => {
  vi.clearAllMocks();
  api.myAvailability.mockResolvedValue({
    rules: [],
    timezone: 'Europe/Berlin',
    saved: false,
    booking: { duration_min: 45, notice_hours: 24 },
  });
  api.saveAvailabilityBooking.mockResolvedValue({ duration_min: 60, notice_hours: 24 });
});

describe('InterviewSchedulingSetup', () => {
  it('continues to Google only after the user agrees', async () => {
    const onconnect = vi.fn();
    render(InterviewSchedulingSetup, { onconnect });

    const go = (await screen.findByRole('button', { name: 'Continue with Google' })) as HTMLButtonElement;
    expect(go.disabled).toBe(true);
    await fireEvent.click(screen.getByRole('checkbox', { name: /answer interview invitations/ }));
    await fireEvent.click(go);
    await vi.waitFor(() => expect(onconnect).toHaveBeenCalledOnce());
    expect(api.saveAvailabilityBooking).toHaveBeenCalledWith({
      timezone: 'Europe/Berlin',
      duration_min: 45,
      notice_hours: 24,
    });
  });

  it('saves meeting length and notice in the stored zone', async () => {
    render(InterviewSchedulingSetup);

    const length = (await screen.findByLabelText('Meeting length')) as HTMLSelectElement;
    expect(length.value).toBe('45');
    await fireEvent.change(length, { target: { value: '60' } });
    await fireEvent.click(screen.getByRole('button', { name: 'Save' }));

    expect(api.saveAvailabilityBooking).toHaveBeenCalledWith({
      timezone: 'Europe/Berlin',
      duration_min: 60,
      notice_hours: 24,
    });
    expect(screen.queryByRole('checkbox', { name: /answer interview invitations/ })).toBeNull();
  });

  it('keeps a stored notice outside the presets', async () => {
    api.myAvailability.mockResolvedValueOnce({
      rules: [],
      timezone: 'Europe/Berlin',
      saved: true,
      booking: { duration_min: 30, notice_hours: 6 },
    });
    render(InterviewSchedulingSetup);

    const notice = (await screen.findByLabelText('Shortest notice')) as HTMLSelectElement;
    expect(notice.value).toBe('6');
    await fireEvent.click(screen.getByRole('button', { name: 'Save' }));
    expect(api.saveAvailabilityBooking).toHaveBeenCalledWith({
      timezone: 'Europe/Berlin',
      duration_min: 30,
      notice_hours: 6,
    });
  });
});
