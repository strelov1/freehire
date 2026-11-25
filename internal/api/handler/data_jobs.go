package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/identity/billing"
	"github.com/strelov1/freehire/internal/job/jobview"
)

// localsDataPlan marks a request on a Data endpoint (/data/jobs/*) whose caller holds the
// Data plan. The job reads it reaches serve the employer's own links (jobview.HideOrigin),
// with no sponsored placements and never a shared cache.
const localsDataPlan = "data.plan"

// dataPlanGate admits only a Data plan holder, after the key gate attached the caller.
func dataPlanGate(entitlements entitlementReader) fiber.Handler {
	return func(c *fiber.Ctx) error {
		userID, ok := c.Locals(auth.LocalsUserID).(int64)
		if !ok {
			return fiber.NewError(http.StatusUnauthorized, "unauthorized")
		}
		row, err := entitlements.GetUserAPIEntitlement(c.Context(), userID)
		if err != nil {
			return err
		}
		if apiPlanFrom(row, time.Now()).Tier != billing.APITierData {
			return fiber.NewError(http.StatusForbidden, "original job links are part of the Data plan")
		}
		c.Locals(localsDataPlan, true)
		// The links are the plan holder's alone: no shared cache may keep the answer.
		c.Set(fiber.HeaderCacheControl, "private, no-store")
		return c.Next()
	}
}

func isDataPlanRequest(ctx context.Context) bool {
	data, _ := ctx.Value(localsDataPlan).(bool)
	return data
}

// hideOrigins drops the employer links from views served anywhere but a Data endpoint.
func hideOrigins(ctx context.Context, views []jobview.Job) {
	if isDataPlanRequest(ctx) {
		return
	}
	for i := range views {
		views[i].HideOrigin()
	}
}

// SearchDataJobs is /jobs/search for the Data plan; dataPlanGate marks the request.
func (h *searchHandlers) SearchDataJobs(ctx context.Context, _ apigen.SearchDataJobsParams) (*apigen.JobSearchPageHeaders, error) {
	return h.SearchJobs(ctx, apigen.SearchJobsParams{})
}

// GetDataJob is /jobs/{slug} for the Data plan; dataPlanGate marks the request.
func (h *jobsHandlers) GetDataJob(ctx context.Context, params apigen.GetDataJobParams) (*apigen.GetDataJobOK, error) {
	out, err := h.GetJob(ctx, apigen.GetJobParams{Slug: params.Slug})
	if err != nil {
		return nil, err
	}
	return &apigen.GetDataJobOK{Data: out.Data}, nil
}
