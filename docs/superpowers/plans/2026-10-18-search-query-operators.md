# Search query operators (`OR`, `title:`) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `q` on every jobs search accepts `OR` (alternatives) and a leading `title:` (title-only alternative). A query using neither is sent to Meilisearch exactly as today.

**Architecture:** A pure parser (`ParseQuery`) splits `q` into alternatives. One alternative goes through today's single-index search, with `title:` mapped to `AttributesToSearchOn`. Two or more go out as ONE federated `/multi-search` request carrying the same filter, sort and vector on each query. Federation pages, deduplicates and merges facets. Every jobs search caller (web, agent API, OJCP, MCP, assistant) gets this through `search.Client`.

**Tech Stack:** Go, meilisearch-go v0.36.3, Meilisearch v1.49.0 (prod), testcontainers.

Spec: `docs/superpowers/specs/2026-10-18-search-query-operators-design.md`.

## Global Constraints

- `OR` is an operator only when it is uppercase, a standalone whitespace-delimited token, and outside double quotes.
- `title:` is an operator only at the start of an alternative. It applies to the whole alternative.
- At most 5 alternatives. More is HTTP 400 with the message `q has more than 5 OR alternatives`.
- A query with no operator must build a byte-identical request to today's (`Query == p.Query`).
- Merged facets must pass `maxValuesPerFacet` (1200). Federation's default is 100, which silently truncates.
- Per-query `limit`/`offset`/`facets` are a 400 inside a federated request. Only the `federation` object carries them.
- English only in code and docs. `gofmt -l .` must print nothing. Run `go vet -tags=integration ./...` before pushing.

## File Structure

- Create `internal/search/search/query_syntax.go`: `Alternative`, `ParseQuery`, `ErrTooManyAlternatives`.
- Create `internal/search/search/query_syntax_test.go`: parser table tests.
- Create `internal/search/search/federated.go`: `buildFederatedSearch`, `buildFederatedFacets`, `federatedAsSearchResponse`.
- Create `internal/search/search/federated_test.go`: builder and `Client` unit tests using a fake `ServiceManager`.
- Modify `internal/search/search/client.go` (`Client.Search`, ~line 544).
- Modify `internal/search/search/facets.go` (`FacetCounts`, `DisjunctiveFacetCounts`), adding `Client.facetSearch`.
- Modify `internal/search/search/testhelpers_test.go`: add `fakeManager`.
- Modify `internal/api/handler/errors.go` (`classify`, ~line 159).
- Modify `internal/search/search/search_integration_test.go`: image → v1.49.0, plus a new test.
- Modify `web/static/openapi.yaml` (`Q` parameter, ~line 907) and `internal/search/search/AGENTS.md`.

---

### Task 1: Query parser

**Files:**

- Create: `internal/search/search/query_syntax.go`
- Test: `internal/search/search/query_syntax_test.go`

**Interfaces:**

- Produces: `type Alternative struct { Text string; Fields []string }`, `func ParseQuery(q string) ([]Alternative, error)`, `var ErrTooManyAlternatives error`, `const maxAlternatives = 5`.

- [ ] **Step 1: Write the failing test**

```go
package search

import (
	"errors"
	"reflect"
	"testing"
)

func TestParseQuery(t *testing.T) {
	title := []string{"title"}
	cases := []struct {
		name string
		q    string
		want []Alternative
	}{
		{"empty stays one empty alternative", "", []Alternative{{Text: ""}}},
		{"no operator returns q untouched, spacing included", "  golang   rust ", []Alternative{{Text: "  golang   rust "}}},
		{"lowercase or is a word", "golang or rust", []Alternative{{Text: "golang or rust"}}},
		{"OR splits", "produktchef OR \"product owner\"", []Alternative{{Text: "produktchef"}, {Text: "\"product owner\""}}},
		{"OR inside quotes is text", "\"a OR b\"", []Alternative{{Text: "\"a OR b\""}}},
		{"minus survives per alternative", "golang -senior OR rust", []Alternative{{Text: "golang -senior"}, {Text: "rust"}}},
		{"title: scopes its alternative", "title:\"product owner\" -junior", []Alternative{{Text: "\"product owner\" -junior", Fields: title}}},
		{"title: per alternative", "title:produktchef OR golang", []Alternative{{Text: "produktchef", Fields: title}, {Text: "golang"}}},
		{"detached title: prefix", "title: produktchef", []Alternative{{Text: "produktchef", Fields: title}}},
		{"title: mid-alternative is text", "golang title:lead", []Alternative{{Text: "golang title:lead"}}},
		{"empty alternatives dropped", "OR golang OR OR rust OR", []Alternative{{Text: "golang"}, {Text: "rust"}}},
		{"only operators falls back to q", "OR", []Alternative{{Text: "OR"}}},
		{"bare title: falls back to q", "title:", []Alternative{{Text: "title:"}}},
		{"five alternatives allowed", "a OR b OR c OR d OR e", []Alternative{{Text: "a"}, {Text: "b"}, {Text: "c"}, {Text: "d"}, {Text: "e"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseQuery(tc.q)
			if err != nil {
				t.Fatalf("ParseQuery(%q) error: %v", tc.q, err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("ParseQuery(%q) = %#v, want %#v", tc.q, got, tc.want)
			}
		})
	}
}

func TestParseQueryRefusesMoreThanFiveAlternatives(t *testing.T) {
	_, err := ParseQuery("a OR b OR c OR d OR e OR f")
	if !errors.Is(err, ErrTooManyAlternatives) {
		t.Fatalf("err = %v, want ErrTooManyAlternatives", err)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/search/search/ -run TestParseQuery`
