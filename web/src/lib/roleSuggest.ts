// Role suggestions for the header search box. The jobs feed's search is free text,
// but most of what people type into it names a role the `role` facet already tags
// deterministically — so we offer that facet at the moment of typing instead of
// leaving it to be found in the filter modal.
//
// Pure by design: no Svelte, no DOM, no network. The role catalogue and its aliases
// already ship to the browser in generated/contracts.ts, and the caller passes the
// facet distribution it has already fetched, so a suggestion costs no round-trip.

import { ROLE_ALIASES, ROLE_LABELS } from './generated/contracts';
import { baseRole, optionMatches, roleLabel } from './facets';
import type { FacetCounts } from './types';

/** One role offered under the header search box. `count` is the role's open-vacancy
 *  figure, absent when the facet distribution has not been measured yet. */
export interface RoleSuggestion {
  slug: string;
  label: string;
  count?: number;
}

// The catalogue as facet options, built once: optionMatches — the role picker's own
// matcher — takes an option, and reusing it is what keeps "does this query name this
// role" a single behaviour rather than two that drift apart. Labels come through
// roleLabel for the same reason: it is what the picker renders, so the dropdown and
// the filter chip cannot end up naming one role two ways.
const roleOptions = Object.keys(ROLE_LABELS).map((value) => ({ value, label: roleLabel(value) }));

/** How many roles the dropdown offers. Past a handful the list stops being a
 *  shortcut and starts being a second filter panel. */
const maxSuggestions = 5;

/** Shortest query worth matching. One character matches most of the catalogue and
 *  an empty one matches all of it (fuzzyMatch treats an empty query as matching
 *  everything), so the floor belongs here rather than in each caller. */
const minQueryLength = 2;

// ROLE_ALIASES is generated with literal keys and no index signature, so it cannot be
// indexed by a computed slug without widening it first. Passing it to a
// Record-typed parameter (optionMatches) needs no such view; reading a key out of it
// does.
const aliasesBySlug: Record<string, readonly string[]> = ROLE_ALIASES;

/** The tier a role lands in when only the matcher's typo tolerance reached it.
 *  Matches at or past this tier are dropped — see matchTier. */
const fuzzyOnlyTier = 4;

/** Does every word of the query appear inside some word of the text, with no edit
 *  distance spent? This is what separates a SPELLING difference from a typo: the query
 *  "full stack engineer" has no contiguous run inside "fullstack engineer", yet each of
 *  its words is there.
 *
 *  Only meaningful for a multi-word query. One short word sitting inside a longer one
 *  is a coincidence — `swe` inside "answer engine optimization" — while three words all
 *  landing is not. */
function everyWordPresent(text: string, q: string): boolean {
  const words = text.split(/\s+/).filter(Boolean);
  return q.split(/\s+/).every((token) => words.some((w) => w.includes(token)));
}

/** Does `q` occur in `text` at the start of a word? Distinguishes the `swe` that
 *  names Software Engineer from the one buried inside Marketing's "answer engine
 *  optimization" alias. */
function atWordStart(text: string, q: string): boolean {
  for (let i = text.indexOf(q); i !== -1; i = text.indexOf(q, i + 1)) {
    if (i === 0 || !/[a-z0-9]/.test(text[i - 1] ?? '')) return true;
  }
  return false;
}

