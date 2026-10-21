package mcpapp

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Version is what this server reports itself as. It is the app's version, not the
// catalogue's and not OJCP's.
const Version = "1.0"

// serverInstructions is what the model is told before it picks a tool.
//
// Four things belong here and nowhere else. The first is that this is an aggregator, so a
// posting is attributed rather than offered as ours. The second is which tool answers which
// question: tool descriptions may not point at other tools (Anthropic's directory policy),
// so this is the one place the model learns that, and TestTheInstructionsIntroduceEveryTool
// keeps it complete. The third is the OR: geography filters union, so asking for Germany and
// Lisbon returns both rather than their intersection — a model that assumes AND will
// describe the results wrongly and confidently, and nothing in a result would reveal it.
// The fourth is when to send `semantic`: measured 2026-10-21 over 34 queries, it lifted a
// need described in words from 3.3 to 7.4 relevant hits of 10 and non-English queries from
// 8.1 to 9.6, but cost a named title a little and a non-tech query a lot (9.2 to 7.0) —
// description vectors exist only for technical postings.
//
// Kept under 512 characters, which the Apps SDK guidance asks for and a test enforces.
const serverInstructions = "freehire aggregates IT job postings and attributes each to the board it " +
	"came from: name the source and link the employer's posting. search_jobs finds postings " +
	"(semantic: for a need put in words, not a title); get_job reads one and its liveness; " +
	"check_jobs_open rechecks a shortlist; similar_jobs finds more like one; salary_stats and " +
	"job_market_stats answer pay and market; search_companies and get_company cover employers. " +
	"Geography filters union. Unknown filter values return in ignored_params; retry."

// Tool names. Unlike the OJCP transport's, they feed no manifest — there is nothing here
// that ADVERTISES a tool separately from registering it, so they are constants only because
// the tests name them, not because a second list needs keeping in step.
const (
	toolSearchJobs      = "search_jobs"
	toolGetJob          = "get_job"
	toolSearchCompanies = "search_companies"
	toolGetCompany      = "get_company"
	toolCheckJobsOpen   = "check_jobs_open"
	// search and fetch are fixed names: ChatGPT's deep-research and company-knowledge
	// connectors look for exactly these.
	toolSearch = "search"
	toolFetch  = "fetch"

	toolSimilarJobs = "similar_jobs"
	toolSalaryStats = "salary_stats"
	toolMarketStats = "job_market_stats"
)

// readOnly is every tool's annotation block, written once.
//
// All four read, none writes, and all reach a catalogue that changes outside the
// conversation. Incorrect annotation of exactly these hints is a named rejection reason in
// OpenAI's submission guidelines, so sharing one value is not merely tidy: it removes the
// copy-paste from which a write tool would inherit a read-only claim.
func readOnly() *mcp.ToolAnnotations {
	destructive, openWorld := false, true
	return &mcp.ToolAnnotations{
		ReadOnlyHint:    true,
		DestructiveHint: &destructive,
		OpenWorldHint:   &openWorld,
	}
}

// Handler builds the HTTP handler serving these tools over MCP's streamable transport.
// Mount it with Fiber's adaptor at the path the app declares as its MCP endpoint.
//
// STATELESS, and that is the whole reason this surface works at all for the caller it was
// built for. Measured from a packet capture of OpenAI's app scanner (2026-09-18): it sends
// one request — `server/discover`, declaring protocol version 2026-07-28 — and stops. That
// is the SEP-2575 lifecycle, where the version and client identity ride in `_meta` on every
// request instead of being established once.
//
// The session-based transport cannot serve that lifecycle, and the SDK says so beside its
// own code: it filters 2026-07-28 out of the versions it advertises. The result was a
// `server/discover` answer marked complete that omitted the very version the caller had
// just declared — reported by the portal as "MCP server/discover response was inconsistent",
// which it was.
//
// Stateless costs this surface nothing it uses. Every tool is a read: a request in, an
// answer out, no server-to-client call, nothing to remember between requests. What it gives
// up is the GET event stream and DELETE (405 in this mode), neither of which any tool here
// has ever needed. The handshake lifecycle still works, so clients that send `initialize`
// are unaffected — see discovery_test.go, which holds both lifecycles to that promise.
//
// origin is the site's own address, which the server's icons and website link are built
// from.
func Handler(r Reader, origin string) http.Handler {
	return mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		return newServer(r, origin)
	}, &mcp.StreamableHTTPOptions{Stateless: true})
}

