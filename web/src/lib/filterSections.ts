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
import { categoryLabel } from './facets';

// `SAVED` heads the job modal's "My filters" tab; `FILTERS` heads the (single-section)
// company modal rail. Both ride the same shell as the job sections below.
export type RailSection = 'SAVED' | 'ROLE' | 'PAY & BENEFITS' | 'REQUIREMENTS & ELIGIBILITY' | 'FILTERS';

/** Display order of the specialization section headings. */
export const CATEGORY_GROUP_ORDER = [
  'Engineering',
  'Data & AI',
  'Quality & Security',
  'Design',
  'Product & Management',
  'Go-to-market & Support',
  'Other',
] as const;

export type CategoryGroup = (typeof CATEGORY_GROUP_ORDER)[number];

/** Each category → its section. Keyed by the full Category union: a missing key
 *  fails the type-check, keeping the grouping exhaustive as the vocabulary grows. */
export const CATEGORY_GROUP: Record<Category, CategoryGroup> = {
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
  design: 'Design',
  product: 'Product & Management',
  project_management: 'Product & Management',
  management: 'Product & Management',
  marketing: 'Go-to-market & Support',
  sales: 'Go-to-market & Support',
  support: 'Go-to-market & Support',
  other: 'Other',
};

export interface CategorySectionOption {
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

export type RailKind =
  | 'facet'
  | 'category'
  | 'location'
  | 'salary'
  | 'work'
  | 'industry'
  | 'language'
  | 'relocation'
  | 'posted'
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

export const RAIL: RailEntry[] = [
  // One "Role" pane holds the whole "what role" concept: the role picker
  // (natural/named/composite roles), the seniority pills, and the specialization
  // chips — three controls, one tab. They overlap by design (role=senior is also
  // reachable via the seniority pill); the picker is the natural-language entry,
  // the pills/chips the browse-by-axis entry.
  { key: 'category', label: 'Role', section: 'ROLE', kind: 'category' },
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
