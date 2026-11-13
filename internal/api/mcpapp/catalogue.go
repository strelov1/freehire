package mcpapp

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// registerCatalogueTools adds the catalogue reads the signed-in assistant does not own.
// Its search_jobs, get_job and get_company retain their account-specific schemas.
func registerCatalogueTools(server *mcp.Server, r Reader, surface string) {
	AddObservedTool(
		server,
		surface,
		&mcp.Tool{
			Name:  toolSearchCompanies,
			Title: "Search employers",
			Description: "Find employers in freehire's catalogue by name or by where they hire, " +
				"with how many postings each has open. Use when the question is about a company " +
				"rather than about a role.",
			Annotations: ReadOnly(),
		},
		func(ctx context.Context, _ *mcp.CallToolRequest, in CompanySearchInput) (*mcp.CallToolResult, SearchCompaniesResult, error) {
			return toolResult(r.SearchCompanies(ctx, in))
		},
	)

	AddObservedTool(
		server,
		surface,
		&mcp.Tool{
			Name:  toolCheckJobsOpen,
			Title: "Check whether postings are still open",
			Description: "Check whether postings are still open, by the slugs search results carry — " +
				"up to 50 per call. Use before acting on a saved shortlist: each posting answers " +
				"open, closed or not_found, with when it closed or was last seen live.",
			Annotations: ReadOnly(),
		},
		func(ctx context.Context, _ *mcp.CallToolRequest, in CheckJobsOpenInput) (*mcp.CallToolResult, JobsOpenResult, error) {
			check, unchecked := openCheckSlugs(in.Slugs)
			states, err := r.CheckJobsOpen(ctx, check)
			return toolResult(JobsOpenResult{Jobs: states, Unchecked: unchecked}, err)
		},
	)

	AddObservedTool(
		server,
		surface,
		&mcp.Tool{
			Name:  toolSearch,
			Title: "Search jobs (research)",
			Description: "Search freehire's IT job postings by free text. Returns matching postings " +
				"as {id, title, url}.",
			Annotations: ReadOnly(),
		},
		func(ctx context.Context, _ *mcp.CallToolRequest, in ResearchQuery) (*mcp.CallToolResult, ResearchResults, error) {
			found, err := r.SearchJobs(ctx, SearchInput{Query: in.Query})
			return toolResult(researchResults(found), err)
		},
	)

	AddObservedTool(
		server,
		surface,
		&mcp.Tool{
			Name:  toolFetch,
			Title: "Read one job posting (research)",
			Description: "Read one posting in full by an id search returned: its full text, " +
				"the employer, the board it came from and whether it still looks live.",
			Annotations: ReadOnly(),
		},
		func(ctx context.Context, _ *mcp.CallToolRequest, in ResearchID) (*mcp.CallToolResult, ResearchDocument, error) {
			job, err := r.JobDetail(ctx, in.ID)
			return toolResult(researchDocument(job), err)
		},
	)

	AddObservedTool(
		server,
		surface,
		&mcp.Tool{
			Name:  toolSimilarJobs,
			Title: "Find similar postings",
			Description: "Postings most similar to one posting, by its slug, nearest first — for " +
				"\"show me more like this\". Empty for the newest postings, whose neighbours are " +
				"not computed yet.",
			Annotations: ReadOnly(),
		},
		func(ctx context.Context, _ *mcp.CallToolRequest, in SimilarJobsInput) (*mcp.CallToolResult, SimilarJobsResult, error) {
			return toolResult(r.SimilarJobs(ctx, in.Slug, similarLimit(in.Limit)))
		},
	)

	AddObservedTool(
		server,
		surface,
		&mcp.Tool{
			Name:  toolSalaryStats,
			Title: "Salary ranges",
			Description: "Salary percentiles (25th, median, 75th) from open postings that state " +
				"pay, scoped by role family, seniority and country, with the sample size behind " +
				"each figure. Currencies and pay periods are reported separately, never converted.",
			Annotations: ReadOnly(),
		},
		func(ctx context.Context, _ *mcp.CallToolRequest, in SalaryInput) (*mcp.CallToolResult, SalaryStatsResult, error) {
			return toolResult(r.SalaryStats(ctx, in))
		},
	)

	AddObservedTool(
		server,
		surface,
		&mcp.Tool{
			Name:  toolMarketStats,
			Title: "Job market breakdown",
			Description: "How many open postings match a set of filters, and their most common " +
				"role families, seniorities, skills, work modes, countries and regions. Takes the " +
				"same filters as a job search.",
			Annotations: ReadOnly(),
		},
		func(ctx context.Context, _ *mcp.CallToolRequest, in SearchInput) (*mcp.CallToolResult, MarketStatsResult, error) {
			return toolResult(r.MarketStats(ctx, in))
		},
	)

	AddObservedTool(
		server,
		surface,
		&mcp.Tool{
			Name:  toolFilterValues,
			Title: "List filter values",
			Description: "The values one job-search filter accepts — skills, categories, " +
				"seniorities, sources, countries and the rest — each with how many open " +
				"postings carry it, most common first. A value not listed matches no posting. " +
				"Use it before filtering on a value you are not sure of.",
			Annotations: ReadOnly(),
		},
		func(ctx context.Context, _ *mcp.CallToolRequest, in FilterValuesInput) (*mcp.CallToolResult, FilterValuesResult, error) {
			counts, err := r.FacetCounts(ctx)
			if err != nil {
				return toolResult(FilterValuesResult{}, err)
			}
			return toolResult(FilterValues(in, counts), nil)
		},
	)
}
