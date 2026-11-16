import type { Mentor, MentorSlot } from './types';

// Fixtures shared by mentorship's tests.

export function slot(localStart: string, utcOffset = '+00:00'): MentorSlot {
  return {
    starts_at: '2026-10-15T09:00:00Z',
    ends_at: '2026-10-15T10:00:00Z',
    local_start: localStart,
    local_end: localStart,
    utc_offset: utcOffset,
  };
}

export function mentor(over: Partial<Mentor> = {}): Mentor {
  return {
    slug: 'anna-k',
    name: 'Anna K.',
    company_slug: 'acme',
    company_name: 'Acme',
    headline: 'Staff Engineer',
    bio: '',
    topics: ['career'],
    languages: ['en'],
    timezone: 'Europe/Berlin',
    session_minutes: 60,
    rating_count: 0,
    rating_avg: 0,
    show_photo: false,
    ...over,
  };
}
