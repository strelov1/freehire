import type { components } from '#lib/generated/api.js';

/** The lower-coverage extras stored in the company's `company_info` JSONB. Every
 *  field is optional — the loader omits what the source doesn't provide. */
export interface CompanyInfo {
  homepage?: string;
  parent?: string;
  subsidiaries?: string[];
  activities?: string[];
  funding?: {
    type?: string;
    amount?: number;
    year?: number;
    investors?: string[];
  };
  stock?: {
    symbol?: string;
    exchange?: string;
  };
  // Directory extras (populated by cmd/import-yc and cmd/import-hirebase; absent on
  // companies neither importer touched). `website`/`linkedin` are the hirebase
  // outbound links; `description` is the full company summary.
  description?: string;
  website?: string;
  linkedin?: string;
  stage?: string;
  top_company?: boolean;
  is_hiring?: boolean;
  yc_url?: string;
  logo?: string;
  // The rest of what the hirebase importer stores. These have always been on the wire
  // — the API serves `company_info` as raw JSON, so an undeclared key still reaches the
  // browser — they were simply never typed or rendered. Sampled across 40 YC companies
  // on 2026-08-16: ceo 25, twitter 22, facebook 21, instagram 5, locations 27.
  ceo?: string;
  twitter?: string;
  facebook?: string;
  instagram?: string;
  /** Countries the company has an office in, as ISO 3166-1 alpha-2 plus a display
   *  name. Distinct from a job's location: these are the employer's sites, not where
   *  any particular role sits. */
  locations?: { code?: string; name?: string }[];
}

export interface Company {
  slug: string;
  name: string;
  // Curated-collection slugs this company belongs to (e.g. yc, bigtech); empty
  // when untagged. Served by the company-detail endpoint (GetCompany SELECT *).
  collections: string[];
  created_at: string | null;
  updated_at: string | null;
  // Authoritative company-info facts (populated by the company-info backfill;
  // absent/empty on unenriched rows). GetCompany is SELECT *, so these ride along.
  industries?: string[];
  year_founded?: number | null;
  employee_count?: number | null;
  hq_country?: string | null;
  organization_type?: string | null;
  tagline?: string | null;
  company_info?: CompanyInfo;
  // Public thumbs counters (distinct signed-in up/down voters), served on every
  // company read. my_vote is the caller's own vote (-1, 0, 1) — only non-zero on
  // the auth-aware detail read.
  upvote_count: number;
  downvote_count: number;
  my_vote: number;
  // Materialized feedback counters (internal/companyfeedback): how many users
  // left a rated review and their average star rating. rating_avg is null while
  // feedback_count is 0.
  feedback_count: number;
  /** How many people have reported that this company screens with an AI interviewer
   *  (internal/engage/processreport). Absent when nobody has: the label is required to
   *  be shown with its count, so no reports is the absence of the field rather than a
   *  zero a badge could render beside. */
  ai_interview_reports?: number;
  feedback_rating_avg: number | null;
}

/** A company's recomputed materialized feedback counters, carried alongside a
 *  write response (upsert/delete) so the caller can update its own view of the
 *  company without a second round trip. */
export type CompanyFeedbackSummary = components['schemas']['CompanyFeedbackSummary'];

/** One user's rating + category + text about a company (internal/companyfeedback).
 *  The author is a pseudonymous persona handle, the same one CommunityThread uses —
 *  the backend never sends the real user id. `company` is set only on the
 *  upsert response (see CompanyFeedbackDialog's submit). */
export type CompanyFeedback = components['schemas']['CompanyFeedback'];

/** The moderator queue's shape for a reported review (internal/companyfeedback):
 *  the review itself plus which company it's on and the aggregated report info. */
export type ReportedCompanyFeedback = components['schemas']['ReportedCompanyFeedback'];

/** The result of casting or clearing a thumbs vote: the target's resulting public
 *  counters and the caller's own vote (-1, 0, 1). Returned by the vote endpoints
 *  and shaped identically for jobs and companies. */
export type VoteResult = components['schemas']['VoteResult'];

/** One company on the caller's watchlist (GET /me/watchlist), newest added first. */
export type WatchedCompany = components['schemas']['WatchedCompany'];

/** What a bulk import of names did: how many it added, how many were already on the
 *  list, and the entries that matched no company. */
export type WatchlistImportResult = components['schemas']['WatchlistImportResult'];

/** A row of the companies catalog: company plus its computed job count and a few
 *  company-info facts for the card (present-only — absent on unenriched rows). */
export type CompanyListItem = components['schemas']['CompanyListItem'];

/** Pagination metadata returned alongside list responses. */
export type ListMeta = components['schemas']['ListMeta'];

/** Facet-distribution counts from GET /api/v1/jobs/facets, for the analytics
 *  page. `total` is the estimated vacancy count under the current filters;
 *  `facets` maps each facet param (the same name used to filter, e.g. `regions`,
 *  `seniority`) to a value→count map; `stats` gives numeric facets' min/max
 *  (e.g. `salary_min`). Continuous numeric facets appear only in `stats`. */
export interface FacetCounts {
  total: number;
  facets: Record<string, Record<string, number>>;
  stats: Record<string, { min: number; max: number }>;
  /** Only when `company_slug` was requested: the exact number of distinct
   *  companies among the matching postings. */
  companies_total?: number;
  /** Only when `company_slug` was requested: the (at most 50) companies with the
   *  most matching postings, in descending count. */
  top_companies?: CompanyFacetValue[];
}

/** One company of the facets endpoint's named company distribution. `name` is
 *  the display name, or the slug when the company has none yet. */
interface CompanyFacetValue {
  slug: string;
  name: string;
  count: number;
}
