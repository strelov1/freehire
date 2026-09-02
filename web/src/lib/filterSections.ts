// Presentation groupings for the filter modal.
//
// - RAIL_SECTIONS: the three headings the modal's left rail groups facets under.
// - CATEGORY_GROUP / CATEGORY_GROUPS: the Specialization (category) facet's values
//   grouped into collapsible sections — the modal's "Departments"-style hierarchy.
//
// CATEGORY_GROUP is a Record keyed by every Category value, so a category missing a
// group is a compile error (svelte-check): a newly added backend category can't
// silently fall out of the modal.

import { CATEGORY_VALUES, type Category } from './generated/contracts';
import { categoryLabel } from './labels';

// `SAVED` heads the job modal's "My filters" tab; `FILTERS` heads the (single-section)
// company modal rail. Both ride the same shell as the job sections below.
export type RailSection = 'SAVED' | 'ROLE' | 'PAY & BENEFITS' | 'REQUIREMENTS & ELIGIBILITY' | 'FILTERS';

/** Display order of the specialization section headings. */
const CATEGORY_GROUP_ORDER = [
  'Engineering',
  'Data & AI',
  'Quality & Security',
  'Design & Creative',
  'Product & Management',
  'Go-to-market & Support',
  'People',
  'Business & Legal',
  'Other',
] as const;

type CategoryGroup = (typeof CATEGORY_GROUP_ORDER)[number];

/** Each category → its section. Keyed by the full Category union: a missing key
 *  fails the type-check, keeping the grouping exhaustive as the vocabulary grows. */
const CATEGORY_GROUP: Record<Category, CategoryGroup> = {
  software_engineering: 'Engineering',
  backend: 'Engineering',
  frontend: 'Engineering',
  fullstack: 'Engineering',
  mobile: 'Engineering',
  devops: 'Engineering',
  sre: 'Engineering',
  network_engineering: 'Engineering',
  hardware: 'Engineering',
  embedded: 'Engineering',
  blockchain: 'Engineering',
  architecture: 'Engineering',
  data_engineering: 'Data & AI',
  data_science: 'Data & AI',
  data_analytics: 'Data & AI',
  ml_ai: 'Data & AI',
  ai_engineering: 'Data & AI',
  qa: 'Quality & Security',
  security: 'Quality & Security',
  design: 'Design & Creative',
  creative: 'Design & Creative',
  engineering_design: 'Design & Creative',
  // Not a craft in the design sense — it sits with the engineering disciplines, since
  // that is where a plant or process engineer looks first.
  industrial_engineering: 'Engineering',
  product: 'Product & Management',
  project_management: 'Product & Management',
  management: 'Product & Management',
  business_analysis: 'Product & Management',
  technical_writing: 'Product & Management',
  marketing: 'Go-to-market & Support',
  sales: 'Go-to-market & Support',
  support: 'Go-to-market & Support',
  solutions_engineering: 'Go-to-market & Support',
  developer_relations: 'Go-to-market & Support',
  recruiting: 'People',
  hr: 'People',
  finance: 'Business & Legal',
  legal: 'Business & Legal',
  operations: 'Business & Legal',
  customer_success: 'Business & Legal',
  other: 'Other',
};

interface CategorySectionOption {
  value: Category;
  label: string;
}

export interface CategorySection {
  name: CategoryGroup;
  options: CategorySectionOption[];
}

/** The category options grouped into ordered, non-empty sections — the source the
 *  Specialization pane renders. Order within a section follows CATEGORY_VALUES. */
export const CATEGORY_GROUPS: CategorySection[] = CATEGORY_GROUP_ORDER.map((name) => ({
  name,
  options: CATEGORY_VALUES.filter((v) => CATEGORY_GROUP[v] === name).map((value) => ({
    value,
    label: categoryLabel(value),
  })),
})).filter((s) => s.options.length > 0);

