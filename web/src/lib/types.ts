// Wire types mirroring the backend JSON (internal/db models and query rows).
// Timestamps marshal as RFC3339 strings when present, or null.

import type {
  Job,
  Report as ATSReportContract,
  Analysis as JobFitAnalysisContract,
} from './generated/contracts';
export type { Job, Enrichment, Verdict, Gap, SkillRow } from './generated/contracts';
// Per-job profile match (how well a job's skills are covered by the caller's profile).
export type { JobMatch, AdjacentSkill } from './generated/contracts';
// atscheck's Report is aliased ATSReport (a local Report — job reports — already exists);
// its category/line-item shapes come along for the report view.
export type {
  Report as ATSReport,
  ScoreCategory as ATSCategory,
  LineItem as ATSLineItem,
} from './generated/contracts';

/** The CV ATS report response: `has_cv` is false when the caller has no stored CV
 *  (the page prompts an upload); `report` is present only when `has_cv` is true. */
export interface ATSResponse {
  has_cv: boolean;
  report: ATSReportContract | null;
}

// The on-demand LLM job-fit analysis wire shapes (five scored dimensions + the
// ATS-style requirement match), generated from internal/jobfit.
export type {
  Analysis as JobFitAnalysis,
  Dimension as JobFitDimension,
  Requirement as JobFitRequirement,
} from './generated/contracts';

/** The caller's rolling-window AI fit-analysis usage. Present on GET reads (with a CV);
 *  `remaining` is floored at 0. A new-job analysis is blocked when `remaining` is 0; a
 *  recompute of an already-analysed job is always free. */
export interface JobFitQuota {
  used: number;
  limit: number;
  remaining: number;
}

/** The job-fit analysis response: `has_cv` is false when the caller has no stored CV
 *  (the block prompts an upload); `analysis` is null when none is cached yet or the LLM
 *  is unconfigured; `stale` marks a cached analysis whose CV or job changed since (the
 *  block then offers a recompute); `quota` reports monthly usage on reads (omitted on the
 *  no-CV read and on compute responses). */
export interface JobFitResponse {
  has_cv: boolean;
  stale: boolean;
  analysis: JobFitAnalysisContract | null;
  quota?: JobFitQuota;
}

/** One row of the Tracking → AI fit tab: a compact projection of a cached fit analysis
 *  (not the full analysis). `closed` marks a job that has since closed; `stale` marks an
 *  analysis whose CV/job/model changed since it ran. `analysed_at` is an ISO timestamp. */
export interface MyAnalysisItem {
  slug: string;
  title: string;
  company: string;
  closed: boolean;
  overall_score: number;
  verdict: string;
  analysed_at: string;
  stale: boolean;
}

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
}

/** A row of the companies catalog: company plus its computed job count and a few
 *  company-info facts for the card (present-only — absent on unenriched rows). */
export interface CompanyListItem {
  slug: string;
  name: string;
  job_count: number;
  tagline?: string | null;
  industries?: string[] | null;
  hq_country?: string | null;
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
  // Authorization role ('user' | 'moderator' | 'admin'). A UI affordance only —
  // the server re-checks it on every privileged request.
  role: string;
  // Beta-tester group membership, independent of `role`. Gates the agent
  // assistant (/my/assistant) in the UI.
  beta_tester: boolean;
  // Contribution reward balance — one point per accepted link contribution
  // (see /my/contributions).
  points: number;
  created_at: string | null;
}

/** A crowdsourced board contribution: a job link a user pasted for a company board we do
 *  not crawl yet. The unit is the board — `source` (ATS) + `board` (company slug) — not a
 *  single vacancy; the ingest side later scrapes all its vacancies. */
export interface Contribution {
  id: number;
  url: string;
  source: string;
  board: string;
  status: string;
  created_at: string | null;
}

/** A job submitted for moderation. `status` is the review lifecycle; `review_reason`
 *  carries an optional rejection note. `submitter_email` is present only on the
 *  moderator review queue, never on a submitter's own view. */
export interface Submission {
  id: number;
  url: string;
  source?: string;
  title: string;
  company: string;
  location?: string;
  remote: boolean;
  description?: string;
  posted_at?: string | null;
  status: 'pending' | 'approved' | 'rejected';
  review_reason?: string;
  reviewed_at?: string | null;
  created_at: string | null;
  submitter_email?: string;
  // Public slug of the minted live vacancy, present on an approved submission in the
  // caller's own list, so the UI can link to /jobs/<job_slug>.
  job_slug?: string;
}

