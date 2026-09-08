// The mentor cabinet's tab structure: four real routes sharing one strip
// (`/my/mentorship/+layout.svelte`). Which one a pathname is on is the shared rule in
// routeTabs.ts.
//
// Real routes rather than a client-side view switch, following the notification centre and
// `/my/profile`, `/my/activity` and `/my/tracking`. Two reasons beyond consistency: each
// pane can be linked to and reloaded, and each `load` fetches only its own data — the one
// page this replaced read the profile, the availability, the bookings and the sessions on
// every visit, including for a visitor who only wanted to see when their next session is.
//
// The strip appears only for somebody who HAS a mentor profile. A seeker has exactly one
// thing in this section, and a row of tabs where three of them are about being a mentor
// invites them to wonder what they are missing.

export type MentorshipTabId = 'sessions' | 'bookings' | 'profile' | 'schedule';

// `as const` keeps each href a literal route so callers can pass it to `resolve()`
// type-safely (mirroring accountNav.ts's own use of the pattern).
export const MENTORSHIP_TABS = [
  { id: 'sessions', label: 'Your sessions', href: '/my/mentorship' },
  { id: 'bookings', label: 'Booked with you', href: '/my/mentorship/bookings' },
  { id: 'profile', label: 'Profile', href: '/my/mentorship/profile' },
  { id: 'schedule', label: 'Schedule', href: '/my/mentorship/schedule' },
] as const satisfies readonly { id: MentorshipTabId; label: string; href: string }[];
