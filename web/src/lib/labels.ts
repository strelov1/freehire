// Single source of display labels for the closed-vocabulary facet codes, shared
// by the detail-page facet rows (enrichment.ts) and the filter panel (facets.ts).
// Values come from the generated contracts; only the codes whose label differs
// from the title-cased fallback are listed here. Keeping ONE map prevents the
// drift that previously left stale region codes and inconsistent casing in two
// places. REGIONS is the curated macro-region set (enrich.RegionValues) — keep it
// in sync with the backend vocabulary.

// Region code → display names, one row per enrich.RegionValues entry. `short`
// is the compact UI label (filter pills, facet rows); `long` is the full place
// name schema.org gets for applicantLocationRequirements. `global` has no
// `long` — a worldwide reach intentionally carries no location requirement.
export const REGIONS: { code: string; short: string; long?: string }[] = [
  { code: 'global', short: 'Worldwide' },
  { code: 'north_america', short: 'North America', long: 'North America' },
  { code: 'latam', short: 'LATAM', long: 'Latin America' },
  { code: 'eu', short: 'Europe', long: 'European Union' },
  { code: 'uk', short: 'UK', long: 'United Kingdom' },
  { code: 'mena', short: 'MENA', long: 'MENA' },
  { code: 'africa', short: 'Africa', long: 'Africa' },
  { code: 'apac', short: 'APAC', long: 'Asia-Pacific' },
  { code: 'cis', short: 'CIS', long: 'CIS' },
];

export const REGION_LABELS: Record<string, string> = Object.fromEntries(
  REGIONS.map((r) => [r.code, r.short]),
);

// Full place names for schema.org (seo.ts); codes without one omit the
// location requirement.
export const REGION_NAMES: Record<string, string> = Object.fromEntries(
  REGIONS.flatMap((r) => (r.long ? [[r.code, r.long]] : [])),
);

export const SENIORITY_LABELS: Record<string, string> = { c_level: 'C-level' };

// English proficiency levels (enrich.EnglishLevelValues); `none` is the
// no-requirement sentinel — detail pages filter it out before rendering.
export const ENGLISH_LEVEL_LABELS: Record<string, string> = {
  a1: 'A1', a2: 'A2', b1: 'B1', b2: 'B2', c1: 'C1', c2: 'C2', native: 'Native', none: 'None',
};

export const EMPLOYMENT_LABELS: Record<string, string> = {
  full_time: 'Full-time',
  part_time: 'Part-time',
};

export const WORK_MODE_LABELS: Record<string, string> = { onsite: 'On-site' };

export const CATEGORY_LABELS: Record<string, string> = {
  ml_ai: 'ML / AI',
  ai_engineering: 'AI Engineer',
  data_engineering: 'Data Engineering',
  data_science: 'Data Science',
  data_analytics: 'Data Analytics',
  qa: 'QA',
  devops: 'DevOps',
  sre: 'SRE',
  project_management: 'Project Management',
  hr: 'HR',
};

export const DOMAIN_LABELS: Record<string, string> = {
  fintech: 'FinTech',
  ecommerce: 'E-commerce',
  gamedev: 'GameDev',
  edtech: 'EdTech',
  adtech: 'AdTech',
  govtech: 'GovTech',
  healthcare: 'Healthcare',
  devtools: 'DevTools',
  cybersecurity: 'Cybersecurity',
  ai: 'AI',
  hrtech: 'HRTech',
  proptech: 'PropTech',
  climatetech: 'ClimateTech',
};

export const COMPANY_TYPE_LABELS: Record<string, string> = { inhouse: 'In-house' };
