import type { Http } from '../api-core';
import type { MentorSlots } from '../types';
import type { components } from '#lib/generated/api.js';
import { jsonBody } from '../api-core';

type Schemas = components['schemas'];
type BookingLinkBooking = Schemas['BookingLinkBooking'];

export function bookingApi({ request, requestData }: Http) {
  /** Turn on the caller's booking link for one application; answers its public URL, the
   *  same one on every call. 402 for a free account. */
  async function enableBookingLink(applicationId: number, timezone: string): Promise<string> {
    const out = await requestData<Schemas['BookingLinkOut']>(
      `/api/v1/me/applications/${applicationId}/booking-link`,
      jsonBody('POST', { timezone } satisfies Schemas['BookingLinkEnableBody']),
    );
    return out.url;
  }

  /** A booking link's free slots, in the viewer's zone (UTC when unknown; `timezone` says
   *  which). No account needed; 404 for a link that does not resolve. */
  async function getBookingSlots(token: string, timezone: string, signal?: AbortSignal): Promise<MentorSlots> {
    const params = new URLSearchParams();
    if (timezone) params.set('tz', timezone);
    const res = await request<Schemas['MentorshipSlotsPage']>(
      `/api/v1/booking/${encodeURIComponent(token)}/slots?${params}`,
      { signal },
    );
    return { slots: res.data, timezone: res.meta.timezone };
  }

  /** Book one slot as a recruiter. 409 `slot_unavailable` when it is gone. */
  async function bookBookingSlot(token: string, body: Schemas['BookingLinkBookBody']): Promise<BookingLinkBooking> {
    return requestData<BookingLinkBooking>(
      `/api/v1/booking/${encodeURIComponent(token)}/bookings`,
      jsonBody('POST', body satisfies Schemas['BookingLinkBookBody']),
    );
  }

  return { enableBookingLink, getBookingSlots, bookBookingSlot };
}
