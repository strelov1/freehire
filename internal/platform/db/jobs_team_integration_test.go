//go:build integration

package db

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// team is the ATS's department label. It reaches jobs_read, an unchanged re-crawl takes the
// cheap write, and an edited team falls through to the full upsert, which reports `changed`
// although the content hash did not move — otherwise the edit would never reach the index.
func TestJobTeamPersistsAndPropagates(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	p := ingestParams("acme:team", "Engineer")
	p.ContentHash = pgtype.Text{String: "h1", Valid: true}
	p.Team = "Platform"
	first, err := q.UpsertJob(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	row, err := q.GetJob(ctx, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if row.Team != "Platform" {
		t.Fatalf("jobs_read team = %q, want Platform", row.Team)
	}

	refresh := RefreshUnchangedJobParams{Source: p.Source, ExternalID: p.ExternalID, ContentHash: p.ContentHash, Cities: p.Cities, Team: "Platform"}
	if _, err := q.RefreshUnchangedJob(ctx, refresh); err != nil {
		t.Fatalf("unchanged re-crawl missed the cheap write: %v", err)
	}
	refresh.Team = "Infrastructure"
	if _, err := q.RefreshUnchangedJob(ctx, refresh); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("edited team took the cheap write: err = %v", err)
	}

	p.Team = "Infrastructure"
	second, err := q.UpsertJob(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	if !second.Changed {
		t.Error("an edited team with an unchanged hash was not reported changed")
	}
	if row, _ = q.GetJob(ctx, first.ID); row.Team != "Infrastructure" {
		t.Errorf("team after edit = %q, want Infrastructure", row.Team)
	}

	p.Team = ""
	if _, err := q.UpsertJob(ctx, p); err != nil {
		t.Fatal(err)
	}
	if row, _ = q.GetJob(ctx, first.ID); row.Team != "" {
		t.Errorf("a source that stopped stating a team kept %q", row.Team)
	}
}
