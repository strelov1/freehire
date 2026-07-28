package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/strelov1/freehire/internal/assistant"
	"github.com/strelov1/freehire/internal/db"
)

// presentJobsMaxEntries bounds one deck. A recommendation longer than this stops
// being a recommendation, and the model has already paid for every rationale it
// writes.
const presentJobsMaxEntries = 10

// presentJobsMaxWhyFits and presentJobsMaxConcerns bound the two rationale lists.
// Both are small on purpose: they render as chips beside the note, and a long
// list is a sign the model is padding rather than selecting.
const (
	presentJobsMaxWhyFits  = 4
	presentJobsMaxConcerns = 3
)

// jobSlugResolver answers which of a set of public slugs exist. It is one method
// so the tool can be unit-tested without a database — `assistantHandlers.queries`
// is a concrete *db.Queries, and a tool that called it directly would need
// Postgres to test at all.
type jobSlugResolver interface {
	ResolveSlugsToJobIDs(ctx context.Context, slugs []string) ([]db.ResolveSlugsToJobIDsRow, error)
}

// presentJobsEntry is one vacancy the model chose to show, carrying only what the
// model knows that the catalogue does not: why this vacancy, for this user.
type presentJobsEntry struct {
	Slug     string   `json:"slug"`
	Note     string   `json:"note"`
	WhyFits  []string `json:"why_fits,omitempty"`
	Concerns []string `json:"concerns,omitempty"`
}

type presentJobsArgs struct {
	Heading string             `json:"heading,omitempty"`
	Jobs    []presentJobsEntry `json:"jobs"`
}

// presentJobsDropped names a slug that did not resolve, so the model can replace
// it without regenerating the rationales that survived.
type presentJobsDropped struct {
	Slug   string `json:"slug"`
	Reason string `json:"reason"`
}

// presentJobsResult is a receipt, not a payload. It goes back into the model's
// history and stays there for the rest of the session, so it carries slugs and
// reasons and nothing else — the vacancies themselves are already in history from
// the search that found them, and the client fetches them by slug to render.
type presentJobsResult struct {
	Presented []string             `json:"presented"`
	Dropped   []presentJobsDropped `json:"dropped"`
}

// presentJobsTool is how a vacancy reaches the user's screen. It is the one tool
// whose purpose is presentation: it writes no state and reads only enough to
// refuse a slug that does not exist.
func presentJobsTool(resolver jobSlugResolver) assistant.Tool {
	return assistant.Tool{
		Name: "present_jobs",
		Description: "Show vacancies to the user. This is the ONLY way a vacancy reaches the " +
			"user's screen — never write a job link or a job's title into your reply text. " +
			"Each entry pairs a public_slug, copied from a search or read result, with your " +
			"reason for choosing it. Call it once per group of vacancies; call it again with a " +
			"different heading to show a second group.",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"heading": map[string]any{
					"type": "string",
					"description": "Optional short heading for this group, e.g. \"Top matches\". " +
						"Omit it when you are showing a single unlabelled set.",
				},
				"jobs": map[string]any{
					"type":     "array",
					"minItems": 1,
					"maxItems": presentJobsMaxEntries,
					"description": "The vacancies to show, in the order you want them ranked. " +
						"At most " + strconv.Itoa(presentJobsMaxEntries) + ".",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"slug": map[string]any{
								"type":        "string",
								"description": "The vacancy's public_slug, copied verbatim from a tool result. Never construct one from a title.",
							},
							"note": map[string]any{
								"type": "string",
								"description": "One sentence on what this role is and why it is worth this user's time. " +
									"Do NOT restate the title, company, location, seniority or posting date — the card " +
									"already shows all of them, and repeating them wastes the only line you get.",
							},
							"why_fits": map[string]any{
								"type":     "array",
								"maxItems": presentJobsMaxWhyFits,
								"items":    map[string]any{"type": "string"},
								"description": "Up to " + strconv.Itoa(presentJobsMaxWhyFits) + " short phrases naming concrete overlaps with " +
									"this user's stated experience, e.g. \"Terraform + EKS\". Omit the field entirely rather than padding it.",
							},
							"concerns": map[string]any{
								"type":     "array",
								"maxItems": presentJobsMaxConcerns,
								"items":    map[string]any{"type": "string"},
								"description": "Up to " + strconv.Itoa(presentJobsMaxConcerns) + " real gaps or caveats for this user, e.g. a " +
									"required language they did not list. Omit the field entirely rather than inventing one.",
							},
						},
						"required": []string{"slug", "note"},
					},
				},
			},
			"required": []string{"jobs"},
		},
		Run: func(ctx context.Context, _ int64, raw json.RawMessage) (any, error) {
			var in presentJobsArgs
			if err := assistant.DecodeArgs(raw, &in); err != nil {
				return nil, err
			}
			if err := in.validate(); err != nil {
				return nil, err
			}
			if resolver == nil {
				return nil, fmt.Errorf("cannot present vacancies: the catalogue is unavailable")
			}

			slugs := make([]string, len(in.Jobs))
			for i, job := range in.Jobs {
				slugs[i] = job.Slug
			}
			rows, err := resolver.ResolveSlugsToJobIDs(ctx, slugs)
			if err != nil {
				return nil, fmt.Errorf("cannot present vacancies: %w", err)
			}
			live := make(map[string]bool, len(rows))
			for _, row := range rows {
				live[row.PublicSlug] = true
			}

			// Built in the model's order: the deck renders as its ranking, and a map
			// iteration would scramble it.
			res := presentJobsResult{Presented: []string{}, Dropped: []presentJobsDropped{}}
			for _, job := range in.Jobs {
				if live[job.Slug] {
					res.Presented = append(res.Presented, job.Slug)
					continue
				}
				res.Dropped = append(res.Dropped, presentJobsDropped{
					Slug:   job.Slug,
					Reason: "no vacancy has this public_slug — copy it from a search or read result",
				})
			}
			if len(res.Presented) == 0 {
				return nil, fmt.Errorf("none of these slugs exist, so there is nothing to show: %s",
					strings.Join(slugs, ", "))
			}
			return res, nil
		},
	}
}

// validate rejects a deck the renderer could not draw, naming what is wrong so
// the model can fix it within the same turn.
func (a presentJobsArgs) validate() error {
	if len(a.Jobs) == 0 {
		return fmt.Errorf("jobs must contain at least one vacancy to show")
	}
	if len(a.Jobs) > presentJobsMaxEntries {
		return fmt.Errorf("jobs has %d entries; show at most %d in one call, and use a second call for a further group",
			len(a.Jobs), presentJobsMaxEntries)
	}
	for i, job := range a.Jobs {
		if strings.TrimSpace(job.Slug) == "" {
			return fmt.Errorf("jobs[%d] has no slug; copy the vacancy's public_slug from a search or read result", i)
		}
		if strings.TrimSpace(job.Note) == "" {
			return fmt.Errorf("jobs[%d] (%s) has no note; say in one sentence why this vacancy is worth the user's time", i, job.Slug)
		}
		if len(job.WhyFits) > presentJobsMaxWhyFits {
			return fmt.Errorf("jobs[%d] (%s) has %d why_fits; keep at most %d", i, job.Slug, len(job.WhyFits), presentJobsMaxWhyFits)
		}
		if len(job.Concerns) > presentJobsMaxConcerns {
			return fmt.Errorf("jobs[%d] (%s) has %d concerns; keep at most %d", i, job.Slug, len(job.Concerns), presentJobsMaxConcerns)
		}
	}
	return nil
}
