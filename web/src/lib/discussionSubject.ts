import type { Company, Job } from './types';

/** What a discussion page prints above the thread list or the thread itself: the thing
 *  the discussion is ABOUT, as one link back to it.
 *
 *  A discussion page is reached by a slug alone, so without this the reader is told
 *  which slug they are under and nothing else — the job pages carried a bare "Back to
 *  vacancy" and the company pages printed the raw slug where a name belongs. */
export interface DiscussionSubject {
  kind: 'job' | 'company';
  /** The heading: a vacancy's title, or the company's name. */
  title: string;
  /** The employer, and the key the logo proxy resolves by. For a company subject this
   *  IS the title, so the caller prints it once. */
  company: string;
  /** Only a vacancy closes. Worth the line: the discussions that exist on vacancies
   *  are people asking whether the posting is still real, and this answers it without
   *  making them follow the link. */
  closed: boolean;
}

/** The subject line for a vacancy. */
export function jobSubject(job: Pick<Job, 'title' | 'company' | 'closed_at'>): DiscussionSubject {
  return {
    kind: 'job',
    title: job.title,
    company: job.company,
    closed: Boolean(job.closed_at),
  };
}

/** The subject line for a company. Its name is both the heading and the logo key, and
 *  a company has no closed state. */
export function companySubject(company: Pick<Company, 'name'>): DiscussionSubject {
  return { kind: 'company', title: company.name, company: company.name, closed: false };
}
