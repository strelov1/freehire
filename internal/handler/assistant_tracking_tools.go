package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/strelov1/freehire/internal/assistant"
	"github.com/strelov1/freehire/internal/jobtracking"
	"github.com/strelov1/freehire/internal/userjob"
)

// assistantTrackingTools are the tools that change the caller's own state: the
// bookmark, the application mark, the board stage and note, and the listing of
// what they are tracking. Every one acts as the session's owner — there is no
// credential to scope, and no way to address another user's rows.
func (a *API) assistantTrackingTools() []assistant.Tool {
	return []assistant.Tool{
		a.slugActionTool(
			"save_job",
			"Bookmark a vacancy for the user. Idempotent.",
			func(ctx context.Context, userID int64, slug string) (jobtracking.Interaction, error) {
				return a.tracking.SaveJob(ctx, userID, slug)
			}),
		a.slugActionTool(
			"unsave_job",
			"Remove a vacancy from the user's bookmarks. Idempotent.",
			func(ctx context.Context, userID int64, slug string) (jobtracking.Interaction, error) {
				return a.tracking.Unsave(ctx, userID, slug)
			}),
		a.slugActionTool(
			"apply_job",
			"Mark a vacancy as applied to. This records the user's own application; it does not submit anything to the employer.",
			func(ctx context.Context, userID int64, slug string) (jobtracking.Interaction, error) {
				return a.tracking.MarkApplied(ctx, userID, slug)
			}),
		a.trackJobTool(),
		a.myJobsTool(),
	}
}

// slugActionTool builds the tools that take a single vacancy slug and perform one
// idempotent action on the caller's interaction with it.
func (a *API) slugActionTool(name, description string, act func(context.Context, int64, string) (jobtracking.Interaction, error)) assistant.Tool {
	return assistant.Tool{
		Name:        name,
		Description: description,
		Schema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"slug": map[string]any{"type": "string", "description": "The vacancy's public_slug."}},
			"required":   []string{"slug"},
		},
		Run: func(ctx context.Context, userID int64, raw json.RawMessage) (any, error) {
			var in struct {
				Slug string `json:"slug"`
			}
			if err := assistant.DecodeArgs(raw, &in); err != nil {
				return nil, err
			}
			if in.Slug == "" {
				return nil, errors.New("slug is required")
			}
			if a.tracking == nil {
				return nil, errors.New("job tracking is not available")
			}
			res, err := act(ctx, userID, in.Slug)
			if err != nil {
				return nil, trackingToolError(in.Slug, err)
			}
			return toResponse(res), nil
		},
	}
}

// trackJobTool sets a vacancy's application stage and/or free-text note.
func (a *API) trackJobTool() assistant.Tool {
	return assistant.Tool{
		Name: "track_job",
		Description: "Set the application stage and/or a free-text note on a vacancy the user is tracking. " +
			"Omit a field to leave it unchanged. Valid stages: " + strings.Join(userjob.Stages, ", ") + ".",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"slug":  map[string]any{"type": "string", "description": "The vacancy's public_slug."},
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
			if in.Slug == "" {
				return nil, errors.New("slug is required")
			}
			if in.Stage == nil && in.Note == nil {
				return nil, errors.New("nothing to change — pass a stage, a note, or both")
			}
			// Reject an invented stage here rather than letting the service's generic
			// error come back: the model needs to see the closed vocabulary to retry.
			if in.Stage != nil && !userjob.ValidStage(*in.Stage) {
				return nil, fmt.Errorf("unknown stage %q — valid stages are: %s", *in.Stage, strings.Join(userjob.Stages, ", "))
			}
			if a.tracking == nil {
				return nil, errors.New("job tracking is not available")
			}
			res, err := a.tracking.Track(ctx, userID, in.Slug, in.Stage, in.Note)
			if err != nil {
				return nil, trackingToolError(in.Slug, err)
			}
			return toResponse(res), nil
		},
	}
}

// myJobsTool lists what the caller is tracking, under one of the board's filters.
func (a *API) myJobsTool() assistant.Tool {
	return assistant.Tool{
		Name:        "my_jobs",
		Description: "List the vacancies the user is tracking — saved, applied, viewed or the whole board — with each one's stage and note.",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"filter": map[string]any{"type": "string", "description": "Which slice to list: all, viewed, saved, applied, board or dismissed. Defaults to the whole board."},
				"limit":  map[string]any{"type": "integer", "description": fmt.Sprintf("How many to return (default %d, max %d).", assistantDefaultSearchLimit, assistantMaxSearchLimit)},
			},
		},
		Run: func(ctx context.Context, userID int64, raw json.RawMessage) (any, error) {
			var in struct {
				Filter string `json:"filter"`
				Limit  int    `json:"limit"`
			}
			if err := assistant.DecodeArgs(raw, &in); err != nil {
				return nil, err
			}
			if a.tracking == nil {
				return nil, errors.New("job tracking is not available")
			}
			limit := in.Limit
			if limit <= 0 {
				limit = assistantDefaultSearchLimit
			}
			limit = min(limit, assistantMaxSearchLimit)

			listing, err := a.tracking.ListTracked(ctx, userID, in.Filter, int32(limit), 0)
			if errors.Is(err, jobtracking.ErrInvalidFilter) {
				return nil, fmt.Errorf("unknown filter %q — use all, viewed, saved, applied, board or dismissed", in.Filter)
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

// trackingToolError renders a tracking failure for the model. An unknown slug is
// the one case worth naming precisely — it is usually the model quoting a slug it
// invented rather than one a search returned.
func trackingToolError(slug string, err error) error {
	if errors.Is(err, jobtracking.ErrJobNotFound) {
		return fmt.Errorf("no vacancy with slug %q — use the public_slug from a search result", slug)
	}
	return err
}
