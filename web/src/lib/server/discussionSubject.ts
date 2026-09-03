import { companySubject, jobSubject, type DiscussionSubject } from '$lib/discussionSubject';
import { serverApi } from './api';

/** Fetch the subject a discussion page hangs off, for the header that links back to it.
 *
 *  Returns null rather than throwing when the subject cannot be read. A thread outlives
 *  its subject — no FK binds them and `cmd/prune` hard-deletes vacancies — so a 404 here
 *  is a state the page must survive, not an error: the discussion is still readable and
 *  the caller falls back to the slug. An unreachable API degrades the same way, since a
 *  header is not what the reader came for. */
export async function loadDiscussionSubject(
  fetchImpl: typeof fetch,
  subjectType: 'company' | 'job',
  slug: string,
): Promise<DiscussionSubject | null> {
  const api = serverApi(fetchImpl);
  try {
    if (subjectType === 'job') {
      return jobSubject(await api.getJob(slug));
    }
    // One job, not none: the endpoint pages its jobs and this call wants only the
    // company beside them. Zero would be the honest ask if the API allowed it.
    const { company } = await api.getCompany(slug, 1, 0);
    return companySubject(company);
  } catch {
    return null;
  }
}
