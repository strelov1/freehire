import type { components } from '#lib/generated/api.js';

/** A verified-employer account: the claim on one company. `status` drives which view the
 *  employer dashboard shows — the claim form (no account), a status page (pending/revoked),
 *  or the dashboard (active). Reading it (GET /employer/company) never requires the account
 *  to be active, unlike every write below. */
export type EmployerAccount = components['schemas']['EmployerAccount'];

/** The account plus the company's current curated profile — everything GET /employer/company
 *  returns once there is a companies row to read. Curated fields are all optional: a
 *  brand-new claim's company row may not exist yet, and any individual field may simply be
 *  unset. Never carries the job-derived facets (company_types/company_sizes etc.) — those
 *  are not employer-editable and fill in automatically once the employer's own vacancies are
 *  published and enriched. */
export type EmployerCompany = components['schemas']['EmployerCompany'];

/** PATCH /employer/company body: a nil/absent field is left unchanged. `industries`
 *  replaces the whole curated set when present, even as an empty array. */
export interface EmployerCompanyProfileInput {
  tagline?: string;
  description?: string;
  website?: string;
  industries?: string[];
  year_founded?: number;
  employee_count?: number;
  hq_country?: string;
  subindustry?: string;
}

/** How a candidate applies to an employer's vacancy: through its own link, or on freehire.
 *  Chosen when the vacancy is created and fixed after. */
export type EmployerApplyMode = 'external' | 'freehire';

/** One question an employer asks with a vacancy that takes applications on freehire. */
export interface EmployerQuestion {
  label: string;
  type: 'text' | 'boolean' | 'select';
  required: boolean;
  /** A `select` question's choices. */
  options?: string[];
}

/** The application form of a vacancy that takes applications on freehire
 *  (GET /jobs/:slug/direct-apply). `prefill` and `applied` are the signed-in caller's own. */
export type DirectApplyForm = components['schemas']['DirectApplyForm'];

/** An application to such a vacancy. It sends exactly one CV: the uploaded one
 *  (`resume: true`) or the one tailored for this vacancy (`cv_id`). A yes/no answer is
 *  `"true"` or `"false"`. */
export type DirectApplication = ({ resume: true } | { cv_id: string }) & {
  answers: Record<string, string>;
  note?: string;
};

/** POST /employer/jobs body. Deliberately no `company` field — the server always uses the
 *  account's own locked company name, never request content (see
 *  internal/ingest/employer/AGENTS.md). */
export interface EmployerVacancyInput {
  /** Absent means `external`. */
  apply_mode?: EmployerApplyMode;
  /** Only for `freehire`. */
  questions?: EmployerQuestion[];
  /** Required for `external`, refused for `freehire`. */
  url?: string;
  title: string;
  location?: string;
  remote?: boolean;
  description?: string;
  posted_at?: string | null;
  skills?: string[];
  regions?: string[];
  cities?: string[];
  work_mode?: string;
  employment_type?: string;
  seniority?: string;
  salary_min?: number;
  salary_max?: number;
  salary_currency?: string;
  salary_period?: string;
}

/** PATCH /employer/jobs/:slug body: a nil/absent field is left unchanged. The URL and
 *  company identity are not present — neither is editable after creation. */
export interface EmployerVacancyPatch {
  /** Replaces the questions of a vacancy that takes applications on freehire. */
  questions?: EmployerQuestion[];
  title?: string;
  location?: string;
  remote?: boolean;
  description?: string;
  posted_at?: string | null;
}
