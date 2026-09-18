// Pure helpers behind the /insights SEO pages: the data-quality gate that decides
// which categories get published pages, the seniority rank order, and the
// deterministic auto-intro sentences. Kept free of fetch and Svelte so it is
// unit-testable in isolation; the routes fetch via the API and feed these functions.
//
// Display text is NOT owned here — these pages read the same labels.ts maps the
// filter panel and the job page read, so a category cannot be spelled one way on an
// indexed page and another way in the app. The one exception is the category-wide
// seniority band, which is an /insights concept rather than a vocabulary value.

import type { InsightRole, InsightSalaryBand, InsightSkill } from './api';
import { CATEGORY_LABELS, SENIORITY_LABELS, categoryLabel, titleCase } from './labels';

/** A category is published only when its open-job demand clears this floor, so no
 *  thin page ships. Tunable; deliberately conservative. */
export const MIN_CATEGORY_OPEN = 25;

/** Seniority tokens in rank order. '' is the category-wide band. */
const SENIORITY_ORDER = [
  'intern',
  'junior',
  'middle',
  'senior',
  'lead',
  'staff',
  'principal',
  'c_level',
] as const;

/** The seniority label for an /insights table row. Every real token resolves through
 *  the shared vocabulary; the empty string is this capability's own category-wide
 *  band, which is not a seniority value and so is not in the shared map. */
export function seniorityLabel(seniority: string): string {
  if (seniority === '') return 'All levels';
  return SENIORITY_LABELS[seniority] ?? titleCase(seniority);
}

export interface CoveredCategory {
  category: string;
  label: string;
  openCount: number;
}

/** Derive the published category set from the global roles ranking: a category is
 *  covered when its total open-count across seniorities clears MIN_CATEGORY_OPEN.
 *  `other` and blanks are excluded. Sorted by demand, so the hub lists the biggest
 *  categories first. */
export function coveredCategories(roles: InsightRole[]): CoveredCategory[] {
  const totals = new Map<string, number>();
  for (const r of roles) {
    if (!r.category || r.category === 'other') continue;
    totals.set(r.category, (totals.get(r.category) ?? 0) + r.open_count);
  }
  return [...totals.entries()]
    .filter(([, open]) => open >= MIN_CATEGORY_OPEN)
    .map(([category, openCount]) => ({ category, label: categoryLabel(category), openCount }))
    .sort((a, b) => b.openCount - a.openCount);
}

/** Whether a specific category clears the gate (drives the per-page 404). */
export function isCovered(roles: InsightRole[], category: string): boolean {
  return coveredCategories(roles).some((c) => c.category === category);
}

/** Whether a path segment names a real seniority. Not exported: the gate below is the
 *  only thing that should ask, so a caller cannot check the level and forget the rest of
 *  the gate. An unrecognised level in a URL is a wrong address, not a bad request. */
function isSeniority(seniority: string): boolean {
  return (SENIORITY_ORDER as readonly string[]).includes(seniority);
}

/** Whether a (category, seniority) pair is even an ADDRESS: both halves named by the
 *  vocabulary. Deliberately says nothing about DEMAND — how busy a role is decides
 *  whether its page is worth indexing, not whether it exists — so this is answerable
 *  from the two strings alone, with no ranking and no request.
 *
 *  That matters twice over. The API answers an invented level with a 400, which a
 *  SvelteKit load turns into a 500, so a mistyped URL must be refused before it is
 *  asked about. And the job page links here from any posting carrying both facets,
 *  knowing nothing about how busy the role is — a coverage requirement here would send
 *  a real posting's real role to a 404.
 *
 *  `other` is excluded: it is a real vocabulary value and not a role anybody hires for,
 *  so "Other jobs" is a page with nothing to say. `coveredCategories` drops it for the
 *  same reason. */
export function roleAddressExists(category: string, seniority: string): boolean {
  return category !== 'other' && category in CATEGORY_LABELS && isSeniority(seniority);
}

/** Whether a role deserves its own leaf page: a real address carrying enough open
 *  postings that a distribution over them means something. Takes the role's OWN
 *  open-count, so it can be asked of a single-role read as readily as of a ranking. */
export function roleQualifies(
  roles: InsightRole[],
  category: string,
  seniority: string,
  openCount: number,
): boolean {
  return (
    roleAddressExists(category, seniority) &&
    isCovered(roles, category) &&
    openCount >= MIN_CATEGORY_OPEN
  );
}

