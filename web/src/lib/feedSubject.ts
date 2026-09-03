import type { CommunityFeedThread } from './types';

/** Everything a feed row's subject rail prints. */
export interface FeedSubjectLine {
  /** Which kind of thing the thread hangs off, in the site's own words. Shown on
   *  EVERY row, not only the ambiguous one: a marker present on one type and absent
   *  on the other reads as a property of that row rather than as its type. */
  kind: 'Vacancy' | 'Company';
  /** The bold half, and the key the logo proxy resolves by. */
  employer: string;
  /** The truncating half — a vacancy's title. Empty for a company subject. */
  posting: string;
  /** False when `employer` is the stored slug standing in for a subject that no longer
   *  resolves. The caller renders it as an identifier rather than as a name, so a
   *  slug is not mistaken for what the employer is called. */
  resolved: boolean;
}

/** Shown when the subject resolved but carries no employer name — a posting whose
 *  `jobs.company` is empty. Matches the wording JobRow already uses for the same gap,
 *  so one absent company reads the same way wherever it surfaces. */
const UNKNOWN_EMPLOYER = 'Unknown company';

/** What a feed row prints above its title.
 *
 *  A thread carries only its subject's slug and no FK to it, so the server resolves
 *  the names on read and sends them empty when the subject is gone (a thread outlives
 *  its subject — cmd/prune hard-deletes vacancies). Both cases land here:
 *
 *  - a vacancy thread reads "Vacancy · <company> · <posting title>";
 *  - a company thread reads "Company · <company>", since its subject IS the company;
 *  - an unresolved subject falls back to the stored slug, flagged unresolved, so the
 *    row stays readable and linkable instead of printing nothing.
 *
 *  A pure function rather than logic inside the component because that is the only
 *  form this test environment can exercise — it has no Svelte runtime for runes. */
export function feedSubjectLine(t: CommunityFeedThread): FeedSubjectLine {
  // Resolution is decided by `subject_title`, not by `subject_company`: the server
  // fills the title from the vacancy OR the company, so a non-empty one means the
  // subject was found. Keying on the company instead conflates "the subject is gone"
  // with "the subject is here but its jobs.company column is empty" — a real state
  // in this catalogue — and printed a live posting's slug as its employer.
  const resolved = t.subject_title !== '';
  return {
    kind: t.subject_type === 'company' ? 'Company' : 'Vacancy',
    employer: resolved ? t.subject_company || UNKNOWN_EMPLOYER : t.subject_slug,
    // Only a vacancy has a posting title distinct from its employer; for a company
    // subject `subject_title` IS the company name and would print twice.
    posting: t.subject_type === 'job' ? t.subject_title : '',
    resolved,
  };
}
