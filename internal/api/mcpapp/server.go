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

// serverInstructions describes capabilities and result semantics shared across tools.
// The packaged find-tech-jobs skill carries the optional search workflow; initialization
// does not prescribe a call sequence, shortlist size or how the client presents results.
// Keep this introduction within the 512-character budget enforced by the tests.
const serverInstructions = "freehire provides anonymous, read-only access to aggregated job postings. " +
	"search_jobs/get_job: postings; check_jobs_open: open status; similar_jobs: related postings; " +
	"search_companies/get_company: employers; salary_stats/job_market_stats: pay and demand; " +
	"list_filter_values: supported filters. Results include source boards and employer links " +
	"where available. Geography filters form a union. ignored_params identifies unrecognized filter values." +
	" Search sees the start of a description; get_job has the rest."

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
	return newCatalogueServer(forAnonymous(r), origin, serverInstructions, "public", signedOutNext(origin))
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
		Annotations: readOnly(),
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
		Annotations: readOnly(),
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
		Annotations: readOnly(),
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
