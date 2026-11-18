package handler

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/ai/assistant"
)

// unavailable is the 503 a tool answers when the service behind it was not wired in this
// deployment, as a fiber error so the MCP metrics classify it by status.
func unavailable(what string) error {
	return fiber.NewError(fiber.StatusServiceUnavailable, what+" is not available")
}

// toolPage normalises a tool's limit and offset: a missing limit takes the default, an
// oversized one the cap, and a negative offset zero.
func toolPage(limit, offset int) (int, int) {
	if limit <= 0 {
		limit = assistantDefaultSearchLimit
	}
	return min(limit, assistantMaxSearchLimit), max(offset, 0)
}

// jobNotFound is the one answer to a vacancy slug that matches nothing: usually the model
// quoting a slug it invented rather than one a search returned. A 404 so the MCP metrics
// count it as not_found.
func jobNotFound(slug string) error {
	return fiber.NewError(fiber.StatusNotFound, fmt.Sprintf("no vacancy with slug %q — use the public_slug from a search result", slug))
}

// requireSlug refuses an empty slug argument; arg is the argument's name.
func requireSlug(arg, slug string) error {
	if slug == "" {
		return fiber.NewError(fiber.StatusBadRequest, arg+" is required")
	}
	return nil
}

var vacancySlugProperty = map[string]any{"type": "string", "description": "The vacancy's public_slug, as search_jobs and my_jobs return it."}

// slugSchema is the schema of a tool whose only argument is a vacancy slug named arg
// (`slug` or, for the auto-apply tools, `job_slug`).
func slugSchema(arg string) map[string]any {
	return map[string]any{
		"type":                 "object",
		"properties":           map[string]any{arg: vacancySlugProperty},
		"required":             []string{arg},
		"additionalProperties": false,
	}
}

// slugTool builds a tool whose only argument is a vacancy slug named arg. It decodes
// strictly, refuses an empty slug and hands the slug to run.
func slugTool(arg, name, title, description string, effect assistant.Effect, idempotent bool,
	run func(ctx context.Context, userID int64, slug string) (any, error)) assistant.Tool {
	return assistant.Tool{
		Name: name, Title: title, Description: description, Effect: effect, Idempotent: idempotent,
		Schema: slugSchema(arg),
		Run: func(ctx context.Context, userID int64, raw json.RawMessage) (any, error) {
			var in map[string]string
			if err := assistant.DecodeArgs(raw, &in); err != nil {
				return nil, err
			}
			for k := range in {
				if k != arg {
					return nil, assistant.BadArg(k, fmt.Errorf("%w: json: unknown field %q", assistant.ErrInvalidArguments, k))
				}
			}
			if err := requireSlug(arg, in[arg]); err != nil {
				return nil, err
			}
			return run(ctx, userID, in[arg])
		},
	}
}
