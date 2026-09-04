// Turning the suggestions endpoint's rows into what the dropdown renders and applies.
//
// The endpoint completes a PHRASE, so one row can name several things at once —
// "Senior Software Engineer Google" carries the specialization and the company. That is the
// whole reason this file exists rather than the component mapping the response
// inline: choosing what a row LOOKS like and what picking it DOES are two decisions
// with edges, and both are testable without a Svelte runtime.

import type { Suggestion } from './suggestions';
import type { ApiSuggestion, ApiSuggestionPart } from './types';

/** The facet each suggestion kind applies. `title` is absent: it names no facet and
 *  becomes the free-text query instead — no facet value spells "Product Owner". */
const facetFor: Partial<Record<ApiSuggestionPart['kind'], string>> = {
  skill: 'skills',
  category: 'category',
  company: 'company_slug',
};

/** What picking a row does: the facet values to set, and the free text to search. */
export interface ApplyPlan {
  facets: [param: string, value: string][];
  q?: string;
}

/** Everything a row names, applied together.
 *
 *  Applying one part of two would silently discard what the visitor typed — the
 *  composed search is the point, not a bonus. */
export function applyParams(parts: readonly ApiSuggestionPart[]): ApplyPlan {
  const plan: ApplyPlan = { facets: [] };
  for (const part of parts) {
    if (part.kind === 'title') {
      plan.q = part.text;
      continue;
    }
    const param = facetFor[part.kind];
    // A facet part with no value is malformed. Dropping it beats writing
    // `category=undefined` into the URL, which reads as a filter that matches nothing.
    if (param && part.slug) plan.facets.push([param, part.slug]);
  }
  return plan;
}

/** Render the endpoint's rows as dropdown suggestions.
 *
 *  The kind comes from the LAST part — the completion, the thing this row actually
 *  adds. The earlier parts are context the visitor has already typed, so taking the
 *  glyph from them would label every row after the first by its prefix. */
export function fromApi(rows: readonly ApiSuggestion[]): Suggestion[] {
  return rows.map((row, i) => {
    const last = row.parts.at(-1);
    return {
      kind: last?.kind ?? 'category',
      // The index is part of the key because two rows CAN complete to the same slug
      // under different kinds — `backend` is a plausible category and a plausible skill —
      // and a duplicate {#each} key takes the page down rather than rendering oddly.
      slug: `${i}:${last?.kind ?? ''}:${last?.slug ?? row.text}`,
      label: row.text,
      count: row.jobs,
    };
  });
}