/** How well the query names this role, lowest is best:
 *
 *  0 — the query prefixes its label or an alias AND finishes a word there
 *      (`data` → "data analyst", `swe` → the alias "swe");
 *  1 — the query prefixes one but stops mid-word (`data` → "database developer",
 *      and every half-typed query on its way to tier 0);
 *  2 — the query starts a word further in (`design` → "product design lead");
 *  3 — a multi-word query whose every word is present, but not as one contiguous run
 *      (`full stack engineer` → "Fullstack Engineer", which spells as one word what the
 *      query spells as two);
 *  4 — the matcher admitted it on typo tolerance alone, and it is NOT offered.
 *
 *  Tier 4 is dropped rather than ranked last. Nothing separates two typo-tolerant
 *  hits, so within that tier the biggest bucket wins on count alone: searching
 *  `backedn` led with Marketing Specialist (55,768, via edit distance against its
 *  `growth hacker` alias) ahead of Backend Engineer. Offering nothing is honest — the
 *  dropdown always keeps its free-text row, and the search index tolerates typos
 *  itself.
 *
 *  This is the PRIMARY ranking key, ahead of vacancy count, because optionMatches is
 *  alias-aware and typo-tolerant: `devops` reaches Sales Specialist through `revops`
 *  and `backend` reaches Marketing Specialist through `growth hacker`. Ranking that
 *  set by count alone hands first place to whichever unrelated role owns the largest
 *  bucket — measured on the live catalogue, Sales Specialist's 147,223 against DevOps
 *  Engineer's 44,804.
 *
 *  Tier 0 and 1 are split for the same reason one level down: "database developer" is
 *  a Software Generalist alias with 75,427 jobs behind it, so a plain prefix rule puts
 *  it above Data Analyst for a search of `data`. Finishing a word is the difference
 *  between naming a role and merely sharing its opening letters. */
function matchTier(option: { value: string; label: string }, q: string): number {
  const texts = [option.label.toLowerCase(), ...(aliasesBySlug[baseRole(option.value)] ?? [])];
  const prefixes = texts.filter((t) => t.startsWith(q));
  if (prefixes.some((t) => !/[a-z0-9]/.test(t[q.length] ?? ''))) return 0;
  if (prefixes.length > 0) return 1;
  if (texts.some((t) => atWordStart(t, q))) return 2;
  if (q.includes(' ') && texts.some((t) => everyWordPresent(t, q))) return 3;
  return 4;
}

/** A graded slug (senior_backend) loses to its ungraded sibling (backend) when
 *  nothing else separates them. Naming the grade is what promotes it: that lifts the
 *  graded label into a better tier, where this never applies. */
const gradePenalty = (slug: string): number => (slug === baseRole(slug) ? 0 : 1);

export function suggestRoles(
  query: string,
  counts: FacetCounts | null,
  active: readonly string[] = [],
): RoleSuggestion[] {
  if (query.trim().length < minQueryLength) return [];

  // Two different absences, deliberately not folded together: no distribution at all
  // means "not measured yet" and every match is offered without a figure, while a
  // role missing FROM a distribution means "measured, and it is zero" — offering that
  // would send the user to an empty page.
  const dist = counts?.facets?.role;
  const taken = new Set(active);

  // Each slug is a distinct key of ROLE_LABELS, so a role several of whose aliases
  // match is still visited once — no dedup pass needed. Eligibility is filtered
  // before matching on purpose: optionMatches runs an edit-distance pass per role,
  // and there is no reason to spend it on one we would drop anyway.
  const q = query.trim().toLowerCase();
  const ranked = roleOptions
    .filter((o) => !taken.has(o.value) && (!dist || Object.hasOwn(dist, o.value)))
    .filter((o) => optionMatches(o, query, ROLE_ALIASES))
    .map((o) => {
      const suggestion: RoleSuggestion = { slug: o.value, label: o.label };
      // The eligibility filter guarantees the lookup whenever dist exists; when it
      // does not, the suggestion carries no count at all rather than a zero.
      if (dist) suggestion.count = dist[o.value];
      return { suggestion, tier: matchTier(o, q) };
    })
    .filter((r) => r.tier < fuzzyOnlyTier)
    .toSorted(
      (a, b) =>
        a.tier - b.tier ||
        gradePenalty(a.suggestion.slug) - gradePenalty(b.suggestion.slug) ||
        (b.suggestion.count ?? 0) - (a.suggestion.count ?? 0) ||
        a.suggestion.label.localeCompare(b.suggestion.label),
    );

  // One row per BASE role. The catalogue carries every grade as its own slug and
  // graded slugs outrun ungraded ones about six to one, so without this a query like
  // "data analyst" spends all five rows on Data Analyst's grades and never reaches
  // Data Engineer. Walking the ranked list keeps the best variant of each role, which
  // is the named grade when the query named one.
  const kept = new Map<string, RoleSuggestion>();
  for (const { suggestion } of ranked) {
    const key = baseRole(suggestion.slug);
    if (!kept.has(key)) kept.set(key, suggestion);
    if (kept.size === maxSuggestions) break;
  }
  return [...kept.values()];
}
