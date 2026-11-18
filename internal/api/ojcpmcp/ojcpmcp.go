// Package ojcpmcp is the MCP transport for freehire's OJCP read surface.
//
// It is an adapter only: every tool loads data through the same Reader the REST handlers
// use and projects it with the same internal/api/ojcp functions, so the transports cannot
// answer differently. REST stays because OJCP's conformance suite uses it.
package ojcpmcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/strelov1/freehire/internal/api/mcpapp"
	"github.com/strelov1/freehire/internal/api/ojcp"
)

// Reader is everything this transport needs: the three tool answers, already projected.
// The REST handlers satisfy it.
type Reader interface {
	SearchJobs(ctx context.Context, input ojcp.SearchInput) (ojcp.SearchJobsResponse, error)
	JobDetail(ctx context.Context, ojcpID string) (ojcp.JobDetailResponse, error)
	EmployerContext(ctx context.Context, employerID string) (ojcp.EmployerContextResponse, error)
}

// NotFoundError marks the one failure an agent must tell apart: the posting or employer is
// not here. Everything else is this deployment's problem.
type NotFoundError struct{ What string }

func (e NotFoundError) Error() string { return e.What + " not found" }

// Code is the standard's error code for what was not found (job and employer are distinct).
func (e NotFoundError) Code() string {
	if e.What == "employer" {
		return ojcp.ErrorEmployerNotFound
	}
	return ojcp.ErrorJobNotFound
}

// jobDetailInput and employerContextInput are the tools' inputs, declared here because the
// SDK derives each tool's JSON Schema from the Go type.
// The field names are the standard's: `get_job_detail` takes `job_id`
// (tools/get-job-detail-input.json), though the value is what a posting publishes as
// `ojcp_id`; naming it `ojcp_id` makes the tool unreachable for conforming clients.
type jobDetailInput struct {
	JobID string `json:"job_id" jsonschema:"the posting's identifier, as published in ojcp_id"`
}

type employerContextInput struct {
	EmployerID string `json:"employer_id" jsonschema:"the employer's OJCP identifier"`
}

// Handler builds the HTTP handler serving the OJCP tools over MCP's streamable transport.
// Mount it with Fiber's adaptor at the path the manifest declares as `mcp_endpoint`.
//
// It is stateless (see mcpapp.StatelessHandler).
func Handler(r Reader, origin string) http.Handler {
	return mcpapp.StatelessHandler(func() *mcp.Server { return newServer(r, origin) })
}

func newServer(r Reader, origin string) *mcp.Server {
	server := mcp.NewServer(mcpapp.Identity("freehire-ojcp", ojcp.Version, origin), nil)
	mcpapp.ObserveServer(server, "ojcp")

	mcpapp.AddObservedTool(server, "ojcp", &mcp.Tool{
		Name:         toolSearchJobs,
		Annotations:  mcpapp.ReadOnly(),
		OutputSchema: searchJobsOutputSchema,
		Description: "Search freehire's job catalogue. Returns OJCP JobPostings, each with " +
			"the application paths it can be applied through and whether an agent can submit " +
			"unattended. Filters this provider could not honour are named in ignored_params. " +
			"The semantic flag is currently not available: a request that sets it is answered " +
			"by keywords and its semantic field says unavailable.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in ojcp.SearchInput) (*mcp.CallToolResult, ojcp.SearchJobsResponse, error) {
		out, err := r.SearchJobs(ctx, in)
		return nil, out, toolError(err)
	})

	mcpapp.AddObservedTool(server, "ojcp", &mcp.Tool{
		Name:         toolGetJobDetail,
		Annotations:  mcpapp.ReadOnly(),
		OutputSchema: jobDetailOutputSchema,
		Description:  "Read one posting in full, by the ojcp_id a search result carries.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in jobDetailInput) (*mcp.CallToolResult, ojcp.JobDetailResponse, error) {
		out, err := r.JobDetail(ctx, in.JobID)
		return nil, out, toolError(err)
	})

	mcpapp.AddObservedTool(server, "ojcp", &mcp.Tool{
		Name:        toolGetEmployerContext,
		Annotations: mcpapp.ReadOnly(),
		Description: "Read what is known about an employer, by the ojcp_employer_id a " +
			"posting carries — including how many roles they have open right now.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in employerContextInput) (*mcp.CallToolResult, ojcp.EmployerContextResponse, error) {
		out, err := r.EmployerContext(ctx, in.EmployerID)
		return nil, out, toolError(err)
	})

	return server
}

