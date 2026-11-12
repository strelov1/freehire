import type { components } from '../generated/api.js';
import type { Stage } from '../generated/contracts';

/** Per-tab row counts for the my-jobs page, from the listing's meta. `viewed`
 *  is the view-only subset: rows neither saved nor applied. */
export interface MyJobCounts {
  all: number;
  viewed: number;
  saved: number;
  applied: number;
  dismissed: number;
}

/** The caller's application count at each stage of the vocabulary. Every stage is
 *  present, zero included, and the counts always sum to the application total —
 *  so a count of nothing is readable without being confused for a missing key. */
type PipelineStageCounts = Record<Stage, number>;

/** One side of the reply-rate comparison: how many observable applications there
 *  were and how many received a reply. Raw counts, never a pre-divided percentage —
 *  see replyRate() in pipeline.ts. */
export interface ReplyRateSide {
  applications: number;
  answered: number;
}

/** The personal-vs-global employer reply-rate benchmark. Present on PipelineStats
 *  only when both sides clear the sample gate server-side — absent, never zero or
 *  an estimate, below it. Not exported: referenced only by PipelineStats below. */
interface ReplyRateBenchmark {
  you: ReplyRateSide;
  global: ReplyRateSide;
}

/** The application-pipeline snapshot for the Pipeline tab: the caller's total
 *  application count and the count at each stage. Grouping those stages into the
 *  bands the funnel draws is the renderer's job, from the generated STAGE_GROUPS —
 *  the server does not ship a second vocabulary alongside the first. */
export interface PipelineStats {
  applications: number;
  stages: PipelineStageCounts;
  reply_rate?: ReplyRateBenchmark;
}

/** The bucketing period for the job-activity time series. */
export type ActivityGranularity = 'day' | 'week' | 'month';

/** One period on the job-activity dashboard: the ISO date labelling the bucket
 *  and the counts of vacancies added vs. removed within it. The backend returns a
 *  dense, gap-free series (empty periods carry zeros). */
export type ActivityPoint = components['schemas']['ActivityPoint'];

/** How many of a company's OBSERVABLE applications were answered — the applicant had a
 *  connected mailbox, so a reply would have been seen. Absent from the company payload
 *  below its sample gate; `median_reply_days` is absent below its own, smaller one. */
export interface CompanyResponse {
  applications: number;
  answered: number;
  unanswered: number;
  median_reply_days?: number;
}

/** The company page's Hiring tab: the company's added/removed series, its open postings
 *  now and 30 days ago, and — from 5 employer-side closures up — the median days its
 *  postings stay open. */
export interface CompanyHiring {
  open: number;
  open_prev: number;
  growth: number;
  /** Page views of the company's postings over the last 30 days and the 30 before —
   *  one visitor counted once per posting per day, crawlers excluded. */
  views: number;
  views_prev: number;
  series: ActivityPoint[];
  lifetime?: { closed: number; median_days: number };
}

/** One point on the member-growth series: the ISO date, the cumulative count of
 *  registered members as of that day, and that day's own (non-cumulative)
 *  new-signup count. The backend returns a dense, gap-free series (days with no
 *  new signups repeat the running total and carry `new: 0`); `total` is
 *  monotonically non-decreasing. */
export type UserGrowthPoint = components['schemas']['GrowthPoint'];

/** Aggregate engagement counts across all users: jobs saved, applications marked,
 *  jobs viewed, and what people build on top — CVs uploaded and tailored, people with
 *  job alerts on, inboxes connected, searches saved. `inboxes_connected` sums live Gmail
 *  grants and claimed hosted mailboxes, so a user holding both counts twice.
 *  Aggregate-only — no per-user or row-level field.
 *
 *  `viewed` is the ONLY field here that is not a signed-in count: it is bot-filtered
 *  job-page opens by every visitor, signed in or not, so it is orders of magnitude
 *  larger than its neighbours by construction. Whatever renders it must say so —
 *  presenting the eight side by side as one population reads as a broken ratio.
 *
 *  It is also the only one that is not all-time. `viewed_since` is the first day the
 *  underlying column carries a count (null before the first rollup): the column shipped
 *  without a backfill and the nginx history to recover is past logrotate's window, so
 *  render the two together — the number alone reads as the life of the site. */
export type EngagementStats = components['schemas']['Engagement'];

/** How big the catalogue is, as one snapshot.
 *
 *  Every surface that quotes catalogue scale reads this, so two pages rendered from
 *  the same response cannot disagree. `open_jobs` and `companies` are exact counts over
 *  the set the public listings paginate; `unique_open_jobs` is the de-duplicated count
 *  Meilisearch's index actually holds — what /jobs itself would show unfiltered, and
 *  reads far lower than `open_jobs` since that one counts every repost across boards.
 *  `sources`, `ats_platforms` and `telegram_channels` describe reach — what the crawler
 *  can read, whether or not each currently holds an open posting.
 *
 *  `exact` is false when the backend had no published snapshot and fell back to an
 *  approximate open-job count. On that path only `open_jobs`, `sources` and
 *  `ats_platforms` are meaningful; the rest are zero. */
