import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { describe, expect, it, vi } from 'vitest';
import MentorScheduleEditor from './MentorScheduleEditor.svelte';
import type { AvailabilityApi, MentorAvailabilityRule } from '#lib/types.js';

const tuesday: MentorAvailabilityRule = { id: 1, weekday: 2, date: null, start: '18:00', end: '20:00', closure: false };
const away: MentorAvailabilityRule = { id: 7, weekday: null, date: '2026-12-01', start: '00:00', end: '00:00', closure: true };

function fakeSchedule() {
  return {
    replaceWeek: vi.fn<AvailabilityApi['replaceWeek']>().mockResolvedValue(undefined),
    reload: vi.fn<AvailabilityApi['reload']>().mockResolvedValue([tuesday]),
    addOverride: vi.fn<AvailabilityApi['addOverride']>().mockResolvedValue(away),
    remove: vi.fn<AvailabilityApi['remove']>().mockResolvedValue(undefined),
  } satisfies AvailabilityApi;
}

describe('MentorScheduleEditor', () => {
  // The editor serves the mentor cabinet and the buddy mock page: every write must go
  // through the schedule it was handed, never through one API it knows by name.
  it('saves the week through the schedule it was given, then re-reads it', async () => {
    const schedule = fakeSchedule();
    render(MentorScheduleEditor, { rules: [tuesday], schedule, emptyMessage: 'Nothing yet.' });

    const save = screen.getByRole('button', { name: 'Save weekly hours' }) as HTMLButtonElement;
    expect(save.disabled).toBe(true); // nothing to save until the week differs from what is stored

    await fireEvent.click(screen.getByRole('checkbox', { name: 'Thursday' }));
    expect(save.disabled).toBe(false);
    await fireEvent.click(save);

    await waitFor(() => expect(schedule.reload).toHaveBeenCalledOnce());
    expect(schedule.replaceWeek).toHaveBeenCalledWith([
      { weekday: 2, start: '18:00', end: '20:00' },
      { weekday: 4, start: '18:00', end: '20:00' },
    ]);
  });

  it('discards an unsaved edit back to the stored week', async () => {
    render(MentorScheduleEditor, { rules: [tuesday], schedule: fakeSchedule(), emptyMessage: 'Nothing yet.' });

    await fireEvent.click(screen.getByRole('checkbox', { name: 'Tuesday' }));
    expect((screen.getByRole('checkbox', { name: 'Tuesday' }) as HTMLInputElement).checked).toBe(false);
    await fireEvent.click(screen.getByRole('button', { name: 'Discard changes' }));

    expect((screen.getByRole('checkbox', { name: 'Tuesday' }) as HTMLInputElement).checked).toBe(true);
    expect((screen.getByRole('button', { name: 'Save weekly hours' }) as HTMLButtonElement).disabled).toBe(true);
  });

  it('removes an exception through the schedule it was given', async () => {
    const schedule = fakeSchedule();
    render(MentorScheduleEditor, { rules: [tuesday, away], schedule, emptyMessage: 'Nothing yet.' });

    await fireEvent.click(screen.getByRole('button', { name: 'Remove the exception on Tue 1 Dec 2026' }));

    await waitFor(() => expect(schedule.remove).toHaveBeenCalledWith(7));
  });

  it('says what an empty week means in the words of the page that hosts it', () => {
    render(MentorScheduleEditor, { rules: [], schedule: fakeSchedule(), emptyMessage: 'Nobody can book you yet.' });
    expect(screen.getByText('Nobody can book you yet.')).toBeTruthy();
  });
});
