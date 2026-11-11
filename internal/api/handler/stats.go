package handler

import (
	"context"
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/ingest/catalogstats"
	"github.com/strelov1/freehire/internal/platform/cache"
	"github.com/strelov1/freehire/internal/platform/db"
)

// statsHandlers serves the public transparency and market-insights reads:
// catalogue activity, member growth, engagement counts, the facet snapshot, the
// insights rollups, and the site/ingest-fleet status. All are unauthenticated,
// aggregate-only reads — no record-level field or user identifier is exposed.
type statsHandlers struct {
	queries *db.Queries

	// cache and estimator back the catalogue-scale read. The cache holds the snapshot
	// the rollup worker publishes; the estimator is the approximate open-job count
	// served when no snapshot is available. Neither is required — a nil cache degrades
	// to the estimate, which is the same path a cold or unreachable cache takes.
	cache     cache.Cache
	estimator catalogstats.Estimator

	// pool backs the site-status database check (IngestStatus): the same live
	// pool.Ping approach the /health handler uses, kept separate from it
	// rather than shared because Health is mounted on *API, not statsHandlers.
	pool *pgxpool.Pool
}

func newStatsHandlers(queries *db.Queries, c cache.Cache, pool *pgxpool.Pool) *statsHandlers {
	return &statsHandlers{queries: queries, cache: c, estimator: queries, pool: pool}
}

// register mounts the stats and insights reads. optionalAuth attaches the caller when
// signed in and never rejects — only /insights/roles takes it, and only so a single-role
// read can overlay that caller's own skill coverage. Every other route here stays
// strictly anonymous and aggregate-only.
func (h *statsHandlers) register(api fiber.Router, optionalAuth fiber.Handler, contract fiber.Handler) {
	// Public catalogue-activity time series (added vs. removed vacancies per period),
	// unauthenticated like the other public reads. Served from the job_daily_stats
	// rollup (cmd/rollup-stats); the /trends SPA page renders it as a bar chart.
	// Public catalogue-scale figures, unauthenticated like the other public reads.
	// One response carries every number a surface quotes about how big the catalogue
	// is, so /about and /open render the same snapshot instead of taking their own
	// totals at their own moments and disagreeing.
	api.Get("/stats/catalog", contract)

	api.Get("/stats/jobs-activity", contract)

	// Public member-growth time series (cumulative registrations per UTC day),
	// unauthenticated like the other public reads. Computed on the fly from
	// users.created_at (no rollup); the /open transparency page renders it as a
	// bar chart. Aggregate-only — no user identifier is exposed.
	api.Get("/stats/user-growth", contract)

	// Public engagement counts (jobs saved / applied / viewed across all users),
	// unauthenticated like the other public reads. Aggregate-only from user_jobs;
	// the /open transparency page renders them as a stat-strip.
	api.Get("/stats/engagement", contract)

	// Public facet-distribution snapshot (countries, skills, seniority, work_mode),
	// unauthenticated like the other public reads. Served from the insights_facet_stats
	// rollup (cmd/rollup-facets) so the /open transparency page's "what's inside"
	// section stays off the live Meilisearch facet count. Aggregate-only — per-value
	// counts only.
	api.Get("/stats/facets", contract)

	// Public skill co-occurrence snapshot: the skills most often listed beside one
	// skill, for the /skills/<slug> glossary page. Served from insights_skill_neighbours
	// (cmd/rollup-facets) so a page render never asks the engine for a skills facet.
	api.Get("/stats/skills/:slug/neighbours", contract)

	// Public Trends & Insights reads: aggregate market intelligence (role & skill
	// demand, hiring velocity, salary bands) served from the insights_* rollups
	// (cmd/rollup-stats), unauthenticated like the other public reads. Aggregate-only
	// — no record-level field is exposed.
	api.Get("/insights/roles", optionalAuth, contract)
	api.Get("/insights/skills", contract)
	api.Get("/insights/velocity", contract)
	api.Get("/insights/salary", contract)
	api.Get("/insights/salary/history", contract)
	api.Get("/insights/companies", contract)
	api.Get("/insights/role-countries", contract)
	api.Get("/insights/role-pulse", contract)

	// Public ingest-fleet status, unauthenticated like the other public reads.
	// A per-provider health rollup over board_health, sanitized (no error text or
	// board identifiers); the /status page renders it as a status board.
	api.Get("/status", contract)

	// Public source catalogue, unauthenticated like the other public reads. The same
	// board_health rollup /status reads, joined with the per-source snapshot
	// (cmd/rollup-stats) and classified through the adapter registry; the /sources page
	// renders it as a searchable catalogue. Sanitized the same way /status is — no board
	// identifier, no error text, no posting URL.
	api.Get("/sources", contract)
}

