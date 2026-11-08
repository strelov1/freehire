//go:build integration

// Integration test for the watchlist's SQL: the alias following, the atomic cap, and the
// import that resolves names against a real catalogue. Only a real Postgres can say these hold.
// Run with: go test -tags=integration ./internal/engage/companywatch/
package companywatch_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/strelov1/freehire/internal/engage/companywatch"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

func TestWatchlist_FollowsMergesAndKeepsCompaniesWithNoJobs(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	svc := companywatch.New(db.New(pool))

	var user, other int64
	for email, id := range map[string]*int64{"w@example.test": &user, "x@example.test": &other} {
		if err := pool.QueryRow(ctx, `INSERT INTO users (email) VALUES ($1) RETURNING id`, email).Scan(id); err != nil {
			t.Fatalf("seed user: %v", err)
		}
	}
	for _, c := range [][2]string{{"acme", "Acme"}, {"globex", "Globex"}, {"dollar-tree", "Dollar Tree"}} {
		if _, err := pool.Exec(ctx, `INSERT INTO companies (slug, name, job_count) VALUES ($1, $2, 3)`, c[0], c[1]); err != nil {
			t.Fatalf("seed company: %v", err)
		}
	}

	if _, err := svc.Add(ctx, user, "acme"); err != nil {
		t.Fatalf("Add acme: %v", err)
	}
	if _, err := svc.Add(ctx, user, "acme"); err != nil {
		t.Fatalf("Add acme again: %v", err)
	}
	if _, err := svc.Add(ctx, user, "nobody"); !errors.Is(err, companywatch.ErrNotFound) {
		t.Fatalf("Add nobody = %v, want ErrNotFound", err)
	}

	// A merge retires "dollartree" into "dollar-tree" after the person watched it by the old
	// spelling: the stored row is untouched and every read follows the alias.
	if _, err := pool.Exec(ctx, `INSERT INTO company_watchlist (user_id, company_slug) VALUES ($1, 'dollartree')`, user); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO company_slug_aliases (alias_slug, canonical_slug, folded_key, reason)
		VALUES ('dollartree', 'dollar-tree', 'dollartree', 'spelling')`); err != nil {
		t.Fatal(err)
	}

	slugs, err := svc.Slugs(ctx, user)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"acme", "dollar-tree"}; !slices.Equal(slugs, want) {
		t.Errorf("Slugs = %v, want %v", slugs, want)
	}
	if got, _ := svc.Slugs(ctx, other); len(got) != 0 {
		t.Errorf("another person's list = %v, want empty", got)
	}
	if ok, _ := svc.Watching(ctx, user, "dollar-tree"); !ok {
		t.Error("a company watched under its retired spelling is not reported as watched")
	}
	if ok, _ := svc.Watching(ctx, user, "globex"); ok {
		t.Error("globex reported as watched")
	}
	rows, total, err := svc.List(ctx, user, "", 10, 0)
	if err != nil || total != 2 || len(rows) != 2 {
		t.Fatalf("List = %v, total %d, %v; the alias and its canonical slug must show once", rows, total, err)
	}
	if _, n, _ := svc.List(ctx, user, "dollar", 10, 0); n != 1 {
		t.Errorf("name filter matched %d, want 1", n)
	}

	// An employer whose last posting went is deleted from companies; the entry stays.
	if _, err := pool.Exec(ctx, `DELETE FROM companies WHERE slug = 'acme'`); err != nil {
		t.Fatal(err)
	}
	if got, _ := svc.Slugs(ctx, user); !slices.Contains(got, "acme") {
		t.Errorf("Slugs = %v, lost a company with no jobs", got)
	}

	// Removing by the canonical slug takes the stored alias with it.
	if err := svc.Remove(ctx, user, "dollar-tree"); err != nil {
		t.Fatal(err)
	}
	if got, _ := svc.Slugs(ctx, user); !slices.Equal(got, []string{"acme"}) {
		t.Errorf("after Remove, Slugs = %v", got)
	}
	if err := svc.Remove(ctx, user, "dollar-tree"); err != nil {
		t.Errorf("removing what is not there: %v", err)
	}
}

func TestWatchlist_CapIsAtomicAndNeverRefusesWhatIsAlreadyThere(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	svc := companywatch.New(db.New(pool))

	var user int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (email) VALUES ('cap@example.test') RETURNING id`).Scan(&user); err != nil {
		t.Fatal(err)
	}
	// Fill the list to one short of the limit directly, then drive the service across it.
	if _, err := pool.Exec(ctx, `INSERT INTO company_watchlist (user_id, company_slug)
		SELECT $1, 'co-' || g FROM generate_series(1, $2::int) g`, user, companywatch.MaxEntries-1); err != nil {
		t.Fatal(err)
	}
	for _, slug := range []string{"last", "over"} {
		if _, err := pool.Exec(ctx, `INSERT INTO companies (slug, name, job_count) VALUES ($1, $1, 1)`, slug); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := svc.Add(ctx, user, "last"); err != nil {
		t.Fatalf("the last free place: %v", err)
	}
	if _, err := svc.Add(ctx, user, "over"); !errors.Is(err, companywatch.ErrFull) {
		t.Fatalf("Add on a full list = %v, want ErrFull", err)
	}
	if _, err := svc.Add(ctx, user, "last"); err != nil {
		t.Errorf("re-adding on a full list must stay idempotent: %v", err)
	}

	res, err := svc.Import(ctx, user, []string{"last"})
	if err != nil || res.Added != 0 || res.Already != 1 {
		t.Errorf("import of a stored company on a full list = %+v, %v", res, err)
	}
	if _, err := svc.Import(ctx, user, []string{"over"}); !errors.Is(err, companywatch.ErrFull) {
		t.Errorf("import past the limit = %v, want ErrFull", err)
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM company_watchlist WHERE user_id = $1`, user).Scan(&n); err != nil || n != companywatch.MaxEntries {
		t.Errorf("rows = %d (%v), want exactly %s", n, err, fmt.Sprint(companywatch.MaxEntries))
	}
}

func TestWatchlist_ImportResolvesNamesAgainstTheCatalogue(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	svc := companywatch.New(db.New(pool))

	var user int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (email) VALUES ('imp@example.test') RETURNING id`).Scan(&user); err != nil {
		t.Fatal(err)
	}
	for _, c := range [][2]string{{"acme", "Acme"}, {"dollar-tree", "Dollar Tree"}} {
		if _, err := pool.Exec(ctx, `INSERT INTO companies (slug, name, job_count) VALUES ($1, $2, 2)`, c[0], c[1]); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pool.Exec(ctx, `INSERT INTO company_slug_aliases (alias_slug, canonical_slug, folded_key, reason)
		VALUES ('dollartree', 'dollar-tree', 'dollartree', 'spelling')`); err != nil {
		t.Fatal(err)
	}

	res, err := svc.Import(ctx, user, []string{"Acme Inc.", "DollarTree", "Nowhere Ltd", "acme"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Added != 2 || res.Already != 0 || !slices.Equal(res.Unmatched, []string{"Nowhere Ltd"}) {
		t.Errorf("import = %+v, want 2 added and Nowhere Ltd unmatched", res)
	}
	got, _ := svc.Slugs(ctx, user)
	if !slices.Equal(got, []string{"acme", "dollar-tree"}) {
		t.Errorf("Slugs = %v", got)
	}
	again, err := svc.Import(ctx, user, []string{"Acme"})
	if err != nil || again.Added != 0 || again.Already != 1 {
		t.Errorf("second import = %+v, %v", again, err)
	}
}
