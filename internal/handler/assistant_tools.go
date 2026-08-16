package handler

import (
	"github.com/google/uuid"

	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/strelov1/freehire/internal/assistant"
	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/jobview"
	"github.com/strelov1/freehire/internal/search"
)

// assistantMaxSearchLimit caps how many vacancies one search tool call returns.
// Each hit carries its full description, so the ceiling is the model's context
// window rather than the API's usual paging limit.
const assistantMaxSearchLimit = 20

// assistantDefaultSearchLimit matches the CLI's default page size.
const assistantDefaultSearchLimit = 10

// assistantDiscoveryTools are the read-only tools every session gets: the filter
// vocabulary, vacancy search and reads, the market-fit score, and the one tool
// that puts a vacancy on the user's screen. They call the same services the HTTP
// handlers call, so a tool result and the public API can never disagree.
func (h *assistantHandlers) assistantDiscoveryTools() []assistant.Tool {
	return []assistant.Tool{
		h.getProfileTool(),
		h.facetsTool(),
		h.searchJobsTool(),
		h.getJobTool(),
		h.getCompanyTool(),
		h.marketFitTool(),
		presentJobsTool(h.queries),
	}
}

// assistantFilterArgs is the facet filter shared by the searching tools: a map of
// public facet name to the values to include. It is deliberately the same
// vocabulary the `facets` tool reports, so the agent can only filter on values it
// has seen.
type assistantFilterArgs map[string][]string

// values renders the filter as the query form the search layer already parses,
// rejecting any facet outside the published vocabulary. An unknown facet is an
// error rather than a silent no-op: FilterFromValues ignores parameters it does
// not know, so a hallucinated filter would come back as an unfiltered result set
// that looks like a confident answer.
func (f assistantFilterArgs) values() (url.Values, error) {
	vals := url.Values{}
	for name, list := range f {
		if _, ok := search.StringFacets[name]; !ok {
			return nil, fmt.Errorf("unknown filter %q — call the facets tool for the valid filter names and values; valid names are: %s",
				name, strings.Join(assistantFacetNames(), ", "))
		}
		for _, v := range list {
			if v = strings.TrimSpace(v); v != "" {
				vals.Add(name, v)
			}
		}
	}
	return vals, nil
}

// assistantFacetNames lists the filterable facet names, sorted so the error a
// model reads is stable.
func assistantFacetNames() []string {
	out := make([]string, 0, len(search.StringFacets))
	for name := range search.StringFacets {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// facetsTool reports the live filter vocabulary — every facet's values with a
// vacancy count each, plus the numeric ranges. It is the tool the agent is told to
// call before filtering, so its filter values and skill slugs are the market's own
// rather than invented.
func (h *assistantHandlers) facetsTool() assistant.Tool {
	return assistant.Tool{
		Name: "facets",
		Description: "List the live filter vocabulary: every filter's valid values with a vacancy count each, " +
			"plus numeric ranges. Call this BEFORE filtering a search, and take skill slugs from its `skills` " +
			"distribution — never invent a filter value.",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"filters": assistantFilterSchema("Narrow the vocabulary to a slice of the market (e.g. one category)."),
			},
		},
		Run: func(ctx context.Context, _ int64, raw json.RawMessage) (any, error) {
			var in struct {
				Filters assistantFilterArgs `json:"filters"`
			}
			if err := assistant.DecodeArgs(raw, &in); err != nil {
				return nil, err
			}
			if h.search.facets == nil {
				return nil, errors.New("search is not available")
			}
			vals, err := in.Filters.values()
			if err != nil {
				return nil, err
			}
			res, err := h.search.facets.FacetCounts(ctx, search.FacetParams{
				Filter: search.FilterFromValues(vals),
				Facets: facetAttributes(),
			})
			if err != nil {
				return nil, err
			}
			return facetView(res), nil
		},
	}
}