Expected: FAIL to compile, `undefined: ParseQuery`.

- [ ] **Step 3: Write the implementation**

```go
package search

import (
	"errors"
	"strings"
	"unicode"
)

// maxAlternatives bounds how many OR alternatives one q may carry. The search
// endpoints are public and each alternative is one more Meilisearch query.
const maxAlternatives = 5

// titlePrefix scopes the alternative it starts to the title field.
const titlePrefix = "title:"

// ErrTooManyAlternatives is returned for a q with more than maxAlternatives OR
// alternatives. Handlers render its message as a 400.
var ErrTooManyAlternatives = errors.New("q has more than 5 OR alternatives")

// Alternative is one OR branch of q: the text handed to Meilisearch as written
// (so its quotes and leading-minus exclusions keep working) and, when the branch
// began with title:, the fields it is restricted to. Nil Fields means the
// caller's q_fields, or every searchable field.
type Alternative struct {
	Text   string
	Fields []string
}

// ParseQuery splits q on standalone uppercase OR tokens outside double quotes
// and reads a leading title: on each branch. A q that uses neither operator comes
// back as ONE alternative whose Text is q byte-for-byte, so the request built
// from it is exactly the request built before operators existed.
func ParseQuery(q string) ([]Alternative, error) {
	groups := [][]string{{}}
	hasOperator := false
	for _, tok := range splitOutsideQuotes(q) {
		if tok == "OR" {
			hasOperator = true
			groups = append(groups, []string{})
			continue
		}
		groups[len(groups)-1] = append(groups[len(groups)-1], tok)
	}

	var alts []Alternative
	for _, g := range groups {
		var fields []string
		if len(g) > 0 && strings.HasPrefix(g[0], titlePrefix) {
			hasOperator = true
			fields = []string{"title"}
			g[0] = strings.TrimPrefix(g[0], titlePrefix)
			if g[0] == "" {
				g = g[1:]
			}
		}
		if len(g) == 0 {
			continue
		}
		alts = append(alts, Alternative{Text: strings.Join(g, " "), Fields: fields})
	}
	if !hasOperator || len(alts) == 0 {
		return []Alternative{{Text: q}}, nil
	}
	if len(alts) > maxAlternatives {
		return nil, ErrTooManyAlternatives
	}
	return alts, nil
}

// splitOutsideQuotes splits q on whitespace that is not inside double quotes,
// keeping the quotes in the tokens.
func splitOutsideQuotes(q string) []string {
	var (
		tokens  []string
		b       strings.Builder
		inQuote bool
	)
	for _, r := range q {
		switch {
		case r == '"':
			inQuote = !inQuote
			b.WriteRune(r)
		case unicode.IsSpace(r) && !inQuote:
			if b.Len() > 0 {
				tokens = append(tokens, b.String())
				b.Reset()
			}
		default:
			b.WriteRune(r)
		}
	}
	if b.Len() > 0 {
		tokens = append(tokens, b.String())
	}
	return tokens
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/search/search/ -run TestParseQuery -v`
Expected: PASS for every case.

- [ ] **Step 5: Commit**

```bash
gofmt -w internal/search/search/query_syntax.go internal/search/search/query_syntax_test.go
git add internal/search/search/query_syntax.go internal/search/search/query_syntax_test.go
git commit -m "search: parse OR and title: operators in q"
```

---

### Task 2: Federated Search for hits

**Files:**

- Create: `internal/search/search/federated.go`
- Create: `internal/search/search/federated_test.go`
- Modify: `internal/search/search/testhelpers_test.go`
- Modify: `internal/search/search/client.go` (`func (c *Client) Search`, ~line 544)
- Modify: `internal/api/handler/errors.go` (`classify`, the `ErrBadQuery` case, ~line 159)

**Interfaces:**

