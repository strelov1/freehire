package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/ai/assistant"
	"github.com/strelov1/freehire/internal/application/appevent"
	"github.com/strelov1/freehire/internal/application/jobtracking"
	"github.com/strelov1/freehire/internal/application/userjob"
)

// assistantTrackingTools are the tools that change the caller's own state: the
// bookmark, the application mark, the board stage and note, and the listing of
// what they are tracking. Every one acts as the session's owner — there is no
// credential to scope, and no way to address another user's rows.
func (h *assistantHandlers) assistantTrackingTools() []assistant.Tool {
	return []assistant.Tool{
		h.slugActionTool(
			"save_job",
			"Save a job",
			"Bookmark a vacancy for the user. Idempotent.",
			assistant.EffectAdds, true,
			func(ctx context.Context, userID int64, slug string) (jobtracking.Interaction, error) {
				return h.tracking.tracking.SaveJob(ctx, userID, slug)
			}),
		h.slugActionTool(
			"unsave_job",
			"Remove a saved job",
			"Remove a vacancy from the user's bookmarks. Idempotent.",
			assistant.EffectOverwrites, true,
			func(ctx context.Context, userID int64, slug string) (jobtracking.Interaction, error) {
				return h.tracking.tracking.Unsave(ctx, userID, slug)
			}),
		h.slugActionTool(
			"apply_job",
			"Mark a job as applied",
			"Mark a vacancy as applied to. This records the user's own application; it does not submit anything to the employer.",
			assistant.EffectAdds, false,
			func(ctx context.Context, userID int64, slug string) (jobtracking.Interaction, error) {
				// The assistant acts as the caller, but the ledger records who
				// observed the application, and that was the agent — not a person
				// filling in their own board.
				return h.tracking.tracking.MarkApplied(ctx, userID, slug, appevent.SourceAssistant)
			}),
		h.trackJobTool(),
		h.myJobsTool(),
	}
}

// slugActionTool builds the tools that take a single vacancy slug and perform one
// idempotent action on the caller's interaction with it.
func (h *assistantHandlers) slugActionTool(name, title, description string, effect assistant.Effect, idempotent bool, act func(context.Context, int64, string) (jobtracking.Interaction, error)) assistant.Tool {
	return slugTool("slug", name, title, description, effect, idempotent,
		func(ctx context.Context, userID int64, slug string) (any, error) {
			if h.tracking == nil || h.tracking.tracking == nil {
				return nil, unavailable("job tracking")
			}
			res, err := act(ctx, userID, slug)
			if err != nil {
				return nil, trackingToolError(slug, err)
			}
			return toResponse(res), nil
		})
}

// trackJobTool sets a vacancy's application stage and/or free-text note.
func (h *assistantHandlers) trackJobTool() assistant.Tool {
	return assistant.Tool{
		Name:  "track_job",
		Title: "Set a stage or note on a job",
		// Replaces the note it is given: the previous text is gone.
		Effect: assistant.EffectOverwrites,
		Description: "Set the application stage and/or a free-text note on a vacancy the user is tracking. " +
			"Omit a field to leave it unchanged. Valid stages: " + strings.Join(userjob.Stages, ", ") + ".",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"slug":  vacancySlugProperty,
				"stage": map[string]any{"type": "string", "enum": userjob.Stages, "description": "The application stage to set."},
				"note":  map[string]any{"type": "string", "description": "A free-text note to store against the application."},
			},
			"required": []string{"slug"},
		},
		Run: func(ctx context.Context, userID int64, raw json.RawMessage) (any, error) {
			var in struct {
				Slug  string  `json:"slug"`
				Stage *string `json:"stage"`
				Note  *string `json:"note"`
			}
			if err := assistant.DecodeArgs(raw, &in); err != nil {
				return nil, err
			}
			if err := requireSlug("slug", in.Slug); err != nil {
				return nil, err
			}
			if in.Stage == nil && in.Note == nil {
				return nil, fiber.NewError(fiber.StatusBadRequest, "nothing to change — pass a stage, a note, or both")
			}
			// Reject an invented stage here rather than letting the service's generic
			// error come back: the model needs to see the closed vocabulary to retry.
			if in.Stage != nil && !userjob.ValidStage(*in.Stage) {
				return nil, fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("unknown stage %q — valid stages are: %s", *in.Stage, strings.Join(userjob.Stages, ", ")))
			}
			if h.tracking == nil || h.tracking.tracking == nil {
				return nil, unavailable("job tracking")
			}
			res, err := h.tracking.tracking.Track(ctx, userID, in.Slug, in.Stage, in.Note, appevent.SourceAssistant)
			if err != nil {
				return nil, trackingToolError(in.Slug, err)
			}
			return toResponse(res), nil
		},
	}
}

// myJobsTool lists what the caller is tracking, under one of the board's filters.
func (h *assistantHandlers) myJobsTool() assistant.Tool {
	return assistant.Tool{
		Name:        "my_jobs",
		Title:       "List my tracked jobs",
		Effect:      assistant.EffectReads,
		Description: "List the vacancies the user is tracking — saved, applied, viewed or the whole board — with each one's stage and note.",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"filter": map[string]any{"type": "string", "description": "Which slice to list: all, viewed, saved, applied, board or dismissed. Defaults to the whole board."},
				"limit":  map[string]any{"type": "integer", "description": fmt.Sprintf("How many to return (default %d, max %d).", assistantDefaultSearchLimit, assistantMaxSearchLimit)},
				"offset": map[string]any{"type": "integer", "description": "Skip this many (paging); total says how many there are."},
			},
		},
		Run: func(ctx context.Context, userID int64, raw json.RawMessage) (any, error) {
			var in struct {
				Filter string `json:"filter"`
				Limit  int    `json:"limit"`
				Offset int    `json:"offset"`
			}
			if err := assistant.DecodeArgs(raw, &in); err != nil {
				return nil, err
			}
			if h.tracking == nil || h.tracking.tracking == nil {
				return nil, unavailable("job tracking")
			}
			limit, offset := toolPage(in.Limit, in.Offset)

			listing, err := h.tracking.tracking.ListTracked(ctx, userID, in.Filter, jobtracking.CardFull, int32(limit), int32(min(offset, math.MaxInt32)))
			if errors.Is(err, jobtracking.ErrInvalidFilter) {
				return nil, fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("unknown filter %q — use all, viewed, saved, applied, board or dismissed", in.Filter))
			}
			if err != nil {
				return nil, err
			}
			return map[string]any{
				"filter": string(listing.Filter),
				"total":  listing.Total(),
				"jobs":   listing.Items,
			}, nil
		},
	}
}

// trackingToolError renders a tracking failure for the model; an unknown slug is named
// precisely (jobNotFound).
func trackingToolError(slug string, err error) error {
	if errors.Is(err, jobtracking.ErrJobNotFound) {
		return jobNotFound(slug)
	}
	return err
}