// searchJobsTool searches the catalogue with a keyword and facet filters, hitting
// the same index the public search does and rehydrating each result's full
// description so the agent screens a whole set in one call.
func (h *assistantHandlers) searchJobsTool() assistant.Tool {
	return assistant.Tool{
		Name: "search_jobs",
		Description: "Search open vacancies by keyword and facet filters. Every result carries its full " +
			"description as markdown and its public_slug — link a vacancy as https://freehire.me/jobs/<public_slug>. " +
			"Take filter names and values from the facets tool.",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query":   map[string]any{"type": "string", "description": "Free-text query. Omit to browse the freshest postings under the filters."},
				"filters": assistantFilterSchema("Facet filters; values must come from the facets tool."),
				"limit":   map[string]any{"type": "integer", "description": fmt.Sprintf("How many vacancies to return (default %d, max %d).", assistantDefaultSearchLimit, assistantMaxSearchLimit)},
				"offset":  map[string]any{"type": "integer", "description": "Skip this many results (paging)."},
			},
		},
		Run: func(ctx context.Context, _ int64, raw json.RawMessage) (any, error) {
			var in struct {
				Query   string              `json:"query"`
				Filters assistantFilterArgs `json:"filters"`
				Limit   int                 `json:"limit"`
				Offset  int                 `json:"offset"`
			}
			if err := assistant.DecodeArgs(raw, &in); err != nil {
				return nil, err
			}
			if h.search.search == nil {
				return nil, errors.New("search is not available")
			}
			vals, err := in.Filters.values()
			if err != nil {
				return nil, err
			}

			limit := in.Limit
			if limit <= 0 {
				limit = assistantDefaultSearchLimit
			}
			limit = min(limit, assistantMaxSearchLimit)
			offset := max(in.Offset, 0)

			// An empty query has no relevance to rank by, so browse the freshest first —
			// the same rule the HTTP search applies.
			var order []string
			if in.Query == "" {
				order = []string{"posted_at:desc"}
			}

			res, err := h.search.search.Search(ctx, search.SearchParams{
				Query:  in.Query,
				Filter: search.FilterFromValues(vals),
				Sort:   order,
				Limit:  limit,
				Offset: offset,
			})
			if err != nil {
				return nil, err
			}
			if err := h.search.hydrateDescriptions(ctx, res.Hits); err != nil {
				return nil, err
			}
			jobs := make([]jobview.Job, len(res.Hits))
			for i, hit := range res.Hits {
				hit.Description = formatDescription(hit.Description, "markdown")
				jobs[i] = hit.Job
			}
			return map[string]any{"total": res.Total, "limit": limit, "offset": offset, "jobs": jobs}, nil
		},
	}
}

// getJobTool reads one vacancy by its public slug.
func (h *assistantHandlers) getJobTool() assistant.Tool {
	return assistant.Tool{
		Name:        "get_job",
		Description: "Read one vacancy in full by its public_slug.",
		Schema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"slug": map[string]any{"type": "string", "description": "The vacancy's public_slug."}},
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
				return nil, errors.New("slug is required")
			}
			job, err := h.queries.GetJobBySlug(ctx, in.Slug)
			if err != nil {
				return nil, fmt.Errorf("no vacancy with slug %q", in.Slug)
			}
			view, err := jobview.FromRow(job)
			if err != nil {
				return nil, err
			}
			view.Description = formatDescription(view.Description, "markdown")
			return view, nil
		},
	}
}

// getCompanyTool reads one company and a page of its open vacancies.
func (h *assistantHandlers) getCompanyTool() assistant.Tool {
	return assistant.Tool{
		Name:        "get_company",
		Description: "Read one company and a page of its open vacancies, by company slug.",
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
				return nil, errors.New("slug is required")
			}
			company, err := h.queries.GetCompany(ctx, in.Slug)
			if err != nil {
				return nil, fmt.Errorf("no company with slug %q", in.Slug)
			}
			jobs, err := h.queries.ListJobsByCompany(ctx, db.ListJobsByCompanyParams{
				CompanySlug: in.Slug,
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
			return map[string]any{"company": companyViewFrom(company), "jobs": views}, nil
		},
	}
}