- Consumes: `ParseQuery`, `Alternative`, `ErrTooManyAlternatives` (Task 1); `buildSearchRequest(SearchParams) *meilisearch.SearchRequest` and `facetIndexUID` (existing).
- Produces: `func buildFederatedSearch(p SearchParams, alts []Alternative) *meilisearch.MultiSearchRequest`; `type fakeManager` in `testhelpers_test.go` with field `multiFn func(ctx context.Context, req *meilisearch.MultiSearchRequest) (*meilisearch.MultiSearchResponse, error)`.
- [ ] **Step 1: Add the fake manager to `testhelpers_test.go`**

Append:

```go
// fakeManager stubs meilisearch.ServiceManager for unit tests that only need
// MultiSearchWithContext (the federated OR path). Embedding the interface
// satisfies every other method.
type fakeManager struct {
	meilisearch.ServiceManager
	multiFn func(ctx context.Context, req *meilisearch.MultiSearchRequest) (*meilisearch.MultiSearchResponse, error)
}

func (f fakeManager) MultiSearchWithContext(ctx context.Context, req *meilisearch.MultiSearchRequest) (*meilisearch.MultiSearchResponse, error) {
	return f.multiFn(ctx, req)
}
```

- [ ] **Step 2: Write the failing tests in `federated_test.go`**

```go
package search

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/meilisearch/meilisearch-go"
)

func TestFederatedSearchCarriesFilterSortAndFieldsPerQuery(t *testing.T) {
	p := SearchParams{Query: "ignored", Filter: "source = \"x\"", Sort: []string{"posted_at:desc"}, Limit: 25, Offset: 50, QFields: []string{"company"}}
	req := buildFederatedSearch(p, []Alternative{{Text: "produktchef", Fields: []string{"title"}}, {Text: "\"product owner\""}})

	if req.Federation == nil || req.Federation.Limit != 25 || req.Federation.Offset != 50 {
		t.Fatalf("Federation = %+v, want limit 25 offset 50", req.Federation)
	}
	if len(req.Queries) != 2 {
		t.Fatalf("got %d queries, want 2", len(req.Queries))
	}
	for i, q := range req.Queries {
		if q.IndexUID != facetIndexUID {
			t.Errorf("query %d IndexUID = %q, want %q", i, q.IndexUID, facetIndexUID)
		}
		if q.Filter != p.Filter || !reflect.DeepEqual(q.Sort, p.Sort) {
			t.Errorf("query %d lost filter/sort: %+v", i, q)
		}
		if q.Limit != 0 || q.Offset != 0 {
			t.Errorf("query %d carries limit/offset (%d/%d); federation rejects that", i, q.Limit, q.Offset)
		}
	}
	if req.Queries[0].Query != "produktchef" || !reflect.DeepEqual(req.Queries[0].AttributesToSearchOn, []string{"title"}) {
		t.Errorf("query 0 = %q on %v, want produktchef on [title]", req.Queries[0].Query, req.Queries[0].AttributesToSearchOn)
	}
	if req.Queries[1].Query != "\"product owner\"" || !reflect.DeepEqual(req.Queries[1].AttributesToSearchOn, []string{"company"}) {
		t.Errorf("query 1 = %q on %v, want the q_fields fallback [company]", req.Queries[1].Query, req.Queries[1].AttributesToSearchOn)
	}
}

func TestSearchWithoutOperatorsSendsTodaysSingleRequest(t *testing.T) {
	var gotQuery string
	c := &Client{
		facet: fakeSearchIndex{searchFn: func(_ context.Context, q string, _ *meilisearch.SearchRequest) (*meilisearch.SearchResponse, error) {
			gotQuery = q
			return &meilisearch.SearchResponse{}, nil
		}},
		manager: fakeManager{multiFn: func(context.Context, *meilisearch.MultiSearchRequest) (*meilisearch.MultiSearchResponse, error) {
			t.Fatal("a query without operators must not go federated")
			return nil, nil
		}},
	}
	if _, err := c.Search(context.Background(), SearchParams{Query: "golang  rust", Limit: 10}); err != nil {
		t.Fatal(err)
	}
	if gotQuery != "golang  rust" {
		t.Fatalf("query = %q, want q untouched", gotQuery)
	}
}

func TestSearchSingleTitleAlternativeStaysSingleIndex(t *testing.T) {
	var got *meilisearch.SearchRequest
	var gotQuery string
	c := &Client{facet: fakeSearchIndex{searchFn: func(_ context.Context, q string, r *meilisearch.SearchRequest) (*meilisearch.SearchResponse, error) {
		gotQuery, got = q, r
		return &meilisearch.SearchResponse{}, nil
	}}}
	if _, err := c.Search(context.Background(), SearchParams{Query: "title:\"product owner\"", Limit: 10}); err != nil {
		t.Fatal(err)
	}
	if gotQuery != "\"product owner\"" || !reflect.DeepEqual(got.AttributesToSearchOn, []string{"title"}) {
		t.Fatalf("sent %q on %v, want \"product owner\" on [title]", gotQuery, got.AttributesToSearchOn)
	}
}

func TestSearchWithOrGoesFederated(t *testing.T) {
	var got *meilisearch.MultiSearchRequest
	c := &Client{manager: fakeManager{multiFn: func(_ context.Context, r *meilisearch.MultiSearchRequest) (*meilisearch.MultiSearchResponse, error) {
		got = r
		return &meilisearch.MultiSearchResponse{EstimatedTotalHits: 7}, nil
	}}}
	res, err := c.Search(context.Background(), SearchParams{Query: "golang OR rust", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || len(got.Queries) != 2 || res.Total != 7 {
		t.Fatalf("request = %+v, total = %d; want 2 federated queries and total 7", got, res.Total)
	}
}

func TestSearchRefusesTooManyAlternatives(t *testing.T) {
	c := &Client{}
	_, err := c.Search(context.Background(), SearchParams{Query: "a OR b OR c OR d OR e OR f", Limit: 10})
	if !errors.Is(err, ErrTooManyAlternatives) {
		t.Fatalf("err = %v, want ErrTooManyAlternatives", err)
	}
}
```

