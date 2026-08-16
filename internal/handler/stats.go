package handler

import (
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/cache"
	"github.com/strelov1/freehire/internal/catalogstats"
	"github.com/strelov1/freehire/internal/db"
)

// statsHandlers serves the public transparency and market-insights reads:
// catalogue activity, member growth, engagement counts, the facet snapshot, the
// insights rollups, and the ingest-fleet status. All are unauthenticated,
// aggregate-only reads — no record-level field or user identifier is exposed.
type statsHandlers struct {
	queries *db.Queries

	// cache and estimator back the catalogue-scale read. The cache holds the snapshot
	// the rollup worker publishes; the estimator is the approximate open-job count
	// served when no snapshot is available. Neither is required — a nil cache degrades
	// to the estimate, which is the same path a cold or unreachable cache takes.
	cache     cache.Cache
	estimator catalogstats.Estimator
}

func newStatsHandlers(queries *db.Queries, c cache.Cache) *statsHandlers {
	return &statsHandlers{queries: queries, cache: c, estimator: queries}
}

func (h *statsHandlers) register(api fiber.Router) {
	// Public catalogue-activity time series (added vs. removed vacancies per period),
	// unauthenticated like the other public reads. Served from the job_daily_stats
	// rollup (cmd/rollup-stats); the /trends SPA page renders it as a bar chart.
	// Public catalogue-scale figures, unauthenticated like the other public reads.
	// One response carries every number a surface quotes about how big the catalogue
	// is, so /about and /open render the same snapshot instead of taking their own
	// totals at their own moments and disagreeing.
	api.Get("/stats/catalog", h.CatalogScale)

	api.Get("/stats/jobs-activity", h.JobsActivity)

	// Public member-growth time series (cumulative registrations per UTC day),
	// unauthenticated like the other public reads. Computed on the fly from
	// users.created_at (no rollup); the /open transparency page renders it as a
	// bar chart. Aggregate-only — no user identifier is exposed.
	api.Get("/stats/user-growth", h.UserGrowth)

	// Public engagement counts (jobs saved / applied / viewed across all users),
	// unauthenticated like the other public reads. Aggregate-only from user_jobs;
	// the /open transparency page renders them as a stat-strip.
	api.Get("/stats/engagement", h.EngagementStats)

	// Public facet-distribution snapshot (countries, skills, seniority, work_mode),
	// unauthenticated like the other public reads. Served from the insights_facet_stats
	// rollup (cmd/rollup-facets) so the /open transparency page's "what's inside"
	// section stays off the live Meilisearch facet count. Aggregate-only — per-value
	// counts only.
	api.Get("/stats/facets", h.StatsFacets)

	// Public Trends & Insights reads: aggregate market intelligence (role & skill
	// demand, hiring velocity, salary bands) served from the insights_* rollups
	// (cmd/rollup-stats), unauthenticated like the other public reads. Aggregate-only
	// — no record-level field is exposed.
	api.Get("/insights/roles", h.InsightsRoles)
	api.Get("/insights/skills", h.InsightsSkills)
	api.Get("/insights/velocity", h.InsightsVelocity)
	api.Get("/insights/salary", h.InsightsSalary)
	api.Get("/insights/companies", h.InsightsCompanies)

	// Public ingest-fleet status, unauthenticated like the other public reads.
	// A per-provider health rollup over board_health, sanitized (no error text or
	// board identifiers); the /status page renders it as a status board.
	api.Get("/status", h.IngestStatus)
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

// growthPoint is one point on the member-growth series: a UTC calendar date and
// the cumulative member count as of that day.
type growthPoint struct {
	Date  string `json:"date"`
	Total int32  `json:"total"`
}

// UserGrowth serves the public, unauthenticated member-growth time series: the
// cumulative count of registered members per UTC day, from the first registration
// through today. The dense, gap-free, monotonically non-decreasing series is
// produced by the SQL query; this handler only maps rows to the wire envelope.
// Aggregate-only — the query selects no user identifier, so no personal field can
// leak here. An empty catalogue yields an empty series (200 with data: []).
func (h *statsHandlers) UserGrowth(c *fiber.Ctx) error {
	rows, err := h.queries.ListUserGrowth(c.Context())
	if err != nil {
		return err
	}

	points := make([]growthPoint, len(rows))
	for i, r := range rows {
		points[i] = growthPoint{Date: r.Day.Time.Format(dateLayout), Total: r.Total}
	}

	return c.JSON(fiber.Map{"data": points})
}

// EngagementStats serves the public, unauthenticated engagement counts: how many
// user_jobs rows have been saved, applied to, and viewed, plus how many résumés
// have been uploaded, CVs tailored to a vacancy, matches analyzed, inboxes
// connected, and searches saved. Aggregate-only — the query selects nothing but
// integer totals, so no per-user field can leak. An empty database yields all
// zeros (200).
// CatalogScale serves the catalogue-scale snapshot: how many open postings the
// catalogue holds, from how many companies, across how many sources, ATS platforms and
// Telegram channels.
//
// It never fails. A cold cache (before the first rollup run), an unreachable one, and a
// payload left by an older build all degrade to the approximate open-job count, and
// `exact` reports which of the two the caller holds — a transparency page showing a
// labelled estimate beats one showing a 500.
func (h *statsHandlers) CatalogScale(c *fiber.Ctx) error {
	result := catalogstats.Load(c.Context(), h.cache, h.estimator)

	return c.JSON(fiber.Map{
		"data": fiber.Map{
			"open_jobs":         result.OpenJobs,
			"companies":         result.Companies,
			"sources":           result.Sources,
			"ats_platforms":     result.ATSPlatforms,
			"telegram_channels": result.TelegramChannels,
			"computed_at":       result.ComputedAt,
			"exact":             result.Exact,
		},
	})
}

func (h *statsHandlers) EngagementStats(c *fiber.Ctx) error {
	s, err := h.queries.GetEngagementStats(c.Context())
	if err != nil {
		return err
	}

	return c.JSON(fiber.Map{
		"data": fiber.Map{
			"saved":             s.Saved,
			"applied":           s.Applied,
			"viewed":            s.Viewed,
			"cvs_uploaded":      s.CvsUploaded,
			"cvs_tailored":      s.CvsTailored,
			"match_analyses":    s.MatchAnalyses,
			"inboxes_connected": s.InboxesConnected,
			"saved_searches":    s.SavedSearches,
		},
	})
}

// JobsActivity serves the public, unauthenticated job-activity time series:
// added vs. removed vacancies per period, aggregated to the requested granularity
// over a date range. The dense, gap-free series (missing periods → 0) is produced
// by the SQL generate_series queries; this handler only validates the window,
// picks the matching query, and maps rows to the wire envelope.
func (h *statsHandlers) JobsActivity(c *fiber.Ctx) error {
	q, err := parseActivityQuery(c.Query("granularity"), c.Query("from"), c.Query("to"), time.Now().UTC())
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	rows, err := h.queries.ListJobActivity(c.Context(), db.ListJobActivityParams{
		Unit:   q.Granularity,
		FromTs: pgtype.Timestamp{Time: q.From, Valid: true},
		ToTs:   pgtype.Timestamp{Time: q.To, Valid: true},
	})
	if err != nil {
		return err
	}

	points := make([]activityPoint, len(rows))
	for i, r := range rows {
		points[i] = activityPoint{Period: r.Period.Time.Format(dateLayout), Added: r.Added, Removed: r.Removed}
	}

	return c.JSON(fiber.Map{
		"data": points,
		"meta": fiber.Map{
			"granularity": q.Granularity,
			"from":        q.From.Format(dateLayout),
			"to":          q.To.Format(dateLayout),
		},
	})
}
