// The facets of the company and talent directories, over the option lists the job
// facets in facets.ts define.

import {
  CATEGORY,
  COLLECTION,
  COMPANY_TYPE,
  COUNTRY,
  INDUSTRIES,
  REGION,
  SENIORITY,
  TIMEZONE_REGION,
  options,
  type FacetDef,
  type FacetOption,
} from './facets';

// Company-size buckets — the vocab.CompanySizeValues vocabulary. Not exported as
// a generated values array (it's a scalar enrichment field, not a search facet on
// jobs), so the closed set is spelled out here like facets.ts's CURRENCY; the values are
// already display-ready.
const COMPANY_SIZE: FacetOption[] = ['1-10', '11-50', '51-200', '201-500', '501-1000', '1000+'].map((value) => ({
  value,
  label: value,
}));

// The company catalog's filter facets: mostly a subset of the job facets whose
// values are mostly derived from a company's open jobs and denormalized onto the
// companies row (collections + the RefreshCompanyFacets arrays), including
// `remote_regions` (regions scoped to remote jobs). `yc_batch`/`yc_status` are the
// curated YC-directory facets loaded by cmd/import-yc. No exclude/AND-OR modes — the
// companies list endpoint filters by plain array overlap. Reuses the same option
// vocabularies as the job facets so labels never drift.
const YC_STATUS: FacetOption[] = options(['Active', 'Acquired', 'Public', 'Inactive']);

const YC_STAGE: FacetOption[] = options(['Early', 'Growth']);

// Deterministic company maturity (see the company-classification-facets change),
// derived server-side from company signals; unknown (NULL) companies match nothing.
const MATURITY: FacetOption[] = options(['startup', 'scaleup', 'enterprise', 'government'], {
  startup: 'Startup',
  scaleup: 'Scaleup',
  enterprise: 'Enterprise',
  government: 'Government',
});

const YC_FLAGS: FacetOption[] = options(['top_company', 'hiring'], {
  top_company: 'YC Top Company',
  hiring: 'Hiring',
});

// YC batch labels are the verbatim source strings ("Winter 2012"). Generate the full
// season×year grid so the searchable select covers any batch; phantom combinations
// that no company has simply match nothing. The grid runs from the first YC batch
// (2005) through next year so a fresh batch is covered as soon as it appears.
const YC_BATCH: FacetOption[] = (() => {
  const max = new Date().getFullYear() + 1;
  return Array.from({ length: max - 2005 + 1 }, (_, i) => max - i).flatMap((y) =>
    ['Winter', 'Spring', 'Summer', 'Fall'].map((s) => ({ value: `${s} ${y}`, label: `${s} ${y}` })),
  );
})();

/** The Talent Network catalogue's facets.
 *
 *  All include-only — the catalogue endpoint has no `_exclude` and no AND/OR mode — so
 *  every entry sets `excludable: false` and none carries `hasAndOr`.
 *
 *  Skills and city are `dynamic`: their options come from the live facet distribution
 *  (`/talent/facets`) rather than a static list, which is what makes them searchable and
 *  what puts a count beside each value. A closed vocabulary offered as pills and an open
 *  one offered as a searched select are not two styles of the same control — thousands of
 *  skills as pills is the wrong control, and a skill offered without a count cannot be
 *  told apart from one nobody carries.
 *
 *  `specializations` and `categories` share the category vocabulary and ask different
 *  questions of it: what a candidate SAYS they want, and what their most recent title
 *  resolves to. Both are selects rather than pills because 48 values is a wall. */
export const TALENT_FACETS: FacetDef[] = [
  {
    param: 'specializations',
    label: 'Open to',
    control: 'select',
    options: CATEGORY,
    excludable: false,
    placeholder: 'Search disciplines',
  },
  {
    param: 'categories',
    label: 'Has worked in',
    control: 'select',
    options: CATEGORY,
    excludable: false,
    placeholder: 'Search disciplines',
  },
  { param: 'seniorities', label: 'Grade', control: 'pills', options: SENIORITY, excludable: false },
  {
    param: 'skills',
    label: 'Skills',
    control: 'select',
    dynamic: true,
    excludable: false,
    placeholder: 'Search skills',
    techIcons: true,
  },
  { param: 'tz', label: 'Timezone', control: 'pills', options: TIMEZONE_REGION, excludable: false },
  { param: 'cities', label: 'City', control: 'select', dynamic: true, excludable: false, placeholder: 'Search cities' },
];

export const COMPANY_FACETS: FacetDef[] = [
  { param: 'collections', label: 'Collection', control: 'pills', options: COLLECTION, excludable: false },
  { param: 'regions', label: 'Region', control: 'pills', options: REGION, excludable: false },
  { param: 'remote_regions', label: 'Remote hiring', control: 'pills', options: REGION, excludable: false },
  {
    param: 'countries',
    label: 'Country',
    control: 'select',
    options: COUNTRY,
    excludable: false,
    placeholder: 'Search countries',
  },
  // The catalogue's only industry control. The backend matches it against the curated
  // industries column AND the job-derived domains one (internal/industrytag maps
  // between the vocabularies), so the Domain facet that used to sit here asked the
  // same question of one of the same columns and is gone. `domains` remains a working
  // query parameter for a caller who wants the raw job-derived value.
  {
    param: 'industries',
    label: 'Industry',
    control: 'select',
    options: INDUSTRIES,
    excludable: false,
    placeholder: 'Search industries',
  },
  { param: 'company_type', label: 'Company type', control: 'pills', options: COMPANY_TYPE, excludable: false },
  { param: 'company_size', label: 'Company size', control: 'pills', options: COMPANY_SIZE, excludable: false },
  { param: 'maturity', label: 'Company stage', control: 'pills', options: MATURITY, excludable: false },
  { param: 'yc_status', label: 'YC status', control: 'pills', options: YC_STATUS, excludable: false },
  { param: 'yc_stage', label: 'YC stage', control: 'pills', options: YC_STAGE, excludable: false },
  { param: 'yc_flags', label: 'YC highlights', control: 'pills', options: YC_FLAGS, excludable: false },
  {
    param: 'yc_batch',
    label: 'YC batch',
    control: 'select',
    options: YC_BATCH,
    excludable: false,
    placeholder: 'Search YC batches',
  },
];
