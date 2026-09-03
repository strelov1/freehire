// The header dropdown's row model.
//
// The dropdown holds three sections — what to search (suggestions), what exists
// (postings), and who is hiring (companies) — plus a final free-text row. The
// keyboard has to run through all of them as ONE list, and every off-by-one in arrow
// navigation lives in that flattening. So it is a function with tests rather than
// three index offsets computed inside the template.
//
// Pure by design: no Svelte, no DOM, no network.

import type { Suggestion } from './suggestions';
import type { Job, CompanyListItem } from './types';

/** One navigable row. `first` marks a section's opening row, which is where its
 *  heading renders — so the heading is a property of the row rather than a second
 *  structure the index has to be reconciled against. */
export type DropdownRow = { key: string; first: boolean } & (
  | { kind: 'suggestion'; suggestion: Suggestion }
  | { kind: 'job'; job: Job }
  | { kind: 'company'; company: CompanyListItem }
  | { kind: 'text'; text: string }
);

/** Keep only the companies the typed text actually NAMES.
 *
 *  The companies endpoint matches fuzzily, which is right for the companies page —
 *  where a near miss is a helpful result among many — and wrong for a dropdown
 *  section three rows tall. Observed in the browser: `product own` offered
 *  Energise.pro, CLOVERDALE FOODS CO and GreenStar Food Co+op, none of which have
 *  anything to do with the query, and three such rows make the whole dropdown look
 *  broken.
 *
 *  Plain substring, so a half-typed name still matches (`goog` → Google) and so does
 *  a word inside one (`reject` → The Reject Shop). Precision over recall: a company
 *  the visitor is actually looking for is one more keystroke away, while a wrong row
 *  is there immediately.
 *
 *  Deliberately NOT pushed into the endpoint: fuzzy matching is correct for its other
 *  caller, and this is a rule about what a three-row section may show. */
export function namedCompanies(
  companies: readonly CompanyListItem[],
  text: string,
  limit = 3,
): CompanyListItem[] {
  const q = text.trim().toLowerCase();
  if (q === '') return [];
  return companies.filter((c) => c.name.toLowerCase().includes(q)).slice(0, limit);
}

export interface DropdownContent {
  suggestions: readonly Suggestion[];
  jobs: readonly Job[];
  companies: readonly CompanyListItem[];
  /** What is typed in the box. Empty means no free-text row: there is nothing to
   *  offer searching for. */
  text: string;
}

/** Flatten the sections into the single list the keyboard walks.
 *
 *  A section with nothing in it contributes no rows at all, so an empty postings
 *  section leaves no gap in the numbering and no orphan heading. */
export function dropdownRows(content: DropdownContent): DropdownRow[] {
  const rows: DropdownRow[] = [];

  content.suggestions.forEach((suggestion, i) => {
    rows.push({
      kind: 'suggestion',
      suggestion,
      // Keys are namespaced by section: `backend` is a plausible role slug AND a
      // plausible company slug, and a duplicate key in an {#each} takes the page down
      // rather than merely rendering oddly.
      key: `s:${suggestion.kind}:${suggestion.slug}`,
      first: i === 0,
    });
  });

  content.jobs.forEach((job, i) => {
    rows.push({ kind: 'job', job, key: `j:${job.public_slug}`, first: i === 0 });
  });

  content.companies.forEach((company, i) => {
    rows.push({ kind: 'company', company, key: `c:${company.slug}`, first: i === 0 });
  });

  const text = content.text.trim();
  if (text !== '') rows.push({ kind: 'text', text, key: 't', first: true });

  return rows;
}
