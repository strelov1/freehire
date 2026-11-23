// The counts on the job filter modal's rail: how many values each tab has staged.
import { EMPLOYER_CREDENTIALS, JOB_COLLECTION } from '#lib/facets.js';
import type { RailEntry } from '#lib/filterSections.js';
import type { JobFilters } from '#lib/filters.js';

/** The personal toggles a tab counts only when the modal shows them. */
export type RailToggles = {
  showHideSeen: boolean;
  matchAvailable: boolean;
  showFormerEmployers: boolean;
  showWatchlist: boolean;
  showNewHiring?: boolean;
};

const jobCollectionValues = JOB_COLLECTION.map((o) => o.value);
const employerCredentialValues = EMPLOYER_CREDENTIALS.map((o) => o.value);

// Values selected for one facet — included plus excluded — so the rail count reflects
// any staged selection regardless of sign. `values`, when passed, scopes the count to
// just that subset — for a param split across two panes (see ChipFacet's `options`
// override), so a badge doesn't count values shown under a different tab.
export function selCount(f: JobFilters, param: string, values?: string[]): number {
  const st = f.facets[param];
  if (!st) return 0;
  if (!values) return st.include.length + st.exclude.length;
  const allowed = new Set(values);
  return st.include.filter((v) => allowed.has(v)).length + st.exclude.filter((v) => allowed.has(v)).length;
}

export function railEntryCount(e: RailEntry, f: JobFilters, show: RailToggles): number {
  const { showHideSeen, matchAvailable, showFormerEmployers, showWatchlist, showNewHiring } = show;
  if (e.kind === 'category') return selCount(f, 'category') + selCount(f, 'ai_archetype');
  if (e.kind === 'experience')
    return selCount(f, 'seniority') + selCount(f, 'role_type') + (f.experienceYearsMax != null ? 1 : 0);
  if (e.kind === 'location')
    return selCount(f, 'regions') + selCount(f, 'countries') + selCount(f, 'metros') + selCount(f, 'cities');
  if (e.kind === 'salary')
    return (
      selCount(f, 'salary_currency') + (f.salaryMin != null || f.salaryMax != null ? 1 : 0) + selCount(f, 'equity')
    );
  if (e.kind === 'work') return selCount(f, 'work_mode') + selCount(f, 'employment_type') + selCount(f, 'engagement');
  if (e.kind === 'industry')
    return selCount(f, 'domains') + selCount(f, 'company_type') + selCount(f, 'collections', jobCollectionValues);
  if (e.kind === 'language') return selCount(f, 'english_level') + selCount(f, 'posting_language');
  if (e.kind === 'relocation')
    return (
      selCount(f, 'relocation') +
      (f.visa ? 1 : 0) +
      (f.hideAIInterview ? 1 : 0) +
      (f.autoApplyAvailable ? 1 : 0) +
      (f.applyQuestionsMax != null ? 1 : 0) +
      selCount(f, 'clearance_levels') +
      selCount(f, 'collections', employerCredentialValues)
    );
  if (e.kind === 'posted')
    return (
      (f.postedWithinDays != null ? 1 : 0) +
      (f.openWithinDays != null ? 1 : 0) +
      (showHideSeen && f.hideSeen ? 1 : 0) +
      selCount(f, 'reality')
    );
  // The Minimum skill match threshold lives at the top of the Skills pane, so it
  // counts toward that tab's badge alongside the skills facet selections.
  if (e.key === 'skills') return selCount(f, 'skills') + (matchAvailable && f.minSkillMatch != null ? 1 : 0);
  if (e.key === 'company_slug')
    return (
      selCount(f, 'company_slug') +
      (showFormerEmployers && f.formerEmployers ? 1 : 0) +
      (showWatchlist && f.watchlist ? 1 : 0) +
      (showNewHiring && f.newHiring ? 1 : 0)
    );
  if (e.key === 'source') return selCount(f, 'source_type') + selCount(f, 'source');
  return selCount(f, e.facetParam ?? e.key);
}