func newServer(r Reader, origin string) *mcp.Server {
	server := mcp.NewServer(Identity("freehire", Version, origin),
		&mcp.ServerOptions{Instructions: serverInstructions, CompletionHandler: completePromptArgument})

	addPrompts(server)
	addWidget(server)

	mcp.AddTool(server, &mcp.Tool{
		Name:  toolSearchJobs,
		Title: "Search jobs",
		Description: "Search freehire's catalogue of IT job postings. Use for any question " +
			"about which jobs are open — by technology, seniority, location, pay or employer. " +
			"Returns a short preview of each posting. Set semantic for a need described in words " +
			"(\"keep our payment systems secure\"), not for a job title or a non-technical role, " +
			"where plain keywords answer better.",
		Annotations: readOnly(),
		Meta:        rendersInWidget(),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in SearchInput) (*mcp.CallToolResult, SearchJobsResult, error) {
		return toolResult(r.SearchJobs(ctx, in))
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:  toolGetJob,
		Title: "Read one job posting",
		Description: "Read one posting in full by the slug a search result carries: the whole " +
			"description, the stated requirements, which applicant tracking system handles " +
			"applications, and whether the posting still looks live — when it was last seen " +
			"open, a reality class, and any observed signs that it is not being filled.",
		Annotations: readOnly(),
		Meta:        rendersInWidget(),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in JobDetailInput) (*mcp.CallToolResult, JobResult, error) {
		return toolResult(r.JobDetail(ctx, in.Slug))
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:  toolSearchCompanies,
		Title: "Search employers",
		Description: "Find employers in freehire's catalogue by name or by where they hire, " +
			"with how many postings each has open. Use when the question is about a company " +
			"rather than about a role.",
		Annotations: readOnly(),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in CompanySearchInput) (*mcp.CallToolResult, SearchCompaniesResult, error) {
		return toolResult(r.SearchCompanies(ctx, in))
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:  toolGetCompany,
		Title: "Read one employer",
		Description: "Read what freehire holds about one employer by the slug a result " +
			"carries: what they do, which industries they are in, where they are based, and how " +
			"many postings they have open.",
		Annotations: readOnly(),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in CompanyDetailInput) (*mcp.CallToolResult, CompanyResult, error) {
		return toolResult(r.CompanyDetail(ctx, in.Slug))
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:  toolCheckJobsOpen,
		Title: "Check whether postings are still open",
		Description: "Check whether postings are still open, by the slugs search results carry — " +
			"up to 50 per call. Use before acting on a saved shortlist: each posting answers " +
			"open, closed or not_found, with when it closed or was last seen live.",
		Annotations: readOnly(),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in CheckJobsOpenInput) (*mcp.CallToolResult, JobsOpenResult, error) {
		check, unchecked := openCheckSlugs(in.Slugs)
		states, err := r.CheckJobsOpen(ctx, check)
		return toolResult(JobsOpenResult{Jobs: states, Unchecked: unchecked}, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:  toolSearch,
		Title: "Search jobs (research)",
		Description: "Search freehire's IT job postings by free text. Returns matching postings " +
			"as {id, title, url}.",
		Annotations: readOnly(),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in ResearchQuery) (*mcp.CallToolResult, ResearchResults, error) {
		found, err := r.SearchJobs(ctx, SearchInput{Query: in.Query})
		return toolResult(researchResults(found), err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:  toolFetch,
		Title: "Read one job posting (research)",
		Description: "Read one posting in full by an id search returned: its full text, " +
			"the employer, the board it came from and whether it still looks live.",
		Annotations: readOnly(),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in ResearchID) (*mcp.CallToolResult, ResearchDocument, error) {
		job, err := r.JobDetail(ctx, in.ID)
		return toolResult(researchDocument(job), err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:  toolSimilarJobs,
		Title: "Find similar postings",
		Description: "Postings most similar to one posting, by its slug, nearest first — for " +
			"\"show me more like this\". Empty for the newest postings, whose neighbours are " +
			"not computed yet.",
		Annotations: readOnly(),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in SimilarJobsInput) (*mcp.CallToolResult, SimilarJobsResult, error) {
		return toolResult(r.SimilarJobs(ctx, in.Slug, similarLimit(in.Limit)))
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:  toolSalaryStats,
		Title: "Salary ranges",
		Description: "Salary percentiles (25th, median, 75th) from open postings that state " +
			"pay, scoped by role family, seniority and country, with the sample size behind " +
			"each figure. Currencies and pay periods are reported separately, never converted.",
		Annotations: readOnly(),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in SalaryInput) (*mcp.CallToolResult, SalaryStatsResult, error) {
		return toolResult(r.SalaryStats(ctx, in))
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:  toolMarketStats,
		Title: "Job market breakdown",
		Description: "How many open postings match a set of filters, and their most common " +
			"role families, seniorities, skills, work modes, countries and regions. Takes the " +
			"same filters as a job search.",
		Annotations: readOnly(),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in SearchInput) (*mcp.CallToolResult, MarketStatsResult, error) {
		return toolResult(r.MarketStats(ctx, in))
	})

	return server
}

// Identity is how a server introduces itself: its name, and the logo and website a client
// shows beside it in its list of connections. Every freehire MCP server introduces itself
// through this, so all of them carry the same face.
//
// With no origin it is the name alone. An icon is a URL the client fetches, and a path
// with no host is one it cannot — the same rule the projector follows for job links.
func Identity(name, version, origin string) *mcp.Implementation {
	id := &mcp.Implementation{Name: name, Title: "freehire", Version: version}
	origin = strings.TrimSuffix(origin, "/")
	if origin == "" {
		return id
	}
	id.WebsiteURL = origin
	// The SVG scales and follows the client's light or dark theme on its own; the PNG
	// is for a client that does not render SVG.
	id.Icons = []mcp.Icon{
		{Source: origin + "/favicon.svg", MIMEType: "image/svg+xml", Sizes: []string{"any"}},
		{Source: origin + "/apple-touch-icon.png", MIMEType: "image/png", Sizes: []string{"180x180"}},
	}
	return id
}

// toolResult renders a failure the way this audience can act on: an error RESULT carrying a
// sentence, never a JSON-RPC protocol error.
//
// That is the opposite of the OJCP transport beside it, and deliberately. There a failure
// must reach the wire as a code an agent branches on; here the caller is a language model,
// and a protocol error surfaces to a ChatGPT user as a generic failure with nothing to do
// next. An error result is text the model reads: "no posting with that identifier" leads it
// to search instead of retrying.
//
// The generic branch discloses nothing. What went wrong inside this deployment is ours, not
// a caller's — but it stays an ERROR rather than an empty answer, which would read as a
// catalogue holding nothing.
//
// It takes a reader call's two return values directly, so every tool wraps its own read in
// one expression and no call site can forget the branch.
func toolResult[T any](out T, err error) (*mcp.CallToolResult, T, error) {
	if err == nil {
		return nil, out, nil
	}

	var zero T
	var notFound NotFoundError
	message := "freehire could not answer this request"
	if errors.As(err, &notFound) {
		message = notFound.Error() + "; search for it instead"
	}
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{&mcp.TextContent{Text: message}},
	}, zero, nil
}