- [ ] **Step 3: Run the tests to verify they fail**

Run: `go test ./internal/search/search/ -run 'Federated|TestSearch(Without|Single|WithOr|Refuses)'`
Expected: FAIL to compile, `undefined: buildFederatedSearch`.

- [ ] **Step 4: Create `federated.go` with the hits builder**

```go
package search

import "github.com/meilisearch/meilisearch-go"

// buildFederatedSearch turns a q with several OR alternatives into ONE federated
// multi-search over the jobs index. Each alternative carries the caller's filter,
// sort and vector; federation pages, deduplicates and ranks the merged list. The
// per-query limit/offset are cleared because Meilisearch rejects them inside a
// federation, which is where paging lives.
func buildFederatedSearch(p SearchParams, alts []Alternative) *meilisearch.MultiSearchRequest {
	queries := make([]*meilisearch.SearchRequest, 0, len(alts))
	for _, a := range alts {
		q := p
		q.Query = a.Text
		if a.Fields != nil {
			q.QFields = a.Fields
		}
		req := buildSearchRequest(q)
		req.IndexUID = facetIndexUID
		req.Query = a.Text
		req.Limit, req.Offset = 0, 0
		queries = append(queries, req)
	}
	return &meilisearch.MultiSearchRequest{
		Federation: &meilisearch.MultiSearchFederation{Limit: int64(p.Limit), Offset: int64(p.Offset)},
		Queries:    queries,
	}
}
```

- [ ] **Step 5: Route `Client.Search` through the parser**

Replace the body of `func (c *Client) Search` in `client.go` with:

```go
func (c *Client) Search(ctx context.Context, p SearchParams) (SearchResult, error) {
	alts, err := ParseQuery(p.Query)
	if err != nil {
		return SearchResult{}, err
	}
	var (
		hits  meilisearch.Hits
		total int64
	)
	if len(alts) == 1 {
		p.Query = alts[0].Text
		if alts[0].Fields != nil {
			p.QFields = alts[0].Fields
		}
		resp, err := c.facet.SearchWithContext(ctx, p.Query, buildSearchRequest(p))
		if err != nil {
			return SearchResult{}, queryErr(ctx, "search: query", err)
		}
		hits, total = resp.Hits, resp.EstimatedTotalHits
	} else {
		resp, err := c.manager.MultiSearchWithContext(ctx, buildFederatedSearch(p, alts))
		if err != nil {
			return SearchResult{}, queryErr(ctx, "search: federated query", err)
		}
		hits, total = resp.Hits, resp.EstimatedTotalHits
	}

	var docs []JobDocument
	if err := hits.DecodeInto(&docs); err != nil {
		return SearchResult{}, fmt.Errorf("search: decode hits: %w", err)
	}
	return SearchResult{Hits: docs, Total: total}, nil
}
```

Note: the existing `TestSearch*` unit tests in this package that build `&Client{facet: ...}` with no manager still pass, because they send no operator.

- [ ] **Step 6: Render the parse error as a 400 with its own message**

In `internal/api/handler/errors.go` `classify`, add this case directly ABOVE `case errors.Is(err, search.ErrBadQuery):`:

```go
	// q carried more OR alternatives than search allows. Say which limit was hit:
	// the generic "invalid search query" would leave the caller guessing.
	case errors.Is(err, search.ErrTooManyAlternatives):
		return fiber.StatusBadRequest, search.ErrTooManyAlternatives.Error(), false
```

