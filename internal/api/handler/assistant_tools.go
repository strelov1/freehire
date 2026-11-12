package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/strelov1/freehire/internal/ai/assistant"
	"github.com/strelov1/freehire/internal/api/mcpapp"
	"github.com/strelov1/freehire/internal/application/agentview"
	"github.com/strelov1/freehire/internal/job/jobview"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/llm"
	"github.com/strelov1/freehire/internal/search/search"
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

// UnmarshalJSON also reads a bare string as a one-value list. Models routinely send
// work_mode: "remote"; the strict decode failed the whole call before searching, which on
// 2026-11-05 was every internal_error search_jobs on the account MCP server (~57/day).
// The schema keeps publishing lists only, so no model provider is handed a union type.
func (f *assistantFilterArgs) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	out := make(assistantFilterArgs, len(raw))
	for name, value := range raw {
		var list []string
		if len(value) > 0 && value[0] == '"' {
			var one string
			if err := json.Unmarshal(value, &one); err != nil {
				return err
			}
			list = []string{one}
		} else if err := json.Unmarshal(value, &list); err != nil {
			return fmt.Errorf("filter %q: give a string or a list of strings from the facets tool, got %s", name, value)
		}
		out[name] = list
	}
	*f = out
	return nil
}

// values renders the filter as the query form the search layer already parses,
// rejecting any facet outside the published vocabulary. An unknown facet is an
// error rather than a silent no-op: FilterFromValues ignores parameters it does
// not know, so a hallucinated filter would come back as an unfiltered result set
// that looks like a confident answer.
func (f assistantFilterArgs) values() (url.Values, error) {
	vals := url.Values{}
	for name, list := range f {
		if _, ok := search.StringFacets[name]; !ok {
			return nil, assistant.BadArg("filters."+name, fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("unknown filter %q — call the facets tool for the valid filter names and values; valid names are: %s",
				name, strings.Join(assistantFacetNames(), ", "))))
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
	return h.facets(true)
}

// facets builds the facets tool. full lists every value for the in-app assistant, whose turn
// caps a tool result. An MCP client caps nothing and keeps every answer in its context,
// where the whole vocabulary ran to ~125 KB (2026-11-05) — metros, cities, skills and
// sources hold up to 1,200 values each — so MCP answers with the filter names and how many
// values each has, and lists one filter's values when the caller names it.
func (h *assistantHandlers) facets(full bool) assistant.Tool {
	description := "List the live filter vocabulary: every filter's valid values with a vacancy count each, " +
		"plus numeric ranges. Call this BEFORE filtering a search, and take skill slugs from its `skills` " +
		"distribution — never invent a filter value."
	properties := map[string]any{
		"filters": assistantFilterSchema("Narrow the vocabulary to a slice of the market (e.g. one category)."),
	}
	if !full {
		description = "List the live filter vocabulary. Without `facet`: every filter's name with how many values it " +
			"has, plus numeric ranges. With `facet`: that filter's values with a vacancy count each, optionally only " +
			"those containing `contains`. Filter values are the market's own; an invented one matches nothing. " +
			"`company_slug` lists the 50 busiest companies (slug, name, count) under the filters; `contains` searches the " +
			"1,200 busiest of them, so narrow the filters to reach a small employer."
		properties["facet"] = map[string]any{"type": "string", "description": "A filter name to list the values of, e.g. skills, cities, metros, company_slug."}
		properties["contains"] = map[string]any{"type": "string", "description": "With facet: keep only values containing this text, e.g. berlin; for skills a known alias also finds its value, e.g. golang finds go."}
	}
	return assistant.Tool{
		Name:        "facets",
		Title:       "List search filter values",
		Effect:      assistant.EffectReads,
		Description: description,
		Schema:      map[string]any{"type": "object", "properties": properties},
		Run: func(ctx context.Context, _ int64, raw json.RawMessage) (any, error) {
			var in struct {
				Filters  assistantFilterArgs `json:"filters"`
				Facet    string              `json:"facet"`
				Contains string              `json:"contains"`
			}
			if err := assistant.DecodeArgs(raw, &in); err != nil {
				return nil, err
			}
			if h.search == nil || h.search.facets == nil {
				return nil, fiber.NewError(fiber.StatusServiceUnavailable, "search is not available")
			}
			vals, err := in.Filters.values()
			if err != nil {
				return nil, err
			}
			facet := strings.TrimSpace(in.Facet)
			attrs := facetAttributes()
			if !full && facet != "" {
				// Count only the named facet: one attribute costs Meili a fraction of all of them.
				if !listableFacet(facet) {
					return nil, assistant.BadArg("facet", fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("unknown facet %q; valid facets are: %s", facet, strings.Join(listableFacets(), ", "))))
				}
				attrs = facetAttributes(facet)
			}
			res, err := h.search.facets.FacetCounts(ctx, search.FacetParams{
				Filter: search.FilterFromValues(vals),
				Facets: attrs,
			})
			if err != nil {
				return nil, searchToolError(err)
			}
			if !full && facet == companyFacetParam {
				return h.companyIndex(ctx, res, in.Contains), nil
			}
			view := facetView(res)
			if full {
				return view, nil
			}
			return facetIndex(view, facet, in.Contains), nil
		},
	}
}

