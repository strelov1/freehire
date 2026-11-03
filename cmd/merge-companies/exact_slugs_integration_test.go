//go:build integration

package main

import (
	"context"
	"testing"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

// --exact-slugs plans over the named slugs and nothing else, so an unlisted company with a
// duplicate of its own is not swept into the wave.
func TestExactSlugsPlansOnlyTheNamedSlugs(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	for _, r := range [][3]string{
		{"krka-1", "Krka", "krka"},
		{"krka-2", "KRKA, d.d., Novo mesto", "krka-d-d-novo-mesto"},
		{"acme-1", "Acme", "acme"},
		{"acme-2", "Acme GmbH", "acme-gmbh"},
	} {
		if _, err := pool.Exec(ctx,
			`INSERT INTO jobs (source, external_id, url, title, company, company_slug, public_slug)
			 VALUES ('test', $1, 'https://example.test/'||$1, 'Engineer', $2, $3, $1)`, r[0], r[1], r[2]); err != nil {
			t.Fatalf("seed %s: %v", r[0], err)
		}
	}
	plan, err := loadPlan(ctx, db.New(pool), 0, []string{"krka", "krka-d-d-novo-mesto"})
	if err != nil {
		t.Fatalf("loadPlan: %v", err)
	}
	if len(plan) != 1 || plan[0].Canonical != "krka" || len(plan[0].Aliases) != 1 || plan[0].Aliases[0].Slug != "krka-d-d-novo-mesto" {
		t.Fatalf("plan = %+v, want only krka-d-d-novo-mesto -> krka", plan)
	}
}
