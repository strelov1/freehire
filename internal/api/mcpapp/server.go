package mcpapp

import (
	"context"
	"errors"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Version is what this server reports itself as. It is the app's version, not the
// catalogue's and not OJCP's.
const Version = "1.0"

// serverInstructions is what the model is told before it picks a tool.
//
// Two things belong here and nowhere else. The first is that this is an aggregator, so a
// posting is attributed rather than offered as ours. The second is the OR: geography
// filters union, so asking for Germany and Lisbon returns both rather than their
// intersection — a model that assumes AND will describe the results wrongly and confidently,
// and nothing in a result would reveal the mistake.
//
// Kept under 512 characters, which the Apps SDK guidance asks for and a test enforces.
const serverInstructions = "freehire aggregates IT job postings from many sources and " +
	"attributes each to the board it came from. Search first, then read one posting in full " +
	"with get_job. Geography filters union rather than intersect: countries plus cities " +
	"returns postings matching any of them. Values outside our vocabularies are dropped and " +
	"named in ignored_params, so the answer is wider than asked — retry with a named value " +
	"rather than reporting an empty catalogue."

// Tool names. Unlike the OJCP transport's, they feed no manifest — there is nothing here
// that ADVERTISES a tool separately from registering it, so they are constants only because
// the tests name them, not because a second list needs keeping in step.
const (
	toolSearchJobs      = "search_jobs"
	toolGetJob          = "get_job"
	toolSearchCompanies = "search_companies"
	toolGetCompany      = "get_company"
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
func Handler(r Reader) http.Handler {
	return mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		return newServer(r)
	}, &mcp.StreamableHTTPOptions{Stateless: true})
}

func newServer(r Reader) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "freehire",
		Version: Version,
	}, &mcp.ServerOptions{Instructions: serverInstructions})

	mcp.AddTool(server, &mcp.Tool{
		Name:  toolSearchJobs,
		Title: "Search jobs",
		Description: "Search freehire's catalogue of IT job postings. Use for any question " +
			"about which jobs are open — by technology, seniority, location, pay or employer. " +
			"Returns a short preview of each posting; call get_job for one posting's full text.",
		Annotations: readOnly(),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in SearchInput) (*mcp.CallToolResult, SearchJobsResult, error) {
		return toolResult(r.SearchJobs(ctx, in))
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:  toolGetJob,
		Title: "Read one job posting",
		Description: "Read one posting in full by the slug a search result carries: the whole " +
			"description, the stated requirements, and which applicant tracking system handles " +
			"applications.",
		Annotations: readOnly(),
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

	return server
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
