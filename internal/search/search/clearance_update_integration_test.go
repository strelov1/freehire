//go:build integration

package search

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/platform/db"
)

func TestClearanceRemovedFromIndexAfterDescriptionEdit(t *testing.T) {
	ctx := context.Background()
	c := startMeili(t)
	if err := c.EnsureIndex(ctx); err != nil {
		t.Fatal(err)
	}
	row := db.Job{ID: 1, Source: "greenhouse", ExternalID: "acme:1", Title: "Engineer", Company: "Acme", PublicSlug: "engineer-acme", Description: "Secret clearance required", RequiresClearance: pgtype.Bool{Bool: true, Valid: true}, ClearanceLevels: []string{"secret"}}
	for _, cleared := range []bool{true, false} {
		if !cleared {
			row.Description = "Build public web applications."
			row.RequiresClearance = pgtype.Bool{}
			row.ClearanceLevels = nil
		}
		doc, err := FromJob(row, nil, "ats")
		if err != nil {
			t.Fatal(err)
		}
		if err := c.IndexJobs(ctx, []JobDocument{doc}); err != nil {
			t.Fatal(err)
		}
		for _, level := range []string{"secret", "none"} {
			result, err := c.Search(ctx, SearchParams{Filter: FilterFromValues(map[string][]string{"clearance_levels": {level}}), Limit: 10})
			if err != nil {
				t.Fatal(err)
			}
			want := 0
			if (level == "secret") == cleared {
				want = 1
			}
			if len(result.Hits) != want {
				t.Errorf("cleared=%v level=%s: hits=%d want=%d", cleared, level, len(result.Hits), want)
			}
		}
	}
}
