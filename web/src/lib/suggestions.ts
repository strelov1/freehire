// What the header search box offers, and what an EMPTY box offers in particular.
//
// The dropdown used to hold one kind of thing (roles) and so needed no vocabulary for
// "what sort of suggestion is this". It now holds two, and will hold more once the
// suggestions index lands — so `Suggestion` is the shape the dropdown renders and
// applies, independent of where the rows came from.
//
// Pure by design: no Svelte, no DOM, no network. The caller passes the facet
// distribution it has already fetched.

import { CATEGORY_GROUPS } from './filterSections';
import type { FacetCounts } from './types';

/** One row of the dropdown. `kind` is the vocabulary the row comes from, which decides
 *  its glyph — and, for the locally-built starter rows, which facet a pick applies.
 *  Rows from the suggestions endpoint carry their own parts and are applied from
 *  those, so there `kind` is presentation only. */
export interface Suggestion {
  kind: 'title' | 'role' | 'skill' | 'category' | 'company';
  slug: string;
  label: string;
  /** Open postings behind it. Absent when the distribution has not been measured yet
   *  — an absent measurement must not render as a zero. */
  count?: number;
}

/** How many rows an empty box offers. Ten is what a visitor will actually read. */
const maxStarters = 10;

/** Rows each group contributes before the next group is reached.
 *
 *  One would be a flat map of the catalogue, and the catalogue is only half a tech
 *  catalogue: measured against production, one-per-group spent five of the ten rows on
 *  Management, Sales, HR, Operations and Healthcare. Two spends the budget on the
 *  groups the curated order puts first, which is who this is for. Those categories
 *  stay one keystroke away — this list is a starting point, not the vocabulary. */
const perGroup = 2;

/** The catch-all category. A real facet value and a useless suggestion: it names no
 *  craft, so it cannot answer the question an empty box is asking. */
const catchAll = 'other';

/** What to offer when the box is empty and the visitor does not know what they may
 *  type. Categories, in the curated group order the filter modal already renders —
 *  Engineering, Data & AI, Quality & Security, and so on, with the consumer
 *  industries last.
 *
 *  NOT the busiest values. Measured on the live catalogue those are Management
 *  (266,883), Sales (179,993) and Support (127,110): a tech job board that opens by
 *  offering those reads as somebody else's website.
 *
 *  And not a flat walk of that order either. Engineering alone carries 13 categories,
 *  so taking the first ten in sequence spends every row on it and never reaches a
 *  designer or a PM. Instead each group contributes its busiest category before any
 *  group contributes a second — Engineering still leads, but the list spans the map. */
export function starterSuggestions(
  counts: FacetCounts | null,
  limit: number = maxStarters,
): Suggestion[] {
  const dist = counts?.facets?.category;
  if (!dist) return [];

  // Each group's own options, busiest first. A category the distribution does not
  // carry is dropped rather than offered with a zero: it would lead to an empty page.
  const groups = CATEGORY_GROUPS.map((section) =>
    section.options
      .filter((o) => o.value !== catchAll && Object.hasOwn(dist, o.value))
      .sort((a, b) => (dist[b.value] ?? 0) - (dist[a.value] ?? 0)),
  ).filter((options) => options.length > 0);

  const out: Suggestion[] = [];
  for (const options of groups) {
    for (const option of options.slice(0, perGroup)) {
      out.push({
        kind: 'category',
        slug: option.value,
        label: option.label,
        count: dist[option.value],
      });
      if (out.length === limit) return out;
    }
  }
  return out;
}
