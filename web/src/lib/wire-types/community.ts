import type { components } from '#lib/generated/api.js';
/** A community discussion thread (see the add-community-threads change). The author
 *  is a pseudonymous persona handle — the backend never sends the real user id. */
export type CommunityThread = components['schemas']['Thread'];

/** A thread in the cross-subject feed (/discussions), carrying the display name of
 *  the subject it hangs off — the subject-scoped listing does not, because there the
 *  subject is already known from the route.
 *
 *  `subject_title` is the vacancy's title or the company's name; `subject_company` is
 *  the employer in both cases, and the key the logo proxy resolves by. Both are empty
 *  when the subject no longer exists (a thread outlives it — there is no FK), and the
 *  caller falls back to `subject_slug`. */
export type CommunityFeedThread = components['schemas']['RecentThread'];

/** A reply within a thread, optionally nested. `author` is a persona handle, or
 *  "AI" for a (future) system-authored reply. `parent_id` is 0 for a top-level
 *  reply, or another reply's id when nested under it. */
export type CommunityReply = components['schemas']['ThreadReply'];
