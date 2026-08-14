import type { CommunityThread } from '$lib/types';

/** Whether a resolved thread actually belongs to the subject its detail-page URL
 *  names. `getThread` resolves by id alone, so a syntactically valid id that
 *  belongs to a different company, a different job, or the other subject kind
 *  entirely would otherwise "load" fine under that URL — the route's own params
 *  never factor into the lookup at all. Kept in its own dependency-free module
 *  (no `$env`, no `serverApi`) so this one piece of actual logic stays
 *  unit-testable without the SvelteKit server plumbing around it. */
export function threadMatchesSubject(
  thread: Pick<CommunityThread, 'subject_type' | 'subject_slug'>,
  subjectType: 'company' | 'job',
  subjectSlug: string,
): boolean {
  return thread.subject_type === subjectType && thread.subject_slug === subjectSlug;
}
