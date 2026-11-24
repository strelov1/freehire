package handler

import (
	"context"
	"errors"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/ingest/employer"
	"github.com/strelov1/freehire/internal/platform/db"
)

// Cap unauthenticated telemetry by the same trusted client IP the server uses for
// public requests. The key is transient and never part of the stored counters.
func employerApplyClickLimit() fiber.Handler {
	return limiter.New(limiter.Config{
		Max:        60,
		Expiration: time.Minute,
		LimitReached: func(_ *fiber.Ctx) error {
			return fiber.NewError(fiber.StatusTooManyRequests, "too many Apply clicks")
		},
	})
}

func employerApplyClickRequest(c *fiber.Ctx) error {
	if err := refuseEncodedBody(c); err != nil {
		return err
	}
	if c.Get("Sec-Fetch-Site") == "cross-site" {
		return fiber.NewError(fiber.StatusForbidden, "cross-site Apply click")
	}
	if len(c.BodyRaw()) != 0 {
		return fiber.NewError(fiber.StatusBadRequest, "Apply click takes no body")
	}
	return c.Next()
}

func (h *employerHandlers) RecordEmployerVacancyApplyClick(ctx context.Context, params apigen.RecordEmployerVacancyApplyClickParams) error {
	return h.queries.RecordEmployerVacancyApplyClick(ctx, params.Slug)
}

func (h *employerHandlers) GetEmployerVacancyAnalytics(ctx context.Context, params apigen.GetEmployerVacancyAnalyticsParams) (*apigen.GetEmployerVacancyAnalyticsOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	counts, err := h.queries.GetEmployerVacancyAnalytics(ctx, db.GetEmployerVacancyAnalyticsParams{
		PublicSlug: params.Slug,
		ActorID:    userID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, employerError(employer.ErrJobNotFound)
	}
	if err != nil {
		return nil, err
	}
	return &apigen.GetEmployerVacancyAnalyticsOK{Data: apigen.EmployerVacancyAnalytics{
		Views:        counts.Views,
		ApplyClicks:  counts.ApplyClicks,
		Applications: counts.Applications,
	}}, nil
}

// outboundClicksDefaultDays is the window when `from` is absent; outboundClicksMaxDays caps the
// span so one read stays a bounded aggregate over a single job's index range.
const (
	outboundClicksDefaultDays = 30
	outboundClicksMaxDays     = 366
)

// outboundClicksWindow resolves the inclusive UTC day range of the report: `to` defaults to
// today, `from` to the default span before it.
func outboundClicksWindow(from, to string, now time.Time) (time.Time, time.Time, error) {
	toDay := truncateToDate(now)
	if to != "" {
		d, err := time.Parse(dateLayout, to)
		if err != nil {
			return time.Time{}, time.Time{}, fiber.NewError(fiber.StatusBadRequest, "invalid to date (want YYYY-MM-DD)")
		}
		toDay = d
	}
	fromDay := toDay.AddDate(0, 0, 1-outboundClicksDefaultDays)
	if from != "" {
		d, err := time.Parse(dateLayout, from)
		if err != nil {
			return time.Time{}, time.Time{}, fiber.NewError(fiber.StatusBadRequest, "invalid from date (want YYYY-MM-DD)")
		}
		fromDay = d
	}
	if fromDay.After(toDay) {
		return time.Time{}, time.Time{}, fiber.NewError(fiber.StatusBadRequest, "from is after to")
	}
	if fromDay.Before(toDay.AddDate(0, 0, 1-outboundClicksMaxDays)) {
		return time.Time{}, time.Time{}, fiber.NewError(fiber.StatusBadRequest, "range too large (max 366 days)")
	}
	return fromDay, toDay, nil
}

// GetEmployerVacancyOutboundClicks reports the human clicks freehire sent to the employer's site
// for a vacancy the caller's team owns. Any other slug is a 404, like the analytics report.
func (h *employerHandlers) GetEmployerVacancyOutboundClicks(ctx context.Context, params apigen.GetEmployerVacancyOutboundClicksParams) (*apigen.GetEmployerVacancyOutboundClicksOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	from, to, err := outboundClicksWindow(params.From.Or(""), params.To.Or(""), time.Now())
	if err != nil {
		return nil, err
	}
	jobID, err := h.queries.GetEmployerOwnedJobID(ctx, db.GetEmployerOwnedJobIDParams{PublicSlug: params.Slug, ActorID: userID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, employerError(employer.ErrJobNotFound)
	}
	if err != nil {
		return nil, err
	}
	rows, err := h.queries.GetJobOutboundClickStats(ctx, db.GetJobOutboundClickStatsParams{
		JobID:  jobID,
		FromAt: pgtype.Timestamptz{Time: from, Valid: true},
		ToAt:   pgtype.Timestamptz{Time: to.AddDate(0, 0, 1), Valid: true},
	})
	if err != nil {
		return nil, err
	}
	out := apigen.EmployerOutboundClicks{
		Days: []apigen.EmployerOutboundDay{}, Countries: []apigen.EmployerOutboundCount{}, Sources: []apigen.EmployerOutboundCount{},
	}
	for _, r := range rows {
		switch r.Dim {
		case "day":
			out.Days = append(out.Days, apigen.EmployerOutboundDay{Day: r.Key, Clicks: r.Clicks, Visitors: r.Visitors})
			out.Clicks += r.Clicks
			out.Visitors += r.Visitors
		case "country":
			out.Countries = append(out.Countries, apigen.EmployerOutboundCount{Key: r.Key, Clicks: r.Clicks})
		case "src":
			out.Sources = append(out.Sources, apigen.EmployerOutboundCount{Key: r.Key, Clicks: r.Clicks})
		}
	}
	return &apigen.GetEmployerVacancyOutboundClicksOK{Data: out}, nil
}
