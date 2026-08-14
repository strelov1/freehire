import { error } from '@sveltejs/kit';
import { ApiError } from '$lib/api';
import { serverApi } from './api';
import { threadMatchesSubject } from './threadSubject';

/** Fetch a community thread for its detail page, scoped to the subject the route
 *  actually names — a thread that resolves to the wrong subject 404s the same as
 *  an id that resolves to nothing at all. Both discussion/[threadId] routes
 *  (companies/ and jobs/) share this. */
export async function loadThread(
  fetchImpl: typeof fetch,
  subjectType: 'company' | 'job',
  params: { slug: string; threadId: string },
) {
  const id = Number(params.threadId);
  if (!Number.isInteger(id)) error(404, 'Thread not found');
  const api = serverApi(fetchImpl);
  try {
    const { thread, replies, nextCursor } = await api.getThread(id);
    if (!threadMatchesSubject(thread, subjectType, params.slug)) error(404, 'Thread not found');
    return { slug: params.slug, thread, replies, nextCursor };
  } catch (e) {
    if (e instanceof ApiError && e.status === 404) error(404, 'Thread not found');
    throw e;
  }
}
