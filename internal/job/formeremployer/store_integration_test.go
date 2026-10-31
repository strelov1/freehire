//go:build integration

// Store.Slugs against real Postgres: the bank read is owner-scoped and skips current roles
// and projects, the registry and the fold both resolve, and a fold two hiring slugs share is
// refused. Run with: go test -tags=integration ./internal/job/formeremployer/
package formeremployer

import (
	"context"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

func seedUser(t *testing.T, pool *pgxpool.Pool, email string) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO users (email) VALUES ($1) RETURNING id`, email).Scan(&id); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return id
}

func seedEmployment(t *testing.T, pool *pgxpool.Pool, userID int64, kind, company, location string, current bool) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO experience_employments (user_id, kind, company, location, is_current) VALUES ($1, $2, $3, $4, $5)`,
		userID, kind, company, location, current); err != nil {
		t.Fatalf("seed employment: %v", err)
	}
}

func seedCompany(t *testing.T, pool *pgxpool.Pool, slug string, jobs int, countries []string) {
	t.Helper()
	if countries == nil {
		countries = []string{}
	}
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO companies (slug, name, job_count, countries) VALUES ($1, $1, $2, $3)`,
		slug, jobs, countries); err != nil {
		t.Fatalf("seed company: %v", err)
	}
}

func TestStoreSlugs(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()

	seedCompany(t, pool, "fe-globex", 5, []string{"us"})
	seedCompany(t, pool, "fe-initech", 0, []string{"us"}) // carries no searchable job
	seedCompany(t, pool, "fe-umbrella", 3, nil)           // the canon of a squashed spelling
	seedCompany(t, pool, "feumbrella", 2, nil)            // its unmerged twin
	seedCompany(t, pool, "fe-hooli", 4, nil)              // two slugs, one fold, no registry
	seedCompany(t, pool, "feho-oli", 4, nil)
	seedCompany(t, pool, "fe-stark", 9, nil)
	// Different aliases can share a folded key and still name the same settled company.
	if _, err := pool.Exec(ctx,
		`INSERT INTO company_slug_aliases (alias_slug, canonical_slug, folded_key, reason)
		 VALUES ('feumbrella', 'fe-umbrella', 'feumbrella', 'spelling'),
		        ('fe-umbrella-inc', 'fe-umbrella', 'feumbrella', 'spelling')`); err != nil {
		t.Fatalf("seed alias: %v", err)
	}

	me := seedUser(t, pool, "former-employer-me@example.test")
	other := seedUser(t, pool, "former-employer-other@example.test")
	seedEmployment(t, pool, me, "job", "FE Globex Inc.", "", false)
	seedEmployment(t, pool, me, "job", "FE Initech", "", false)
	seedEmployment(t, pool, me, "job", "FEUmbrella", "", false)
	seedEmployment(t, pool, me, "job", "FE Hooli", "", false)
	seedEmployment(t, pool, me, "job", "FE Stark", "", true)      // current, not former
	seedEmployment(t, pool, me, "project", "FE Stark", "", false) // a project is no employer
	seedEmployment(t, pool, other, "job", "FE Stark", "", false)  // somebody else's history

	got, err := NewStore(db.New(pool)).Slugs(ctx, me)
	if err != nil {
		t.Fatalf("Slugs: %v", err)
	}
	if want := []string{"fe-globex", "fe-umbrella"}; !slices.Equal(got, want) {
		t.Errorf("Slugs = %v, want %v", got, want)
	}

	none, err := NewStore(db.New(pool)).Slugs(ctx, seedUser(t, pool, "former-employer-empty@example.test"))
	if err != nil || none != nil {
		t.Errorf("Slugs(empty bank) = %v, %v; want nothing, no error", none, err)
	}
}
