import type { ResumeProfile } from './types';

/** What the onboarding wizard is holding for the confirm step, before anything is saved. */
export interface StagedFacets {
  specializations: string[];
  seniorities: string[];
  skills: string[];
}

/** The result of folding one import into the staged set, plus whether that import
 *  recognised anything at all — the wizard says different things to the user for
 *  "filled in what we found" and "couldn't read details". */
export interface MergedFacets extends StagedFacets {
  resolved: boolean;
}

/** Folds what a CV or a LinkedIn profile resolved into what the wizard already holds.
 *
 *  Merge, never replace. The wizard reappears on every visit until a CV exists, so the
 *  staged set can already carry a previous visit's saved profile, an earlier upload, or
 *  the user's own picks — and an import is one more source of evidence about them, not a
 *  correction of them. A field the import resolved nothing for is therefore left exactly
 *  as it was; only a field it did resolve grows.
 *
 *  `resolved` reports whether the import recognised anything, which is not the same as
 *  whether the staged set changed: an import that resolves only values already staged did
 *  recognise them, and telling the user it read nothing would be wrong.
 */
export function mergeFacets(staged: StagedFacets, incoming: ResumeProfile): MergedFacets {
  const resolved = incoming.categories.length > 0 || incoming.skills.length > 0 || !!incoming.seniority;
  return {
    specializations: union(staged.specializations, incoming.categories),
    seniorities: union(staged.seniorities, incoming.seniority ? [incoming.seniority] : []),
    skills: union(staged.skills, incoming.skills),
    resolved,
  };
}

/** Appends what is new, keeping the order the user already sees. Returns the original array
 *  untouched when there is nothing to add, so a field the import said nothing about does not
 *  churn. */
function union(staged: string[], incoming: string[]): string[] {
  if (incoming.length === 0) return staged;
  return [...new Set([...staged, ...incoming])];
}
