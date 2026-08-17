//go:build integration

// Integration test for the widened industry facet: GET /api/v1/companies?industries=
// must reach a company through the curated companies.industries an importer wrote OR
// through the job-derived companies.domains its own postings imply. Only a real
// Postgres exercises the query that does it. Run with:
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
		// Both, and disagreeing: it must appear under either industry, not be excluded
		// for failing to match on one of them.
		{slug: "both", industries: []string{"healthcare"}, domains: []string{"edtech"}},
		// Domains the mapping deliberately refuses, plus one retired from the
		// vocabulary altogether. None of them may produce an industry.
		{slug: "unmapped", industries: []string{}, domains: []string{"other", "media", "mobility", "saas"}},
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

	t.Run("either column alone is enough when the two disagree", func(t *testing.T) {
		assert(t, "/api/v1/companies?industries=healthcare", []string{"both"})
		assert(t, "/api/v1/companies?industries=edtech", []string{"both"})
	})

	t.Run("several industries are OR-ed across both columns", func(t *testing.T) {
		assert(t, "/api/v1/companies?industries=fintech&industries=developer-tools",
			[]string{"curated-fin", "derived-fin", "derived-tools"})
	})

	t.Run("a deliberately unmapped domain yields no industry", func(t *testing.T) {
		// entertainment/automotive are the nearest curated values to the media and
		// mobility domains this company carries; neither may reach it.
		for _, industry := range []string{"entertainment", "automotive", "transportation"} {
			assert(t, "/api/v1/companies?industries="+industry, nil)
		}
	})

	t.Run("the domains facet still filters the raw value", func(t *testing.T) {
		assert(t, "/api/v1/companies?domains=other", []string{"unmapped"})
		assert(t, "/api/v1/companies?domains=media", []string{"unmapped"})
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
				attr, value, ok := strings.Cut(fragment, " = ")
				if !ok {
					t.Fatalf("fragment %q is not `attr = value`", fragment)
				}
				values, known := attrs[attr]
				if !known {
					t.Fatalf("fragment %q names an attribute this evaluator does not model", fragment)
				}
				if slices.Contains(values, strings.Trim(value, `"`)) {
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