/** The content a user submits for review (mirrors the moderator create body). */
export interface SubmissionInput {
  url: string;
  title: string;
  company: string;
  location?: string;
  remote?: boolean;
  description?: string;
  source?: string;
}

/** Why a job was reported. A closed vocabulary mirroring the backend's
 *  internal/report reasons; labels live in $lib/reports. */
export type ReportReason = 'no_response' | 'not_relevant' | 'spam' | 'fraud' | 'other';

/** A user's report of a problem with a live vacancy. `reporter_email` and the
 *  `job_*` fields are present only on the moderator review queue, never on the
 *  reporter's own create response (which also never carries the reporter id). */
export interface Report {
  id: number;
  reason: ReportReason;
  details: string;
  contact_telegram?: string;
  status: 'pending' | 'resolved' | 'dismissed';
  review_reason?: string;
  reviewed_at?: string | null;
  created_at: string | null;
  reporter_email?: string;
  job_slug?: string;
  job_title?: string;
}

/** The content a user submits when reporting a job. */
export interface ReportInput {
  reason: ReportReason;
  details: string;
  contact_telegram?: string;
}

/** A signed-in user's interaction with one job: when they viewed it, saved it
 *  for later, and (once they confirm an application) applied. `saved_at` and
 *  `applied_at` are null until then. Returned by the view/apply/save endpoints. */
export interface UserJob {
  job_id: number;
  viewed_at: string;
  saved_at: string | null;
  applied_at: string | null;
  // Swipe-mode triage mark; null until the job is dismissed (swipe left).
  dismissed_at: string | null;
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
  /** Live inbox messages linked to this job — the board card's ✉ badge. 0 for
   *  users without a connected mailbox / no linked mail. */
  email_count: number;
}

/** The classification/link overlay an inbox email carries: its classified status
 *  and, when resolved, the linked application (slug + company) or a pending
 *  suggestion the reading pane confirms inline. All optional/omitempty. */
export interface EmailLinking {
  status_signal?: string;
  link_source?: string;
  linked_slug?: string;
  linked_company?: string;
  suggested_slug?: string;
  suggested_company?: string;
}

/** One email linked to an application, on the application detail page. */
export interface ApplicationEmail {
  id: number;
  source: string;
  from_addr: string;
  from_name: string;
  subject: string;
  status_signal?: string;
  link_source?: string;
  received_at: string;
  read: boolean;
}

/** The caller's application for one job slug, with the emails linked to it. */
export interface TrackedApplication {
  job: Job;
  viewed_at: string | null;
  saved_at: string | null;
  applied_at: string | null;
  stage?: string;
  notes?: string;
  emails: ApplicationEmail[];
}

/** Per-tab row counts for the my-jobs page, from the listing's meta. `viewed`
 *  is the view-only subset: rows neither saved nor applied. */
export interface MyJobCounts {
  all: number;
  viewed: number;
  saved: number;
  applied: number;
}

/** A snapshot of the caller's applications across the seven pipeline buckets.
 *  Buckets always sum to the application total. */
export interface PipelineBuckets {
  no_answer: number;
  in_progress: number;
  interviewing: number;
  offer: number;
  accepted: number;
  rejected: number;
  declined: number;
}

/** The application-pipeline snapshot for the Pipeline tab: the caller's total
 *  application count and its distribution across the status buckets. */
export interface PipelineStats {
  applications: number;
  buckets: PipelineBuckets;
}

/** The bucketing period for the job-activity time series. */
export type ActivityGranularity = 'day' | 'week' | 'month';

/** One period on the job-activity dashboard: the ISO date labelling the bucket
 *  and the counts of vacancies added vs. removed within it. The backend returns a
 *  dense, gap-free series (empty periods carry zeros). */
export interface ActivityPoint {
  period: string;
  added: number;
  removed: number;
}

/** One point on the member-growth series: the ISO date and the cumulative count of
 *  registered members as of that day. The backend returns a dense, gap-free series
 *  (days with no new signups repeat the running total), monotonically
 *  non-decreasing. */
export interface UserGrowthPoint {
  date: string;
  total: number;
}

/** Aggregate engagement counts across all users: jobs saved, applications marked,
 *  and jobs viewed. Aggregate-only — no per-user or row-level field. */
export interface EngagementStats {
  saved: number;
  applied: number;
  viewed: number;
}

/** The derived health verdict for a provider (and the fleet) on the public
 *  /status page. */
export type HealthStatus = 'operational' | 'degraded' | 'down';

/** How a source is shaped: a multi-tenant ATS platform, a many-company job
 *  aggregator, a single company's own careers page, or an unclassified source. */