// dateLayout is the wire format for every date the activity endpoint reads and
// writes (ISO 8601 calendar date, UTC).
const dateLayout = "2006-01-02"

// maxRangeDays caps the [from, to] span the public activity endpoint will serve.
// The read builds a per-day generate_series, so an unbounded range on an
// unauthenticated endpoint is a cheap resource-abuse vector; ~11 years comfortably
// covers the coarsest default window and any realistic custom range.
const maxRangeDays = 4000

// activityQuery is the validated, defaulted read window for the job-activity
// endpoint: a whitelisted granularity plus a UTC date range.
type activityQuery struct {
	Granularity string
	From        time.Time
	To          time.Time
}

// activityPoint is one bar-pair on the wire: a period label and its added/removed
// counts.
type activityPoint struct {
	Period  string `json:"period"`
	Added   int32  `json:"added"`
	Removed int32  `json:"removed"`
}

// parseActivityQuery validates the granularity/from/to query params and resolves
// the read window. Granularity defaults to "day" and must be one of day/week/month
// (anything else is an error → 400). `to` defaults to today (from the injected
// now, truncated to the UTC date so the result is deterministic); `from` defaults
// to a per-granularity window before `to` (coarser granularities look back
// further). Explicit dates override the defaults. now is a parameter so the
// defaulting is unit-testable without wall-clock coupling.
func parseActivityQuery(granularity, from, to string, now time.Time) (activityQuery, error) {
	if granularity == "" {
		granularity = "day"
	}
	var window func(time.Time) time.Time
	switch granularity {
	case "day":
		window = func(t time.Time) time.Time { return t.AddDate(0, 0, -90) }
	case "week":
		window = func(t time.Time) time.Time { return t.AddDate(0, 0, -7*52) }
	case "month":
		window = func(t time.Time) time.Time { return t.AddDate(0, -24, 0) }
	default:
		return activityQuery{}, fmt.Errorf("unknown granularity %q (want day, week, or month)", granularity)
	}

	toDate := truncateToDate(now)
	if to != "" {
		parsed, err := time.Parse(dateLayout, to)
		if err != nil {
			return activityQuery{}, fmt.Errorf("invalid to date %q (want YYYY-MM-DD)", to)
		}
		toDate = parsed
	}

	fromDate := window(toDate)
	if from != "" {
		parsed, err := time.Parse(dateLayout, from)
		if err != nil {
			return activityQuery{}, fmt.Errorf("invalid from date %q (want YYYY-MM-DD)", from)
		}
		fromDate = parsed
	}

	if fromDate.After(toDate) {
		return activityQuery{}, fmt.Errorf("from %s is after to %s", fromDate.Format(dateLayout), toDate.Format(dateLayout))
	}
	// Compare via AddDate rather than toDate.Sub(fromDate): a multi-millennium span
	// would overflow time.Duration (int64 ns, ~292y max) and silently defeat the cap.
	if fromDate.Before(toDate.AddDate(0, 0, -maxRangeDays)) {
		return activityQuery{}, fmt.Errorf("range too large (max %d days)", maxRangeDays)
	}
	return activityQuery{Granularity: granularity, From: fromDate, To: toDate}, nil
}

