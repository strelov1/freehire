package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
	"github.com/strelov1/freehire/internal/ai/assistant"
	"github.com/strelov1/freehire/internal/job/jobview"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/llm"
	"github.com/strelov1/freehire/internal/search/search"
)

// lookupMiss answers a slug lookup that failed: a row that does not exist is the sentence the
// agent acts on, as a 404 so the MCP metrics count it as not_found. Any other failure keeps
// its cause for the metrics and logs (errors.Is/As see through it) but reads as a plain retry
// to the agent: the account server sends a tool error's text verbatim, and a driver's message
// is ours, not the caller's.
func lookupMiss(err error, missing string) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return fiber.NewError(fiber.StatusNotFound, missing)
	}
	return lookupFault{err}
}

type lookupFault struct{ err error }

func (f lookupFault) Error() string { return "could not look this up right now; try again" }

func (f lookupFault) Unwrap() error { return f.err }

// getJobTool reads one vacancy by its public slug.
func (h *assistantHandlers) getJobTool() assistant.Tool {
	return assistant.Tool{
		Name:        "get_job",
		Title:       "Read one job posting",
		Effect:      assistant.EffectReads,
		Description: "Read one vacancy in full by its public_slug.",
		Schema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"slug": vacancySlugProperty},
			"required":   []string{"slug"},
		},
		Run: func(ctx context.Context, _ int64, raw json.RawMessage) (any, error) {
			var in struct {
				Slug string `json:"slug"`
			}
			if err := assistant.DecodeArgs(raw, &in); err != nil {
				return nil, err
			}
			if in.Slug == "" {
				return nil, assistant.BadArg("slug", fiber.NewError(fiber.StatusBadRequest, "slug is required"))
			}
			_, view, err := loadAgentJob(ctx, func(ctx context.Context, slug string) (db.Job, error) {
				return agentJobBySlug(ctx, h.queries, slug)
			}, nil, in.Slug)
			if err != nil {
				return nil, lookupMiss(err, jobNotFound(in.Slug).Error())
			}
			view.Description = formatDescription(view.Description, "markdown")
			return view, nil
		},
	}
}

// getCompanyTool reads one company and a page of its open vacancies.
func (h *assistantHandlers) getCompanyTool() assistant.Tool {
	return assistant.Tool{
		Name:   "get_company",
		Title:  "Read one employer",
		Effect: assistant.EffectReads,
		Description: "Read one company and a page of its open vacancies, by company slug. Each vacancy " +
			"carries the first 400 characters of its description.",
		Schema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"slug": map[string]any{"type": "string", "description": "The company slug (as carried by a vacancy)."}},
			"required":   []string{"slug"},
		},
		Run: func(ctx context.Context, _ int64, raw json.RawMessage) (any, error) {
			var in struct {
				Slug string `json:"slug"`
			}
			if err := assistant.DecodeArgs(raw, &in); err != nil {
				return nil, err
			}
			if in.Slug == "" {
				return nil, assistant.BadArg("slug", fiber.NewError(fiber.StatusBadRequest, "slug is required"))
			}
			company, canonical, err := companyBySlugOrAlias(ctx, h.queries, in.Slug)
			if canonical != "" {
				company, err = h.queries.GetCompany(ctx, canonical)
			}
			if err != nil {
				return nil, lookupMiss(err, fmt.Sprintf("no company with slug %q", in.Slug))
			}
			jobs, err := h.queries.ListJobsByCompany(ctx, db.ListJobsByCompanyParams{
				CompanySlug: company.Slug,
				Limit:       assistantDefaultSearchLimit,
				Offset:      0,
			})
			if err != nil {
				return nil, err
			}
			views, err := jobview.FromRows(jobs)
			if err != nil {
				return nil, err
			}
			// The page is for picking a vacancy; ten full bodies are too big for one call.
			for i := range views {
				views[i].Description = llm.ClipRunesEllipsis(formatDescription(views[i].Description, "markdown"), mcpSearchExcerptRunes)
			}
			return map[string]any{"company": companyViewFrom(company), "jobs": views}, nil
		},
	}
}

// marketFitTool scores a skill list against the live market for a filtered role:
// the coverage headline, the must-have skills held, and the missing skills that
// unlock the most vacancies.
func (h *assistantHandlers) marketFitTool() assistant.Tool {
	return assistant.Tool{
		Name:   "market_fit",
		Title:  "Check skills against the market",
		Effect: assistant.EffectReads,
		Description: "Score a skill list against the live open-vacancy market for a filtered role: what share of " +
			"vacancies list at least one of the skills, which must-have skills are held, and which missing skills " +
			"unlock the most vacancies. Here `skills` is the MEASURED set, not a filter.",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"skills":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Canonical skill slugs to measure (from the facets tool)."},
				"filters": assistantFilterSchema("Facet filters defining the role to measure against."),
			},
			"required": []string{"skills"},
		},
		Run: func(ctx context.Context, _ int64, raw json.RawMessage) (any, error) {
			var in struct {
				Skills  []string            `json:"skills"`
				Filters assistantFilterArgs `json:"filters"`
			}
			if err := assistant.DecodeArgs(raw, &in); err != nil {
				return nil, err
			}
			if h.search == nil || h.search.facets == nil {
				return nil, unavailable("search")
			}
			skills := nonEmptyStrings(in.Skills)
			if len(skills) == 0 {
				return nil, assistant.BadArg("skills", fiber.NewError(fiber.StatusBadRequest, "skills is required — list the canonical slugs to measure"))
			}
			if len(skills) > maxCoverageSkills {
				return nil, assistant.BadArg("skills", fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("too many skills (max %d)", maxCoverageSkills)))
			}
			vals, err := in.Filters.values()
			if err != nil {
				return nil, err
			}
			// The measured skills must not also filter the role, or coverage would be
			// 100% by construction — the same reason the HTTP endpoint strips them.
			stripSkillParams(vals)

			v, err := h.resume.coverageFor(ctx, search.FilterFromValues(vals), skills, skills, nil, skills)
			if err != nil {
				return nil, searchToolError(err)
			}
			// Coherence is a CV-section metric; a flat skill list has no sections.
			v.CoherencePercent = 0
			return v, nil
		},
	}
}
