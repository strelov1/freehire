//go:build integration

package search

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/platform/db"
)

func TestEquityRemovedFromIndexAfterDescriptionEdit(t *testing.T) {
	ctx := context.Background()
	c := startMeili(t)
	if err := c.EnsureIndex(ctx); err != nil {
		t.Fatal(err)
	}
	row := db.Job{ID: 1, Source: "greenhouse", ExternalID: "acme:1", Title: "Engineer", Company: "Acme", PublicSlug: "engineer-acme", Description: "Salary plus equity", Equity: pgtype.Text{String: "salary_and_equity", Valid: true}}
	for _, offered := range []bool{true, false} {
		if !offered {
			row.Description = "Build public web applications."
			row.Equity = pgtype.Text{}
		}
		doc, err := FromJob(row, nil, "ats")
		if err != nil {
			t.Fatal(err)
		}
		if err := c.IndexJobs(ctx, []JobDocument{doc}); err != nil {
			t.Fatal(err)
		}
		for _, value := range []string{"salary_and_equity", "not_stated"} {
			result, err := c.Search(ctx, SearchParams{Filter: FilterFromValues(map[string][]string{"equity": {value}}), Limit: 10})
			if err != nil {
				t.Fatal(err)
			}
			want := 0
			if (value == "salary_and_equity") == offered {
				want = 1
			}
			if len(result.Hits) != want {
				t.Errorf("offered=%v value=%s: hits=%d want=%d", offered, value, len(result.Hits), want)
			}
		}
	}
}
