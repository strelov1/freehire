// Package ojcpmcp is the MCP transport for freehire's OJCP read surface.
//
// It is an ADAPTER and nothing else. Every tool here loads its data through the same
// Reader the REST handlers use and projects it with the same internal/api/ojcp functions,
// so the two transports cannot answer the same question differently — which the OJCP
// specification treats as one answer rendered two ways, not two implementations.
//
// It exists beside REST rather than instead of it because REST is what OJCP's conformance
// suite exercises and what any plain HTTP client can reach, while MCP is what ChatGPT and
// Claude actually connect to today.
package ojcpmcp

import (
	"context"
	"errors"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/strelov1/freehire/internal/api/ojcp"
)

// Reader is everything this transport needs from the rest of the system: the three tool
// answers, already projected. The REST handlers satisfy it, which is how the two transports
// are held to one implementation rather than being asked to agree.
type Reader interface {
	SearchJobs(ctx context.Context, input ojcp.SearchInput) (ojcp.SearchJobsResponse, error)
	JobDetail(ctx context.Context, ojcpID string) (ojcp.JobDetailResponse, error)
	EmployerContext(ctx context.Context, employerID string) (ojcp.EmployerContextResponse, error)
}

// NotFoundError marks the one failure an agent must be able to tell apart: the posting or
// employer simply is not here. Everything else is this deployment's problem, not the
// agent's, and says so.
type NotFoundError struct{ What string }

func (e NotFoundError) Error() string { return e.What + " not found" }

// jobDetailInput and employerContextInput are the tools' inputs. They are declared here
// rather than in internal/api/ojcp because the SDK derives each tool's JSON Schema from the
// Go type, so the shape belongs with the transport that publishes it.
type jobDetailInput struct {
	OJCPID string `json:"ojcp_id" jsonschema:"the posting's OJCP identifier"`
}

type employerContextInput struct {
	EmployerID string `json:"employer_id" jsonschema:"the employer's OJCP identifier"`
}

// Handler builds the HTTP handler serving the OJCP tools over MCP's streamable transport.
// Mount it with Fiber's adaptor at the path the manifest declares as `mcp_endpoint`.
func Handler(r Reader) http.Handler {
	return mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		return newServer(r)
	}, nil)
}

func newServer(r Reader) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "freehire-ojcp",
		Version: ojcp.Version,
	}, nil)

	mcp.AddTool(server, &mcp.Tool{
		Name: toolSearchJobs,
		Description: "Search freehire's job catalogue. Returns OJCP JobPostings, each with " +
			"the application paths it can be applied through and whether an agent can submit " +
			"unattended. Filters this provider could not honour are named in ignored_params.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in ojcp.SearchInput) (*mcp.CallToolResult, ojcp.SearchJobsResponse, error) {
		out, err := r.SearchJobs(ctx, in)
		return nil, out, toolError(err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        toolGetJobDetail,
		Description: "Read one posting in full, by the ojcp_id a search result carries.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in jobDetailInput) (*mcp.CallToolResult, ojcp.JobDetailResponse, error) {
		out, err := r.JobDetail(ctx, in.OJCPID)
		return nil, out, toolError(err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: toolGetEmployerContext,
		Description: "Read what is known about an employer, by the ojcp_employer_id a " +
			"posting carries — including how many roles they have open right now.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in employerContextInput) (*mcp.CallToolResult, ojcp.EmployerContextResponse, error) {
		out, err := r.EmployerContext(ctx, in.EmployerID)
		return nil, out, toolError(err)
	})

	return server
}

// Tool names, declared once and used BOTH to register a tool and to answer ToolNames.
// A second hand-written list would be the one that goes stale, and the manifest's `tools`
// is a binding claim: an agent reads it and calls what it names.
const (
	toolSearchJobs         = "search_jobs"
	toolGetJobDetail       = "get_job_detail"
	toolGetEmployerContext = "get_employer_context"
)

// ToolNames is what the manifest may advertise over this transport.
//
// It returns the SAME constants newServer registers with, so a name cannot be advertised
// under one spelling and served under another. That is all it guarantees: the SDK exposes
// no way to read a server's registrations back, so nothing here can prove a constant was
// actually passed to AddTool.
//
// What closes that gap is on the other side — the handler's own test walks this list and
// requires a method behind every name. An earlier version of this comment claimed the list
// was derived from the registrations, which it was not, and a comment that overstates a
// guarantee is worse than one that admits the limit: the next reader stops looking.
func ToolNames() []string {
	return []string{toolSearchJobs, toolGetJobDetail, toolGetEmployerContext}
}

// toolError renders a failure the way MCP carries one: the SDK turns a returned error into
// a JSON-RPC error for the caller.
//
// A not-found is stated plainly, because an agent must be able to tell "no such posting"
// from "this provider is broken" — the same distinction the REST transport draws with a
// 404 rather than an empty object. Every other failure is reported WITHOUT its detail: what
// went wrong inside this deployment is ours, not a caller's. It is still an error and never
// an empty answer, which would read as a catalogue holding nothing.
func toolError(err error) error {
	if err == nil {
		return nil
	}
	var notFound NotFoundError
	if errors.As(err, &notFound) {
		return err
	}
	return errors.New("freehire could not answer this request")
}
