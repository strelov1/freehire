//go:build integration

package db

import (
	"context"
	"testing"
)

// A campaign with a timezones list reaches only the accounts in one of those zones; one
// without it still reaches everyone, including accounts that never reported a timezone.
// The second half is the trap: Go's empty list arrives as NULL, and a filter that read
// NULL as "no match" would silently empty every other campaign.
func TestBroadcastCandidates_TimezoneAudience(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncateSubs(t, pool)

	brazil := insertUser(t, pool, "tz-brazil@example.test")
	lisbon := insertUser(t, pool, "tz-lisbon@example.test")
	unknown := insertUser(t, pool, "tz-unknown@example.test")
	for _, id := range []int64{brazil, lisbon, unknown} {
		verifyEmail(t, pool, id)
	}
	if _, err := pool.Exec(ctx, `UPDATE users SET timezone = 'America/Sao_Paulo' WHERE id = $1`, brazil); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE users SET timezone = 'Europe/Lisbon' WHERE id = $1`, lisbon); err != nil {
		t.Fatal(err)
	}

	audience := func(zones []string) map[int64]bool {
		t.Helper()
		rows, err := q.ListBroadcastCandidates(ctx, ListBroadcastCandidatesParams{Campaign: "tz-test", Timezones: zones, MaxRows: 100})
		if err != nil {
			t.Fatalf("ListBroadcastCandidates: %v", err)
		}
		got := map[int64]bool{}
		for _, r := range rows {
			got[r.ID] = true
		}
		count, err := q.CountBroadcastCandidates(ctx, CountBroadcastCandidatesParams{Campaign: "tz-test", Timezones: zones})
		if err != nil {
			t.Fatalf("CountBroadcastCandidates: %v", err)
		}
		if count != int64(len(got)) {
			t.Fatalf("count %d disagrees with the list %v", count, got)
		}
		return got
	}

	if got := audience([]string{"America/Sao_Paulo", "America/Recife"}); !got[brazil] || got[lisbon] || got[unknown] || len(got) != 1 {
		t.Errorf("Brazil campaign reached %v, want only %d", got, brazil)
	}
	if got := audience(nil); !got[brazil] || !got[lisbon] || !got[unknown] {
		t.Errorf("campaign without timezones reached %v, want all three", got)
	}
}

// A Data-plan campaign reaches only accounts whose Data plan is still live: not an expired one,
// and not an API-plan holder.
func TestBroadcastCandidates_DataPlanAudience(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncateSubs(t, pool)

	live := insertUser(t, pool, "data-live@example.test")
	expired := insertUser(t, pool, "data-expired@example.test")
	apiOnly := insertUser(t, pool, "api-only@example.test")
	for _, id := range []int64{live, expired, apiOnly} {
		verifyEmail(t, pool, id)
	}
	for _, stmt := range []struct {
		sql string
		id  int64
	}{
		{`UPDATE users SET api_data_until_stripe = now() + interval '30 days' WHERE id = $1`, live},
		{`UPDATE users SET api_data_until_stripe = now() - interval '1 day' WHERE id = $1`, expired},
		{`UPDATE users SET api_until_stripe = now() + interval '30 days' WHERE id = $1`, apiOnly},
	} {
		if _, err := pool.Exec(ctx, stmt.sql, stmt.id); err != nil {
			t.Fatal(err)
		}
	}

	rows, err := q.ListBroadcastCandidates(ctx, ListBroadcastCandidatesParams{Campaign: "data-test", DataPlanOnly: true, MaxRows: 100})
	if err != nil {
		t.Fatalf("ListBroadcastCandidates: %v", err)
	}
	if len(rows) != 1 || rows[0].ID != live {
		t.Errorf("Data campaign reached %v, want only %d", rows, live)
	}
	count, err := q.CountBroadcastCandidates(ctx, CountBroadcastCandidatesParams{Campaign: "data-test", DataPlanOnly: true})
	if err != nil {
		t.Fatalf("CountBroadcastCandidates: %v", err)
	}
	if count != 1 {
		t.Errorf("count = %d, want 1", count)
	}

	// An API-plan campaign reaches the API-plan holder and not the Data one.
	rows, err = q.ListBroadcastCandidates(ctx, ListBroadcastCandidatesParams{Campaign: "api-test", ApiPlanOnly: true, MaxRows: 100})
	if err != nil {
		t.Fatalf("ListBroadcastCandidates: %v", err)
	}
	if len(rows) != 1 || rows[0].ID != apiOnly {
		t.Errorf("API campaign reached %v, want only %d", rows, apiOnly)
	}
}
