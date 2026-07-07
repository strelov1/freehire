// Curated adjacency map for the role picker's "Related" suggestions: a hub role
// (keyed by its seniority-stripped base slug) → sibling/child roles a searcher
// probably also wants but whose names the text search would NOT surface (typing
// "mobile" never matches "iOS Developer"). It's a hand-curated dictionary, not an
// inferred graph — same "never guess" discipline as the roletag/skilltag
// dictionaries. Every slug here must exist in the ROLE_LABELS catalog; a
// suggestion only renders when the role also has jobs in the current distribution
// (see relatedOptions in facets.ts), so a stale entry is inert, never broken.
//
// Keys and values are BASE slugs (no senior_/lead_/… prefix); the picker strips a
// value's grade before lookup, so one entry serves every seniority of the hub.
export const ROLE_RELATED: Record<string, string[]> = {
  // Mobile: the coarse "mobile" bucket rarely surfaces the platform specialisations.
  mobile: ['ios_developer', 'android_developer', 'react_native_developer', 'flutter_developer'],
  ios_developer: ['android_developer', 'react_native_developer', 'flutter_developer', 'mobile'],
  android_developer: ['ios_developer', 'react_native_developer', 'flutter_developer', 'mobile'],
  react_native_developer: ['ios_developer', 'android_developer', 'flutter_developer', 'mobile'],
  flutter_developer: ['ios_developer', 'android_developer', 'react_native_developer', 'mobile'],

  // Web engineering: the split a "developer" search flattens.
  backend: ['fullstack', 'frontend', 'software_engineer'],
  frontend: ['fullstack', 'backend', 'software_engineer'],
  fullstack: ['backend', 'frontend', 'software_engineer'],
  software_engineer: ['backend', 'frontend', 'fullstack'],

  // Data / ML: adjacent-but-distinctly-named specialisations.
  data_science: ['ml_ai', 'data_engineering', 'data_analytics', 'ai_engineering'],
  data_analytics: ['data_science', 'data_engineering', 'business_analyst'],
  data_engineering: ['data_science', 'data_platform_engineer', 'ml_ai'],
  ml_ai: ['ai_engineering', 'data_science', 'mlops_engineer'],
  ai_engineering: ['ml_ai', 'prompt_engineer', 'data_science'],

  // Infra: "devops" hides the SRE/platform/cloud family.
  devops: ['sre', 'platform_engineer', 'cloud_engineer', 'infrastructure_engineer'],
  sre: ['devops', 'platform_engineer', 'infrastructure_engineer'],
  platform_engineer: ['devops', 'sre', 'cloud_engineer'],
  cloud_engineer: ['devops', 'platform_engineer', 'cloud_architect'],

  // Security & architecture: named sub-disciplines a broad term won't match.
  security: ['cybersecurity_engineer'],
  architecture: ['solutions_architect', 'software_architect', 'cloud_architect', 'data_architect', 'enterprise_architect'],
  solutions_architect: ['software_architect', 'cloud_architect', 'enterprise_architect'],

  // Design / product / management: cross-name siblings.
  design: ['product_designer', 'ux_designer'],
  product: ['product_designer', 'business_analyst'],
  management: ['engineering_manager', 'team_lead', 'director', 'scrum_master'],
  engineering_manager: ['team_lead', 'director', 'management'],
  team_lead: ['engineering_manager', 'scrum_master'],

  // Go-to-market: sales/marketing sub-roles.
  sales: ['account_executive', 'account_manager'],
  marketing: ['growth_marketer', 'seo_specialist'],
};
