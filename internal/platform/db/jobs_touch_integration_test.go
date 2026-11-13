//go:build integration

// Integration test for the hydrating-source liveness refresh: TouchJob refreshes last_seen_at
// and reopens a closed row WITHOUT rewriting its content, so a re-listed already-ingested
// posting keeps the description/facets it was hydrated with. Verifiable only against a real
// Postgres. Run with: go test -tags=integration ./internal/platform/db/
package db

import (
	"context"
	"testing"
	"time"
)

func TestTouchJobRefreshesLivenessAndPreservesContent(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	// Ingest a job that carries a description and skills (as a hydrated justjoin row would).
	p := ingestParams("justjoin:1", "Engineer")
	p.Source = "justjoin"
	p.Description = "Rich hydrated body."
	p.Skills = []string{"go", "typescript"}
	if _, err := ingestUpsert(ctx, q, p); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	// Simulate a stale, closed row: last_seen_at far in the past, closed_at set.
	if _, err := pool.Exec(ctx,
		`UPDATE jobs SET last_seen_at = now() - interval '10 days', closed_at = now() WHERE source='justjoin' AND external_id='justjoin:1'`,
	); err != nil {
		t.Fatalf("stale/close setup: %v", err)
	}
	// job_activity is what the touch compares and the readers see; the bridge never lowers it, so
	// age it directly, after the legacy write.
	if _, err := pool.Exec(ctx,
		`UPDATE job_activity SET last_seen_at = now() - interval '10 days'
		 WHERE job_id = (SELECT id FROM jobs WHERE source='justjoin' AND external_id='justjoin:1')`,
	); err != nil {
		t.Fatalf("age activity: %v", err)
	}

	touched, err := q.TouchJob(ctx, TouchJobParams{Source: "justjoin", ExternalID: "justjoin:1", AllowReopen: true})
	if err != nil {
		t.Fatalf("TouchJob: %v", err)
	}
	// The touched row's company is returned so the caller can keep it in the sweep scope.
	if touched.CompanySlug != "acme" {
		t.Errorf("TouchJob company_slug = %q, want acme", touched.CompanySlug)
	}
	// A closed row coming back is reported, so ingest re-derives what the touch did not write.
	if !touched.Reopened {
		t.Error("TouchJob reopened = false for a closed row, want true")
	}

	got, err := q.GetJobBySourceExternalID(ctx, GetJobBySourceExternalIDParams{Source: "justjoin", ExternalID: "justjoin:1"})
	if err != nil {
		t.Fatalf("readback: %v", err)
	}
	if got.ClosedAt.Valid {
		t.Error("closed_at still set, want reopened (NULL)")
	}
	if time.Since(got.LastSeenAt.Time) > time.Minute {
		t.Errorf("last_seen_at = %v, want refreshed to ~now", got.LastSeenAt.Time)
	}
	// Content must be untouched.
	if got.Description != "Rich hydrated body." {
		t.Errorf("Description = %q, want preserved", got.Description)
	}
	if len(got.Skills) != 2 {
		t.Errorf("Skills = %v, want preserved [go typescript]", got.Skills)
	}
}

