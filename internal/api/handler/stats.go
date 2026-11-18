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

// statsHandlers serves the public transparency and market-insights reads. All are
// unauthenticated and aggregate-only: no record-level field or user identifier is exposed.
type statsHandlers struct {
	queries *db.Queries

	// cache holds the snapshot the rollup worker publishes; estimator is the approximate
	// open-job count served when there is none. A nil cache degrades to the estimate.
	cache     cache.Cache
	estimator catalogstats.Estimator

	// pool backs the site-status database check (IngestStatus), a live pool.Ping like /health.
	pool *pgxpool.Pool
}

func newStatsHandlers(queries *db.Queries, c cache.Cache, pool *pgxpool.Pool) *statsHandlers {
	return &statsHandlers{queries: queries, cache: c, estimator: queries, pool: pool}
}

// register mounts the stats and insights reads. optionalAuth attaches the caller when
// signed in and never rejects; only /insights/roles takes it, to overlay the caller's own
// skill coverage. Every other route stays anonymous and aggregate-only.
func (h *statsHandlers) register(api fiber.Router, optionalAuth fiber.Handler, contract fiber.Handler) {
	// One response carries every catalogue-size figure, so /about and /open render the
	// same snapshot instead of disagreeing.
	api.Get("/stats/catalog", contract)

	// Added vs. removed vacancies per period, from the job_daily_stats rollup (cmd/rollup-stats).
	api.Get("/stats/jobs-activity", contract)

	// Catalogue size per UTC day (catalog_size_daily).
	api.Get("/stats/catalog-history", contract)

	// Cumulative registrations per UTC day, computed on the fly from users.created_at (no rollup).
	api.Get("/stats/user-growth", contract)

	// Engagement counts across all users, aggregated from user_jobs.
	api.Get("/stats/engagement", contract)

	// Facet-distribution snapshot from the insights_facet_stats rollup (cmd/rollup-facets),
	// which keeps /open off the live Meilisearch facet count.
	api.Get("/stats/facets", contract)

	// Skills most often listed beside one skill, from insights_skill_neighbours
	// (cmd/rollup-facets) so a page render never asks the engine for a skills facet.
	api.Get("/stats/skills/:slug/neighbours", contract)

	// Market intelligence served from the insights_* rollups (cmd/rollup-stats).
	api.Get("/insights/roles", optionalAuth, contract)
	api.Get("/insights/skills", contract)
	api.Get("/insights/velocity", contract)
	api.Get("/insights/salary", contract)
	api.Get("/insights/salary/history", contract)
	api.Get("/insights/companies", contract)
	api.Get("/insights/role-countries", contract)
	api.Get("/insights/role-pulse", contract)

	// Per-provider health rollup over board_health, sanitized (no error text or board identifiers).
	api.Get("/status", contract)

	// The /status board_health rollup joined with the per-source snapshot (cmd/rollup-stats)
	// and classified through the adapter registry. Sanitized like /status, and no posting URL.
	api.Get("/sources", contract)
}

// dateLayout is the wire format for every date the activity endpoint reads and
// writes (ISO 8601 calendar date, UTC).
const dateLayout = "2006-01-02"

// maxRangeDays caps the [from, to] span served. The read builds a per-day generate_series,
// so an unbounded range on an unauthenticated endpoint is a cheap abuse vector.
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

// parseActivityQuery validates the granularity/from/to params and resolves the read window.
// Granularity defaults to "day" and must be day/week/month. `to` defaults to today (UTC date
// of now); `from` defaults to a per-granularity window before `to`. now is a parameter so
// the defaults are testable.
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

// UserGrowth serves the cumulative member count per UTC day plus each day's new signups.
// The dense series comes from the SQL query; the query selects no user identifier.
// No members yields 200 with data: [].
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

// CatalogHistory serves the catalogue's size per UTC day: measured days from cmd/rollup-stats,
// earlier days reconstructed by cmd/backfill-catalog-history and marked so.
func (h *statsHandlers) CatalogHistory(ctx context.Context) (*apigen.CatalogHistoryOK, error) {
	rows, err := h.queries.ListCatalogSize(ctx)
	if err != nil {
		return nil, err
	}
	points := make([]apigen.CatalogSizePoint, len(rows))
	for i, r := range rows {
		points[i] = apigen.CatalogSizePoint{Date: r.Day.Time.Format(dateLayout), OpenJobs: r.OpenJobs, Reconstructed: r.Reconstructed}
	}
	return &apigen.CatalogHistoryOK{Data: points}, nil
}

// CatalogScale serves the catalogue-scale snapshot: open postings (raw and de-duplicated),
// companies, sources, ATS platforms and Telegram channels.
//
// It never fails. A cold or unreachable cache, or a payload from an older build, degrades
// to the approximate open-job count, and `exact` reports which one the caller holds.
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

// engagementCacheKey and engagementCacheTTL hold the engagement row between requests:
// the query sums the whole, ever-growing job_daily_views rollup and nobody reads the totals to the minute.
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

// EngagementStats serves the engagement counts: user_jobs saved, applied and viewed, plus CVs
// uploaded and tailored, alert subscribers, inboxes connected and saved searches. Integer
// totals only; an empty database yields zeros.
func (h *statsHandlers) EngagementStats(ctx context.Context) (*apigen.EngagementStatsOK, error) {
	s, err := h.engagementStats(ctx)
	if err != nil {
		return nil, err
	}

	// `viewed` covers only the days the rollup recorded page views for, not the life of the
	// site (no backfill exists). ViewedSince publishes that window so the figure does not
	// read as all-time; null means nothing has been rolled up yet.
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

// JobsActivity serves added vs. removed vacancies per period at the requested granularity.
// The dense series (missing periods are 0) comes from the SQL generate_series queries.
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
