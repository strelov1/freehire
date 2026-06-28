// Single source of display labels for the closed-vocabulary facet codes, shared
// by the detail-page facet rows (enrichment.ts) and the filter panel (facets.ts).
// Values come from the generated contracts; only the codes whose label differs
// from the title-cased fallback are listed here. Keeping ONE map prevents the
// drift that previously left stale region codes and inconsistent casing in two
// places. REGION is the curated macro-region set (enrich.RegionValues) — keep it
// in sync with the backend vocabulary.

export const REGION_LABELS: Record<string, string> = {
  global: 'Global',
  north_america: 'North America',
  latam: 'LATAM',
  eu: 'Europe',
  uk: 'UK',
  mena: 'MENA',
  africa: 'Africa',
  apac: 'APAC',
  cis: 'CIS',
};

export const SENIORITY_LABELS: Record<string, string> = { c_level: 'C-level' };

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
};

export const DOMAIN_LABELS: Record<string, string> = {
  fintech: 'FinTech',
  ecommerce: 'E-commerce',
  saas: 'SaaS',
  gamedev: 'GameDev',
  edtech: 'EdTech',
  adtech: 'AdTech',
  govtech: 'GovTech',
  healthcare: 'Healthcare',
};

export const COMPANY_TYPE_LABELS: Record<string, string> = { inhouse: 'In-house' };
