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

model CatalogSizePoint {
  /** YYYY-MM-DD (UTC). */
  date: string;

  /** Open postings the public listings could page through at the end of that day. */
  open_jobs: int64;

  /**
   * True for a day estimated after the fact from today's records rather than measured that
   * day. Such a day applies today's duplicate verdicts to the past.
   */
  reconstructed: boolean;
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

// The catalogue size. Never fails: a cold cache degrades to an estimate and `exact`
// says so.
/**
 * The canonical numbers behind every "how big is this" figure on the site (the jobs list
 * total, the /about and /open pages). Backed by one snapshot `cmd/rollup-stats`
 * republishes periodically, not a live count. `exact: false` means the cache was cold and
 * the figures fell back to a planner estimate — treat them as approximate when it is
 * false.
 */
@summary("The headline catalogue-scale figures (open jobs, companies, sources, …).")
@useAuth(NoAuth)
@tag("Stats")
@route("/stats/catalog")
@get
op catalogScale(): Data<CatalogScale> | Problem;

// Added and removed vacancies per period. A bad granularity or date, or a range over
// 4000 days, is a 400.
/**
 * Aggregated to the requested granularity over a date range; the series is dense (missing
 * periods are 0). Defaults: `granularity=day`, `to`=today, `from` a per-granularity
 * window before `to`. An unknown granularity or a range over 4000 days is a `400`.
 */
@summary("Public time series of added vs. removed vacancies per period.")
@useAuth(NoAuth)
@opExample(#{
  returnType: #{
    data: #[
      #{
        period: "2026-06-01",
        added: 120,
        removed: 40,
      },
      #{
        period: "2026-06-08",
        added: 98,
        removed: 55,
      },
    ],
    meta: #{
      granularity: "week",
      from: "2025-06-09",
      to: "2026-06-08",
    },
  },
})
@tag("Stats")
@route("/stats/jobs-activity")
@get
op jobsActivity(
  /** One of `day`, `week`, `month` (default `day`). (e.g. `week`) */
  @query granularity?: string,
  /**
   * Start date (UTC). Defaults to a per-granularity window before `to`. (e.g.
   * `2026-01-01`)
   */
  @query from?: string,
  /** End date (UTC). Defaults to today. (e.g. `2026-06-30`) */
  @query to?: string,
): JobsActivity | Problem;

// Cumulative registrations per UTC day.
/** Aggregate-only — no user identifier is exposed. */
@summary("Cumulative member growth per UTC day, plus each day's own new-signup count.")
@useAuth(NoAuth)
@opExample(#{
  returnType: #{
    data: #[
      #{
        date: "2026-07-27",
        total: 12840,
        new: 37,
      },
    ],
  },
})
@tag("Stats")
@route("/stats/user-growth")
@get
op userGrowth(): Data<GrowthPoint[]> | Problem;

/** The same set `/stats/catalog` counts as `open_jobs`, one figure per UTC day. */
@summary("The catalogue's size per UTC day, measured or reconstructed.")
@useAuth(NoAuth)
@opExample(#{
  returnType: #{
    data: #[
      #{
        date: "2026-07-04",
        open_jobs: 2935357,
        reconstructed: true,
      },
    ],
  },
})
@tag("Stats")
@route("/stats/catalog-history")
@get
op catalogHistory(): Data<CatalogSizePoint[]> | Problem;

/**
 * `viewed` is the only field here that is not a signed-in count: it is bot-filtered
 * job-page opens by every visitor, signed in or not, so it is orders of magnitude larger
 * than the rest. It is also the only one that is not all-time — `viewed_since` is the
 * first day it covers (null before the first rollup), because the bot-filtered counter
 * shipped without a backfill of the days before it. Every other field is all-time.
 * Aggregate-only — no user identifier is exposed.
 */
@summary("Jobs saved, applied to and viewed across all users, plus CV and inbox usage.")
@useAuth(NoAuth)
@opExample(#{
  returnType: #{
    data: #{
      saved: 41200,
      applied: 18730,
      viewed: 903400,
      viewed_since: "2026-09-04",
      cvs_uploaded: 2140,
      cvs_tailored: 860,
      alert_subscribers: 970,
      inboxes_connected: 410,
      saved_searches: 1290,
    },
  },
})
@tag("Stats")
@route("/stats/engagement")
@get
op engagementStats(): Data<Engagement> | Problem;

// The value-to-count distribution of each covered facet, from the daily snapshot.
/**
 * Served from the facet rollup rather than a live Meilisearch facet count, so a page can
 * show "what's inside" without paying search for it.
 */
