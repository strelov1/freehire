// How the job filter modal's search rows map onto the staged filters.
import { FACETS } from '#lib/facets.js';
import { emptyFacet, type JobFilters } from '#lib/filters.js';
import type { FilterSearchHit } from '#lib/filterSearch.js';

/** A search hit's action: a toggle (`visa`, `watchlist`, …) or `kind:value` (`posted:7`). */
export function splitAction(action: string | undefined): { kind: string; rest: string } | null {
  if (!action) return null;
  const colon = action.indexOf(':');
  if (colon < 0) {
    if (
      action === 'visa' ||
      action === 'hide-ai' ||
      action === 'auto-apply' ||
      action === 'short-apply' ||
      action === 'former-employers' ||
      action === 'watchlist' ||
      action === 'hide-seen'
    )
      return { kind: action, rest: '' };
    return null;
  }
  const kind = action.slice(0, colon);
  if (kind === 'posted' || kind === 'open' || kind === 'experience') {
    return { kind, rest: action.slice(colon + 1) };
  }
  return null;
}

/** Whether a search hit is on in the staged filters, and whether it can be excluded. */
export function searchHitSelection(hit: FilterSearchHit, f: JobFilters, plain: boolean) {
  const off = { included: false, excluded: false, excludable: false };
  const action = splitAction(hit.action);
  switch (action?.kind) {
    case 'former-employers':
      return { ...off, included: f.formerEmployers };
    case 'watchlist':
      return { ...off, included: f.watchlist };
    case 'hide-seen':
      return { ...off, included: f.hideSeen };
    case 'visa':
      return { ...off, included: f.visa };
    case 'hide-ai':
      return { ...off, included: f.hideAIInterview };
    case 'auto-apply':
      return { ...off, included: f.autoApplyAvailable };
    case 'short-apply':
      return { ...off, included: f.applyQuestionsMax != null };
    case 'posted':
      return { ...off, included: f.postedWithinDays === Number(action.rest) };
    case 'open':
      return { ...off, included: f.openWithinDays === Number(action.rest) };
    case 'experience':
      return { ...off, included: f.experienceYearsMax === Number(action.rest) };
  }
  if (!hit.param || hit.value == null) return off;
  const selected = f.facets[hit.param] ?? emptyFacet();
  return {
    included: selected.include.includes(hit.value),
    excluded: selected.exclude.includes(hit.value),
    excludable: !plain && (FACETS.find((d) => d.param === hit.param)?.excludable ?? false),
  };
}
