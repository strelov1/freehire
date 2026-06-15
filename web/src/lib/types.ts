// Wire types mirroring the backend JSON (internal/db models and query rows).
// Timestamps marshal as RFC3339 strings when present, or null.

import type { Job } from './generated/contracts';
export type { Job, Enrichment } from './generated/contracts';

export interface Company {
  slug: string;
  name: string;
  created_at: string | null;
  updated_at: string | null;
}

/** A row of the companies catalog: company plus its computed job count. */
export interface CompanyListItem {
  slug: string;
  name: string;
  job_count: number;
}

/** Pagination metadata returned alongside list responses. */
export interface ListMeta {
  total: number;
  limit: number;
  offset: number;
}

/** Facet-distribution counts from GET /api/v1/jobs/facets, for the analytics
 *  page. `total` is the estimated vacancy count under the current filters;
 *  `facets` maps each facet param (the same name used to filter, e.g. `regions`,
 *  `seniority`) to a value→count map; `stats` gives numeric facets' min/max
 *  (e.g. `salary_min`). Continuous numeric facets appear only in `stats`. */
export interface FacetCounts {
  total: number;
  facets: Record<string, Record<string, number>>;
  stats: Record<string, { min: number; max: number }>;
}

/** An authenticated account, as returned by the auth endpoints. */
export interface User {
  id: number;
  email: string;
  created_at: string | null;
}

/** A signed-in user's interaction with one job: when they viewed it, saved it
 *  for later, and (once they confirm an application) applied. `saved_at` and
 *  `applied_at` are null until then. Returned by the view/apply/save endpoints. */
export interface UserJob {
  job_id: number;
  viewed_at: string;
  saved_at: string | null;
  applied_at: string | null;
  // Application pipeline stage + free-text notes; null until set via `track`.
  stage: string | null;
  notes: string | null;
}

/** One item of the my-jobs listing: the job in the shared wire shape with the
 *  caller's interaction timestamps riding alongside. */
export interface MyJob {
  job: Job;
  viewed_at: string;
  saved_at: string | null;
  applied_at: string | null;
  stage: string | null;
  notes: string | null;
}

/** Per-tab row counts for the my-jobs page, from the listing's meta. `viewed`
 *  is the view-only subset: rows neither saved nor applied. */
export interface MyJobCounts {
  all: number;
  viewed: number;
  saved: number;
  applied: number;
}

/** An API key as returned by the management endpoints — metadata only; the
 *  plaintext token is never part of this shape. `token_prefix` is a short,
 *  non-secret leading slice (e.g. "fhk_Ab12cd") shown so the user can tell keys
 *  apart. Timestamps are RFC3339 strings or null. */
export interface ApiKey {
  id: number;
  name: string;
  token_prefix: string;
  created_at: string | null;
  last_used_at: string | null;
  expires_at: string | null;
}

/** The response of creating a key: the metadata plus the plaintext `token`,
 *  returned exactly once and never retrievable again. */
export interface CreatedApiKey extends ApiKey {
  token: string;
}