@summary("Facet distribution snapshot: countries, skills, seniority, work mode.")
@useAuth(NoAuth)
@tag("Stats")
@route("/stats/facets")
@get
op statsFacets(): Data<FacetStats> | Problem;

/** Skills most often listed beside one skill. An unknown skill is an empty list. */
@useAuth(NoAuth)
@tag("Stats")
@route("/stats/skills/{slug}/neighbours")
@get
op skillNeighbours(@path slug: string): SkillNeighbours | Problem;

/**
 * Roles ranked by open count or growth. Naming both category and seniority adds the
 * role's skill distribution and, for a signed-in caller, their coverage; that answer
 * is then private.
 */
@summary("Roles (category × seniority) ranked by openings or growth.")
@useAuth(NoAuth)
@tag("Insights")
@route("/insights/roles")
@get
op insightsRoles(
  /** Scope to one country (ISO code). (e.g. `de`) */
  @query country?: string,
  /** Rank by open count or by growth. (e.g. `growth`) */
  @query sort?: string,
  /** How many rows. */
  @query limit?: string,
  /** Scope to one job category. (e.g. `backend`) */
  @query category?: string,
  @query seniority?: string,
): {
  @header("Cache-Control") cacheControl?: string;
  @body body: RolesAnswer;
} | Problem;

/**
 * Scope by category or by country, but not both — the rollup behind it is
 * single-dimensional, so asking for both is a 400.
 */
@summary("Skills ranked by openings or growth.")
@useAuth(NoAuth)
@tag("Insights")
@route("/insights/skills")
@get
op insightsSkills(
  /** Scope to one category. */
  @query category?: string,
  /** Scope to one country. */
  @query country?: string,
  /** Rank by open count or growth. */
  @query sort?: string,
  /** How many rows. */
  @query limit?: string,
): SkillsAnswer | Problem;

@summary("How fast a slice of the market is hiring, over time.")
@useAuth(NoAuth)
@tag("Insights")
@route("/insights/velocity")
@get
op insightsVelocity(
  /** Bucket size for the series. (e.g. `week`) */
  @query granularity?: string,
  /** Start of the window. */
  @query from?: string,
  /** End of the window. */
  @query to?: string,
  /** Scope to one category. */
  @query category?: string,
  /** Scope to one seniority. */
  @query seniority?: string,
  @query country?: string,
): VelocityAnswer | Problem;

@summary("Salary bands by category, seniority and country.")
@useAuth(NoAuth)
@tag("Insights")
@route("/insights/salary")
@get
op insightsSalary(
  /** Scope to one category. */
  @query category?: string,
  /** Scope to one seniority. */
  @query seniority?: string,
  /** Scope to one country. */
  @query country?: string,
): SalaryAnswer | Problem;

/** One role × grade × country scope's monthly history; an absent part means every value. */
@useAuth(NoAuth)
@tag("Insights")
@route("/insights/salary/history")
@get
op insightsSalaryHistory(
  @query category?: string,
  @query seniority?: string,
  @query country?: string,
): SalaryHistoryAnswer | Problem;

@summary("Companies ranked by hiring activity.")
@useAuth(NoAuth)
@tag("Insights")
@route("/insights/companies")
@get
op insightsCompanies(
  /** Rank by openings or by growth. */
  @query sort?: string,
  /** Ignore companies below this many open roles. */
  @query min_open?: string,
  /** How many rows. */
  @query limit?: string,
): CompaniesAnswer | Problem;

@useAuth(NoAuth)
@tag("Insights")
@route("/insights/role-countries")
@get
op insightsRoleCountries(@query countries?: string): RoleCountriesAnswer | Problem;

/** The category is required. */
@useAuth(NoAuth)
@tag("Insights")
@route("/insights/role-pulse")
@get
op insightsRolePulse(@query category?: string): RolePulseAnswer | Problem;

// The site's own live status plus a sanitized per-provider rollup of the ingest fleet.
/**
 * Sanitized: which boards are healthy, cooled or failing, without error text or internal
 * identifiers.
 */
@summary("Ingest-fleet health, per provider.")
@useAuth(NoAuth)
@tag("Stats")
@route("/status")
@get
op ingestStatus(): Data<IngestStatus> | Problem;

/** Every source the catalogue is built from, with what it holds and how its crawl is doing. */
@useAuth(NoAuth)
@tag("Stats")
@route("/sources")
@get
op listSources(): Data<SourceEntry[]> | Problem;
