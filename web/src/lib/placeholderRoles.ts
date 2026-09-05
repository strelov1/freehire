// The rotating examples the jobs search box offers when it is empty and untouched.
//
// The list is category KEYS, not display strings. A hand-written ['QA', 'DevOps'] would
// be a second copy of a vocabulary the backend already owns, and it would rot silently —
// a category renamed or retired upstream would leave the box confidently offering a value
// the feed can no longer filter on. Typing the array as Category[] turns that into a
// build failure, the same guard CATEGORY_LABELS uses for the same reason.
//
// The module composes whole placeholder strings rather than exposing bare labels for a
// component to interpolate: the phrasing is then testable and lives in one place, and
// HeaderSearch stays a box that renders what it is handed instead of learning that one of
// its callers is about jobs.
//
// Pure by design (no Svelte, no DOM), like facetModel.ts and suggestions.ts beside it.

import type { Category } from './generated/contracts';
import { categoryLabel } from './labels';

/** The roles the box cycles through, in rotation order.
 *
 *  Busiest-looking first: under prefers-reduced-motion the rotation never starts, so the
 *  first entry is the only one anyone sees and carries the whole weight of answering
 *  "what can I type here". Six is about what a visitor will sit through before typing. */
export const PLACEHOLDER_ROLES: Category[] = [
  'backend',
  'frontend',
  'devops',
  'qa',
  'data_science',
  'product',
];

/** The full placeholder strings for the jobs search box, in rotation order. */
export function rolePlaceholders(): string[] {
  return PLACEHOLDER_ROLES.map((role) => `Search jobs — e.g. ${categoryLabel(role)}`);
}
