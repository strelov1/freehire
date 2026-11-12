import "@typespec/http";

using Http;

namespace Freehire;

// The public transparency and market-insight reads: aggregate-only, no record-level
// field and no user identifier, served from snapshots and rollups. Caching is decided
// by the edge, not by these answers; the one header they set is on /insights/roles.

/** The size of the catalogue, from one published snapshot. */
model CatalogScale {
  open_jobs: int64;
  unique_open_jobs: int64;
  companies: int64;
  sources: int64;
  ats_platforms: int64;
  telegram_channels: int64;

  /** RFC 3339 with fractional seconds, as the snapshot carries it. */
  computed_at: string;

  /** False when the snapshot is unavailable and `open_jobs` is the approximate fallback. */
  exact: boolean;
}

/** Vacancies added and removed in one period. */
model ActivityPoint {
  /** YYYY-MM-DD. */
  period: string;

  added: int64;
  removed: int64;
}

model ActivityMeta {
  granularity: string;
  from: string;
  to: string;
}

model JobsActivity {
  data: ActivityPoint[];
  meta: ActivityMeta;
}

model GrowthPoint {
  /** YYYY-MM-DD. */
  date: string;

  /** Cumulative members as of that day. */
  total: int64;

  /** That day's own signups. */
  new: int64;
}

model Engagement {
  saved: int64;
  applied: int64;
  viewed: int64;

  /** The first day `viewed` covers, YYYY-MM-DD; null when nothing is rolled up yet. */
  viewed_since: string | null;

  cvs_uploaded: int64;
  cvs_tailored: int64;
  alert_subscribers: int64;
  inboxes_connected: int64;
  saved_searches: int64;
}

model FacetStats {
  /** Facet param, then value, then count. Every covered facet is present. */
  facets: unknown;
}

model SkillNeighbour {
  skill: string;
  count: int64;
}

model SkillNeighboursMeta {
  skill: string;
}

model SkillNeighbours {
  data: SkillNeighbour[];
  meta: SkillNeighboursMeta;
}

model RoleSkill {
  skill: string;
  open_count: int64;
  share: float64;
}

model RoleInsight {
  category: string;
  seniority: string;
  open_count: int64;
  growth: int64;

  /** Only when a single role was named. */
  sample_size?: int64;

  /** Only when a single role was named; an empty list is a real answer. */
  skills?: RoleSkill[];

  /** The signed-in caller's own standing against `skills`; shaped like a job match. */
  coverage?: unknown;
}

model RolesMeta {
  country: string;
  category: string;
  seniority: string;
  sort: string;
  limit: int64;

  /** Present, and false, only for a single role. */
  skills_geography_scoped?: boolean;
}

model RolesAnswer {
  data: RoleInsight[];
  meta: RolesMeta;
}

model CompanyInsight {
  company_slug: string;
  company_name: string;
  open_now: int64;
  open_prev_30d: int64;
  growth_30d: int64;
}

model CompaniesMeta {
  sort: string;
  min_open: int64;
  limit: int64;
}

model CompaniesAnswer {
  data: CompanyInsight[];
  meta: CompaniesMeta;
}

model RoleCountryCell {
  category: string;

  /** Empty for the all-countries column. */
  country: string;

  open_count: int64;
  open_count_prev: int64;
}

model RoleCountriesMeta {
  countries: string[];
}

model RoleCountriesAnswer {
  data: RoleCountryCell[];
  meta: RoleCountriesMeta;
}

model PulseCompetition {
  views: int64;
  views_prev: int64;
  exposure_days: float64;
  exposure_days_prev: float64;
  views_through: string | null;
}

model PulseTimeToClose {
  closed: int64;
  closed_prev: int64;
  median_days: float32 | null;
  median_days_prev: float32 | null;
}

model PulseInflow {
  count: int64;
}

model RolePulse {
  category: string;
  open_count: int64;
  competition: PulseCompetition;
  time_to_close: PulseTimeToClose;
  inflow: PulseInflow;
}

model CategoryMeta {
  category: string;
}

model RolePulseAnswer {
  data: RolePulse;
  meta: CategoryMeta;
}

model SkillInsight {
  skill: string;
  open_count: int64;
  growth: int64;
}

model SkillsMeta {
  category: string;
  country: string;
  sort: string;
  limit: int64;
}

model SkillsAnswer {
  data: SkillInsight[];
  meta: SkillsMeta;
}

model VelocityMeta {
  ...ActivityMeta;
  facet_kind: string;
  facet_value: string;
}

model VelocityAnswer {
  data: ActivityPoint[];
  meta: VelocityMeta;
}

model SalaryMonth {
  currency: string;
  period: string;

  /** YYYY-MM. */
  month: string;

  sample_size: int64;
  p25: int64;
  p50: int64;
  p75: int64;
}

model SalaryHistoryAnswer {
  data: SalaryMonth[];
  meta: SalaryMeta;
}

model SalaryBand {
  seniority: string;
  currency: string;
  period: string;
  sample_size: int64;
  p25: int64;
  p50: int64;
  p75: int64;

  /** The salary at every 5th percentile from 5 to 95 (19 values); empty until the rollup has computed them. */
  quantiles: int64[];
}

model SalaryMeta {
  category: string;
  seniority: string;
  country: string;

  /** "seniority" when every seniority's bands are returned. */
  breakdown?: string;
}

model SalaryAnswer {
  data: SalaryBand[];
  meta: SalaryMeta;
}

/** The derived health verdict for a provider, the fleet or the site. */
union HealthStatus {
  "operational",
  "degraded",
  "down",
}