// The output schemas of the tools whose results carry a JobPosting.
//
// The SDK validates results against a schema inferred from the Go type unless one is given.
// Inferred from ojcp.Places (a slice) it admits only an array, but a single place is written
// as an object, so one-place postings would fail validation.
var (
	searchJobsOutputSchema = outputSchema[ojcp.SearchJobsResponse]()
	jobDetailOutputSchema  = outputSchema[ojcp.JobDetailResponse]()
)

// outputSchema infers T's schema the way the SDK would, except that ojcp.Places is described
// as OJCP describes jobLocation. It panics on an undescribable type, at package load.
func outputSchema[T any]() *jsonschema.Schema {
	place, err := jsonschema.For[ojcp.Place](nil)
	if err != nil {
		panic(fmt.Sprintf("ojcpmcp: schema for ojcp.Place: %v", err))
	}
	oneOrMore := 1
	places := &jsonschema.Schema{OneOf: []*jsonschema.Schema{
		place,
		{Type: "array", Items: place, MinItems: &oneOrMore},
	}}
	schema, err := jsonschema.For[T](&jsonschema.ForOptions{
		TypeSchemas: map[reflect.Type]*jsonschema.Schema{reflect.TypeFor[ojcp.Places](): places},
	})
	if err != nil {
		panic(fmt.Sprintf("ojcpmcp: output schema for %T: %v", *new(T), err))
	}
	return schema
}

// Tool names, declared once and used both to register a tool and to answer ToolNames; the
// manifest's `tools` is a binding claim agents call by.
const (
	toolSearchJobs         = "search_jobs"
	toolGetJobDetail       = "get_job_detail"
	toolGetEmployerContext = "get_employer_context"
)

// ToolNames is what the manifest may advertise over this transport.
//
// It returns the same constants newServer registers with. The SDK cannot read registrations
// back, so nothing here proves a constant was passed to AddTool; the handler's test closes
// that by requiring a method behind every name.
func ToolNames() []string {
	return []string{toolSearchJobs, toolGetJobDetail, toolGetEmployerContext}
}

// toolError renders a failure as a JSON-RPC error carrying the OJCP envelope in `data`, as
// the specification asks of an MCP transport.
//
// The SDK forwards an error to the protocol only when it is a `*jsonrpc.Error`; an ordinary
// Go error becomes a CallToolResult with IsError set (server.go:427).
//
// A not-found is stated plainly, as REST's 404 is. Every other failure is reported without
// its detail, and is never an empty answer, which would read as an empty catalogue.
func toolError(err error) error {
	if err == nil {
		return nil
	}

	var notFound NotFoundError
	if errors.As(err, &notFound) {
		return wireError(jsonrpc.CodeInvalidParams, ojcp.NewError(notFound.Code(), err.Error()))
	}
	return wireError(jsonrpc.CodeInternalError,
		ojcp.NewError(ojcp.ErrorProviderError, "freehire could not answer this request"))
}

// wireError packages an OJCP envelope as the `data` of a JSON-RPC error. A payload that
// will not marshal still yields an error, just without the envelope.
func wireError(code int64, payload ojcp.ErrorResponse) error {
	wire := &jsonrpc.Error{Code: code, Message: payload.Message}
	if data, err := json.Marshal(payload); err == nil {
		wire.Data = data
	}
	return wire
}
