//go:build integration

package main

import (
	"context"
	"reflect"
	"testing"

	"github.com/strelov1/freehire/internal/ingest/boardcatalog"
	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

func TestRegisterSearchesCreatesPendingSlicesAndPreservesExistingConfiguration(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	repo := boardcatalog.NewQueriesRepository(db.New(pool))
	registry := sources.Taxonomy()
	old, err := boardcatalog.NewInserter(repo, registry).Insert(ctx, boardcatalog.InsertInput{
		Provider: "linkedin", Board: "software engineer", Region: "Germany",
		Company: "Operator search label", Tenants: map[string]string{"pages": "77", "jobage": "30"},
	}, boardcatalog.StatusActive)
	if err != nil {
		t.Fatal(err)
	}
	inputs, err := registrationPlan([]query{
		{Keywords: "Software Engineer", Location: "germany", Pages: 3, JobAge: 7},
		{Keywords: "data engineer", Location: "Poland", Pages: 2, JobAge: 14},
	})
	if err != nil {
		t.Fatal(err)
	}
	st, err := registerSearches(ctx, boardcatalog.NewInserter(repo, registry), inputs)
	if err != nil || st != (registrationStats{Planned: 2, Added: 1, AlreadyKnown: 1}) {
		t.Fatalf("first registration=%+v, %v", st, err)
	}
	rows, err := repo.ListActiveForProvider(ctx, "linkedin")
	if err != nil || len(rows) != 2 {
		t.Fatalf("catalogue rows=%+v, %v", rows, err)
	}
	for _, b := range rows {
		if b.ID == old.ID {
			if b.Company != old.Company || b.Status != boardcatalog.StatusActive || b.Region != "Germany" || !reflect.DeepEqual(b.Tenants, old.Tenants) {
				t.Fatalf("operator configuration changed: %+v", b)
			}
		} else if b.Board != "data engineer" || b.Region != "Poland" || b.Company != "LinkedIn Poland data engineer" || b.Status != boardcatalog.StatusPending || !reflect.DeepEqual(b.Tenants, map[string]string{"pages": "2", "jobage": "14"}) {
			t.Fatalf("new search slice=%+v", b)
		}
	}
	st, err = registerSearches(ctx, boardcatalog.NewInserter(repo, registry), inputs)
	if err != nil || st != (registrationStats{Planned: 2, AlreadyKnown: 2}) {
		t.Fatalf("repeat registration=%+v, %v", st, err)
	}
	var jobs int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM jobs").Scan(&jobs); err != nil || jobs != 0 {
		t.Fatalf("registering searches must not publish jobs: count=%d, %v", jobs, err)
	}
}