/** How a source is shaped. */
union ProviderKind {
  "ats",
  "aggregator",
  "company",
  "social",
  "other",
}

model StatusProvider {
  provider: string;
  kind: ProviderKind;
  status: HealthStatus;
  total_boards: int64;
  healthy_boards: int64;
  cooled_boards: int64;
  last_run: string | null;
  last_success: string | null;
  ingested_total: int64;
}

model SiteHistoryEntry {
  day: string;
  status: HealthStatus;
}

/** The site's own verdict, independent of the ingest fleet's `overall`. */
model SiteHealth {
  status: HealthStatus;
  database: "up" | "down";

  /** Fraction of 5xx responses over the trailing `window_minutes`, from the process's own traffic. */
  error_rate: float64;

  /** Fraction of the database pool held at once, 0..1. Reported, never judged: it says what the error rate cannot, that requests are unable to start. */
  pool_pressure: float64;
  window_minutes: int64;
  history: SiteHistoryEntry[];
}

model IngestStatus {
  overall: HealthStatus;
  generated_at: string;
  last_job_added_at: string | null;
  providers: StatusProvider[];
  site: SiteHealth;
}

model SourceJobs {
  open: int64;

  /** Null when the search index could not be measured. */
  browsable: int64 | null;

  /** Aggregators only. */
  ats_matched?: int64;

  ats_unmatched?: int64;
  measured_at: string;
}

model SourceHealth {
  status: HealthStatus;
  total_boards: int64;
  healthy_boards: int64;
  cooled_boards: int64;
  last_run: string | null;
  last_success: string | null;
  ingested_total: int64;
}

model SourceEntry {
  source: string;
  kind: ProviderKind;

  /** Whether the source is a crawl adapter at all. */
  adapter: boolean;

  /** Null until a snapshot has been taken for this source. */
  jobs: SourceJobs | null;

  /** Null when no board health record exists. */
  health: SourceHealth | null;
}

/** The catalogue size. Never fails: a cold cache degrades to an estimate and `exact` says so. */
@tag("Stats")
@route("/stats/catalog")
@get
op catalogScale(): Data<CatalogScale> | Problem;

/** Added and removed vacancies per period. A bad granularity or date, or a range over 4000 days, is a 400. */
@tag("Stats")
@route("/stats/jobs-activity")
@get
op jobsActivity(
  @query granularity?: string,
  @query from?: string,
  @query to?: string,
): JobsActivity | Problem;

/** Cumulative registrations per UTC day. */
@tag("Stats")
@route("/stats/user-growth")
@get
op userGrowth(): Data<GrowthPoint[]> | Problem;

@tag("Stats")
@route("/stats/engagement")
@get
op engagementStats(): Data<Engagement> | Problem;

/** The value-to-count distribution of each covered facet, from the daily snapshot. */
@tag("Stats")
@route("/stats/facets")
@get
op statsFacets(): Data<FacetStats> | Problem;

/** Skills most often listed beside one skill. An unknown skill is an empty list. */
@tag("Stats")
@route("/stats/skills/{slug}/neighbours")
@get
op skillNeighbours(@path slug: string): SkillNeighbours | Problem;

/**
 * Roles ranked by open count or growth. Naming both category and seniority adds the
 * role's skill distribution and, for a signed-in caller, their coverage; that answer
 * is then private.
 */
@tag("Insights")
@route("/insights/roles")
@get
op insightsRoles(
  @query country?: string,
  @query sort?: string,
  @query limit?: string,
  @query category?: string,
  @query seniority?: string,
): {
  @header("Cache-Control") cacheControl?: string;
  @body body: RolesAnswer;
} | Problem;

@tag("Insights")
@route("/insights/skills")
@get
op insightsSkills(
  @query category?: string,
  @query country?: string,
  @query sort?: string,
  @query limit?: string,
): SkillsAnswer | Problem;

@tag("Insights")
@route("/insights/velocity")
@get
op insightsVelocity(
  @query granularity?: string,
  @query from?: string,
  @query to?: string,
  @query category?: string,
  @query seniority?: string,
  @query country?: string,
): VelocityAnswer | Problem;

@tag("Insights")
@route("/insights/salary")
@get
op insightsSalary(
  @query category?: string,
  @query seniority?: string,
  @query country?: string,
): SalaryAnswer | Problem;

/** One role × grade × country scope's monthly history; an absent part means every value. */
@tag("Insights")
@route("/insights/salary/history")
@get
op insightsSalaryHistory(
  @query category?: string,
  @query seniority?: string,
  @query country?: string,
): SalaryHistoryAnswer | Problem;

@tag("Insights")
@route("/insights/companies")
@get
op insightsCompanies(
  @query sort?: string,
  @query min_open?: string,
  @query limit?: string,
): CompaniesAnswer | Problem;

@tag("Insights")
@route("/insights/role-countries")
@get
op insightsRoleCountries(@query countries?: string): RoleCountriesAnswer | Problem;

/** The category is required. */
@tag("Insights")
@route("/insights/role-pulse")
@get
op insightsRolePulse(@query category?: string): RolePulseAnswer | Problem;

/** The site's own live status plus a sanitized per-provider rollup of the ingest fleet. */
@tag("Stats")
@route("/status")
@get
op ingestStatus(): Data<IngestStatus> | Problem;

/** Every source the catalogue is built from, with what it holds and how its crawl is doing. */
@tag("Stats")
@route("/sources")
@get
op listSources(): Data<SourceEntry[]> | Problem;
