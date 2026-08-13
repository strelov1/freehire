// Fallback mark for "See also" cards whose collection has no real brand: a
// seniority level, a role/category, a remote/work-mode collection, a
// `skills` slug with no entry in techmarks.ts, or a company-membership
// collection (editorial/credential kind, e.g. "Unicorns" or "H-1B sponsor
// history") that isn't one of the few backer collections with a real mark in
// backers.ts. Colors are the ones approved during the interactive mockup
// review (plus `company`, added after code review caught the
// company-collection gap), not design-system tokens — the design-system's
// palette has no categorical/multi-hue scale to draw from, so this is a
// small local exception rather than an invented token.
//
// Pure data only, no `@lucide/svelte` import: this is imported by
// seeAlsoMark.ts, which vitest runs in plain Node — a Lucide icon component
// is a `.svelte` file the Svelte compiler must transform, so the icon
// components themselves are looked up in JobSeeAlso.svelte instead, keyed by
// the same FamilyIconName.
export type FamilyIconName = 'tech' | 'role' | 'seniority' | 'remote' | 'company';

export const FAMILY_MARKS: Record<FamilyIconName, { color: string }> = {
  tech: { color: '#4f46e5' },
  role: { color: '#7c3aed' },
  seniority: { color: '#059669' },
  remote: { color: '#0891b2' },
  company: { color: '#d97706' },
};