/** The qualifying roles WITHIN a ranking — what the sitemap lists.
 *
 *  This is a SUBSET of what the route serves, and deliberately so. Its input is the
 *  gate's ranked read, which the endpoint caps at 200 roles, while production carries
 *  ~349 roles over the floor (measured 2026-09-18). Before this took the ranking's cap
 *  seriously, the route read the same list and so 404'd every role below rank 200 —
 *  the documented rule above was not what decided; "top 200 by demand" was.
 *
 *  Listing fewer pages than the route serves is the safe direction and the one the
 *  sitemap convention requires: a listed URL must resolve. Listing MORE would be the
 *  bug. Widening the list means paging the ranking, not raising insightsMaxLimit, which
 *  bounds what an unauthenticated caller may ask for and exists for a different reason. */
export function rankedQualifyingRoles(roles: InsightRole[]): InsightRole[] {
  return roles.filter((r) => roleQualifies(roles, r.category, r.seniority, r.open_count));
}

/** The intro line for a role's leaf page. It says what the figures ARE measured over
 *  — the postings that STATE this level and carry a tagged skill — because only 39%
 *  of open technical postings name a seniority at all, so wording this as "the market
 *  for senior backend" would claim a population the data does not cover. */
export function roleSkillsIntro(role: InsightRole): string {
  const name = `${seniorityLabel(role.seniority)} ${categoryLabel(role.category)}`;
  const sample = role.sample_size ?? 0;
  const skills = role.skills ?? [];
  if (sample === 0 || skills.length === 0) {
    return `Open ${name} postings on freehire. Not enough of them list skills yet to rank what they ask for.`;
  }
  const top = skills
    .slice(0, 3)
    .map((s) => s.skill)
    .join(', ');
  return `Across ${sample.toLocaleString('en-US')} open ${name} postings that list skills, the ones mentioned most often are ${top}.`;
}

/** Sort salary bands into seniority order (category-wide '' band last). */
export function sortBandsBySeniority(bands: InsightSalaryBand[]): InsightSalaryBand[] {
  const rank = (s: string) => {
    const i = (SENIORITY_ORDER as readonly string[]).indexOf(s);
    return i === -1 ? SENIORITY_ORDER.length + 1 : i;
  };
  return [...bands].sort((a, b) => rank(a.seniority) - rank(b.seniority) || b.sample_size - a.sample_size);
}

/** Format an integer salary figure in its currency, compactly (e.g. $155,000). */
export function formatSalary(amount: number, currency: string): string {
  try {
    return new Intl.NumberFormat('en-US', {
      style: 'currency',
      currency,
      maximumFractionDigits: 0,
    }).format(amount);
  } catch {
    // Unknown/lowercase currency code → plain number with the code appended.
    return `${amount.toLocaleString('en-US')} ${currency.toUpperCase()}`;
  }
}

// --- Deterministic auto-intro sentences (no LLM) -----------------------------

/** Pick the richest yearly band for the intro figure, preferring the category-wide
 *  ('' seniority) row, else the largest sample. Returns null if none qualify. */
function headlineBand(bands: InsightSalaryBand[]): InsightSalaryBand | null {
  const yearly = bands.filter((b) => b.period === 'year');
  if (yearly.length === 0) return null;
  const wide = yearly.filter((b) => b.seniority === '');
  const pool = wide.length ? wide : yearly;
  return pool.reduce((best, b) => (b.sample_size > best.sample_size ? b : best));
}

export function salaryIntro(category: string, bands: InsightSalaryBand[]): string {
  const label = categoryLabel(category);
  const b = headlineBand(bands);
  if (!b) return `Salary ranges for ${label} roles, aggregated from open postings on freehire.`;
  return `${label} roles pay a median of ${formatSalary(b.p50, b.currency)} per year across ${b.sample_size} postings that disclose pay, ranging from ${formatSalary(b.p25, b.currency)} to ${formatSalary(b.p75, b.currency)}.`;
}

export function skillsIntro(category: string, skills: InsightSkill[]): string {
  const label = categoryLabel(category);
  if (skills.length === 0) return `The most in-demand skills for ${label} roles on freehire.`;
  const top = skills.slice(0, 3).map((s) => s.skill).join(', ');
  return `The most in-demand skills for ${label} roles right now are ${top} — ranked across ${skills.reduce((n, s) => n + s.open_count, 0)} open postings.`;
}

export function rolesIntro(category: string, roles: InsightRole[]): string {
  const label = categoryLabel(category);
  const total = roles.reduce((n, r) => n + r.open_count, 0);
  if (total === 0) return `Open ${label} roles by seniority on freehire.`;
  return `There are ${total} open ${label} roles on freehire right now, broken down by seniority and how fast each level is growing.`;
}
