package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/ai/assistant"
	"github.com/strelov1/freehire/internal/engage/report"
)

// reportJobToolName is the MCP tool that files a job report.
const reportJobToolName = "report_job"

// reportMCPTools is the one tool a person's agent uses to flag a posting. It files through
// the same service as POST /jobs/:slug/reports, so the one-open-report-per-job rule
// and the moderator queue apply unchanged: an agent cannot do here what the
// person could not do on the job page.
//
// MCP only, not the in-app assistant: that surface already sits next to the page's own
// Report button.
func (h *assistantHandlers) reportMCPTools() []assistant.Tool {
	return []assistant.Tool{{
		Name:   reportJobToolName,
		Title:  "Report a job posting",
		Effect: assistant.EffectAdds,
		Description: "File a report on a vacancy that looks fake, spam, a scam, irrelevant, or that never " +
			"answers. It goes to freehire's moderators, who review it by hand; it does not hide the job " +
			"or change the user's own board. Only report when the user asks for it, or after you have " +
			"told them what you will send and they agreed. One open report per job per user.",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"slug": vacancySlugProperty,
				"reason": map[string]any{
					"type": "string",
					"enum": []string{"no_response", "not_relevant", "spam", "fraud", "other"},
					"description": "no_response: applicants never hear back. not_relevant: not a real job for " +
						"this board. spam: repeated or promotional. fraud: scam or fake employer. other.",
				},
				"details": map[string]any{
					"type":        "string",
					"description": "What is wrong, in the user's words. Optional but helps the moderator.",
				},
			},
			"required": []string{"slug", "reason"},
		},
		Run: func(ctx context.Context, userID int64, raw json.RawMessage) (any, error) {
			var in struct {
				Slug    string `json:"slug"`
				Reason  string `json:"reason"`
				Details string `json:"details"`
			}
			if err := assistant.DecodeArgs(raw, &in); err != nil {
				return nil, err
			}
			if err := requireSlug("slug", in.Slug); err != nil {
				return nil, err
			}
			if h.reports == nil {
				return nil, fiber.NewError(fiber.StatusServiceUnavailable, "job reports are not available")
			}
			jobID, err := h.reports.queries.GetJobIDBySlug(ctx, in.Slug)
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, jobNotFound(in.Slug)
			}
			if err != nil {
				return nil, fmt.Errorf("could not look up the vacancy: %w", err)
			}
			rep, err := h.reports.report.File(ctx, userID, jobID, report.FileInput{Reason: in.Reason, Details: in.Details})
			if err != nil {
				return nil, reportToolError(err)
			}
			return map[string]any{"status": rep.Status, "report_id": rep.ID, "reason": rep.Reason}, nil
		},
	}}
}

// reportToolError words the service's refusals for the agent that has to relay them.
func reportToolError(err error) error {
	switch {
	case errors.Is(err, report.ErrDuplicateOpen):
		return fiber.NewError(fiber.StatusConflict, "the user already has an open report for this job; a moderator has not decided it yet")
	case errors.Is(err, report.ErrInvalid):
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	default:
		return err
	}
}