`ErrTooManyAlternatives` does not wrap `ErrBadQuery`, so `handler/search.go`'s retry-without-filter branch (taken only for `ErrBadQuery`) is skipped and the error is returned at once.

- [ ] **Step 7: Run the tests to verify they pass**

Run: `go test ./internal/search/... ./internal/api/handler/ && go vet -tags=integration ./internal/search/... ./internal/api/...`
Expected: PASS, vet clean.

- [ ] **Step 8: Commit**

```bash
gofmt -w internal/search/search internal/api/handler/errors.go
git add internal/search/search/federated.go internal/search/search/federated_test.go internal/search/search/testhelpers_test.go internal/search/search/client.go internal/api/handler/errors.go
git commit -m "search: run an OR query as one federated multi-search"
```

---

### Task 3: Facet counts under OR

**Files:**

- Modify: `internal/search/search/federated.go`
- Modify: `internal/search/search/facets.go` (`FacetCounts` ~line 41, `DisjunctiveFacetCounts` ~line 66)
- Test: `internal/search/search/federated_test.go`

**Interfaces:**

- Consumes: `ParseQuery` (Task 1), `fakeManager` (Task 2), `facetSearcher`, `buildFacetResult`, `maxValuesPerFacet` (existing).
- Produces: `func buildFederatedFacets(alts []Alternative, filter any, facets []string) *meilisearch.MultiSearchRequest`, `func federatedAsSearchResponse(*meilisearch.MultiSearchResponse) (*meilisearch.SearchResponse, error)`, `func (c *Client) facetSearch(ctx context.Context, query string, filter any, facets []string) (*meilisearch.SearchResponse, error)`.
- [ ] **Step 1: Write the failing tests (append to `federated_test.go`)**

```go
func TestFederatedFacetsMergeWithTheIndexValueCap(t *testing.T) {
	req := buildFederatedFacets([]Alternative{{Text: "a"}, {Text: "b", Fields: []string{"title"}}}, "x = 1", []string{"source"})
	f := req.Federation
	if f == nil || !reflect.DeepEqual(f.FacetsByIndex, map[string][]string{facetIndexUID: {"source"}}) {
		t.Fatalf("Federation = %+v, want facetsByIndex for the jobs index", f)
	}
	if f.MergeFacets == nil || f.MergeFacets.MaxValuesPerFacet != maxValuesPerFacet {
		t.Fatalf("MergeFacets = %+v, want maxValuesPerFacet %d (federation defaults to 100 and truncates)", f.MergeFacets, maxValuesPerFacet)
	}
	for i, q := range req.Queries {
		if q.Facets != nil || q.Filter != "x = 1" {
			t.Errorf("query %d = %+v: per-query facets are a 400 in a federation, and the filter must ride along", i, q)
		}
	}
	if !reflect.DeepEqual(req.Queries[1].AttributesToSearchOn, []string{"title"}) {
		t.Errorf("title: alternative lost its field scope")
	}
}

func TestFacetSearchConvertsMergedFacets(t *testing.T) {
	c := &Client{manager: fakeManager{multiFn: func(context.Context, *meilisearch.MultiSearchRequest) (*meilisearch.MultiSearchResponse, error) {
		return &meilisearch.MultiSearchResponse{
			EstimatedTotalHits: 3,
			FacetDistribution:  map[string]json.RawMessage{"source": json.RawMessage(`{"a":2,"b":1}`)},
			FacetStats:         map[string]json.RawMessage{"enrichment.salary_min": json.RawMessage(`{"min":1,"max":9}`)},
		}, nil
	}}}
	resp, err := c.facetSearch(context.Background(), "x OR y", nil, []string{"source"})
	if err != nil {
		t.Fatal(err)
	}
	fr, err := buildFacetResult(resp)
	if err != nil {
		t.Fatal(err)
	}
	if fr.Total != 3 || fr.Facets["source"]["a"] != 2 || fr.Stats["enrichment.salary_min"].Max != 9 {
		t.Fatalf("FacetResult = %+v", fr)
	}
}

func TestFacetSearchWithoutOperatorsStaysSingleIndex(t *testing.T) {
	var gotQuery string
	c := &Client{facet: fakeSearchIndex{searchFn: func(_ context.Context, q string, _ *meilisearch.SearchRequest) (*meilisearch.SearchResponse, error) {
		gotQuery = q
		return &meilisearch.SearchResponse{}, nil
	}}}
	if _, err := c.facetSearch(context.Background(), "golang", nil, []string{"source"}); err != nil {
		t.Fatal(err)
	}
	if gotQuery != "golang" {
		t.Fatalf("query = %q, want golang", gotQuery)
	}
}
```

Add `"encoding/json"` to the test file's imports.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/search/search/ -run 'FederatedFacets|FacetSearch'`
Expected: FAIL to compile, `undefined: buildFederatedFacets`.

