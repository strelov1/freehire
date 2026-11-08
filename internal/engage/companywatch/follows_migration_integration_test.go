//go:build integration

package companywatch_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/strelov1/freehire/internal/platform/testdb"
)

// Migration 0249 turns the old company page's per-company follows into the watchlist and its
// one alert. The template database already ran it over nothing, so the test seeds the old
// shape and runs the file again, as the deploy did over the real rows.
func TestFollowsMigration_MovesFollowsOntoTheWatchlistAlert(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	id := func(sql string, args ...any) int64 {
		t.Helper()
		var out int64
		if err := pool.QueryRow(ctx, sql, args...).Scan(&out); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
		return out
	}

	follower := id(`INSERT INTO users (email) VALUES ('f@example.test') RETURNING id`)
	bookmarker := id(`INSERT INTO users (email) VALUES ('b@example.test') RETURNING id`)
	exec(`INSERT INTO companies (slug, name) VALUES ('acme', 'Acme'), ('globex', 'Globex')`)

	// Two follows the old button made (named after the company), both on telegram, one paused;
	// one follow the person renamed; and a bookmark with no channel at all.
	acme := id(`INSERT INTO saved_searches (user_id, name, query) VALUES ($1, 'Acme', 'company_slug=acme') RETURNING id`, follower)
	globex := id(`INSERT INTO saved_searches (user_id, name, query) VALUES ($1, 'Globex', 'company_slug=globex') RETURNING id`, follower)
	exec(`INSERT INTO subscriptions (user_id, saved_search_id, channel, destination) VALUES ($1, $2, 'telegram', '42')`, follower, acme)
	exec(`INSERT INTO subscriptions (user_id, saved_search_id, channel, destination, active) VALUES ($1, $2, 'telegram', '42', false)`, follower, globex)
	exec(`INSERT INTO subscriptions (user_id, saved_search_id, channel) VALUES ($1, $2, 'email')`, follower, globex)
	mine := id(`INSERT INTO saved_searches (user_id, name, query) VALUES ($1, 'My dream job', 'company_slug=acme&work_mode=remote') RETURNING id`, follower)
	exec(`INSERT INTO subscriptions (user_id, saved_search_id, channel) VALUES ($1, $2, 'email')`, follower, mine)
	renamed := id(`INSERT INTO saved_searches (user_id, name, query) VALUES ($1, 'Globex jobs', 'company_slug=globex') RETURNING id`, bookmarker)
	exec(`INSERT INTO subscriptions (user_id, saved_search_id, channel) VALUES ($1, $2, 'email')`, bookmarker, renamed)
	bookmark := id(`INSERT INTO saved_searches (user_id, name, query) VALUES ($1, 'Acme', 'company_slug=acme') RETURNING id`, bookmarker)

	sql, err := os.ReadFile(filepath.Join("..", "..", "..", "migrations", "0249_company_follows_to_watchlist.sql"))
	if err != nil {
		t.Fatal(err)
	}
	exec(string(sql))

	watched := func(user int64) []string {
		t.Helper()
		rows, err := pool.Query(ctx, `SELECT company_slug FROM company_watchlist WHERE user_id = $1 ORDER BY 1`, user)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		var out []string
		for rows.Next() {
			var s string
			if err := rows.Scan(&s); err != nil {
				t.Fatal(err)
			}
			out = append(out, s)
		}
		return out
	}
	if got := watched(follower); !slices.Equal(got, []string{"acme", "globex"}) {
		t.Errorf("follower watches %v, want [acme globex]", got)
	}
	if got := watched(bookmarker); !slices.Equal(got, []string{"globex"}) {
		t.Errorf("bookmarker watches %v, want [globex] (the bookmark is not a follow)", got)
	}

	alert := id(`SELECT id FROM saved_searches WHERE user_id = $1 AND query = 'watchlist=true'`, follower)
	if n := id(`SELECT count(*) FROM subscriptions WHERE saved_search_id = $1 AND channel = 'telegram' AND destination = '42' AND active`, alert); n != 1 {
		t.Errorf("telegram subscriptions on the watchlist alert = %d, want 1 active (one of the follows was live)", n)
	}
	if n := id(`SELECT count(*) FROM subscriptions WHERE saved_search_id = $1 AND channel = 'email'`, alert); n != 1 {
		t.Errorf("email subscriptions on the watchlist alert = %d, want 1", n)
	}

	// What the old button made is gone; what the person made or named stays.
	for name, ss := range map[string]int64{"acme follow": acme, "globex follow": globex} {
		if n := id(`SELECT count(*) FROM saved_searches WHERE id = $1`, ss); n != 0 {
			t.Errorf("%s saved search survived", name)
		}
	}
	for name, ss := range map[string]int64{"multi-filter search": mine, "renamed follow": renamed, "bookmark": bookmark} {
		if n := id(`SELECT count(*) FROM saved_searches WHERE id = $1`, ss); n != 1 {
			t.Errorf("%s saved search was deleted", name)
		}
	}
	if n := id(`SELECT count(*) FROM subscriptions WHERE saved_search_id = $1`, mine); n != 1 {
		t.Errorf("the multi-filter search lost its subscription")
	}
	if n := id(`SELECT count(*) FROM subscriptions WHERE saved_search_id = $1`, renamed); n != 0 {
		t.Errorf("the renamed follow kept its subscription, so it would deliver twice")
	}
}
