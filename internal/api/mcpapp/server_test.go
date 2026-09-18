package mcpapp

import (
	"context"
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/strelov1/freehire/internal/ingest/applyform"
)

// The whole server, driven through the SDK's in-memory transport — the same thing ChatGPT
// does, minus the network. A tool that registers but cannot be called is the failure this
// catches, and it is invisible to a test that calls the handler function directly.
func connect(t *testing.T, r Reader) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()

	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	server, err := newServer(r).Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	t.Cleanup(func() { _ = server.Close() })

	session, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil).
		Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

// readerThatFinds answers every read with one posting and one employer.
func readerThatFinds(t *testing.T) fakeReader {
	t.Helper()
	p := NewProjector(testOrigin)
	return fakeReader{
		searchJobs: func(_ context.Context, in SearchInput) (SearchJobsResult, error) {
			return SearchJobsResult{
				Query: in.Query, Total: 1,
				Jobs:          []JobSummary{p.JobSummary(aJob(t))},
				IgnoredParams: in.Unsupported(),
			}, nil
		},
		jobDetail: func(_ context.Context, _ string) (JobResult, error) {
			return p.JobDetail(aJob(t), &applyform.Form{Provider: "greenhouse"}), nil
		},
		searchCompanies: func(context.Context, CompanySearchInput) (SearchCompaniesResult, error) {
			return SearchCompaniesResult{Total: 1, Companies: []CompanySummary{{Slug: "acme", Name: "Acme"}}}, nil
		},
		companyDetail: func(context.Context, string) (CompanyResult, error) {
			return CompanyResult{CompanySummary: CompanySummary{Slug: "acme", Name: "Acme"}}, nil
		},
	}
}

func TestEveryAdvertisedToolCanActuallyBeCalled(t *testing.T) {
	// A name in tools/list with no handler behind it is a tool ChatGPT will try once and
	// then stop trusting.
	session := connect(t, readerThatFinds(t))
	ctx := context.Background()

	// The expected set is written HERE rather than read from the package, so the assertion
	// is against what this surface promises and not against whatever it happens to register.
	// A list the production code exports only for its own test to compare with proves
	// nothing — it agrees with itself by construction.
	calls := map[string]map[string]any{
		toolSearchJobs:      {"query": "go"},
		toolGetJob:          {"slug": "senior-go-engineer-acme-abc123"},
		toolSearchCompanies: {"query": "acme"},
		toolGetCompany:      {"slug": "acme"},
	}

	listed, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("tools/list: %v", err)
	}
	names := make([]string, 0, len(listed.Tools))
	for _, tool := range listed.Tools {
		names = append(names, tool.Name)
	}
	if len(names) != len(calls) {
		t.Fatalf("tools/list returned %v, want the %d tools this surface promises", names, len(calls))
	}

	for name := range calls {
		res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: calls[name]})
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if res.IsError {
			t.Errorf("%s answered an error: %+v", name, res.Content)
		}
		if res.StructuredContent == nil {
			t.Errorf("%s returned no structured content; the model has only prose to read", name)
		}
	}
}

func TestEveryToolDeclaresWhatItDoesAndWhatItDoesNotTouch(t *testing.T) {
	// Incorrect annotation of these three hints is a named rejection reason in OpenAI's own
	// submission guidelines. All four tools read; none writes; all reach a catalogue that
	// changes outside the conversation.
	//
	// The test walks the REGISTERED tools rather than a list written beside it, so a fifth
	// tool added by copy-paste cannot inherit a read-only claim it does not deserve.
	session := connect(t, readerThatFinds(t))

	listed, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("tools/list: %v", err)
	}
	for _, tool := range listed.Tools {
		if tool.Annotations == nil {
			t.Errorf("%s declares no annotations", tool.Name)
			continue
		}
		if !tool.Annotations.ReadOnlyHint {
			t.Errorf("%s is not marked read-only", tool.Name)
		}
		if tool.Annotations.DestructiveHint != nil && *tool.Annotations.DestructiveHint {
			t.Errorf("%s is marked destructive", tool.Name)
		}
		if tool.Annotations.OpenWorldHint == nil || !*tool.Annotations.OpenWorldHint {
			t.Errorf("%s is not marked open-world; the catalogue changes outside the conversation", tool.Name)
		}
		if tool.Title == "" {
			t.Errorf("%s has no title", tool.Name)
		}
		if tool.Description == "" {
			t.Errorf("%s has no description", tool.Name)
		}
		if tool.OutputSchema == nil {
			t.Errorf("%s publishes no output schema; ChatGPT cannot tell what it answers", tool.Name)
		}
	}
}

