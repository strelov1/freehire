package mcpapp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/strelov1/freehire/internal/ai/assistant"
)

func connectAccountCatalogue(t *testing.T, tools []assistant.Tool, reader Reader) *mcp.ClientSession {
	t.Helper()
	server := newMixedServer(tools, 42, "", reader, false)
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	ss, err := server.Connect(t.Context(), serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ss.Close() })
	session, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil).
		Connect(t.Context(), clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

func TestAccountCatalogueToolsAreCallableAndRemainReadOnly(t *testing.T) {
	session := connectAccountCatalogue(t, nil, readerThatFinds(t))
	listed, err := session.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	calls := map[string]struct {
		args map[string]any
		key  string
	}{
		"search_jobs":        {args: map[string]any{"query": "go"}, key: "jobs"},
		"get_job":            {args: map[string]any{"slug": "job-a"}, key: "description"},
		"get_company":        {args: map[string]any{"slug": "acme"}, key: "slug"},
		"search_companies":   {args: map[string]any{"query": "acme"}, key: "companies"},
		"check_jobs_open":    {args: map[string]any{"slugs": []string{"job-a"}}, key: "jobs"},
		"search":             {args: map[string]any{"query": "go"}, key: "results"},
		"fetch":              {args: map[string]any{"id": "senior-go-engineer-acme-abc123"}, key: "id"},
		"similar_jobs":       {args: map[string]any{"slug": "job-a"}, key: "jobs"},
		"salary_stats":       {args: map[string]any{"category": "backend"}, key: "bands"},
		"job_market_stats":   {args: map[string]any{"skills": []string{"go"}}, key: "total"},
		"list_filter_values": {args: map[string]any{"filter": "skills"}, key: "values"},
	}
	if len(listed.Tools) != len(calls) {
		t.Fatalf("signed-in catalogue advertises %d tools, want %d", len(listed.Tools), len(calls))
	}
	for _, tool := range listed.Tools {
		if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint {
			t.Errorf("%s is not marked read-only", tool.Name)
		}
		if tool.Annotations == nil || tool.Annotations.DestructiveHint == nil {
			t.Errorf("%s has no destructive hint", tool.Name)
			continue
		}
		if *tool.Annotations.DestructiveHint {
			t.Errorf("%s is marked destructive", tool.Name)
		}
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			var got map[string]json.RawMessage
			callJSON(t, session, name, call.args, &got)
			if _, ok := got[call.key]; !ok {
				t.Errorf("%s response lacks %s: %v", name, call.key, got)
			}
		})
	}
}

func TestAccountCatalogueDoesNotExportPayloadsToAnalytics(t *testing.T) {
	const private = "private-account-search@example.invalid"
	records := captureToolLogs(t)
	reader := readerThatFinds(t)
	reader.searchCompanies = func(context.Context, CompanySearchInput) (SearchCompaniesResult, error) {
		return SearchCompaniesResult{Total: 1, Companies: []CompanySummary{{Name: private}}}, nil
	}
	session := connectAccountCatalogue(t, nil, reader)
	var got SearchCompaniesResult
	callJSON(t, session, "search_companies", map[string]any{"query": private}, &got)
	if got.Companies[0].Name != private {
		t.Fatal("the test call did not return the sensitive payload")
	}
	start, finish := nextToolLog(t, records), nextToolLog(t, records)
	encoded, _ := json.Marshal([]map[string]any{start, finish})
	events := string(encoded)
	if finish["tool"] != "search_companies" || finish["outcome"] != "success" {
		t.Fatalf("missing local analytics: %s", events)
	}
	if strings.Contains(events, private) {
		t.Fatal("an account catalogue query or response leaked into analytics")
	}
}

// The catalogue owns search_jobs, get_job and get_company on the one server: the assistant's
// copies of those names are never registered, so /mcp and /mcp/account publish the same schemas.
func TestSignedInCatalogueOwnsItsNamesAndKeepsPersonalHandlers(t *testing.T) {
	tools := []assistant.Tool{}
	for _, name := range []string{"search_jobs", "get_job", "get_company", "my_jobs"} {
		tools = append(tools, assistant.Tool{
			Name: name,
			Schema: map[string]any{
				"type": "object", "properties": map[string]any{"account_marker": map[string]any{"type": "string"}},
				"required": []string{"account_marker"},
			},
			Run: func(_ context.Context, userID int64, _ json.RawMessage) (any, error) {
				return map[string]any{"account_user": userID}, nil
			},
		})
	}
	session := connectAccountCatalogue(t, tools, readerThatFinds(t))
	listed, err := session.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range listed.Tools {
		schema, _ := json.Marshal(tool.InputSchema)
		marked := strings.Contains(string(schema), "account_marker")
		if marked != (tool.Name == "my_jobs") {
			t.Errorf("%s: account_marker in schema = %v; only the personal my_jobs may carry the assistant's schema", tool.Name, marked)
		}
	}
	var got struct {
		User int64 `json:"account_user"`
	}
	callJSON(t, session, "my_jobs", map[string]any{"account_marker": "keep"}, &got)
	if got.User != 42 {
		t.Errorf("my_jobs returned user %d, want authenticated user 42", got.User)
	}
}

func TestSignedInCatalogueUsesMixedMetrics(t *testing.T) {
	session := connectAccountCatalogue(t, nil, readerThatFinds(t))
	account := mcpToolCalls.WithLabelValues("mixed", "check_jobs_open", "success", "")
	public := mcpToolCalls.WithLabelValues("public", "check_jobs_open", "success", "")
	beforeAccount, beforePublic := testutil.ToFloat64(account), testutil.ToFloat64(public)
	var got JobsOpenResult
	callJSON(t, session, "check_jobs_open", map[string]any{"slugs": []string{"job-a"}}, &got)
	if delta := testutil.ToFloat64(account) - beforeAccount; delta != 1 {
		t.Errorf("account calls incremented by %v, want 1", delta)
	}
	if delta := testutil.ToFloat64(public) - beforePublic; delta != 0 {
		t.Errorf("public calls incremented by %v, want 0", delta)
	}
}