// marketFitTool scores a skill list against the live market for a filtered role:
// the coverage headline, the must-have skills held, and the missing skills that
// unlock the most vacancies.
func (h *assistantHandlers) marketFitTool() assistant.Tool {
	return assistant.Tool{
		Name: "market_fit",
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
			if h.search.facets == nil {
				return nil, errors.New("search is not available")
			}
			skills := nonEmptyStrings(in.Skills)
			if len(skills) == 0 {
				return nil, errors.New("skills is required — list the canonical slugs to measure")
			}
			if len(skills) > maxCoverageSkills {
				return nil, fmt.Errorf("too many skills (max %d)", maxCoverageSkills)
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
				return nil, err
			}
			// Coherence is a CV-section metric; a flat skill list has no sections.
			v.CoherencePercent = 0
			return v, nil
		},
	}
}

// assistantFilterSchema is the JSON schema of the shared facet-filter argument.
func assistantFilterSchema(description string) map[string]any {
	return map[string]any{
		"type":                 "object",
		"description":          description + " Keys are facet names from the facets tool; each value is a list.",
		"additionalProperties": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
	}
}

// assistantResultCap bounds one tool result inside the conversation. Twenty
// vacancies with full descriptions run to tens of thousands of tokens; without a
// cap a single search can push the user's own question out of the window.
const assistantResultCap = 60_000

// assistantRegistry builds the tool set a session runs with. The preset decides:
// every session gets the discovery, tracking and experience-bank tools; a tailoring
// session bound to a CV also gets the CV tools, with the binding closed over; and a
// browsing session gets the page tool. A tailoring session whose binding is gone
// degrades to a plain chat rather than registering tools pointed at nothing.
//
// The bank tools are NOT preset-scoped. A candidate articulates their experience
// whenever they happen to, and a version of this that only listens in one surface
// would miss most of what they say. The page tool is the opposite case: it is scoped,
// because only a browsing session has a browser on the other end of the relay.
//
// sess.Preset is trusted as-is here — it is the caller's job (streamSSE, via
// effectivePreset) to have already demoted a browse session that is not running over
// the extension's own connection to plain chat, BEFORE calling this. Deciding that
// here too, from a second signal this function would have to take on its own, is
// exactly the split answer assistant.NormalizePreset's doc comment warns against: the
// prompt and the tool set must resolve from the SAME preset value or the model is
// told to always call a tool it was never given. See the
// confine-browse-preset-to-extension change.
func (h *assistantHandlers) registry(sess assistant.Session, batchID uuid.UUID) *assistant.Registry {
	// One normaliser answers the preset for BOTH the prompt and the tools. Comparing
	// against the constants directly would let an unrecognised preset fall back to the
	// chat prompt here while matching no branch below — instructions for tools the
	// session was never given.
	preset := assistant.NormalizePreset(sess.Preset)

	tools := append(h.assistantDiscoveryTools(), h.assistantTrackingTools()...)
	tools = append(tools, h.assistantExperienceTools(sess.ID)...)
	tools = append(tools, h.assistantScreeningTools()...)
	if preset == assistant.PresetTailor && sess.CVID != nil && sess.JobID != nil {
		tools = append(tools, h.assistantCVTools(*sess.CVID, *sess.JobID, batchID)...)
	}
	// Mail is the general chat session's. A tailoring session is working one CV
	// against one vacancy, an interview is collecting what the candidate has done,
	// and a side panel is talking about the page on screen — none of them has a
	// reason to read the mailbox, and every tool offered is paid for on every turn.
	if preset == assistant.PresetChat {
		tools = append(tools, h.assistantInboxTools()...)
	}
	// A rehearsal and a debrief are bound to a vacancy and to no CV, and they read the
	// same material: the posting, the stage, the requirements with the bank's evidence,
	// the employer's invitation. One reads it to ask what is coming, the other to judge
	// what was asked — the difference is the prompt, not the tools.
	//
	// Without the binding there is nothing to hold the conversation against, so both
	// degrade to a plain chat rather than registering a context tool pointed at nothing —
	// the same rule a CV-less tailoring session follows.
	if bindsToApplication(preset) && sess.JobID != nil {
		tools = append(tools, h.assistantInterviewTools(*sess.JobID)...)
	}
	// Only a browsing session has a browser on the other end of the relay. Offering
	// the page tool anywhere else would give the model a call that can only fail.
	if preset == assistant.PresetBrowse {
		tools = append(tools, h.readCurrentPageTool())
	}
	reg := assistant.NewRegistry(tools...)
	reg.MaxResultBytes = assistantResultCap
	return reg
}
