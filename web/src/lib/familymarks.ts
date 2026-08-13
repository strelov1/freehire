// Fallback mark for "See also" cards whose collection has no real brand: a
// seniority level, a role/category, a remote/work-mode collection, or a
// `skills` slug with no entry in techmarks.ts. Colors are the ones approved
// during the interactive mockup review, not design-system tokens — the
// design-system's palette has no categorical/multi-hue scale to draw from,
// so this is a small local exception rather than an invented token.
//
// Pure data only, no `@lucide/svelte` import: this is imported by
// seeAlsoMark.ts, which vitest runs in plain Node — a Lucide icon component
// is a `.svelte` file the Svelte compiler must transform, so the icon
// components themselves are looked up in JobSeeAlso.svelte instead, keyed by
// the same FamilyIconName.
export type FamilyIconName = 'tech' | 'role' | 'seniority' | 'remote';

export const FAMILY_MARKS: Record<FamilyIconName, { color: string }> = {
  tech: { color: '#4f46e5' },
  role: { color: '#7c3aed' },
  seniority: { color: '#059669' },
  remote: { color: '#0891b2' },
};