// TouchJob is the hydrating-source half of the same economy RefreshUnchangedJob brings to the
// board path: a re-listed offer that changed nothing must not drag updated_at forward, so the
// column keeps meaning "content last changed". A reopen is the exception, and must stay one —
// it is a change the search reconciler has to see.
func TestTouchJobStampsUpdatedAtOnlyWhenItReopens(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	seed := func(t *testing.T, externalID string, closed bool) Job {
		t.Helper()
		p := ingestParams(externalID, "Engineer")
		p.Source = "justjoin"
		if _, err := ingestUpsert(ctx, q, p); err != nil {
			t.Fatalf("seed upsert: %v", err)
		}
		closeClause := ""
		if closed {
			closeClause = ", closed_at = now(), liveness_strikes = 2"
		}
		if _, err := pool.Exec(ctx,
			`UPDATE jobs SET last_seen_at = now() - interval '10 days',
			                 updated_at = now() - interval '10 days'`+closeClause+`
			 WHERE source = 'justjoin' AND external_id = $1`, externalID,
		); err != nil {
			t.Fatalf("seed stamps: %v", err)
		}
		if _, err := pool.Exec(ctx,
			`UPDATE job_activity SET last_seen_at = now() - interval '10 days'
			 WHERE job_id = (SELECT id FROM jobs WHERE source = 'justjoin' AND external_id = $1)`, externalID,
		); err != nil {
			t.Fatalf("age activity: %v", err)
		}
		got, err := q.GetJobBySourceExternalID(ctx, GetJobBySourceExternalIDParams{Source: "justjoin", ExternalID: externalID})
		if err != nil {
			t.Fatalf("seed readback: %v", err)
		}
		return got
	}

	t.Run("open row keeps its change stamp", func(t *testing.T) {
		before := seed(t, "justjoin:open", false)
		touched, err := q.TouchJob(ctx, TouchJobParams{Source: "justjoin", ExternalID: "justjoin:open", AllowReopen: true})
		if err != nil {
			t.Fatalf("TouchJob: %v", err)
		}
		if touched.Reopened {
			t.Error("TouchJob reopened = true for an open row, want false")
		}
		after, err := q.GetJobBySourceExternalID(ctx, GetJobBySourceExternalIDParams{Source: "justjoin", ExternalID: "justjoin:open"})
		if err != nil {
			t.Fatalf("readback: %v", err)
		}
		if !after.LastSeenAt.Time.After(before.LastSeenAt.Time) {
			t.Errorf("last_seen_at = %v, want advanced — the unseen sweep depends on it", after.LastSeenAt.Time)
		}
		if !after.UpdatedAt.Time.Equal(before.UpdatedAt.Time) {
			t.Errorf("updated_at = %v, want unchanged %v — a re-listing is not a content change",
				after.UpdatedAt.Time, before.UpdatedAt.Time)
		}
	})

	t.Run("a reopen it may not make is deferred untouched", func(t *testing.T) {
		before := seed(t, "justjoin:deferred", true)
		touched, err := q.TouchJob(ctx, TouchJobParams{Source: "justjoin", ExternalID: "justjoin:deferred"})
		if err != nil {
			t.Fatalf("TouchJob: %v", err)
		}
		if !touched.Deferred || touched.Reopened {
			t.Errorf("deferred = %v, reopened = %v; want the reopen deferred to the caller", touched.Deferred, touched.Reopened)
		}
		after, err := q.GetJobBySourceExternalID(ctx, GetJobBySourceExternalIDParams{Source: "justjoin", ExternalID: "justjoin:deferred"})
		if err != nil {
			t.Fatalf("readback: %v", err)
		}
		// Still closed and unstamped: a caller whose re-derive then fails leaves it for the next crawl.
		if !after.ClosedAt.Valid || !after.LastSeenAt.Time.Equal(before.LastSeenAt.Time) {
			t.Errorf("closed_at valid = %v, last_seen_at = %v; want the row left exactly as it was",
				after.ClosedAt.Valid, after.LastSeenAt.Time)
		}
	})

	t.Run("reopen stamps it", func(t *testing.T) {
		before := seed(t, "justjoin:closed", true)
		if _, err := q.TouchJob(ctx, TouchJobParams{Source: "justjoin", ExternalID: "justjoin:closed", AllowReopen: true}); err != nil {
			t.Fatalf("TouchJob: %v", err)
		}
		after, err := q.GetJobBySourceExternalID(ctx, GetJobBySourceExternalIDParams{Source: "justjoin", ExternalID: "justjoin:closed"})
		if err != nil {
			t.Fatalf("readback: %v", err)
		}
		if after.ClosedAt.Valid || after.LivenessStrikes != 0 {
			t.Errorf("closed_at = %v, strikes = %d; want reopened and reset",
				after.ClosedAt.Time, after.LivenessStrikes)
		}
		if !after.UpdatedAt.Time.After(before.UpdatedAt.Time) {
			t.Errorf("updated_at = %v, want stamped %v — a reopen is a change the reindex must see",
				after.UpdatedAt.Time, before.UpdatedAt.Time)
		}
	})
}
