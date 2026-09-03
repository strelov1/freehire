import { error } from '@sveltejs/kit';
import { ApiError } from '$lib/api';
import { serverApi } from './api';
import { loadDiscussionSubject } from './discussionSubject';
import { threadMatchesSubject } from './threadSubject';

/** Fetch a community thread for its detail page, scoped to the subject the route
 *  actually names — a thread that resolves to the wrong subject 404s the same as
 *  an id that resolves to nothing at all. Both discussion/[threadId] routes
 *  (companies/ and jobs/) share this.
 *
 *  The subject itself is fetched alongside, for the header that links back to it: a
 *  thread page addressed by a slug and an id otherwise says nothing about what the
 *  discussion is about. It is fetched in parallel with the thread and may come back
 *  null — a thread outlives its subject, so an absent one is a state, not an error. */
export async function loadThread(
  fetchImpl: typeof fetch,
  subjectType: 'company' | 'job',
  params: { slug: string; threadId: string },
) {
  const id = Number(params.threadId);
  if (!Number.isInteger(id)) error(404, 'Thread not found');
  const api = serverApi(fetchImpl);
  try {
    const [{ thread, replies, nextCursor }, subject] = await Promise.all([
      api.getThread(id),
      loadDiscussionSubject(fetchImpl, subjectType, params.slug),
    ]);
    if (!threadMatchesSubject(thread, subjectType, params.slug)) error(404, 'Thread not found');
    return { slug: params.slug, thread, replies, nextCursor, subject };
  } catch (e) {
    if (e instanceof ApiError && e.status === 404) error(404, 'Thread not found');
    throw e;
  }
}
