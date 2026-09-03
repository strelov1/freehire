import type { CommunityFeedThread } from './types';

/** The two halves of a feed row's subject line.
 *
 *  `employer` is the bold half and the key the logo proxy resolves by; `posting` is
 *  the truncating half. */
export interface FeedSubjectLine {
  employer: string;
  posting: string;
}

/** What a feed row prints above its title.
 *
 *  A thread carries only its subject's slug and no FK to it, so the server resolves
 *  the names on read and sends them empty when the subject is gone (a thread outlives
 *  its subject — cmd/prune hard-deletes vacancies). Both cases land here:
 *
 *  - a vacancy thread reads "<company> · <posting title>";
 *  - a company thread reads "<company>", since its subject IS the company;
 *  - an unresolved subject falls back to the stored slug, so the row stays readable
 *    and linkable instead of printing nothing.
 *
 *  A pure function rather than logic inside the component because that is the only
 *  form this test environment can exercise — it has no Svelte runtime for runes. */
export function feedSubjectLine(t: CommunityFeedThread): FeedSubjectLine {
  return {
    employer: t.subject_company || t.subject_slug,
    // Only a vacancy has a posting title distinct from its employer; for a company
    // subject `subject_title` IS the company name and would print twice.
    posting: t.subject_type === 'job' ? t.subject_title : '',
  };
}
