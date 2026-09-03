import { UNKNOWN_EMPLOYER } from './feedSubject';
import type { Company, Job } from './types';

/** What a discussion page prints above the thread list or the thread itself: the thing
 *  the discussion is ABOUT, as one link back to it.
 *
 *  A discussion page is reached by a slug alone, so without this the reader is told
 *  which slug they are under and nothing else — the job pages carried a bare "Back to
 *  vacancy" and the company pages printed the raw slug where a name belongs.
 *
 *  There is no `kind` here on purpose: the route already knows which subject it is
 *  under, and it knows it even when this could not be loaded. A second copy would be
 *  one the two could disagree on. */
export interface DiscussionSubject {
  /** The heading: a vacancy's title, or the company's name. */
  title: string;
  /** The employer's real name, and ONLY that: the key the logo proxy resolves by, empty
   *  when there is none. Never the stand-in below — the proxy would go and fetch a mark
   *  for the literal "Unknown company". For a company subject this IS the title, so the
   *  caller prints it once. */
  company: string;
  /** What to print for the employer: the name, or the stand-in when there is none. */
  employerLabel: string;
  /** Only a vacancy closes. Worth the line: the discussions that exist on vacancies
   *  are people asking whether the posting is still real, and this answers it without
   *  making them follow the link. */
  closed: boolean;
}

/** Why a subject is absent. `gone` is a statement about the subject — pruned, or never
 *  there. `unreachable` is a statement about US, and the two must not be printed as the
 *  same thing: the feed learned that lesson one change ago. */
export type SubjectAbsence = 'gone' | 'unreachable';

/** The subject line for a vacancy. A posting with no recorded employer still resolved —
 *  only its company name is missing — so it is named, not treated as absent. */
export function jobSubject(job: Pick<Job, 'title' | 'company' | 'closed_at'>): DiscussionSubject {
  return {
    title: job.title,
    company: job.company,
    employerLabel: job.company || UNKNOWN_EMPLOYER,
    closed: Boolean(job.closed_at),
  };
}

/** The subject line for a company. Its name is both the heading and the logo key, and
 *  a company has no closed state. */
export function companySubject(company: Pick<Company, 'name'>): DiscussionSubject {
  return {
    title: company.name,
    company: company.name,
    employerLabel: company.name,
    closed: false,
  };
}
