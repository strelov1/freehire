import { ApiError, MovedError } from '$lib/api';
import {
  companySubject,
  jobSubject,
  type DiscussionSubject,
  type SubjectAbsence,
} from '$lib/discussionSubject';
import { serverApi } from './api';

/** The subject a discussion page hangs off, or why it is absent. */
export interface DiscussionSubjectState {
  subject: DiscussionSubject | null;
  /** Set only when `subject` is null. */
  absence?: SubjectAbsence;
}

/** Fetch the subject a discussion page hangs off, for the header that links back to it.
 *
 *  Never throws: a discussion outlives its subject — no FK binds them and `cmd/prune`
 *  hard-deletes vacancies — so a missing subject is a state the page must survive, and
 *  the discussion is still worth reading without a header. But WHY it is missing is not
 *  one state: a pruned subject is a fact about the subject, an unreachable API is a fact
 *  about us, and printing the second as the first tells the reader the vacancy is gone
 *  when it may be fine. That is the rule the catalogue figures follow, and the one the
 *  feed's own `failed` prop encodes.
 *
 *  A merged company slug is neither. The API answers 301 and the client surfaces it as
 *  MovedError, so the canonical name is one more call away — worth making, because the
 *  page must NOT redirect: threads store the retired slug in `subject_ref`, so the
 *  canonical url would fail the subject check and 404 the very thread being read. The
 *  discussion stays where it is and the header names the company that absorbed it. */
export async function loadDiscussionSubject(
  fetchImpl: typeof fetch,
  subjectType: 'company' | 'job',
  slug: string,
): Promise<DiscussionSubjectState> {
  const api = serverApi(fetchImpl);
  try {
    if (subjectType === 'job') {
      return { subject: jobSubject(await api.getJob(slug)) };
    }
    return { subject: companySubject(await fetchCompany(api, slug)) };
  } catch (e) {
    if (e instanceof ApiError && e.status === 404) return { subject: null, absence: 'gone' };
    return { subject: null, absence: 'unreachable' };
  }
}

/** The company behind a slug, following one merge redirect by hand. One job, not none:
 *  the endpoint pages its jobs and this call wants only the company beside them — zero
 *  would be the honest ask if the API allowed it (the company page carries the same
 *  note; a company-entity-only path is its deferred follow-up). */
async function fetchCompany(api: ReturnType<typeof serverApi>, slug: string) {
  try {
    return (await api.getCompany(slug, 1, 0)).company;
  } catch (e) {
    if (e instanceof MovedError) {
      return (await api.getCompany(e.canonicalSlug, 1, 0)).company;
    }
    throw e;
  }
}