- [ ] **Step 3: Add the facet builder and response adapter to `federated.go`**

Change the import to `import ("encoding/json"; "fmt"; "github.com/meilisearch/meilisearch-go")` in gofmt's grouped form, then append:

```go
// buildFederatedFacets is the facet-count counterpart of buildFederatedSearch:
// the same alternatives under one filter, with the facets requested from the
// federation (per-query facets are a 400 there) and merged per document.
// MaxValuesPerFacet must match the index setting: federation's own default is
// 100 and it truncates silently, the failure maxValuesPerFacet exists to avoid.
// Limit 1, not 0: the SDK omits a zero limit, which federation reads as 20 hits.
func buildFederatedFacets(alts []Alternative, filter any, facets []string) *meilisearch.MultiSearchRequest {
	queries := make([]*meilisearch.SearchRequest, 0, len(alts))
	for _, a := range alts {
		queries = append(queries, &meilisearch.SearchRequest{
			IndexUID:             facetIndexUID,
			Query:                a.Text,
			Filter:               filter,
			AttributesToSearchOn: a.Fields,
		})
	}
	fed := &meilisearch.MultiSearchFederation{Limit: 1}
	if len(facets) > 0 {
		fed.FacetsByIndex = map[string][]string{facetIndexUID: facets}
		fed.MergeFacets = &meilisearch.MultiSearchFederationMergeFacets{MaxValuesPerFacet: maxValuesPerFacet}
	}
	return &meilisearch.MultiSearchRequest{Federation: fed, Queries: queries}
}

// federatedAsSearchResponse reshapes a merged federated response into the
// single-index response buildFacetResult already decodes, so the facet paths
// share one decoder whichever way the query ran.
func federatedAsSearchResponse(resp *meilisearch.MultiSearchResponse) (*meilisearch.SearchResponse, error) {
	out := &meilisearch.SearchResponse{Hits: resp.Hits, EstimatedTotalHits: resp.EstimatedTotalHits}
	if len(resp.FacetDistribution) > 0 {
		raw, err := json.Marshal(resp.FacetDistribution)
		if err != nil {
			return nil, fmt.Errorf("search: re-encode merged facets: %w", err)
		}
		out.FacetDistribution = raw
	}
	if len(resp.FacetStats) > 0 {
		raw, err := json.Marshal(resp.FacetStats)
		if err != nil {
			return nil, fmt.Errorf("search: re-encode merged facet stats: %w", err)
		}
		out.FacetStats = raw
	}
	return out, nil
}
```

- [ ] **Step 4: Add `Client.facetSearch` and route both facet paths through it (`facets.go`)**

Add:

```go
// facetSearch runs one facet-count query, reading q's operators the same way
// Search does: a single alternative is today's single-index request, several are
// one federated request whose merged counts are per document.
func (c *Client) facetSearch(ctx context.Context, query string, filter any, facets []string) (*meilisearch.SearchResponse, error) {
	alts, err := ParseQuery(query)
	if err != nil {
		return nil, err
	}
	if len(alts) == 1 {
		return c.facet.SearchWithContext(ctx, alts[0].Text, &meilisearch.SearchRequest{
			Filter:               filter,
			Facets:               facets,
			Limit:                0,
			AttributesToSearchOn: alts[0].Fields,
		})
	}
	resp, err := c.manager.MultiSearchWithContext(ctx, buildFederatedFacets(alts, filter, facets))
	if err != nil {
		return nil, err
	}
	return federatedAsSearchResponse(resp)
}
```

Change `FacetCounts` to:

```go
func (c *Client) FacetCounts(ctx context.Context, p FacetParams) (FacetResult, error) {
	resp, err := c.facetSearch(ctx, p.Query, p.Filter, p.Facets)
	if err != nil {
		return FacetResult{}, queryErr(ctx, "search: facet query", err)
	}
	return buildFacetResult(resp)
}
```

Change `DisjunctiveFacetCounts` to:

```go
func (c *Client) DisjunctiveFacetCounts(ctx context.Context, query string, reqs []FacetReq, totalFilter any) (FacetResult, error) {
	return disjunctiveFacetCounts(ctx, query, reqs, totalFilter, c.facetSearch)
}
```

`queryErr` wraps with `%w`, so `errors.Is(err, ErrTooManyAlternatives)` still holds for the handler.

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/search/... ./internal/api/handler/`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
gofmt -w internal/search/search
git add internal/search/search/federated.go internal/search/search/federated_test.go internal/search/search/facets.go
git commit -m "search: count facets for an OR query over the merged set"
```

---

### Task 4: Real-engine test on the prod Meilisearch version

**Files:**

- Modify: `internal/search/search/search_integration_test.go` (the `startMeili` image at ~line 34; a new test at the end)

