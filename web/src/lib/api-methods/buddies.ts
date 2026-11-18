import type { Http } from '../api-core';
import type { components } from '../generated/api.js';
import type {
  BuddyActivity,
  BuddyFeedPage,
  BuddyMock,
  BuddyMockHost,
  BuddyMockHosts,
  BuddyMocks,
  BuddyMockSettings,
  BuddyMockSlots,
  BuddyMockSummary,
  BuddyMockTopic,
  BuddyProfile,
  BuddyReactions,
  BuddySimilar,
  BuddyStatus,
  Conversation,
  MentorAvailabilityRule,
  MentorSlot,
  ReactionKind,
} from '../types';
import { jsonBody } from '../api-core';

export function buddiesApi({ call, request, requestData }: Http) {
  // ── Interview buddies ────────────────────────────────────────────────────
  async function buddyStatus(): Promise<BuddyStatus> {
    return requestData('/api/v1/me/buddies');
  }

  async function joinBuddies(): Promise<BuddyStatus> {
    return requestData('/api/v1/me/buddies', { method: 'POST' });
  }

  async function leaveBuddies(): Promise<void> {
    await call('/api/v1/me/buddies', { method: 'DELETE' });
  }

  async function listBuddySimilar(limit = 20, offset = 0): Promise<BuddySimilar[]> {
    return requestData(`/api/v1/me/buddies/similar?limit=${limit}&offset=${offset}`);
  }

  async function listBuddyFeed(mode: 'similar' | 'all', cursor?: string, limit = 30): Promise<BuddyFeedPage> {
    const q = new URLSearchParams({ mode, limit: String(limit) });
    if (cursor) q.set('cursor', cursor);
    return request<BuddyFeedPage>(`/api/v1/me/buddies/feed?${q}`);
  }

  /** One member's own activity: the same feed, narrowed to their handle. */
  async function listBuddyMemberFeed(handle: string, cursor?: string, limit = 30): Promise<BuddyFeedPage> {
    const q = new URLSearchParams({ handle, limit: String(limit) });
    if (cursor) q.set('cursor', cursor);
    return request<BuddyFeedPage>(`/api/v1/me/buddies/feed?${q}`);
  }

  /** Sets the caller's one reaction to a feed event (replacing another); the event's reactions after it. */
  async function reactToBuddyEvent(id: number, kind: ReactionKind): Promise<BuddyReactions> {
    return requestData(
      `/api/v1/me/buddies/feed/${id}/reaction`,
      jsonBody('PUT', { kind } satisfies components['schemas']['BuddyReactBody']),
    );
  }

  async function clearBuddyReaction(id: number): Promise<BuddyReactions> {
    return requestData(`/api/v1/me/buddies/feed/${id}/reaction`, { method: 'DELETE' });
  }

  async function getBuddyProfile(handle: string): Promise<BuddyProfile> {
    return requestData(`/api/v1/me/buddies/profiles/${encodeURIComponent(handle)}`);
  }

  async function getBuddyActivity(handle: string): Promise<BuddyActivity> {
    return requestData(`/api/v1/me/buddies/profiles/${encodeURIComponent(handle)}/activity`);
  }

  async function listBuddyHiddenApplications(): Promise<number[]> {
    return requestData('/api/v1/me/buddies/hidden-applications');
  }

  async function setBuddyApplicationHidden(id: string, hidden: boolean): Promise<void> {
    await call(`/api/v1/me/buddies/hidden-applications/${encodeURIComponent(id)}`, {
      method: hidden ? 'PUT' : 'DELETE',
    });
  }

  async function inviteBuddy(handle: string, body: string): Promise<Conversation> {
    return requestData(
      '/api/v1/me/buddies/invite',
      jsonBody('POST', { handle, body } satisfies components['schemas']['BuddyInviteBody']),
    );
  }

  // ── Buddy mock interviews ────────────────────────────────────────────────
  async function getBuddyMockSettings(): Promise<BuddyMockSettings | null> {
    return requestData('/api/v1/me/buddies/mocks/settings');
  }

  async function saveBuddyMockSettings(settings: BuddyMockSettings): Promise<BuddyMockSettings> {
    return requestData('/api/v1/me/buddies/mocks/settings', jsonBody('PUT', settings));
  }

  async function myBuddyMockAvailability(): Promise<MentorAvailabilityRule[]> {
    return requestData('/api/v1/me/buddies/mocks/availability');
  }

  /** The whole recurring week, as the mentor schedule's PUT. 204, so callers re-read. */
  async function replaceBuddyMockWeekly(rules: { weekday: number; start: string; end: string }[]): Promise<void> {
    await call('/api/v1/me/buddies/mocks/availability/weekly', jsonBody('PUT', { rules }));
  }

  async function addBuddyMockOverride(date: string, start: string, end: string): Promise<MentorAvailabilityRule> {
    return requestData('/api/v1/me/buddies/mocks/availability/overrides', jsonBody('POST', { date, start, end }));
  }

  async function deleteBuddyMockAvailabilityRule(id: number): Promise<void> {
    await call(`/api/v1/me/buddies/mocks/availability/${id}`, { method: 'DELETE' });
  }

  /** A member's open mock slots in the viewer's zone and the topics they host. The answer's
   *  `timezone` is the zone actually used, as with mentor slots. Never carries the link. */
  async function getBuddyMockSlots(
    handle: string,
    from: string,
    to: string,
    timezone: string,
    signal?: AbortSignal,
  ): Promise<BuddyMockSlots> {
    const params = new URLSearchParams({ from, to });
    if (timezone) params.set('timezone', timezone);
    const res = await request<{ data: MentorSlot[]; meta: { timezone: string; topics: BuddyMockTopic[] } }>(
      `/api/v1/me/buddies/profiles/${encodeURIComponent(handle)}/mock-slots?${params}`,
      { signal },
    );
    return { slots: res.data, timezone: res.meta.timezone, topics: res.meta.topics };
  }

  async function bookBuddyMock(
    handle: string,
    body: { starts_at: string; topic: BuddyMockTopic; timezone: string },
  ): Promise<BuddyMock> {
    return requestData(`/api/v1/me/buddies/profiles/${encodeURIComponent(handle)}/mocks`, jsonBody('POST', body));
  }

  /** The members the caller could book a mock with now, soonest slot first, optionally
   *  narrowed to one topic. `timezone` is the zone the slots were resolved in. */
  async function listBuddyMockHosts(timezone: string, topic?: BuddyMockTopic): Promise<BuddyMockHosts> {
    const params = new URLSearchParams();
    if (timezone) params.set('timezone', timezone);
    if (topic) params.set('topic', topic);
    const res = await request<{ data: BuddyMockHost[]; meta: { timezone: string; count: number } }>(
      `/api/v1/me/buddies/mocks/hosts?${params}`,
    );
    return { hosts: res.data, timezone: res.meta.timezone };
  }

  async function listBuddyMocks(): Promise<BuddyMocks> {
    return requestData('/api/v1/me/buddies/mocks');
  }

  async function cancelBuddyMock(id: string): Promise<void> {
    await call(`/api/v1/me/buddies/mocks/${encodeURIComponent(id)}/cancel`, { method: 'POST' });
  }

  async function reviewBuddyMock(id: string, rating: number, comment: string): Promise<void> {
    await call(
      `/api/v1/me/buddies/mocks/${encodeURIComponent(id)}/review`,
      jsonBody('POST', { rating, comment } satisfies components['schemas']['BuddyMockReviewBody']),
    );
  }

  async function getBuddyMockSummary(handle: string): Promise<BuddyMockSummary> {
    return requestData(`/api/v1/me/buddies/profiles/${encodeURIComponent(handle)}/mock-reviews`);
  }

  return {
    buddyStatus,
    joinBuddies,
    leaveBuddies,
    listBuddySimilar,
    listBuddyFeed,
    listBuddyMemberFeed,
    reactToBuddyEvent,
    clearBuddyReaction,
    getBuddyProfile,
    getBuddyActivity,
    listBuddyHiddenApplications,
    setBuddyApplicationHidden,
    inviteBuddy,
    getBuddyMockSettings,
    saveBuddyMockSettings,
    myBuddyMockAvailability,
    replaceBuddyMockWeekly,
    addBuddyMockOverride,
    deleteBuddyMockAvailabilityRule,
    getBuddyMockSlots,
    bookBuddyMock,
    listBuddyMockHosts,
    listBuddyMocks,
    cancelBuddyMock,
    reviewBuddyMock,
    getBuddyMockSummary,
  };
}
