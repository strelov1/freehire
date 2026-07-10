package handler

import (
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/db"
)

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

// JobsActivity serves the public, unauthenticated job-activity time series:
// added vs. removed vacancies per period, aggregated to the requested granularity
// over a date range. The dense, gap-free series (missing periods → 0) is produced
// by the SQL generate_series queries; this handler only validates the window,
// picks the matching query, and maps rows to the wire envelope.
func (a *API) JobsActivity(c *fiber.Ctx) error {
	q, err := parseActivityQuery(c.Query("granularity"), c.Query("from"), c.Query("to"), time.Now().UTC())
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	rows, err := a.queries.ListJobActivity(c.Context(), db.ListJobActivityParams{
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