**Interfaces:**

- Consumes: `startMeili`, `enrichedJSON`, `FromJob`, `Client.Search`, `Client.FacetCounts`, `Client.DisjunctiveFacetCounts` (existing / Tasks 2–3).

- [ ] **Step 1: Pin the test engine to prod's version**

In `startMeili`, change `Image: "getmeili/meilisearch:v1.13"` to `Image: "getmeili/meilisearch:v1.49.0"` and add a comment above it: `// The version prod runs (docker-compose.yml). Federated OR search depends on engine behaviour, so the test engine must not lag it.`

Run: `go test -tags=integration ./internal/search/search/ 2>&1 | tail -20`
Expected: every existing test still passes. If an unrelated test fails only because of the version bump, stop and report it with its output rather than changing that test.

- [ ] **Step 2: Write the integration test (append)**

```go
func TestSearchOperatorsOnRealEngine(t *testing.T) {
	ctx := context.Background()
	c := startMeili(t)
	if err := c.EnsureIndex(ctx); err != nil {
		t.Fatalf("EnsureIndex: %v", err)
	}
	day := func(d int) pgtype.Timestamptz {
		return pgtype.Timestamptz{Time: time.Date(2026, 1, d, 0, 0, 0, 0, time.UTC), Valid: true}
	}
	jobs := []db.Job{
		{ID: 31, Source: "a", Title: "Product Owner", Company: "Acme", Description: "We write golang.", PublicSlug: "po-31", PostedAt: day(1)},
		{ID: 32, Source: "b", Title: "Produktchef", Company: "Beta", Description: "Work with a product owner.", PublicSlug: "pk-32", PostedAt: day(2)},
		{ID: 33, Source: "a", Title: "Junior Product Owner", Company: "Gamma", Description: "Entry level.", PublicSlug: "jpo-33", PostedAt: day(3)},
		{ID: 34, Source: "c", Title: "Rust Developer", Company: "Delta", Description: "Some golang too.", PublicSlug: "rust-34", PostedAt: day(4)},
	}
	for i := range jobs {
		jobs[i].Enrichment = enrichedJSON(t, enrich.Enrichment{})
	}
	if err := c.IndexJobs(ctx, toDocs(t, jobs)); err != nil {
		t.Fatalf("IndexJobs: %v", err)
	}
	slugs := func(res SearchResult) []string {
		out := make([]string, 0, len(res.Hits))
		for _, h := range res.Hits {
			out = append(out, h.PublicSlug)
		}
		return out
	}
	search := func(t *testing.T, p SearchParams) SearchResult {
		t.Helper()
		if p.Limit == 0 {
			p.Limit = 10
		}
		res, err := c.Search(ctx, p)
		if err != nil {
			t.Fatalf("Search(%q): %v", p.Query, err)
		}
		return res
	}

	t.Run("title: excludes a description-only match, minus excludes junior", func(t *testing.T) {
		res := search(t, SearchParams{Query: `title:"product owner" -junior`})
		if got := slugs(res); len(got) != 1 || got[0] != "po-31" {
			t.Fatalf("hits = %v, want [po-31]", got)
		}
	})

	t.Run("OR unions alternatives and returns a doc matched twice once", func(t *testing.T) {
		res := search(t, SearchParams{Query: `title:"product owner" -junior OR produktchef OR golang`, Sort: []string{"posted_at:desc"}})
		if got := slugs(res); !reflect.DeepEqual(got, []string{"rust-34", "pk-32", "po-31"}) || res.Total != 3 {
			t.Fatalf("hits = %v total %d, want [rust-34 pk-32 po-31] newest first, total 3", got, res.Total)
		}
	})

	t.Run("paging walks the merged list", func(t *testing.T) {
		res := search(t, SearchParams{Query: `produktchef OR golang`, Sort: []string{"posted_at:desc"}, Limit: 1, Offset: 1})
		if got := slugs(res); !reflect.DeepEqual(got, []string{"pk-32"}) || res.Total != 3 {
			t.Fatalf("page 2 = %v total %d, want [pk-32] of 3", got, res.Total)
		}
	})

	t.Run("facet counts are per document across alternatives", func(t *testing.T) {
		fr, err := c.FacetCounts(ctx, FacetParams{Query: `produktchef OR golang`, Facets: []string{"source"}})
		if err != nil {
			t.Fatal(err)
		}
		want := map[string]int64{"a": 1, "b": 1, "c": 1}
		if !reflect.DeepEqual(fr.Facets["source"], want) || fr.Total != 3 {
			t.Fatalf("source facet = %v total %d, want %v total 3", fr.Facets["source"], fr.Total, want)
		}
	})

	t.Run("disjunctive facets under OR", func(t *testing.T) {
		fr, err := c.DisjunctiveFacetCounts(ctx, `produktchef OR golang`, []FacetReq{{Attr: "source"}}, nil)
		if err != nil {
			t.Fatal(err)
		}
		if fr.Total != 3 || fr.Facets["source"]["c"] != 1 {
			t.Fatalf("FacetResult = %+v", fr)
		}
	})
}
```