func TestTheServersInstructionsFitTheBudget(t *testing.T) {
	// 512 characters is what the Apps SDK guidance asks for, and a limit nobody re-checks
	// by eye is one that drifts on the next edit.
	if got := utf8.RuneCountInString(serverInstructions); got > 512 {
		t.Errorf("instructions are %d characters, want at most 512", got)
	}
	if strings.TrimSpace(serverInstructions) == "" {
		t.Error("the server offers no instructions at all")
	}
}

func TestAMissingPostingIsRecoverableRatherThanAProtocolError(t *testing.T) {
	// The difference from the OJCP transport beside this one, and it is deliberate. There a
	// failure is a JSON-RPC error carrying a code an agent branches on. Here the caller is
	// a language model: a protocol error surfaces as a generic failure, while an error
	// RESULT is a sentence it reads and recovers from by searching instead.
	r := readerThatFinds(t)
	r.jobDetail = func(context.Context, string) (JobResult, error) {
		return JobResult{}, NotFoundError{What: "posting"}
	}

	res, err := connect(t, r).CallTool(context.Background(), &mcp.CallToolParams{
		Name: toolGetJob, Arguments: map[string]any{"slug": "nope"},
	})
	if err != nil {
		t.Fatalf("a missing posting reached the protocol as an error: %v", err)
	}
	if !res.IsError {
		t.Fatal("a missing posting was answered as a success")
	}
	if !strings.Contains(strings.ToLower(text(t, res)), "no posting") {
		t.Errorf("message = %q, want it to say what was not found", text(t, res))
	}
}

func TestAnInternalFailureTellsTheModelNothingAboutItself(t *testing.T) {
	// What broke inside this deployment is ours, not a caller's — but it stays an error
	// rather than an empty answer, which would read as a catalogue holding nothing.
	r := readerThatFinds(t)
	r.searchJobs = func(context.Context, SearchInput) (SearchJobsResult, error) {
		return SearchJobsResult{}, errors.New("pq: connection refused on 10.0.0.4")
	}

	res, err := connect(t, r).CallTool(context.Background(), &mcp.CallToolParams{
		Name: toolSearchJobs, Arguments: map[string]any{"query": "go"},
	})
	if err != nil {
		t.Fatalf("an internal failure reached the protocol as an error: %v", err)
	}
	if !res.IsError {
		t.Fatal("an internal failure was answered as a success")
	}
	if strings.Contains(text(t, res), "10.0.0.4") {
		t.Errorf("message = %q, want no internal detail", text(t, res))
	}
}

func TestAnUnhonouredFilterReachesTheModelByName(t *testing.T) {
	// The answer is wider than asked, and the caller is told which word was not understood
	// rather than reading a zero as an empty catalogue.
	res, err := connect(t, readerThatFinds(t)).CallTool(context.Background(), &mcp.CallToolParams{
		Name: toolSearchJobs, Arguments: map[string]any{"query": "ai", "category": []string{"ai"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text(t, res), "category=ai") {
		t.Errorf("answer = %q, want it to name the filter it could not honour", text(t, res))
	}
}

func text(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()
	var b strings.Builder
	for _, content := range res.Content {
		if tc, ok := content.(*mcp.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}
