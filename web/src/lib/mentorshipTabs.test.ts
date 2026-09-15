import { describe, expect, test } from 'vitest';
import { MENTORSHIP_TABS } from './mentorshipTabs';

describe('the mentorship cabinet tab order', () => {
  // Profile is a new mentor's first task after approval, so it leads the strip rather
  // than sitting third behind Sessions and Bookings.
  test('Profile is first', () => {
    expect(MENTORSHIP_TABS[0]?.id).toBe('profile');
  });

  test('Schedule is still last, and Sessions/Bookings keep their relative order', () => {
    expect(MENTORSHIP_TABS.map((t) => t.id)).toEqual(['profile', 'sessions', 'bookings', 'schedule']);
  });
});
