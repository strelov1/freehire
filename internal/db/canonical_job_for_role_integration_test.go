//go:build integration

// Integration tests for the intake's synchronous dedup lookup: which posting, if any, a
// freshly imported storefront row should be marked a duplicate of. Both the canon choice and
// the exclusion of the row being written are SQL behaviours, verifiable only against a real
// Postgres.
// Run with: go test -tags=integration ./internal/db/
package db

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// fp wraps a role fingerprint for the query parameter, which is nullable in the column and so
// generated as pgtype.Text.
func fp(s string) pgtype.Text { return pgtype.Text{String: s, Valid: true} }

func TestCanonicalJobForRole(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	q := New(pool)

	seed := func(t *testing.T, source, externalID, slug, fingerprint string, closed bool) int64 {
		t.Helper()
		var id int64
		err := pool.QueryRow(ctx, `
			INSERT INTO jobs (source, external_id, url, title, public_slug, company, company_slug,
			                  role_fingerprint, closed_at)
			VALUES ($1, $2, 'https://x.test/'||$2, 'Senior Go Engineer', $3, 'Acme', 'acme', $4,
			        CASE WHEN $5::bool THEN now() ELSE NULL END)
			RETURNING id`, source, externalID, slug, fingerprint, closed).Scan(&id)
		if err != nil {
			t.Fatalf("seed %s: %v", externalID, err)
		}
		return id
	}

	canonID := seed(t, "greenhouse", "acme:1", "senior-go-acme-1", "fp-go", false)

	t.Run("finds the open canonical posting of the role", func(t *testing.T) {
		row, err := q.CanonicalJobForRole(ctx, CanonicalJobForRoleParams{
			CompanySlug: "acme", RoleFingerprint: fp("fp-go"),
			Source: "weblink", ExternalID: "https://storefront.test/go",
		})
		if err != nil {
			t.Fatalf("CanonicalJobForRole: %v", err)
		}
		if row.ID != canonID || row.PublicSlug != "senior-go-acme-1" {
			t.Errorf("= (%d, %q), want (%d, senior-go-acme-1)", row.ID, row.PublicSlug, canonID)
		}
	})

	t.Run("ignores the row being imported", func(t *testing.T) {
		// A re-import of the same URL must not find itself and become its own duplicate.
		seed(t, "weblink", "https://storefront.test/go", "senior-go-acme-2", "fp-go", false)
		row, err := q.CanonicalJobForRole(ctx, CanonicalJobForRoleParams{
			CompanySlug: "acme", RoleFingerprint: fp("fp-go"),
			Source: "weblink", ExternalID: "https://storefront.test/go",
		})
		if err != nil {
			t.Fatalf("CanonicalJobForRole: %v", err)
		}
		if row.ID != canonID {
			t.Errorf("canon = %d, want %d — the row being written must be excluded", row.ID, canonID)
		}
	})

	t.Run("the oldest eligible posting wins", func(t *testing.T) {
		// The canon choice has to agree with RecomputeRoleDuplicatesForCompanies, which takes the
		// cluster's MIN(id). A later-seeded, equally eligible row must not win.
		seed(t, "lever", "acme:2", "senior-go-acme-3", "fp-go", false)
		row, err := q.CanonicalJobForRole(ctx, CanonicalJobForRoleParams{
			CompanySlug: "acme", RoleFingerprint: fp("fp-go"),
			Source: "weblink", ExternalID: "https://storefront.test/go",
		})
		if err != nil {
			t.Fatalf("CanonicalJobForRole: %v", err)
		}
		if row.ID != canonID {
			t.Errorf("canon = %d, want the oldest eligible row %d", row.ID, canonID)
		}
	})

	t.Run("a closed posting is no canon", func(t *testing.T) {
		seed(t, "greenhouse", "acme:9", "closed-role-acme", "fp-closed", true)
		_, err := q.CanonicalJobForRole(ctx, CanonicalJobForRoleParams{
			CompanySlug: "acme", RoleFingerprint: fp("fp-closed"),
			Source: "weblink", ExternalID: "https://storefront.test/closed",
		})
		if !errors.Is(err, pgx.ErrNoRows) {
			t.Errorf("err = %v, want pgx.ErrNoRows", err)
		}
	})

	t.Run("a posting that is itself a duplicate is no canon", func(t *testing.T) {
		// Marking against a duplicate would build a chain (A -> B -> C) that no reader expects.
		anchor := seed(t, "greenhouse", "acme:20", "anchor-role-acme", "fp-chain-anchor", false)
		demoted := seed(t, "greenhouse", "acme:21", "demoted-role-acme", "fp-chain", false)
		if _, err := pool.Exec(ctx,
			`UPDATE jobs SET duplicate_of_role = $1 WHERE id = $2`, anchor, demoted); err != nil {
			t.Fatalf("demote the candidate: %v", err)
		}
		_, err := q.CanonicalJobForRole(ctx, CanonicalJobForRoleParams{
			CompanySlug: "acme", RoleFingerprint: fp("fp-chain"),
			Source: "weblink", ExternalID: "https://storefront.test/chain",
		})
		if !errors.Is(err, pgx.ErrNoRows) {
			t.Errorf("err = %v, want pgx.ErrNoRows", err)
		}
	})
}

func TestMarkJobDuplicateOfRole(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	q := New(pool)

	var canonID, dupID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO jobs (source, external_id, url, title, public_slug, company_slug)
		VALUES ('greenhouse', 'acme:1', 'https://x.test/1', 'Senior Go', 'canon-slug', 'acme')
		RETURNING id`).Scan(&canonID); err != nil {
		t.Fatalf("seed canon: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO jobs (source, external_id, url, title, public_slug, company_slug)
		VALUES ('weblink', 'https://x.test/store', 'https://x.test/store', 'Senior Go', 'dup-slug', 'acme')
		RETURNING id`).Scan(&dupID); err != nil {
		t.Fatalf("seed the row to demote: %v", err)
	}

	n, err := q.MarkJobDuplicateOfRole(ctx, MarkJobDuplicateOfRoleParams{
		ID:              dupID,
		DuplicateOfRole: pgtype.Int8{Int64: canonID, Valid: true},
	})
	if err != nil || n != 1 {
		t.Fatalf("MarkJobDuplicateOf = (%d, %v), want (1, nil)", n, err)
	}
	var got int64
	if err := pool.QueryRow(ctx, `SELECT duplicate_of FROM jobs WHERE id = $1`, dupID).Scan(&got); err != nil {
		t.Fatalf("read back: %v", err)
	}
	if got != canonID {
		t.Errorf("duplicate_of = %d, want %d", got, canonID)
	}
}
