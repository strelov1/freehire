package handler

import (
	"context"
	"errors"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/jackc/pgx/v5"

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