// listableFacet reports whether the MCP facets can list a facet's values by name: every
// public facet. company_slug is answered as the busiest companies, not all of its values.
func listableFacet(name string) bool {
	_, isString := search.StringFacets[name]
	_, isExtra := facetExtraParams[name]
	return isString || isExtra
}

// companyIndex answers facet=company_slug: the companies whose slug contains the text, busiest
// first and capped at topCompaniesLimit, each with its display name. A space in the text
// matches the slug's hyphen, so "acme corp" finds acme-corp.
func (h *assistantHandlers) companyIndex(ctx context.Context, res search.FacetResult, contains string) map[string]any {
	// shortcut: res holds only the busiest 1,200 companies (the engine's per-facet cap), so a
	// small employer is found only under filters narrow enough to fit; upgrade to an index facet search.
	needle := strings.ReplaceAll(strings.ToLower(strings.TrimSpace(contains)), " ", "-")
	kept := map[string]int64{}
	for slug, n := range res.Facets[companyAttr] {
		if strings.Contains(slug, needle) {
			kept[slug] = n
		}
	}
	top := topCompanies(kept)
	h.search.nameCompanies(ctx, top)
	return map[string]any{"total": res.Total, "facet": companyFacetParam, "companies": top, "matching_companies": len(kept)}
}

