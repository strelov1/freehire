//go:build integration

// Integration tests for the moderator-authored write path's SQL contract: the upsert
// dedups on (source, URL) and records authorship, a manual job enqueues for enrichment,
// and the edit query is scoped to created_by IS NOT NULL so it can only touch a
// moderator-authored posting, never an automated source.
// Run with: go test -tags=integration ./internal/db/
package db

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
)

func manualParams(url, title string, createdBy, updatedBy int64) UpsertManualJobParams {
	return UpsertManualJobParams{
		Source:      "manual",
		ExternalID:  url,
		URL:         url,
		Title:       title,
		Company:     "Acme",
		CompanySlug: "acme",
		// Minted once from the identity (not the title), so a re-create with an edited
		// title keeps the same public slug — mirroring how the service derives it.
		PublicSlug:  "manual-" + url,
		Location:    "Remote",
		Remote:      true,
		Description: "Build things.",
		CreatedBy:   createdBy,
		UpdatedBy:   updatedBy,
	}
}

func TestUpsertManualJobIdempotentOnURLAndAudit(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	const url = "https://acme.example/jobs/1"
	author := insertUser(t, pool, "author@example.test")
	editor := insertUser(t, pool, "editor@example.test")

	first, err := q.UpsertManualJob(ctx, manualParams(url, "Old Title", author, author))
	if err != nil {
		t.Fatalf("initial upsert: %v", err)
	}
	if first.Source != "manual" {
		t.Errorf("Source = %q, want manual", first.Source)
	}
	if !first.CreatedBy.Valid || first.CreatedBy.Int64 != author {
		t.Errorf("CreatedBy = %+v, want %d", first.CreatedBy, author)
	}
	if first.UpdatedBy.Valid {
		t.Errorf("UpdatedBy = %+v, want NULL on insert", first.UpdatedBy)
	}

	// A newly created manual job enqueues for enrichment like any other source.
	n, err := q.EnqueueJobEnrichment(ctx, EnqueueJobEnrichmentParams{TargetVersion: 1, JobID: first.ID})
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if n != 1 {
		t.Errorf("enqueue affected %d rows, want 1", n)
	}

	// Re-create the same URL with an edited title and a different acting user.
	second, err := q.UpsertManualJob(ctx, manualParams(url, "New Title", author, editor))
	if err != nil {
		t.Fatalf("re-upsert: %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("re-create made a new row (id %d != %d) — dedup on URL broken", second.ID, first.ID)
	}
	if second.Title != "New Title" {
		t.Errorf("Title = %q, want the re-created value", second.Title)
	}
	if second.PublicSlug != first.PublicSlug {
		t.Errorf("PublicSlug changed on re-create: %q -> %q", first.PublicSlug, second.PublicSlug)
	}
	if !second.CreatedBy.Valid || second.CreatedBy.Int64 != author {
		t.Errorf("CreatedBy = %+v, want unchanged %d", second.CreatedBy, author)
	}
	if !second.UpdatedBy.Valid || second.UpdatedBy.Int64 != editor {
		t.Errorf("UpdatedBy = %+v, want %d (set on conflict)", second.UpdatedBy, editor)
	}
}

func updateManualParams(slug, title string, updatedBy int64) UpdateManualJobParams {
	return UpdateManualJobParams{
		PublicSlug:  slug,
		Title:       title,
		Company:     "Acme",
		CompanySlug: "acme",
		Location:    "Remote",
		Remote:      true,
		Description: "Build things.",
		UpdatedBy:   updatedBy,
	}
}

func TestUpdateManualJobScopedByCreatedBy(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	editor := insertUser(t, pool, "editor@example.test")

	const url = "https://acme.example/jobs/2"
	manual, err := q.UpsertManualJob(ctx, manualParams(url, "Manual Job", editor, editor))
	if err != nil {
		t.Fatalf("create manual: %v", err)
	}

	// Editing the manual job succeeds and records the editor.
	updated, err := q.UpdateManualJob(ctx, updateManualParams(manual.PublicSlug, "Edited Title", editor))
	if err != nil {
		t.Fatalf("update manual: %v", err)
	}
	if updated.ID != manual.ID || updated.Title != "Edited Title" {
		t.Errorf("update = id %d title %q, want id %d title Edited Title", updated.ID, updated.Title, manual.ID)
	}
	if !updated.UpdatedBy.Valid || updated.UpdatedBy.Int64 != editor {
		t.Errorf("UpdatedBy = %+v, want %d", updated.UpdatedBy, editor)
	}

	// An automated-source (ingested) job has created_by NULL, so it is invisible to the
	// moderator update — no row matches the created_by IS NOT NULL scope, and the moderator
	// path can never rewrite an ATS vacancy.
	ats, err := ingestUpsert(ctx, q, ingestParams("gh:1", "ATS Job"))
	if err != nil {
		t.Fatalf("create ats: %v", err)
	}
	if _, err := q.UpdateManualJob(ctx, updateManualParams(ats.PublicSlug, "Hijacked", editor)); !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("update of an automated-source job: err = %v, want pgx.ErrNoRows", err)
	}
}
