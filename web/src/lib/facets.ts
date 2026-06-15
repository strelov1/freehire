// The job-search facets, as a data-driven registry. Each entry declares its
// query param (matching the backend search API), section label, the control to
// render, its options (for pills/select), and whether it supports an "exclude"
// mode. The panel iterates this list; the store keys facet state by `param`.
//
// SOURCE_VALUES and the other generated value arrays (WORK_MODE_VALUES, etc.)
// in ./generated/contracts are the single source of truth for closed-vocabulary
// option values. A new backend value appears automatically here (humanized);
// only the label-override map below needs updating when the label differs from
// the title-cased fallback. REGION, POSTING_LANGUAGE, and CURRENCY are curated
// subsets not driven by generated arrays — leave those alone.

import {
  SOURCE_VALUES, WORK_MODE_VALUES, SENIORITY_VALUES, CATEGORY_VALUES,
  EMPLOYMENT_TYPE_VALUES, RELOCATION_VALUES, ENGLISH_LEVEL_VALUES,
  COMPANY_TYPE_VALUES, DOMAIN_VALUES,
} from './generated/contracts';

export interface FacetOption {
  value: string;
  label: string;
}

export type FacetControl = 'pills' | 'select' | 'tokens';

export interface FacetDef {
  param: string;
  label: string;
  control: FacetControl;
  options?: FacetOption[];
  excludable: boolean;
  /** Show a per-facet AND/OR toggle (match all vs match any) over selected values. */
  hasAndOr?: boolean;
  placeholder?: string;
}

/** A title-cased fallback label for a value with no explicit label. */
function humanize(value: string): string {
  return value
    .split('_')
    .map((w) => w.charAt(0).toUpperCase() + w.slice(1))
    .join(' ');
}

/** Build facet options from generated values, overriding labels where given. */
function options(values: readonly string[], labels: Record<string, string> = {}): FacetOption[] {
  return values.map((value) => ({ value, label: labels[value] ?? humanize(value) }));
}

// Values come from the generated SOURCE_VALUES (sources.All adapter registry +
// "telegram" from the tg-extract worker). A new ATS adapter appears here
// automatically; only add a label override below when the label differs from
// the title-cased fallback (e.g. "smartrecruiters" → "SmartRecruiters").
const SOURCE: FacetOption[] = options(SOURCE_VALUES, {
  telegram: 'Telegram', greenhouse: 'Greenhouse', smartrecruiters: 'SmartRecruiters',
  bamboohr: 'BambooHR', successfactors: 'SuccessFactors',
  workatastartup: 'Work at a Startup', remoteok: 'RemoteOK', arc: 'Arc',
});

// The backend's full `regions` reach vocabulary (enrich.RegionValues). Values mix
// levels by design (global / macro-region / country-as-area); keep this in sync
// with that list so every tagged region is filterable.
const REGION: FacetOption[] = [
  { value: 'global', label: 'Global' },
  { value: 'us', label: 'USA' },
  { value: 'north_america', label: 'North America' },
  { value: 'latam', label: 'LATAM' },
  { value: 'americas', label: 'Americas' },
  { value: 'eu', label: 'Europe' },
  { value: 'uk', label: 'UK' },
  { value: 'emea', label: 'EMEA' },
  { value: 'eea', label: 'EEA' },
  { value: 'mena', label: 'MENA' },
  { value: 'africa', label: 'Africa' },
  { value: 'apac', label: 'APAC' },
  { value: 'ru', label: 'Russia' },
  { value: 'cis', label: 'CIS' },
  { value: 'central_asia', label: 'Central Asia' },
];

const WORK_MODE: FacetOption[] = options(WORK_MODE_VALUES, { onsite: 'On-site' });
const SENIORITY: FacetOption[] = options(SENIORITY_VALUES, { c_level: 'C-level' });
const COMPANY_TYPE: FacetOption[] = options(COMPANY_TYPE_VALUES, { inhouse: 'In-house' });
const EMPLOYMENT: FacetOption[] = options(EMPLOYMENT_TYPE_VALUES, {
  full_time: 'Full-time', part_time: 'Part-time',
});
const RELOCATION: FacetOption[] = options(RELOCATION_VALUES, {
  not_supported: 'None', supported: 'Supported', required: 'Required',
});
const ENGLISH: FacetOption[] = options(ENGLISH_LEVEL_VALUES, {
  a1: 'A1', a2: 'A2', b1: 'B1', b2: 'B2', c1: 'C1', c2: 'C2', native: 'Native', none: 'None',
});
const CATEGORY: FacetOption[] = options(CATEGORY_VALUES, {
  ml_ai: 'ML / AI', data_engineering: 'Data Engineering', data_science: 'Data Science',
  data_analytics: 'Data Analytics', qa: 'QA', devops: 'DevOps', sre: 'SRE',
  project_management: 'Project Management',
});
const DOMAINS: FacetOption[] = options(DOMAIN_VALUES, {
  ecommerce: 'E-commerce', saas: 'SaaS', edtech: 'Edtech', adtech: 'Adtech',
  govtech: 'Govtech', fintech: 'Fintech', gamedev: 'Gamedev', healthcare: 'Healthcare',
});

const POSTING_LANGUAGE: FacetOption[] = [
  { value: 'en', label: 'EN' },
  { value: 'ru', label: 'RU' },
  { value: 'uk', label: 'UA' },
];

const CURRENCY: FacetOption[] = [
  { value: 'USD', label: 'USD' },
  { value: 'EUR', label: 'EUR' },
  { value: 'GBP', label: 'GBP' },
  { value: 'RUB', label: 'RUB' },
];

export const FACETS: FacetDef[] = [
  { param: 'regions', label: 'Region', control: 'pills', options: REGION, excludable: true },
  { param: 'work_mode', label: 'Work format', control: 'pills', options: WORK_MODE, excludable: true },
  { param: 'category', label: 'Specialization', control: 'select', options: CATEGORY, excludable: true, placeholder: 'Search specializations' },
  { param: 'seniority', label: 'Seniority', control: 'pills', options: SENIORITY, excludable: true },
  { param: 'skills', label: 'Skills', control: 'tokens', excludable: true, hasAndOr: true, placeholder: 'Add a skill, press Enter' },
  { param: 'domains', label: 'Industry', control: 'select', options: DOMAINS, excludable: true, placeholder: 'Search industries' },
  { param: 'company_type', label: 'Company type', control: 'pills', options: COMPANY_TYPE, excludable: true },
  { param: 'countries', label: 'Countries', control: 'tokens', excludable: true, placeholder: 'ISO code, e.g. DE' },
  { param: 'relocation', label: 'Relocation', control: 'pills', options: RELOCATION, excludable: true },
  { param: 'employment_type', label: 'Employment', control: 'pills', options: EMPLOYMENT, excludable: true },
  { param: 'english_level', label: 'English', control: 'pills', options: ENGLISH, excludable: true },
  { param: 'posting_language', label: 'Job language', control: 'pills', options: POSTING_LANGUAGE, excludable: true },
  { param: 'salary_currency', label: 'Currency', control: 'pills', options: CURRENCY, excludable: true },
  { param: 'source', label: 'Source', control: 'pills', options: SOURCE, excludable: true },
];
