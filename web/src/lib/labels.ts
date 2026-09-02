// Single source of display labels for the closed-vocabulary facet codes, shared by
// the detail-page facet rows (enrichment.ts), the filter panel (facets.ts) and the
// /insights pages (insights.ts). Values come from the generated contracts. Keeping
// ONE map prevents the drift that previously left stale region codes and inconsistent
// casing in two places. REGIONS is the curated macro-region set (vocab.RegionValues) —
// keep it in sync with the backend vocabulary.
//
// Most maps here list only the codes whose label differs from the title-cased
// fallback. CATEGORY_LABELS is the deliberate exception and is exhaustive; the reason
// is recorded above it.

import type { Category } from './generated/contracts';

/** A title-cased fallback label for a code with no explicit label
 *  (e.g. "network_engineering" → "Network Engineering"). Lives here, with the maps,
 *  so "how a label is produced" has one home; enrichment.ts keeps a separate
 *  sentence-cased fallback for the facets it renders. */
export function titleCase(value: string): string {
  return value
    .split('_')
    .map((w) => w.charAt(0).toUpperCase() + w.slice(1))
    .join(' ');
}

// Region code → display names, one row per vocab.RegionValues entry. `short`
// is the compact UI label (filter pills, facet rows).
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

export const REGION_LABELS: Record<string, string> = Object.fromEntries(
  REGIONS.map((r) => [r.code, r.short]),
);

export const SENIORITY_LABELS: Record<string, string> = { c_level: 'C-level' };

// The one role-type value (vocab.RoleTypeValues). There is deliberately no second
// entry: the catalogue can show that a posting IS a management role and cannot show
// that it is not, so excluding this pill means "no management marker in the title",
// which must never be labelled "Individual contributor" anywhere.
export const ROLE_TYPE_LABELS: Record<string, string> = { people_manager: 'People manager' };

// English proficiency levels (vocab.EnglishLevelValues); `none` is the
// no-requirement sentinel — detail pages filter it out before rendering.
export const ENGLISH_LEVEL_LABELS: Record<string, string> = {
  a1: 'A1', a2: 'A2', b1: 'B1', b2: 'B2', c1: 'C1', c2: 'C2', native: 'Native', none: 'None',
};

export const EMPLOYMENT_LABELS: Record<string, string> = {
  full_time: 'Full-time',
  part_time: 'Part-time',
};

export const WORK_MODE_LABELS: Record<string, string> = { onsite: 'On-site' };

// All three relocation values, not just the overrides: the filter panel and the
// detail-page row previously spelled the negative one differently ("None" vs "Not
// supported"), and "None" was ambiguous with "not stated". Spelling the whole facet
// out here keeps the three values reading as one set.
export const RELOCATION_LABELS: Record<string, string> = {
  not_supported: 'Not supported',
  supported: 'Supported',
  required: 'Required',
};

// The one map that is EXHAUSTIVE rather than override-only, listed in the order of
// the generated CATEGORY_VALUES so a diff against the vocabulary reads straight down.
// Categories are the only vocabulary rendered on all three surfaces — the filter
// panel, the job-detail facet row, and the indexed /insights pages — and those
// surfaces reach a label through two different fallbacks (facets.ts title-cases every
// word, enrichment.ts capitalizes only the first). Any code left to the fallback
// therefore renders two ways by construction, which is what previously forked an
// entire second map into insights.ts. Listing every code removes the fallback from the
// path instead of asking three call sites to agree.
//
// `satisfies Record<Category, string>` is what keeps it exhaustive: adding a category
// to the backend vocabulary and regenerating contracts fails `pnpm run check` (TS1360,
// naming the code) until it is labelled here, and removing one fails the same check
// (TS2353) rather than leaving a stale entry. The declared type stays
// Record<string, string> because the `label(map, value)` helpers take that shape.
export const CATEGORY_LABELS: Record<string, string> = {
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
} satisfies Record<Category, string>;

/** Display label for a category code (e.g. ml_ai → "ML / AI"), shared by the filter
 *  panel, the profile view and the /insights pages. The map above is exhaustive over
 *  the vocabulary, so the fallback only fires for a code the SPA has not been taught. */
export function categoryLabel(value: string): string {
  return CATEGORY_LABELS[value] ?? titleCase(value);
}

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

// The six AI skill-signature archetypes (vocab.AIArchetypeValues). Listed in full
// rather than override-only: the title-cased fallback would render "Rag App
// Builder" for an acronym that must read "RAG", so every value needs an explicit
// entry regardless.
export const AI_ARCHETYPE_LABELS: Record<string, string> = {
  rag_app_builder: 'RAG Application Builder',
  agent_builder: 'Agent Builder',
  cloud_ml_platform_engineer: 'Cloud/ML Platform Engineer',
  ml_trainer_researcher: 'ML Trainer/Researcher',
  fullstack_ai_engineer: 'Full-Stack AI Engineer',
  devops_infra_engineer: 'DevOps/Infra Engineer',
};
