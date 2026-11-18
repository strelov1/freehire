//go:build integration

// Integration tests for the public_slug write/read path: UpsertJob persists the
// caller-computed slug, GetJobBySlug resolves it back to the row, and the slug
// stays put across a re-ingest of the same (source, external_id). SQL behavior,
// so verified against a real Postgres. Run with: go test -tags=integration ./internal/platform/db/
package db

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestJobPublicSlug(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	// Literal rather than normalize.JobSlug(...): this package is the bottom of the
	// dependency graph and must not reach up into the facet dictionaries, and a test that
	// recomputes the slug with the production function asserts nothing about its format.
	const slug = "senior-go-developer-acme-t35nijto"
	upsert := func(description string) (Job, error) {
		return ingestUpsert(ctx, q, UpsertJobParams{
			Source:      "manual",
			ExternalID:  "42",
			URL:         "http://example.test/42",
			Title:       "Senior Go Developer",
			Company:     "Acme",
			CompanySlug: "acme",
			Description: description,
			PublicSlug:  slug,
		})
	}

	job, err := upsert("first")
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if job.PublicSlug != slug {
		t.Errorf("PublicSlug = %q, want %q", job.PublicSlug, slug)
	}

	got, err := q.GetJobBySlug(ctx, slug)
	if err != nil {
		t.Fatalf("GetJobBySlug: %v", err)
	}
	if got.ID != job.ID {
		t.Errorf("GetJobBySlug id = %d, want %d", got.ID, job.ID)
	}

	// Re-ingest the same (source, external_id) with an edited description: the
	// slug is stable, so it must update the same row, not create a second one.
	job2, err := upsert("edited description")
	if err != nil {
		t.Fatalf("re-upsert: %v", err)
	}
	if job2.ID != job.ID {
		t.Errorf("re-upsert created a new row: id %d != %d", job2.ID, job.ID)
	}
	var n int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM jobs").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("job rows = %d, want 1", n)
	}

	if _, err := q.GetJobBySlug(ctx, "no-such-slug"); !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("GetJobBySlug(unknown) error = %v, want pgx.ErrNoRows", err)
	}
}

// TestGetJobIDBySlug covers the slim id-only lookup used by the view/apply
// interaction path: it resolves a known slug to the internal id and surfaces an
// unknown slug as pgx.ErrNoRows (which the handler maps to 404).
func TestGetJobIDBySlug(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	const slug = "staff-engineer-globex-vcm4o3gv"
	job, err := ingestUpsert(ctx, q, UpsertJobParams{
		Source:      "manual",
		ExternalID:  "7",
		URL:         "http://example.test/7",
		Title:       "Staff Engineer",
		Company:     "Globex",
		CompanySlug: "globex",
		PublicSlug:  slug,
	})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}

	id, err := q.GetJobIDBySlug(ctx, GetJobIDBySlugParams{PublicSlug: slug})
	if err != nil {
		t.Fatalf("GetJobIDBySlug: %v", err)
	}
	if id != job.ID {
		t.Errorf("GetJobIDBySlug = %d, want %d", id, job.ID)
	}

	if _, err := q.GetJobIDBySlug(ctx, GetJobIDBySlugParams{PublicSlug: "no-such-slug"}); !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("GetJobIDBySlug(unknown) error = %v, want pgx.ErrNoRows", err)
	}
}