// ---- Rail registry ----------------------------------------------------------
//
// The modal's left rail is a presentation grouping over the underlying param
// facets, not 1:1 with them. Composite entries fold several params or specials into
// one pane: `location` covers regions+countries+cities as a tree; `salary` covers
// the currency facet + the min-salary special (so there's no standalone Currency
// entry — that is the salary/currency merge). `kind` selects which pane renders.

export const RAIL_SECTIONS: RailSection[] = ['ROLE', 'PAY & BENEFITS', 'REQUIREMENTS & ELIGIBILITY'];

type RailKind =
  | 'facet'
  | 'category'
  | 'location'
  | 'salary'
  | 'work'
  | 'industry'
  | 'language'
  | 'relocation'
  | 'posted'
  | 'experience'
  // The job modal's "My filters" (saved searches) tab — renders SavedSearches, not a facet control.
  | 'saved';

export interface RailEntry {
  /** Stable pane id (also the URL-hashable key). */
  key: string;
  label: string;
  section: RailSection;
  kind: RailKind;
  /** For kind 'facet': the FacetDef param whose control this entry renders. */
  facetParam?: string;
}

/** One pane of the company modal's rail: the FacetSections it stacks, in order. */
export interface CompanyRailGroup {
  key: string;
  label: string;
  params: string[];
}

// The company modal's rail. Unlike RAIL every pane is plain facet controls, so a
// group is just its params — related facets fold into one pane so the geography,
// company-shape and YC-directory facets read as three panes instead of nine rows.
//
// A COMPANY_FACETS param missing here is unreachable in the UI, not merely
// ungrouped, so companyRailGroups.test.ts asserts the two sets match.
export const COMPANY_RAIL_GROUPS: CompanyRailGroup[] = [
  { key: 'collections', label: 'Collection', params: ['collections'] },
  { key: 'region', label: 'Region', params: ['regions', 'remote_regions'] },
  { key: 'countries', label: 'Country', params: ['countries'] },
  { key: 'industries', label: 'Industry', params: ['industries'] },
  { key: 'company', label: 'Company', params: ['company_type', 'company_size', 'maturity'] },
  { key: 'yc', label: 'Y Combinator', params: ['yc_status', 'yc_stage', 'yc_flags', 'yc_batch'] },
];

export const RAIL: RailEntry[] = [
  // One "Role" pane holds the whole "what role" concept: the role picker
  // (natural/named/composite roles), the specialization chips, and the AI
  // Specialization facet. The picker is the natural-language entry, the chips and
  // AI-specialization the browse-by-axis entry.
  //
  // Seniority used to sit here too, because a role slug can carry a grade prefix
  // (`senior_backend`) and the two read as one thought. It moved to Experience: a
  // grade states how much experience a posting wants, which is the question that
  // pane answers, and it sits there next to the years ceiling that qualifies it.
  // The two params stay independent, as they already were.
  { key: 'category', label: 'Role', section: 'ROLE', kind: 'category' },
  { key: 'experience', label: 'Experience', section: 'ROLE', kind: 'experience' },
  { key: 'location', label: 'Location', section: 'ROLE', kind: 'location' },
  { key: 'work', label: 'Work & employment', section: 'ROLE', kind: 'work' },
  { key: 'skills', label: 'Skills', section: 'ROLE', kind: 'facet', facetParam: 'skills' },
  { key: 'industry', label: 'Industry & collection', section: 'ROLE', kind: 'industry' },
  { key: 'company_slug', label: 'Company', section: 'ROLE', kind: 'facet', facetParam: 'company_slug' },
  { key: 'salary', label: 'Salary', section: 'PAY & BENEFITS', kind: 'salary' },
  { key: 'language', label: 'Language', section: 'REQUIREMENTS & ELIGIBILITY', kind: 'language' },
  { key: 'relocation', label: 'Relocation', section: 'REQUIREMENTS & ELIGIBILITY', kind: 'relocation' },
  { key: 'posted', label: 'Posted', section: 'REQUIREMENTS & ELIGIBILITY', kind: 'posted' },
];