// listableFacets is every name listableFacet accepts, sorted so the error a model reads is stable.
func listableFacets() []string {
	names := make([]string, 0, len(search.StringFacets)+len(facetExtraParams))
	for name := range search.StringFacets {
		if listableFacet(name) {
			names = append(names, name)
		}
	}
	for name := range facetExtraParams {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// facetIndex is the MCP facets answer: the filter names with how many values each has, or
// the named filter's values matching contains — or its range, for a numeric filter.
func facetIndex(view facetPayload, facet, contains string) map[string]any {
	if facet == "" {
		// Every value-bearing facet is named, with 0 when none of the matching postings carries
		// it, so a narrow filter does not hide that the facet exists. Numeric ones are in stats.
		sizes := map[string]int{}
		for _, name := range listableFacets() {
			if !facetExtraParams[name].statOnly && name != companyFacetParam {
				sizes[name] = 0
			}
		}
		for name, dist := range view.Facets {
			sizes[name] = len(dist)
		}
		return map[string]any{"total": view.Total, "filters": sizes, "stats": view.Stats}
	}
	if facetExtraParams[facet].statOnly {
		// A numeric facet has a range, not values; none when no matching posting states it.
		var stats *search.FacetStat
		if stat, ok := view.Stats[facet]; ok {
			stats = &stat
		}
		return map[string]any{"total": view.Total, "facet": facet, "stats": stats}
	}
	match := mcpapp.ContainsMatcher(facet, contains)
	values := map[string]int64{}
	for value, count := range view.Facets[facet] {
		if match(value) {
			values[value] = count
		}
	}
	return map[string]any{"total": view.Total, "facet": facet, "values": values}
}

// searchJobsTool searches the catalogue with a keyword and facet filters, hitting
// the same index the public search does and rehydrating each result's full
// description so the agent screens a whole set in one call.
func (h *assistantHandlers) searchJobsTool() assistant.Tool {
	return h.searchJobs(true)
}

// mcpSearchExcerptRunes bounds the description each MCP search result carries.
const mcpSearchExcerptRunes = 400

// searchJobs builds search_jobs. full rehydrates each result's whole description for the
// in-app assistant, whose turn caps a tool result. An MCP client caps nothing and keeps
// every answer in its context, where ten full descriptions ran to ~130 KB (2026-11-05),
// so MCP gets the opening of each from the index preview and reads the rest via get_job.
func (h *assistantHandlers) searchJobs(full bool) assistant.Tool {
	description := "Search open vacancies by keyword and facet filters. Every result carries its full " +
		"description as markdown and its public_slug — link a vacancy as https://freehire.me/jobs/<public_slug>. " +
		"Take filter names and values from the facets tool."
	if !full {
		// No other tool is named here: the Claude directory policy keeps cross-tool guidance
		// out of tool descriptions (see mcpapp's TestNoToolDescriptionDirectsTheModelToAnotherTool).
		description = "Search open vacancies by keyword and facet filters. Every result carries the first " +
			"400 characters of its description as markdown and its public_slug. " +
			"Link a vacancy as https://freehire.me/jobs/<public_slug>."
	}
	return assistant.Tool{
		Name:        "search_jobs",
		Title:       "Search jobs",
		Effect:      assistant.EffectReads,
		Description: description,
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query":   map[string]any{"type": "string", "description": "Free-text query, matched against title, company, location and the start of the description (not the full text, so details such as equity or visa may be missing from the match). Use few, specific words. Omit to browse the freshest postings under the filters. Operators: double quotes require every quoted word in any order (not a phrase); -word excludes; uppercase OR between terms matches any alternative (at most 5); title: at the start of an alternative matches it in the title only, e.g. title:\"product owner\" OR title:produktchef."},
				"filters": assistantFilterSchema("Facet filters; values must come from the facets tool."),
				"limit":   map[string]any{"type": "integer", "description": fmt.Sprintf("How many vacancies to return (default %d, max %d).", assistantDefaultSearchLimit, assistantMaxSearchLimit)},
				"offset":  map[string]any{"type": "integer", "description": "Skip this many results (paging)."},
				// The non-facet filters the public search_jobs already takes. Agents sent them
				// here too and got "unknown filter" (~145 calls a week, 2026-11).
				"salary_min":         map[string]any{"type": "integer", "description": "Minimum salary, in the currency named by filters.salary_currency."},
				"posted_within_days": map[string]any{"type": "integer", "description": "Only postings published within this many days."},
				"visa_sponsorship":   map[string]any{"type": "boolean", "description": "true: only postings that offer visa sponsorship."},
				"exclude_skills":     map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Skills the posting must not ask for."},
				"require_all_skills": map[string]any{"type": "boolean", "description": "true: every skill in filters.skills must be present, not just one."},
			},
		},
		Run: func(ctx context.Context, _ int64, raw json.RawMessage) (any, error) {
			var in struct {
				Query            string              `json:"query"`
				Filters          assistantFilterArgs `json:"filters"`
				Limit            int                 `json:"limit"`
				Offset           int                 `json:"offset"`
				SalaryMin        int                 `json:"salary_min"`
				PostedWithinDays int                 `json:"posted_within_days"`
				VisaSponsorship  *bool               `json:"visa_sponsorship"`
				ExcludeSkills    []string            `json:"exclude_skills"`
				RequireAllSkills bool                `json:"require_all_skills"`
			}
			if err := assistant.DecodeArgs(raw, &in); err != nil {
				return nil, err
			}
			if h.search == nil || h.search.search == nil {
				return nil, fiber.NewError(fiber.StatusServiceUnavailable, "search is not available")
			}
			vals, err := in.Filters.values()
			if err != nil {
				return nil, err
			}
			if in.SalaryMin > 0 {
				vals.Set("salary_min", strconv.Itoa(in.SalaryMin))
			}
			if in.PostedWithinDays > 0 {
				vals.Set("posted_within_days", strconv.Itoa(in.PostedWithinDays))
			}
			if in.VisaSponsorship != nil {
				vals.Set("visa_sponsorship", strconv.FormatBool(*in.VisaSponsorship))
			}
			for _, s := range in.ExcludeSkills {
				if s = strings.TrimSpace(s); s != "" {
					vals.Add("skills_exclude", s)
				}
			}
			if in.RequireAllSkills && vals.Has("skills") {
				vals.Set("skills_mode", "and")
			}

			limit := in.Limit
			if limit <= 0 {
				limit = assistantDefaultSearchLimit
			}
			limit = min(limit, assistantMaxSearchLimit)
			offset := max(in.Offset, 0)

			res, err := h.search.search.Search(ctx, search.SearchParams{
				Query:  in.Query,
				Filter: search.FilterFromValues(vals),
				Sort:   defaultSort(in.Query),
				Limit:  limit,
				Offset: offset,
			})
			if err != nil {
				return nil, searchToolError(err)
			}
			if full {
				if err := h.search.hydrateDescriptions(ctx, res.Hits); err != nil {
					return nil, err
				}
			}
			agentview.Record(ctx, agentview.KindSearch, hitIDs(res.Hits))
			jobs := make([]jobview.Job, len(res.Hits))
			for i, hit := range res.Hits {
				hit.Description = formatDescription(hit.Description, "markdown")
				if !full {
					hit.Description = llm.ClipRunesEllipsis(hit.Description, mcpSearchExcerptRunes)
				}
				jobs[i] = hit.Job
			}
			return map[string]any{"total": res.Total, "limit": limit, "offset": offset, "jobs": jobs}, nil
		},
	}
}

