//go:build integration

package db

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestAssistantSearchContextIsDurableAndOwnerScoped(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	owner := seedAssistantUser(t, pool, "shortlist-owner@example.test")
	stranger := seedAssistantUser(t, pool, "shortlist-stranger@example.test")
	id := uuid.New()
	created, err := q.CreateAssistantSearchSession(ctx, CreateAssistantSearchSessionParams{ID: id, UserID: owner, SearchQuery: pgtype.Text{String: "salary_min=80000&skills=go", Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	if !created.SearchQuery.Valid || created.SearchQuery.String != "salary_min=80000&skills=go" {
		t.Fatalf("lost search: %+v", created)
	}
	if n, err := q.UpdateAssistantSelectionBrief(ctx, UpdateAssistantSelectionBriefParams{ID: id, UserID: stranger, SelectionBrief: "Changed by stranger"}); err != nil || n != 0 {
		t.Fatalf("foreign write = %d, %v", n, err)
	}
	if n, err := q.UpdateAssistantSelectionBrief(ctx, UpdateAssistantSelectionBriefParams{ID: id, UserID: owner, SelectionBrief: "Prefer individual contributor roles"}); err != nil || n != 1 {
		t.Fatalf("owner write = %d, %v", n, err)
	}
	restored, err := q.GetAssistantSession(ctx, GetAssistantSessionParams{ID: id, UserID: owner})
	if err != nil {
		t.Fatal(err)
	}
	if restored.SearchQuery.String != created.SearchQuery.String || restored.SelectionBrief != "Prefer individual contributor roles" {
		t.Fatalf("restored = %+v", restored)
	}
	if _, err := q.GetAssistantSession(ctx, GetAssistantSessionParams{ID: id, UserID: stranger}); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("foreign read = %v", err)
	}
	if _, err := q.CreateAssistantSearchSession(ctx, CreateAssistantSearchSessionParams{ID: id, UserID: owner, SearchQuery: pgtype.Text{String: "", Valid: true}}); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("duplicate request = %v", err)
	}
}