export type CatalogScale = components['schemas']['CatalogScale'];

/** Semantic-embedding coverage of the catalogue, behind the public /semantic-search
 *  page (`GET /api/v1/stats/embeddings` and its stream). Every figure is `null`
 *  until it has been measured — render that as "measuring", never as 0. */
export interface EmbedProgress {
  eligible: number | null;
  covered: number | null;
  snapshot_covered: number | null;
  queued: number | null;
  rate_per_minute: number | null;
  eta_seconds: number | null;
  snapshot_at: string | null;
  measured_at: string | null;
}

/** The derived health verdict for a provider (and the fleet) on the public
 *  /status page. */
export type HealthStatus = 'operational' | 'degraded' | 'down';

/** How a source is shaped: a multi-tenant ATS platform, a many-company job
 *  aggregator, a single company's own careers page, or an unclassified source. */
export type ProviderKind = 'ats' | 'aggregator' | 'company' | 'social' | 'other';

/** One provider's health on the public /status page: board counts, freshness, and
 *  the derived status. Sanitized — no error text or board identifiers are exposed.
 *  Timestamps are RFC3339 strings, or null when the provider has never run/succeeded. */
interface ProviderHealth {
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

/** One day's recorded worst-of-day site status. A day with no recorded
 *  sample simply has no entry in `SiteHealth.history` — it is never
 *  reported as "operational" by default. */
interface SiteHistoryEntry {
  day: string;
  status: HealthStatus;
}

/** The site/API's own live health, independent of the ingest fleet's status
 *  (`overall`). `database` is "up" or "down"; `error_rate` is the fraction of
 *  5xx responses over the trailing `window_minutes`, computed from the
 *  server's own recent traffic. `history` is the trailing 90 days' recorded
 *  daily status, oldest first, omitting any day with no recorded sample. */
interface SiteHealth {
  status: HealthStatus;
  database: 'up' | 'down';
  error_rate: number;
  /** Fraction of the database connection pool held at once, 0..1. Answers a different
   *  question from `error_rate`: that one describes requests that FINISHED, this one
   *  describes requests that cannot start. Reported, never judged: it is a single
   *  instant, and the live pool touches its ceiling in ordinary bursts, so `status`
   *  above does not read it. Judging it needs an average over minutes, which the
   *  Grafana rule does and this page cannot. */
  pool_pressure: number;
  window_minutes: number;
  history: SiteHistoryEntry[];
}

/** The public status read: the site/API's own live status, plus the
 *  ingest-fleet overall (worst-provider) verdict, the generation timestamp,
 *  and one entry per provider. */
export interface IngestStatus {
  overall: HealthStatus;
  generated_at: string;
  providers: ProviderHealth[];
  site: SiteHealth;
}

/** What the catalogue currently holds under one source, as measured by the daily
 *  snapshot. Two counts on purpose: `open` is every open posting, duplicates and all,
 *  and is the denominator the overlap figures are arithmetic on; `browsable` is the
 *  de-duplicated count the search index holds, which is what /jobs?source=… actually
 *  shows and therefore what the page displays.
 *
 *  `browsable` is null when the search index could not be measured — NOT zero. A zero
 *  meaning "we could not measure this" must never be rendered as "this source has no
 *  jobs".
 *
 *  `ats_matched`/`ats_unmatched` are present for aggregators only. "Unmatched" means the
 *  dedup pass found no first-party ATS posting to pair a posting with — the absence of
 *  evidence, not evidence of absence. Do not render it as "exclusive". */
interface SourceJobs {
  open: number;
  browsable: number | null;
  ats_matched?: number;
  ats_unmatched?: number;
  measured_at: string;
}

/** One source's crawl-fleet health, derived exactly as the /status page derives it.
 *  Absent on a source with no board_health record at all — the normal state for a source
 *  that is not a crawl adapter. */
interface SourceHealth {
  status: HealthStatus;
  total_boards: number;
  healthy_boards: number;
  cooled_boards: number;
  last_run: string | null;
  last_success: string | null;
  ingested_total: number;
}

/** One entry on the public source catalogue. `jobs` is null when the snapshot has never
 *  covered this source; `health` is null when it has no crawl-health record. Both absences
 *  are answers, not missing data.
 *
 *  No logo field: a source's brand mark is resolved from its DISPLAY NAME client-side (see
 *  sourceLogoUrl). A host was tried and shipped the wrong brand — an ATS posting's URL is
 *  often on the employer's own domain. */
export interface SourceEntry {
  source: string;
  kind: ProviderKind;
  /** Whether the source is a crawl adapter; false for the Telegram pipeline, employer
   *  vacancies and hand-curated intake, which have no board to crawl. */
  adapter: boolean;
  jobs: SourceJobs | null;
  health: SourceHealth | null;
}