// searchToolError gives a query the search layer refused the same 400 and words the HTTP
// search answers it with (classify); any other failure stays an internal one.
func searchToolError(err error) error {
	switch {
	case errors.Is(err, search.ErrTooManyAlternatives):
		return assistant.BadArg("query", fiber.NewError(fiber.StatusBadRequest, search.ErrTooManyAlternatives.Error()))
	case errors.Is(err, search.ErrBadQuery):
		return assistant.BadArg("query", fiber.NewError(fiber.StatusBadRequest, "invalid search query"))
	}
	return err
}

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
				return nil, assistant.BadArg("slug", fiber.NewError(fiber.StatusBadRequest, "slug is required"))
			}
			job, err := agentJobBySlug(ctx, h.queries, in.Slug)
			if err != nil {
				return nil, lookupMiss(err, fmt.Sprintf("no vacancy with slug %q", in.Slug))
			}
			agentview.Record(ctx, agentview.KindDetail, []int64{job.ID})
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
			// Ten full bodies ran to ~160 KB per call (2026-11); the page is for picking a vacancy.
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
				return nil, fiber.NewError(fiber.StatusServiceUnavailable, "search is not available")
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

// assistantFilterSchema is the JSON schema of the shared facet-filter argument.
//
// Every facet name is declared as a property and nothing else is admitted. An open
// map left the model to guess the keys, and it guessed from the nearest vocabulary
// it had seen — get_profile's `specializations` instead of `category` — which cost a
// refused call and an error chip on screen before it retried. values() still rejects
// an unknown name: a model is free to ignore a schema.
func assistantFilterSchema(description string) map[string]any {
	list := map[string]any{"type": "array", "items": map[string]any{"type": "string"}}
	props := make(map[string]any, len(search.StringFacets))
	for name := range search.StringFacets {
		props[name] = list
	}
	return map[string]any{
		"type":                 "object",
		"description":          description + " Each value is a list of values from the facets tool.",
		"properties":           props,
		"additionalProperties": false,
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
	if preset == assistant.PresetSupport {
		reg := assistant.NewRegistry(h.supportTools(sess.ID)...)
		reg.MaxResultBytes = assistantResultCap
		return reg
	}
	if preset == assistant.PresetChat && sess.SearchContext != nil {
		reg := assistant.NewRegistry(h.shortlistDiscoveryTools(sess, 0)...)
		reg.MaxResultBytes = assistantResultCap
		return reg
	}

	tools := append(h.assistantDiscoveryTools(), h.assistantTrackingTools()...)
	tools = append(tools, h.assistantExperienceTools(sess.ID)...)
	tools = append(tools, h.assistantScreeningTools()...)
	// A tailoring session needs its CV and its vacancy; a BASE CV session (the general CV, no
	// vacancy) needs only the CV and gets the CV tools without the vacancy ones. Missing its
	// binding either degrades to a plain chat rather than registering a tool pointed at nothing.
	if sess.CVID != nil && (preset == assistant.PresetBase || (preset == assistant.PresetTailor && sess.JobID != nil)) {
		tools = append(tools, h.assistantCVTools(*sess.CVID, sess.JobID, batchID)...)
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
	if (preset == assistant.PresetInterview || preset == assistant.PresetDebrief) && sess.JobID != nil {
		tools = append(tools, h.assistantInterviewTools(*sess.JobID)...)
	}
	// The apply conversation acts on its one automatic application; unbound it has nothing
	// to act on and stays a plain chat, like a CV-less tailoring session.
	if preset == assistant.PresetApply && sess.JobID != nil {
		tools = append(tools, h.assistantApplyTools(*sess.JobID)...)
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