Add `"reflect"` to the file's imports if it is not already there.

- [ ] **Step 3: Run it**

Run: `go test -tags=integration ./internal/search/search/ -run TestSearchOperatorsOnRealEngine -v`
Expected: PASS on every subtest. The ordering subtest depends on the index's `words, sort, ...` ranking rules: all three hits fully match their own alternative, so the sort decides.

- [ ] **Step 4: Commit**

```bash
gofmt -w internal/search/search/search_integration_test.go
git add internal/search/search/search_integration_test.go
git commit -m "search: pin test Meilisearch to v1.49 and test operators on it"
```

---

### Task 5: Docs

**Files:**

- Modify: `web/static/openapi.yaml` (`components.parameters.Q.description`, ~line 912)
- Modify: `internal/search/search/AGENTS.md`
- [ ] **Step 1: Replace the `Q` description in `openapi.yaml`**

```yaml
      description: |
        Full-text query over `title`, `company`, `description`, and `location`.

        Plain words: Meilisearch's default matching. The first words are
        required and trailing ones are dropped when too few postings match,
        so word order can change the result (`golang rust` and `rust golang`
        differ). It is neither a strict AND nor an OR.

        Operators:
        - `"..."` — every quoted word must match, in any order and position.
          It is not a contiguous phrase.
        - `-word` / `-"..."` — exclude postings matching it.
        - `OR` (uppercase, between words, outside quotes) — any alternative
          matches. At most 5 alternatives; more is a 400.
        - `title:` at the start of an alternative — match that alternative in
          the title only, e.g. `title:"product owner" -junior OR title:produktchef`.

        `q_fields` narrows the fields for every alternative that does not
        start with `title:`.
```

Validate it: `npx --yes @redocly/cli@latest lint web/static/openapi.yaml` (or the command the `artifacts` CI job runs; check `.github/workflows` for it). Expected: no new errors.

- [ ] **Step 2: Add a section to `internal/search/search/AGENTS.md`**

```markdown
## Query operators

`ParseQuery` (`query_syntax.go`) reads `OR` and a leading `title:` in `q`. A
query with neither is one alternative whose text is `q` byte-for-byte, so it
builds exactly the single-index request it built before operators existed;
keep it that way, since every person's search on the site goes through it.
Two or more alternatives go out as ONE federated `/multi-search`
(`federated.go`): each query carries the same filter, sort and vector, and the
federation pages, deduplicates and merges facets. Two traps: per-query
`limit`/`offset`/`facets` are a 400 inside a federation, and `mergeFacets`
defaults to 100 values per facet, so it must pass `maxValuesPerFacet`. The
merged sort is global only because the ranking rules put `sort` right after
`words`: each hit fully matches its own alternative, so the sort decides.
Moving `sort` later breaks that, and `TestSearchOperatorsOnRealEngine` catches it.
```

Run: `pnpm check:links`
Expected: all links resolve.

- [ ] **Step 3: Commit**

```bash
git add web/static/openapi.yaml internal/search/search/AGENTS.md
git commit -m "docs: document q operators and correct the plain-words claim"
```

---

### Task 6: Ship

- [ ] **Step 1: Full local gate**

Run: `gofmt -l . && go vet ./... && go vet -tags=integration ./... && go test ./... && go test -tags=integration ./internal/search/...`
Expected: no gofmt output; everything passes.

- [ ] **Step 2: PR → CI → merge → deploy**

Push the `search-query-operators` branch and open a PR. Wait for CI, then merge. Deploy with `ssh root@136.243.174.114 /opt/freehire/bin/release.sh freehire`. No migration and no reindex are needed, because the index settings do not change.

- [ ] **Step 3: Live check**

```bash
for q in '"product owner"' 'produktchef OR "product owner"' 'title:"product owner"' 'a OR b OR c OR d OR e OR f'; do
  printf '%s\t' "$q"; curl -s -G https://freehire.me/api/v1/jobs/search --data-urlencode "q=$q" --data-urlencode limit=1 | jq -c '.meta.total // .error'
done
```

Expected: the OR line is greater than 10,352 (the `"product owner"` figure); the `title:` line is smaller; the last line prints `q has more than 5 OR alternatives`.

- [ ] **Step 4: Tell the skill**

Open a PR in MadsLorentzen/ai-job-search: in `freehire-search/SKILL.md`'s query-syntax guidance, say that freehire honours `OR`, `title:`, quotes and `-`, with the live numbers from Step 3.
