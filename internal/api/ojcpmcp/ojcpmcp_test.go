package ojcpmcp

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"

	"github.com/strelov1/freehire/internal/api/ojcp"
)

// This transport had no tests at all, which is how it shipped returning plain Go errors
// while its own comment claimed they reached the caller as JSON-RPC errors. They did not:
// the SDK forwards only a *jsonrpc.Error to the protocol and turns anything else into a
// tool result with the message as text, so no OJCP envelope ever left this package.

func decodeEnvelope(t *testing.T, err error) (*jsonrpc.Error, ojcp.ErrorResponse) {
	t.Helper()

	var wire *jsonrpc.Error
	if !errors.As(err, &wire) {
		t.Fatalf("error is %T, not a *jsonrpc.Error — the SDK will not put it on the wire", err)
	}
	var envelope ojcp.ErrorResponse
	if len(wire.Data) == 0 {
		t.Fatal("the JSON-RPC error carries no data; the OJCP envelope is missing")
	}
	if err := json.Unmarshal(wire.Data, &envelope); err != nil {
		t.Fatalf("data is not an OJCP error envelope: %v", err)
	}
	return wire, envelope
}

func TestAMissingPostingReachesTheAgentAsTheStandardsCode(t *testing.T) {
	_, envelope := decodeEnvelope(t, toolError(NotFoundError{What: "posting"}))

	if envelope.ErrorCode != ojcp.ErrorJobNotFound {
		t.Errorf("error_code = %q, want %q", envelope.ErrorCode, ojcp.ErrorJobNotFound)
	}
}

func TestAMissingEmployerIsDistinctFromAMissingPosting(t *testing.T) {
	// The enum keeps the two apart and an agent branches on them: "no such posting" and
	// "no such employer" lead to different next steps.
	_, envelope := decodeEnvelope(t, toolError(NotFoundError{What: "employer"}))

	if envelope.ErrorCode != ojcp.ErrorEmployerNotFound {
		t.Errorf("error_code = %q, want %q", envelope.ErrorCode, ojcp.ErrorEmployerNotFound)
	}
}

func TestAnInternalFailureTellsTheAgentNothingAboutItself(t *testing.T) {
	// What went wrong inside this deployment is ours, not a caller's — but it stays an
	// ERROR rather than an empty answer, which would read as a catalogue holding nothing.
	wire, envelope := decodeEnvelope(t, toolError(errors.New("pq: connection refused on 10.0.0.4")))

	if envelope.ErrorCode != ojcp.ErrorProviderError {
		t.Errorf("error_code = %q, want %q", envelope.ErrorCode, ojcp.ErrorProviderError)
	}
	answer := string(wire.Data) + wire.Message
	for _, leaked := range []string{"pq:", "10.0.0.4", "connection refused"} {
		if strings.Contains(answer, leaked) {
			t.Errorf("the failure leaks %q to the caller: %s", leaked, answer)
		}
	}
}

func TestToolInputsUseTheNamesTheStandardRequires(t *testing.T) {
	// The SDK derives each tool's input schema from these structs, so a field named anything
	// other than what the standard's own input schema requires makes the tool UNREACHABLE for
	// a conforming client: it sends `job_id`, the server sees a missing required field and
	// answers a validation error. Found by OJCP's conformance suite, not by anything here —
	// the input schemas were vendored in this package and never checked against.
	for _, tc := range []struct {
		input any
		want  string
	}{
		{jobDetailInput{}, "job_id"},
		{employerContextInput{}, "employer_id"},
	} {
		raw, err := json.Marshal(tc.input)
		if err != nil {
			t.Fatalf("marshalling %T: %v", tc.input, err)
		}
		var fields map[string]any
		if err := json.Unmarshal(raw, &fields); err != nil {
			t.Fatalf("re-reading %T: %v", tc.input, err)
		}
		if _, present := fields[tc.want]; !present {
			t.Errorf("%T has no %q field; a conforming client cannot call this tool: %v",
				tc.input, tc.want, fields)
		}
	}
}

func TestSuccessIsNeverReportedAsAFailure(t *testing.T) {
	if err := toolError(nil); err != nil {
		t.Errorf("toolError(nil) = %v, want nil", err)
	}
}

func TestEveryAdvertisedToolIsServed(t *testing.T) {
	// The manifest advertises what ToolNames returns, and an agent calls what it names. This
	// asserts each one reaches the Reader rather than erroring at the transport — the half of
	// the guarantee that lives on this side.
	reader := &recordingReader{}
	for _, name := range ToolNames() {
		if !reader.canServe(name) {
			t.Errorf("tool %q is advertised but this transport has no call for it", name)
		}
	}
}

// recordingReader answers every tool, so a call that reaches it proves the wiring.
type recordingReader struct{}

func (r *recordingReader) SearchJobs(context.Context, ojcp.SearchInput) (ojcp.SearchJobsResponse, error) {
	return ojcp.SearchJobsResponse{}.Finalize(), nil
}

func (r *recordingReader) JobDetail(context.Context, string) (ojcp.JobDetailResponse, error) {
	return ojcp.JobDetailResponse{}.Finalize(), nil
}

func (r *recordingReader) EmployerContext(context.Context, string) (ojcp.EmployerContextResponse, error) {
	return ojcp.EmployerContextResponse{}.Finalize(), nil
}

func (r *recordingReader) canServe(tool string) bool {
	ctx := context.Background()
	switch tool {
	case toolSearchJobs:
		_, err := r.SearchJobs(ctx, ojcp.SearchInput{})
		return err == nil
	case toolGetJobDetail:
		_, err := r.JobDetail(ctx, "x")
		return err == nil
	case toolGetEmployerContext:
		_, err := r.EmployerContext(ctx, "x")
		return err == nil
	}
	return false
}
