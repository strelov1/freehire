// Display labels for the job-facet codes the match card renders (work mode, region,
// category, country). A small, deliberate port of web/src/lib/labels.ts +
// enrichment.ts's summaryFacets — the extension cannot import those (SvelteKit-only
// modules), so this mirrors their override maps and fallback rule instead of sharing
// code. Keep in sync by hand, same convention as lib/assistant/ (see extension/AGENTS.md).

/** Sentence-case an unknown snake_case code ("data_engineering" → "Data engineering"),
 *  matching enrichment.ts's fallback for facet rows (not labels.ts's titleCase, which
 *  is for the filter panel and title-cases every word). */
function sentenceCase(value: string): string {
  const spaced = value.replace(/_/g, ' ');
  return spaced.charAt(0).toUpperCase() + spaced.slice(1);
}

function label(map: Record<string, string>, value: string): string {
  return map[value] ?? sentenceCase(value);
}

const WORK_MODE_LABELS: Record<string, string> = { onsite: 'On-site' };

export function workModeLabel(code: string): string {
  return label(WORK_MODE_LABELS, code);
}

const REGIONS: { code: string; short: string }[] = [
  { code: 'global', short: 'Worldwide' },
  { code: 'north_america', short: 'North America' },
  { code: 'latam', short: 'LATAM' },
  { code: 'eu', short: 'Europe' },
  { code: 'uk', short: 'UK' },
  { code: 'mena', short: 'MENA' },
  { code: 'africa', short: 'Africa' },
  { code: 'apac', short: 'APAC' },
  { code: 'cis', short: 'CIS' },
];

const REGION_LABELS: Record<string, string> = Object.fromEntries(
  REGIONS.map((r) => [r.code, r.short]),
);

export function regionLabel(code: string): string {
  return label(REGION_LABELS, code);
}

// Exhaustive over the category vocabulary (internal/vocab), same as web's
// CATEGORY_LABELS — listed in full rather than left to the fallback, so a code never
// renders two different ways across the two apps.
const CATEGORY_LABELS: Record<string, string> = {
  software_engineering: 'Software Engineering',
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
  creative: 'Creative & Media',
  engineering_design: 'Engineering Design',
  industrial_engineering: 'Industrial Engineering',
  healthcare: 'Healthcare',
  skilled_trades: 'Skilled Trades',
  retail: 'Retail',
  hospitality: 'Hospitality',
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
  other: 'Other',
};

export function categoryLabel(code: string): string {
  return label(CATEGORY_LABELS, code);
}

/** ISO 3166-1 alpha-2 → English display name via platform Intl data, mirroring
 *  web's countryLabel (facets.ts) — no hand-maintained table to fall out of sync. */
export function countryLabel(code: string): string {
  const up = code.toUpperCase();
  try {
    return new Intl.DisplayNames(['en'], { type: 'region' }).of(up) ?? up;
  } catch {
    return up;
  }
}
