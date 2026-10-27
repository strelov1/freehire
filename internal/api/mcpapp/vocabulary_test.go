package mcpapp

import (
	"context"
	"encoding/json"
	"slices"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Driven through the SDK client, not by calling FilterValues: the SDK validates the
// structured answer against the schema it inferred from the Go type, so a shape it rejects
// (a nil slice where an array is declared, a custom MarshalJSON) fails every real call and
// no direct call would notice.
func callFilterValues(t *testing.T, args map[string]any) FilterValuesResult {
	t.Helper()
	res, err := connect(t, readerThatFinds(t)).CallTool(context.Background(),
		&mcp.CallToolParams{Name: toolFilterValues, Arguments: args})
	if err != nil {
		t.Fatalf("tools/call: %v", err)
	}
	if res.IsError {
		t.Fatalf("answered an error: %+v", res.Content)
	}
	raw, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var out FilterValuesResult
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func values(r FilterValuesResult) []string {
	out := make([]string, 0, len(r.Values))
	for _, v := range r.Values {
		out = append(out, v.Value)
	}
	return out
}

func TestFilterValuesListsMostCommonFirst(t *testing.T) {
	got := callFilterValues(t, map[string]any{"filter": "skills"})
	if want := []string{"go", "rust", "golang-migrate"}; !slices.Equal(values(got), want) {
		t.Errorf("values = %v, want %v", values(got), want)
	}
}

func TestFilterValuesFindsTheCanonicalOfAnAlias(t *testing.T) {
	// "golang" is not a substring of "go"; the model must still be shown the value the
	// search will read its word as.
	got := callFilterValues(t, map[string]any{"filter": "skills", "contains": "golang"})
	if want := []string{"go", "golang-migrate"}; !slices.Equal(values(got), want) {
		t.Errorf("values = %v, want %v", values(got), want)
	}
}

func TestFilterValuesBoundsThePage(t *testing.T) {
	got := callFilterValues(t, map[string]any{"filter": "skills", "limit": 1})
	if len(got.Values) != 1 || got.More != 2 {
		t.Errorf("got %d values and more=%d, want 1 and 2", len(got.Values), got.More)
	}
}

func TestFilterValuesNamesAFilterItDoesNotKnow(t *testing.T) {
	got := callFilterValues(t, map[string]any{"filter": "salary"})
	if len(got.Values) != 0 || !slices.Equal(got.IgnoredParams, []string{"filter=salary"}) {
		t.Errorf("got %+v, want no values and filter=salary named", got)
	}
}

func TestEveryListableFilterIsASearchField(t *testing.T) {
	// The keys are search_jobs' own field names, so the model can write the value straight
	// back. Read from the input schema the SDK publishes, not from a list beside it.
	listed, err := connect(t, readerThatFinds(t)).ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range listed.Tools {
		if tool.Name != toolSearchJobs {
			continue
		}
		raw, _ := json.Marshal(tool.InputSchema)
		var schema struct {
			Properties map[string]any `json:"properties"`
		}
		if err := json.Unmarshal(raw, &schema); err != nil {
			t.Fatal(err)
		}
		for field := range filterValueFacets {
			if _, ok := schema.Properties[field]; !ok {
				t.Errorf("%s is listable but not a search_jobs field", field)
			}
		}
		return
	}
	t.Fatal("search_jobs is not registered")
}
