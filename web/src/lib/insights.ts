// Pure helpers behind the /insights SEO pages: the data-quality gate that decides
// which categories get published pages, human labels for the enrichment category /
// seniority tokens, and the deterministic auto-intro sentences. Kept free of fetch
// and Svelte so it is unit-testable in isolation; the routes fetch via the API and
// feed these functions.

import type { InsightRole, InsightSalaryBand, InsightSkill } from './api';

/** A category is published only when its open-job demand clears this floor, so no
 *  thin page ships. Tunable; deliberately conservative. */
export const MIN_CATEGORY_OPEN = 25;

/** Human labels for the lowercase enrichment category tokens. Unlisted tokens fall
 *  back to a title-cased form. `other` is intentionally never published. */
export const CATEGORY_LABELS: Record<string, string> = {
  backend: 'Backend',
  frontend: 'Frontend',
  fullstack: 'Full-Stack',
  mobile: 'Mobile',
  devops: 'DevOps',
  sre: 'SRE',
  network_engineering: 'Network Engineering',
  data_engineering: 'Data Engineering',
  data_science: 'Data Science',
  data_analytics: 'Data Analytics',
  ml_ai: 'ML / AI',
  ai_engineering: 'AI Engineering',
  qa: 'QA',
  security: 'Security',
  hardware: 'Hardware',
  embedded: 'Embedded',
  blockchain: 'Blockchain',
  architecture: 'Architecture',
  design: 'Design',
  product: 'Product',
  project_management: 'Project Management',
  management: 'Management',
  marketing: 'Marketing',
  sales: 'Sales',
  support: 'Support',
  business_analysis: 'Business Analysis',
  solutions_engineering: 'Solutions Engineering',
  developer_relations: 'Developer Relations',
  technical_writing: 'Technical Writing',
  recruiting: 'Recruiting',
  hr: 'HR',
  finance: 'Finance',
  legal: 'Legal',
  operations: 'Operations',
  customer_success: 'Customer Success',
};

/** Seniority tokens in rank order, with labels. '' is the category-wide band. */
export const SENIORITY_ORDER = [
  'intern',
  'junior',
  'middle',
  'senior',
  'lead',
  'staff',
  'principal',
  'c_level',
] as const;

export const SENIORITY_LABELS: Record<string, string> = {
  '': 'All levels',
  intern: 'Intern',
  junior: 'Junior',
  middle: 'Middle',
  senior: 'Senior',
  lead: 'Lead',
  staff: 'Staff',
  principal: 'Principal',
  c_level: 'C-level',
};

export function categoryLabel(category: string): string {
  return CATEGORY_LABELS[category] ?? category.replace(/_/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase());
}

export function seniorityLabel(seniority: string): string {
  return SENIORITY_LABELS[seniority] ?? seniority;
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
