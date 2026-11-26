import type { Http } from '../api-core';
import type { MentorAvailabilityRule } from '../types';
import type { components } from '#lib/generated/api.js';
import { jsonBody } from '../api-core';

type Schemas = components['schemas'];
export type Availability = Schemas['AvailabilityOut'];

/** The caller's one availability calendar, shared by mentorship, buddy mocks, booking links
 *  and the interview scheduling agent. A `timezone` sent with a write becomes the zone of
 *  every product. */
export function availabilityApi({ call, requestData }: Http) {
  async function myAvailability(): Promise<Availability> {
    return requestData<Availability>('/api/v1/me/availability');
  }

  async function replaceAvailabilityWeek(
    rules: { weekday: number; start: string; end: string }[],
    timezone?: string,
  ): Promise<void> {
    await call(
      '/api/v1/me/availability/weekly',
      jsonBody('PUT', { rules, timezone } satisfies Schemas['AvailabilityWeeklyBody']),
    );
  }

  async function addAvailabilityDated(
    date: string,
    start: string,
    end: string,
    timezone?: string,
  ): Promise<MentorAvailabilityRule> {
    return requestData<MentorAvailabilityRule>(
      '/api/v1/me/availability/overrides',
      jsonBody('POST', { date, start, end, timezone } satisfies Schemas['AvailabilityRuleBody']),
    );
  }

  async function deleteAvailabilityEntry(id: number): Promise<void> {
    await call(`/api/v1/me/availability/${id}`, { method: 'DELETE' });
  }

  /** Booking-link meeting length and notice. 402 for a free account, 422 for a bad value. */
  async function saveAvailabilityBooking(
    body: Schemas['AvailabilityBookingBody'],
  ): Promise<Schemas['AvailabilityBookingOut']> {
    return requestData('/api/v1/me/availability/booking', jsonBody('PUT', body));
  }

  return {
    myAvailability,
    replaceAvailabilityWeek,
    addAvailabilityDated,
    deleteAvailabilityEntry,
    saveAvailabilityBooking,
  };
}
