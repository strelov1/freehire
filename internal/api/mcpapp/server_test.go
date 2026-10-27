package mcpapp

import (
	"context"
	"encoding/json"
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
	server, err := newServer(r, "").Connect(ctx, serverTransport, nil)
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
		checkJobsOpen: func(_ context.Context, slugs []string) ([]JobOpenState, error) {
			out := make([]JobOpenState, 0, len(slugs))
			for _, s := range slugs {
				out = append(out, JobOpenState{Slug: s, Status: StatusOpen})
			}
			return out, nil
		},
		similarJobs: func(context.Context, string, int) (SimilarJobsResult, error) {
			return SimilarJobsResult{Jobs: []JobSummary{p.JobSummary(aJob(t))}}, nil
		},
		salaryStats: func(_ context.Context, in SalaryInput) (SalaryStatsResult, error) {
			return SalaryStatsResult{Bands: []SalaryBand{{Currency: "USD", Period: "year", SampleSize: 120, P50: 150000}}, IgnoredParams: in.Unsupported()}, nil
		},
		marketStats: func(_ context.Context, in SearchInput) (MarketStatsResult, error) {
			return MarketStatsResult{Total: 1, IgnoredParams: in.Unsupported()}, nil
		},
		facetCounts: func(context.Context) (map[string]map[string]int64, error) {
			return map[string]map[string]int64{"skills": {"go": 8522, "golang-migrate": 3, "rust": 900}}, nil
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
		toolCheckJobsOpen:   {"slugs": []string{"senior-go-engineer-acme-abc123"}},
		toolSearch:          {"query": "go"},
		toolFetch:           {"id": "senior-go-engineer-acme-abc123"},
		toolSimilarJobs:     {"slug": "senior-go-engineer-acme-abc123"},
		toolSalaryStats:     {"category": "backend"},
		toolMarketStats:     {"skills": []string{"go"}},
		toolFilterValues:    {"filter": "skills"},
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
	// submission guidelines. Every tool reads; none writes; all reach a catalogue that
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

func TestNoToolDescriptionDirectsTheModelToAnotherTool(t *testing.T) {
	// Anthropic's directory policy rules out tool descriptions that instruct the model about
	// other tools; "call get_job for the full text" was exactly that, in three places. A
	// description says what the tool returns. Cross-tool guidance, where it is needed at all,
	// belongs in the server instructions, which this test does not read.
	//
	// It walks the REGISTERED tools — descriptions and every field description in both
	// schemas — so a new tool or field cannot reintroduce one unnoticed. "search" alone is
	// not checked: it is also the ordinary English word every search tool's prose uses.
	listed, err := connect(t, readerThatFinds(t)).ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("tools/list: %v", err)
	}
	names := []string{toolSearchJobs, toolGetJob, toolSearchCompanies, toolGetCompany, toolCheckJobsOpen, toolFetch,
		toolSimilarJobs, toolSalaryStats, toolMarketStats, toolFilterValues}

	for _, tool := range listed.Tools {
		raw, err := json.Marshal(struct {
			Description string
			In, Out     any
		}{tool.Description, tool.InputSchema, tool.OutputSchema})
		if err != nil {
			t.Fatalf("%s: marshal: %v", tool.Name, err)
		}
		for _, name := range names {
			if name != tool.Name && strings.Contains(string(raw), name) {
				t.Errorf("%s's description or schema names %s", tool.Name, name)
			}
		}
	}
}

// The search engine understands OR, title:, quotes and -word in the query, but a model
// only writes them if the tool schema it reads says so. Read the PUBLISHED schema, the
// thing the model sees, not the struct tag.
func TestSearchJobsQueryAdvertisesTheOperators(t *testing.T) {
	session := connect(t, readerThatFinds(t))
	listed, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("tools/list: %v", err)
	}
	for _, tool := range listed.Tools {
		if tool.Name != toolSearchJobs {
			continue
		}
		raw, err := json.Marshal(tool.InputSchema)
		if err != nil {
			t.Fatal(err)
		}
		var schema struct {
			Properties map[string]struct {
				Description string `json:"description"`
			} `json:"properties"`
		}
		if err := json.Unmarshal(raw, &schema); err != nil {
			t.Fatal(err)
		}
		desc := schema.Properties["query"].Description
		for _, want := range []string{" OR ", "title:", "-word", "quote"} {
			if !strings.Contains(desc, want) {
				t.Errorf("query description %q does not mention %q", desc, want)
			}
		}
		return
	}
	t.Fatalf("%s is not registered", toolSearchJobs)
}

// A client shows a server's logo and website beside it in its list of connections, from
// what the server introduces itself with. Built from the origin, so a deployment with none
// introduces itself by name alone rather than with icon paths no client can fetch.
func TestIdentityCarriesTheLogoWhenTheOriginIsKnown(t *testing.T) {
	id := Identity("freehire", Version, "https://freehire.example/")
	if id.WebsiteURL != "https://freehire.example" {
		t.Errorf("WebsiteURL = %q, want the origin without its trailing slash", id.WebsiteURL)
	}
	if len(id.Icons) == 0 {
		t.Fatal("no icons")
	}
	for _, icon := range id.Icons {
		if !strings.HasPrefix(icon.Source, "https://freehire.example/") {
			t.Errorf("icon %q is not an absolute URL on the origin", icon.Source)
		}
	}

	bare := Identity("freehire", Version, "")
	if bare.WebsiteURL != "" || len(bare.Icons) != 0 {
		t.Errorf("with no origin: WebsiteURL = %q, icons = %v; want neither", bare.WebsiteURL, bare.Icons)
	}
}
