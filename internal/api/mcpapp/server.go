package mcpapp

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/strelov1/freehire/internal/platform/observability"
	"github.com/strelov1/freehire/internal/search/search"
)

// Version is what this server reports itself as. It is the app's version, not the
// catalogue's and not OJCP's.
const Version = "1.0"

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

	toolFilterValues = "list_filter_values"
)

// ReadOnly is every tool's annotation block, written once.
//
// All four read, none writes, and all reach a catalogue that changes outside the
// conversation. Incorrect annotation of exactly these hints is a named rejection reason in
// OpenAI's submission guidelines, so sharing one value is not merely tidy: it removes the
// copy-paste from which a write tool would inherit a read-only claim.
func ReadOnly() *mcp.ToolAnnotations {
	destructive, openWorld := false, true
	return &mcp.ToolAnnotations{
		ReadOnlyHint:    true,
		DestructiveHint: &destructive,
		OpenWorldHint:   &openWorld,
	}
}

func newCatalogueServer(r Reader, origin, instructions, surface string, next nextHints) *mcp.Server {
	server := mcp.NewServer(Identity("freehire", Version, origin),
		&mcp.ServerOptions{
			Instructions:      instructions,
			CompletionHandler: completePromptArgument,
			// Lists are fixed for this per-request server. The SDK otherwise
			// advertises listChanged and accepts subscriptions that never update.
			Capabilities: &mcp.ServerCapabilities{
				Logging:   &mcp.LoggingCapabilities{}, //nolint:staticcheck // Preserve the SDK default for older MCP clients.
				Tools:     &mcp.ToolCapabilities{},
				Prompts:   &mcp.PromptCapabilities{},
				Resources: &mcp.ResourceCapabilities{},
			},
		})
	ObserveServer(server, surface)

	addPrompts(server)
	addWidget(server)

	AddObservedTool(server, surface, &mcp.Tool{
		Name:  toolSearchJobs,
		Title: "Search jobs",
		Description: "Search freehire's catalogue of IT job postings. Use for any question " +
			"about which jobs are open — by technology, seniority, location, pay or employer. " +
			"Returns a short preview of each posting. A role has several names: join two or three " +
			"in one query with OR, e.g. title:SRE OR title:\"site reliability\". Location, work " +
			"mode, seniority and skills go in their filters, never in the query. If suspiciously " +
			"few results come back, loosen one constraint at a time and say which. Present " +
			"results as a table of title, company, location, posted date and link (the employer's " +
			"posting and ours), naming the source.",
		Annotations: ReadOnly(),
		Meta:        rendersInWidget(),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in SearchInput) (*mcp.CallToolResult, SearchJobsResult, error) {
		out, err := r.SearchJobs(ctx, in)
		// The anonymous cap notice already says to connect an account; say it once.
		if err == nil && out.Notice == "" {
			out.Next = next.search
		}
		return toolResult(out, err)
	})

	AddObservedTool(server, surface, &mcp.Tool{
		Name:  toolGetJob,
		Title: "Read one job posting",
		Description: "Read one posting in full by the slug a search result carries: the whole " +
			"description, the stated requirements, which applicant tracking system handles " +
			"applications, and whether the posting still looks live — when it was last seen " +
			"open, a reality class, and any observed signs that it is not being filled. A closed " +
			"posting carries closed_at.",
		Annotations: ReadOnly(),
		Meta:        rendersInWidget(),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in JobDetailInput) (*mcp.CallToolResult, JobResult, error) {
		out, err := r.JobDetail(ctx, in.Slug)
		if err == nil {
			out.Next = next.job
		}
		return toolResult(out, err)
	})

	AddObservedTool(server, surface, &mcp.Tool{
		Name:  toolGetCompany,
		Title: "Read one employer",
		Description: "Read what freehire holds about one employer by the slug a result " +
			"carries: what they do, which industries they are in, where they are based, and how " +
			"many postings they have open.",
		Annotations: ReadOnly(),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in CompanyDetailInput) (*mcp.CallToolResult, CompanyResult, error) {
		return toolResult(r.CompanyDetail(ctx, in.Slug))
	})

	registerCatalogueTools(server, r, surface)

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
	if errors.Is(err, search.ErrTooManyAlternatives) {
		message = "the query has more than 5 OR alternatives; split them into several searches of at most 5 each and combine the results"
	}
	diagnostics := map[string]any{"error_code": publicErrorCode(err)}
	if errors.Is(err, search.ErrTooManyAlternatives) {
		diagnostics[badArgsKey] = []string{"query"}
	}
	return &mcp.CallToolResult{
		Meta:    mcp.Meta{diagnosticsMetaKey: diagnostics},
		IsError: true,
		Content: []mcp.Content{&mcp.TextContent{Text: message}},
	}, zero, nil
}

func publicErrorCode(err error) string {
	var notFound NotFoundError
	if errors.As(err, &notFound) {
		return "not_found"
	}
	if errors.Is(err, search.ErrTooManyAlternatives) {
		return "invalid_request"
	}
	return observability.ErrorCode(err)
}

// StatelessHandler serves a freshly built MCP server per request over the streamable
// transport. Stateless because a stateful session derives its tool contexts from the
// `initialize` request, whose fasthttp RequestCtx the Fiber adaptor recycles once that
// request ends: a later call would read another request's user values, and agent view
// counts would be attributed to the wrong request or lost. Every tool here is a request in,
// an answer out, so statelessness costs nothing.
func StatelessHandler(newServer func() *mcp.Server) http.Handler {
	return mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return newServer() },
		&mcp.StreamableHTTPOptions{Stateless: true})
}
