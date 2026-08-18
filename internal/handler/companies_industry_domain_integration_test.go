//go:build integration

// Integration test for the widened industry facet: GET /api/v1/companies?industries=
// reaches a company through the curated companies.industries an importer wrote, and —
// only where that is empty — through the job-derived companies.domains its own
// postings imply. Only a real Postgres exercises the query that does it. Run with:
// go test -tags=integration ./internal/handler/
package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	neturl "net/url"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/industrytag"
	"github.com/strelov1/freehire/internal/search"
)

func TestListCompaniesIndustryFacetReadsBothColumns(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()

	// One fixture set, seeded into Postgres and evaluated against the Meili filter, so
	// the backend-agreement subtest below compares the two over identical data.
	fixtures := []companyFixture{
		// Curated only — the case that worked before this change.
		{slug: "curated-fin", industries: []string{"fintech"}, domains: []string{}},
		// Derived only, spelled the same in both vocabularies.
		{slug: "derived-fin", industries: []string{}, domains: []string{"fintech"}},
		// Derived only, spelled differently — the case a string comparison would miss.
		{slug: "derived-tools", industries: []string{}, domains: []string{"devtools"}},
		// Both, and disagreeing: the curated value answers and the derived one is not
		// consulted, so this company is reachable as healthcare and NOT as edtech.
		{slug: "both", industries: []string{"healthcare"}, domains: []string{"edtech"}},
		// Domains the mapping deliberately refuses, plus one retired from the
		// vocabulary altogether. None of them may produce an industry.
		{slug: "unmapped", industries: []string{}, domains: []string{"other", "saas"}},
		// Shaped like Uber on production: classified by an importer, and carrying a
		// domains union that drifted across the catalogue because it is aggregated
		// over hundreds of postings. Its curated industries answer for it; the drift
		// must not. This is the defect #2074 shipped — ?industries=gaming returned
		// Uber — so it is a fixture, not a hypothetical.
		{slug: "big-classified", industries: []string{"ai", "data-analytics", "logistics"},
			domains: []string{"adtech", "edtech", "fintech", "gamedev", "govtech", "healthcare", "travel"}},
	}
	for _, f := range fixtures {
		if _, err := pool.Exec(ctx,
			`INSERT INTO companies (slug, name, job_count, industries, domains) VALUES ($1, $1, 1, $2, $3)`,
			f.slug, f.industries, f.domains); err != nil {
			t.Fatalf("seed %q: %v", f.slug, err)
		}
	}

	h := &companiesHandlers{queries: db.New(pool)}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/api/v1/companies", h.ListCompanies)

	query := func(t *testing.T, url string) (slugs []string, total float64) {
		t.Helper()
		resp, err := app.Test(httptest.NewRequest("GET", url, nil))
		if err != nil {
			t.Fatalf("request %q: %v", url, err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != fiber.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("status = %d, want 200 (body %s)", resp.StatusCode, body)
		}
		var body struct {
			Data []struct {
				Slug string `json:"slug"`
			} `json:"data"`
			Meta struct {
				Total float64 `json:"total"`
			} `json:"meta"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		for _, c := range body.Data {
			slugs = append(slugs, c.Slug)
		}
		sort.Strings(slugs)
		return slugs, body.Meta.Total
	}

	// meta.total is asserted alongside the rows on every filtered case: the count and
	// the list are built by separate queries, so a predicate added to one and not the
	// other would leave a page contradicting its own total.
	assert := func(t *testing.T, url string, want []string) {
		t.Helper()
		got, total := query(t, url)
		sort.Strings(want)
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("%s → slugs %v, want %v", url, got, want)
		}
		if int(total) != len(want) {
			t.Errorf("%s → meta.total %v, want %d", url, total, len(want))
		}
	}

	t.Run("matches the curated column and the derived one at once", func(t *testing.T) {
		assert(t, "/api/v1/companies?industries=fintech", []string{"curated-fin", "derived-fin"})
	})

	t.Run("translates an industry into its differently-spelled domain", func(t *testing.T) {
		assert(t, "/api/v1/companies?industries=developer-tools", []string{"derived-tools"})
	})

	t.Run("the curated column wins outright when the two disagree", func(t *testing.T) {
		// `both` is curated healthcare and derives edtech. Its curated value answers…
		assert(t, "/api/v1/companies?industries=healthcare", []string{"both"})
		// …and its domains are not consulted at all, so edtech does not reach it.
		assert(t, "/api/v1/companies?industries=edtech", nil)
	})

	t.Run("several industries are OR-ed across both columns", func(t *testing.T) {
		assert(t, "/api/v1/companies?industries=fintech&industries=developer-tools",
			[]string{"curated-fin", "derived-fin", "derived-tools"})
	})

	t.Run("a deliberately unmapped domain yields no industry", func(t *testing.T) {
		// `other` is the classifier declining to answer and `saas` was retired from the
		// vocabulary; neither names an industry, so nothing may reach this company.
		for _, industry := range industrytag.Canonicals() {
			got, _ := query(t, "/api/v1/companies?industries="+industry)
			if slices.Contains(got, "unmapped") {
				t.Errorf("industries=%s matched the unmapped company", industry)
			}
		}
	})

	// The defect #2074 shipped. A company an importer has classified is answered from
	// that classification alone: its domains are an aggregate over every posting and
	// drift across the catalogue, so consulting them adds no reach and asserts
	// industries the company is not in.
	t.Run("a curated company is never matched through its domains", func(t *testing.T) {
		for _, industry := range []string{"gaming", "edtech", "government", "adtech", "healthcare", "travel"} {
			got, _ := query(t, "/api/v1/companies?industries="+industry)
			if slices.Contains(got, "big-classified") {
				t.Errorf("industries=%s matched a company curated as {ai,data-analytics,logistics}", industry)
			}
		}
		// Its own curated values still answer for it.
		for _, industry := range []string{"ai", "data-analytics", "logistics"} {
			got, _ := query(t, "/api/v1/companies?industries="+industry)
			if !slices.Contains(got, "big-classified") {
				t.Errorf("industries=%s should still match its curated company, got %v", industry, got)
			}
		}
	})

	t.Run("the domains facet still filters the raw value", func(t *testing.T) {
		assert(t, "/api/v1/companies?domains=other", []string{"unmapped"})
		// Including a value the industry vocabulary has no name for at all.
		assert(t, "/api/v1/companies?domains=saas", []string{"unmapped"})
	})

	t.Run("industries ANDs with another facet rather than widening it", func(t *testing.T) {
		assert(t, "/api/v1/companies?industries=fintech&domains=fintech", []string{"derived-fin"})
	})

	// The two backends resolve this facet in separate code, and a request can be
	// served by either — a rating sort forces Postgres, a Meili error falls back to
	// it, and within one page the rows and meta.total come from separate queries. So
	// the risk this change most plausibly produces is the two drifting apart, which
	// would show up as a page contradicting its own total. Assert the agreement
	// directly rather than trusting each side's own tests.
	t.Run("the Meilisearch path matches the same companies", func(t *testing.T) {
		for _, industry := range []string{
			"fintech", "developer-tools", "healthcare", "edtech",
			// Ones the mapping refuses, where agreeing on "nothing" is the point.
			"entertainment", "automotive", "transportation",
		} {
			url := "/api/v1/companies?industries=" + industry
			viaPostgres, _ := query(t, url)

			viaMeili := slugsMatchingMeiliFilter(t,
				search.CompanyFilterFromValues(neturl.Values{"industries": {industry}}), fixtures)

			if strings.Join(viaPostgres, ",") != strings.Join(viaMeili, ",") {
				t.Errorf("industries=%s → postgres %v, meili %v", industry, viaPostgres, viaMeili)
			}
		}
	})
}

// companyFixture is one seeded company as the Meilisearch document would carry it:
// the two array attributes this facet reads.
type companyFixture struct {
	slug       string
	industries []string
	domains    []string
}

// slugsMatchingMeiliFilter applies a filter built by search.CompanyFilterFromValues
// to the fixtures in memory and returns the matching slugs, sorted.
//
// This evaluates the filter rather than running Meilisearch: the question is whether
// the two backends encode the same RULE, and standing up an index would test
// Meilisearch's own filter engine instead. The evaluator understands only the shape
// this filter uses — groups ANDed, fragments ORed within a group, each fragment
// `attr = "value"` against an array attribute — and fails the test on anything else,
// so it cannot quietly pass a filter it does not understand.
func slugsMatchingMeiliFilter(t *testing.T, filter any, fixtures []companyFixture) []string {
	t.Helper()

	var groups [][]string
	if filter != nil {
		g, ok := filter.([][]string)
		if !ok {
			t.Fatalf("filter is %T, want [][]string", filter)
		}
		groups = g
	}

	var out []string
	for _, f := range fixtures {
		attrs := map[string][]string{"industries": f.industries, "domains": f.domains}
		matchesAll := true
		for _, group := range groups {
			matched := false
			for _, fragment := range group {
				if evalFragment(t, fragment, attrs) {
					matched = true
					break
				}
			}
			if !matched {
				matchesAll = false
				break
			}
		}
		if matchesAll {
			out = append(out, f.slug)
		}
	}
	sort.Strings(out)
	return out
}

// evalFragment evaluates one Meilisearch filter fragment against a document's array
// attributes. It models exactly two shapes — `attr = "value"` and the parenthesised
// conjunction the industry facet's derived arm builds — and fails the test on
// anything else, so a filter this evaluator does not understand can never be
// mistaken for one that matched nothing.
func evalFragment(t *testing.T, fragment string, attrs map[string][]string) bool {
	t.Helper()

	// A parenthesised conjunction: every conjunct must hold. The nested
	// `(attr IS EMPTY OR attr IS NULL)` is handled as one conjunct below, so split on
	// the AND that separates it from the equality rather than on every AND.
	if strings.HasPrefix(fragment, "((") {
		unset, rest, ok := strings.Cut(strings.TrimSuffix(strings.TrimPrefix(fragment, "("), ")"), " AND ")
		if !ok {
			t.Fatalf("fragment %q is not `(<unset> AND <eq>)`", fragment)
		}
		return evalUnset(t, unset, attrs) && evalFragment(t, rest, attrs)
	}

	attr, value, ok := strings.Cut(fragment, " = ")
	if !ok {
		t.Fatalf("fragment %q is not `attr = value`", fragment)
	}
	values, known := attrs[attr]
	if !known {
		t.Fatalf("fragment %q names an attribute this evaluator does not model", fragment)
	}
	return slices.Contains(values, strings.Trim(value, `"`))
}

// evalUnset evaluates `(attr IS EMPTY OR attr IS NULL)`. Postgres has one way to say
// "nothing here" for a NOT NULL array column and Meilisearch has two, which is why
// the filter tests both; either satisfies this.
func evalUnset(t *testing.T, fragment string, attrs map[string][]string) bool {
	t.Helper()

	attr, _, ok := strings.Cut(strings.TrimPrefix(fragment, "("), " IS EMPTY")
	if !ok {
		t.Fatalf("fragment %q is not `(attr IS EMPTY OR attr IS NULL)`", fragment)
	}
	values, known := attrs[attr]
	if !known {
		t.Fatalf("fragment %q names an attribute this evaluator does not model", fragment)
	}
	return len(values) == 0
}