// truncateToDate drops the time-of-day, yielding the UTC calendar date at midnight.
func truncateToDate(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// UserGrowth serves the public, unauthenticated member-growth time series: the
// cumulative count of registered members per UTC day, from the first registration
// through today, plus each day's own new-signup count. The dense, gap-free,
// monotonically non-decreasing series is produced by the SQL query; this handler
// only maps rows to the wire envelope. Aggregate-only — the query selects no user
// identifier, so no personal field can leak here. An empty catalogue yields an
// empty series (200 with data: []).
func (h *statsHandlers) UserGrowth(ctx context.Context) (*apigen.UserGrowthOK, error) {
	rows, err := h.queries.ListUserGrowth(ctx)
	if err != nil {
		return nil, err
	}

	points := make([]apigen.GrowthPoint, len(rows))
	for i, r := range rows {
		points[i] = apigen.GrowthPoint{Date: r.Day.Time.Format(dateLayout), Total: int64(r.Total), New: int64(r.New)}
	}

	return &apigen.UserGrowthOK{Data: points}, nil
}

// EngagementStats serves the public, unauthenticated engagement counts: how many
// user_jobs rows have been saved, applied to, and viewed, plus how many résumés
// have been uploaded, CVs tailored to a vacancy, matches analyzed, inboxes
// connected, and searches saved. Aggregate-only — the query selects nothing but
// integer totals, so no per-user field can leak. An empty database yields all
// zeros (200).
// CatalogScale serves the catalogue-scale snapshot: how many open postings the
// catalogue holds (raw and de-duplicated), from how many companies, across how many
// sources, ATS platforms and Telegram channels.
//
// It never fails. A cold cache (before the first rollup run), an unreachable one, and a
// payload left by an older build all degrade to the approximate open-job count, and
// `exact` reports which of the two the caller holds — a transparency page showing a
// labelled estimate beats one showing a 500.
func (h *statsHandlers) CatalogScale(ctx context.Context) (*apigen.CatalogScaleOK, error) {
	result := catalogstats.Load(ctx, h.cache, h.estimator)

	return &apigen.CatalogScaleOK{Data: apigen.CatalogScale{
		OpenJobs:         result.OpenJobs,
		UniqueOpenJobs:   result.UniqueOpenJobs,
		Companies:        result.Companies,
		Sources:          int64(result.Sources),
		AtsPlatforms:     int64(result.ATSPlatforms),
		TelegramChannels: int64(result.TelegramChannels),
		ComputedAt:       result.ComputedAt.Format(time.RFC3339Nano), // what time.Time marshalled to
		Exact:            result.Exact,
	}}, nil
}

// engagementCacheKey and engagementCacheTTL hold the engagement row between requests.
// The query sums the whole job_daily_views rollup, which only grows (2-2.7s measured
// from outside on 2026-11-08, /open's slowest leg), and the figures are totals nobody
// reads to the minute.
const (
	engagementCacheKey = "stats:engagement:v1"
	engagementCacheTTL = 10 * time.Minute
)

func (h *statsHandlers) engagementStats(ctx context.Context) (db.GetEngagementStatsRow, error) {
	if h.cache != nil {
		if s, found, err := cache.GetJSON[db.GetEngagementStatsRow](ctx, h.cache, engagementCacheKey); err == nil && found {
			return s, nil
		}
	}
	s, err := h.queries.GetEngagementStats(ctx)
	if err != nil {
		return s, err
	}
	if h.cache != nil {
		// Best-effort: a failed write only means the next request recomputes.
		_ = cache.SetJSON(ctx, h.cache, engagementCacheKey, s, engagementCacheTTL)
	}
	return s, nil
}

func (h *statsHandlers) EngagementStats(ctx context.Context) (*apigen.EngagementStatsOK, error) {
	s, err := h.engagementStats(ctx)
	if err != nil {
		return nil, err
	}

	// `viewed` covers only the days the rollup actually recorded page views for, which
	// is not the life of the site — migration 0138 added the column without a backfill
	// and the nginx history it would need is past logrotate's window. Publishing the
	// window beside the number is what keeps the figure from reading as all-time; null
	// means nothing has been rolled up yet, and a caller that ignores the field is no
	// worse off than before it existed.
	viewedSince := apigen.NilString{Null: true}
	if s.ViewedSince.Valid {
		viewedSince = apigen.NewNilString(s.ViewedSince.Time.Format(dateLayout))
	}

	return &apigen.EngagementStatsOK{Data: apigen.Engagement{
		Saved:            int64(s.Saved),
		Applied:          int64(s.Applied),
		Viewed:           int64(s.Viewed),
		ViewedSince:      viewedSince,
		CvsUploaded:      int64(s.CvsUploaded),
		CvsTailored:      int64(s.CvsTailored),
		AlertSubscribers: int64(s.AlertSubscribers),
		InboxesConnected: int64(s.InboxesConnected),
		SavedSearches:    int64(s.SavedSearches),
	}}, nil
}

// JobsActivity serves the public, unauthenticated job-activity time series:
// added vs. removed vacancies per period, aggregated to the requested granularity
// over a date range. The dense, gap-free series (missing periods → 0) is produced
// by the SQL generate_series queries; this handler only validates the window,
// picks the matching query, and maps rows to the wire envelope.
func (h *statsHandlers) JobsActivity(ctx context.Context, params apigen.JobsActivityParams) (*apigen.JobsActivity, error) {
	q, err := parseActivityQuery(params.Granularity.Value, params.From.Value, params.To.Value, time.Now().UTC())
	if err != nil {
		return nil, fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	rows, err := h.queries.ListJobActivity(ctx, db.ListJobActivityParams{
		Unit:   q.Granularity,
		FromTs: pgtype.Timestamp{Time: q.From, Valid: true},
		ToTs:   pgtype.Timestamp{Time: q.To, Valid: true},
	})
	if err != nil {
		return nil, err
	}

	points := make([]apigen.ActivityPoint, len(rows))
	for i, r := range rows {
		points[i] = apigen.ActivityPoint{Period: r.Period.Time.Format(dateLayout), Added: int64(r.Added), Removed: int64(r.Removed)}
	}

	return &apigen.JobsActivity{
		Data: points,
		Meta: apigen.ActivityMeta{
			Granularity: q.Granularity,
			From:        q.From.Format(dateLayout),
			To:          q.To.Format(dateLayout),
		},
	}, nil
}
