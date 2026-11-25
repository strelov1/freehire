//go:build integration

// Integration tests for the employee_count-authoritative company_sizes hybrid:
// RefreshCompanyFacets derives company_sizes from the company's stored employee_count
// (bucketed into the company_size vocabulary) when it is known — a recorded fact more
// accurate than the LLM's per-posting guess — and falls back to the bucket most of its
// open jobs' enrichment.company_size name otherwise. Run with: go test -tags=integration ./internal/platform/db/
package db

import (
	"context"
	"fmt"
	"slices"
	"testing"
)

func TestRefreshCompanySizesFromHeadcount(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	if _, err := pool.Exec(ctx, "TRUNCATE companies, jobs RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}

	// employee_count known → authoritative bucket, overriding the noisy LLM union.
	insertCompany(t, pool, "hc", "Headcount Co")
	setCompanySignals(t, pool, "hc", "", 320, 0, nil)
	insertJobWithFacets(t, pool, "hc:1", "hc", []string{}, []string{}, `{"company_size":"11-50"}`)
	insertJobWithFacets(t, pool, "hc:2", "hc", []string{}, []string{}, `{"company_size":"51-200"}`)
	// no employee_count → fall back to the bucket the postings name.
	insertCompany(t, pool, "nohc", "No Headcount Co")
	insertJobWithFacets(t, pool, "nohc:1", "nohc", []string{}, []string{}, `{"company_size":"11-50"}`)
	// no employee_count, conflicting guesses → the bucket most postings name, not every one.
	insertCompany(t, pool, "big", "Big Retailer")
	for i, size := range []string{"1000+", "1000+", "1000+", "1-10", "11-50"} {
		insertJobWithFacets(t, pool, fmt.Sprintf("big:%d", i), "big", []string{}, []string{}, `{"company_size":"`+size+`"}`)
	}
	// a tie goes to the larger bucket.
	insertCompany(t, pool, "tie", "Tie Co")
	insertJobWithFacets(t, pool, "tie:1", "tie", []string{}, []string{}, `{"company_size":"11-50"}`)
	insertJobWithFacets(t, pool, "tie:2", "tie", []string{}, []string{}, `{"company_size":"201-500"}`)
	// tiny headcount, no jobs → still gets the bucket (a company fact).
	insertCompany(t, pool, "tiny", "Tiny Co")
	setCompanySignals(t, pool, "tiny", "", 5, 0, nil)
	// headcount but zero open jobs → bucket, not empty.
	insertCompany(t, pool, "hcnojobs", "Headcount No Jobs")
	setCompanySignals(t, pool, "hcnojobs", "", 800, 0, nil)

	if _, err := q.refreshCompanyFacets(ctx, refreshCompanyFacetsParams{}); err != nil {
		t.Fatalf("refresh: %v", err)
	}

	cases := []struct {
		slug string
		want []string
	}{
		{"hc", []string{"201-500"}},        // bucket wins over the {11-50,51-200} union
		{"nohc", []string{"11-50"}},        // fallback to the postings' bucket
		{"big", []string{"1000+"}},         // the most named bucket, not the union
		{"tie", []string{"201-500"}},       // a tie goes to the larger bucket
		{"tiny", []string{"1-10"}},         // <=10
		{"hcnojobs", []string{"501-1000"}}, // bucket even with no jobs
	}
	for _, c := range cases {
		got := companyTextArray(t, pool, c.slug, "company_sizes")
		if !slices.Equal(got, c.want) {
			t.Errorf("%s company_sizes = %v, want %v", c.slug, got, c.want)
		}
	}
}