export type ProviderKind = 'ats' | 'aggregator' | 'company' | 'other';

/** One provider's health on the public /status page: board counts, freshness, and
 *  the derived status. Sanitized — no error text or board identifiers are exposed.
 *  Timestamps are RFC3339 strings, or null when the provider has never run/succeeded. */
export interface ProviderHealth {
  provider: string;
  kind: ProviderKind;
  status: HealthStatus;
  total_boards: number;
  healthy_boards: number;
  cooled_boards: number;
  last_run: string | null;
  last_success: string | null;
  ingested_total: number;
}

/** The public ingest-fleet status: the overall (worst-provider) verdict, the
 *  generation timestamp, and one entry per provider. */
export interface IngestStatus {
  overall: HealthStatus;
  generated_at: string;
  providers: ProviderHealth[];
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

/** A user's saved search: a named snapshot of the job-search filter state. `query`
 *  is the canonical search query string (the same the filter URL serializes), so
 *  applying it reproduces the exact filters. An empty `query` is the "show all" view.
 *  Timestamps are RFC3339 strings or null. */
export interface SavedSearch {
  id: number;
  name: string;
  query: string;
  /** Public board slug when the set is shared, or "" when private. Shared boards are
   *  readable by anyone at /b/<public_slug>. */
  public_slug: string;
  /** Optional attribution shown on the public board; "" renders the board anonymously. */
  author_label: string;
  created_at: string | null;
  updated_at: string | null;
}

/** A public "board": a shared saved search readable by anyone at /b/<slug>. Carries only
 *  display fields — no owner identity. `query` is the canonical search query string, applied
 *  to the jobs list to render the board's results. `author_label` is "" when anonymous. */
export interface Board {
  name: string;
  query: string;
  author_label: string;
}

/** A user's single profile: their professional self — a non-empty set of
 *  `specializations` (job categories) and a non-empty set of canonical skill tokens. One
 *  per user (no id, no name); the foundation for finding relevant work. Timestamps are
 *  RFC3339 strings or null. */
/** A geographic reach: controlled regions and/or ISO country codes. Empty = anywhere. */
export interface LocationGeoSet {
  regions?: string[];
  countries?: string[];
}

/** The user's current single base: an ISO country code and a free-text city. */
export interface LocationBase {
  country?: string;
  city?: string;
}

/** Willingness to relocate: an open flag plus acceptable destinations (open + empty
 *  targets = anywhere). */
export interface LocationRelocation {
  open: boolean;
  regions?: string[];
  countries?: string[];
  cities?: string[];
}

/** Optional "where & how I want to work" block: accepted work arrangements plus three
 *  geographic parts (remote reach, current base, relocation), freely combined. Mirrors the
 *  backend userprofile.LocationPreferences; null on a profile means the user set none. */
export interface LocationPreferences {
  work_modes?: string[];
  remote: LocationGeoSet;
  base: LocationBase;
  relocation: LocationRelocation;
}

export interface UserProfile {
  specializations: string[];
  skills: string[];
  location_preferences: LocationPreferences | null;
  created_at: string | null;
  updated_at: string | null;
}

/** A notification subscription on a saved search. */
export interface Subscription {
  id: number;
  saved_search_id: number;
  saved_search_name?: string;
  channel: string;
  active: boolean;
  created_at: string | null;
}

/** Telegram link status for the current user. `enabled` is whether the feature is
 *  configured server-side at all (so the UI can hide it); `linked` is whether this
 *  user has connected their chat. */
export interface TelegramStatus {
  enabled: boolean;
  linked: boolean;
  chat_id?: number;
}

/** What a résumé yields through the deterministic dictionaries: canonical skills and
 *  every specialization the résumé spans, plus the seniority grade. `skills` and
 *  `categories` are always arrays (empty when nothing resolved); `seniority` is omitted
 *  when unresolved (never guessed). */
export interface ResumeProfile {
  skills: string[];
  categories: string[];
  seniority?: string;
}

/** The read-only structured résumé parsed from the uploaded CV by the LLM. */
export type {
  Structured as ResumeStructured,
  Experience as ResumeExperience,
  Education as ResumeEducation,
} from './generated/contracts';

/** The résumé status (`GET /me/resume`): whether storage is enabled and a résumé is
 *  stored (with its upload time), plus the read-only structured résumé — null when the
 *  caller has none current (no résumé, unconfigured LLM, not yet extracted, or stale). */
export interface ResumeMeta {
  enabled: boolean;
  present: boolean;
  uploaded_at: string | null;
  structured: import('./generated/contracts').Structured | null;
}
