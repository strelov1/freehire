//go:build integration

// Integration test for the shape of the schema around account deletion. Only a real
// Postgres carrying the migrations can answer it. Run with:
// go test -tags=integration ./internal/db/
// Requires Docker (testcontainers spins up a throwaway Postgres with the migrations).
package db

import (
	"context"
	"testing"
)

// Every foreign key pointing at users must be indexed on the referencing side.
//
// Postgres indexes the referenced side only, so an unindexed reference turns each
// DELETE FROM users into a sequential scan of the referencing table — one per
// constraint. That is what made account deletion outlive the proxy's read timeout in
// production: jobs.created_by and jobs.updated_by were unindexed, and the catalogue is
// the largest table there is. A member's exit must not get slower as the site grows,
// so this holds for every referencing table, not just the ones that are big today.
func TestEveryUserForeignKeyIsIndexed(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()

	rows, err := pool.Query(ctx, `
		SELECT c.conrelid::regclass::text, a.attname
		FROM pg_constraint c
		JOIN pg_attribute a ON a.attrelid = c.conrelid AND a.attnum = c.conkey[1]
		WHERE c.contype = 'f'
		  AND c.confrelid = 'public.users'::regclass
		  AND NOT EXISTS (
			  SELECT 1 FROM pg_index i
			  WHERE i.indrelid = c.conrelid AND i.indkey[0] = a.attnum
		  )
		ORDER BY 1, 2`)
	if err != nil {
		t.Fatalf("query unindexed references: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		var table, column string
		if err := rows.Scan(&table, &column); err != nil {
			t.Fatalf("scan: %v", err)
		}
		t.Errorf("%s.%s references users without an index: deleting an account scans the whole table", table, column)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate: %v", err)
	}
}
