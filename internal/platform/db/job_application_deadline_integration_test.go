//go:build integration

package db

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestApplicationDeadlinePreventsReopening(t *testing.T) {
	for _, write := range []string{"touch", "upsert"} {
		t.Run(write, func(t *testing.T) {
			pool := startPostgres(t)
			q := New(pool)
			ctx := context.Background()
			p := ingestParams("deadline:expired", "Engineer")
			row, err := ingestUpsert(ctx, q, p)
			if err != nil {
				t.Fatal(err)
			}
			_, err = pool.Exec(ctx, `UPDATE jobs SET application_deadline = (now() AT TIME ZONE 'UTC')::date - 1, closed_at = now(), closed_reason = 'expired' WHERE id = $1`, row.ID)
			if err != nil {
				t.Fatal(err)
			}
			if write == "touch" {
				_, err = q.TouchJob(ctx, TouchJobParams{Source: p.Source, ExternalID: p.ExternalID})
			} else {
				_, err = ingestUpsert(ctx, q, p)
			}
			if err != nil {
				t.Fatal(err)
			}
			if !isClosed(t, pool, row.ID) {
				t.Error("a re-listed posting past its application deadline reopened")
			}
		})
	}
}

func TestApplicationDeadlineExpiresOnTouch(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	for _, tc := range []struct {
		name   string
		days   int
		closed bool
	}{
		{"yesterday", -1, true}, {"today", 0, false}, {"tomorrow", 1, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := ingestParams("deadline:"+tc.name, "Engineer")
			row, err := ingestUpsert(ctx, q, p)
			if err != nil {
				t.Fatal(err)
			}
			_, err = pool.Exec(ctx, `UPDATE jobs SET application_deadline = (now() AT TIME ZONE 'UTC')::date + $2::int WHERE id = $1`, row.ID, tc.days)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := q.TouchJob(ctx, TouchJobParams{Source: p.Source, ExternalID: p.ExternalID}); err != nil {
				t.Fatal(err)
			}
			if got := isClosed(t, pool, row.ID); got != tc.closed {
				t.Errorf("closed = %v, want %v", got, tc.closed)
			}
			var queued bool
			if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT FROM search_delete_outbox WHERE job_id=$1)`, row.ID).Scan(&queued); err != nil {
				t.Fatal(err)
			}
			if queued != tc.closed {
				t.Errorf("search deletion queued = %v, want %v", queued, tc.closed)
			}
		})
	}
}

func TestApplicationDeadlineUpsertClosesAndExtends(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	p := ingestParams("deadline:upsert", "Engineer")
	p.ApplicationDeadline = pgtype.Date{Time: time.Now().UTC().AddDate(0, 0, -1), Valid: true}
	row, err := ingestUpsert(ctx, q, p)
	if err != nil {
		t.Fatal(err)
	}
	if !row.ClosedAt.Valid || row.ClosedReason != "expired" {
		t.Fatal("initially expired posting was inserted open")
	}
	p.ApplicationDeadline = pgtype.Date{Time: time.Now().UTC().AddDate(0, 0, 1), Valid: true}
	row, err = ingestUpsert(ctx, q, p)
	if err != nil {
		t.Fatal(err)
	}
	if row.ClosedAt.Valid {
		t.Error("an extended deadline did not reopen the posting")
	}
}

func TestApplicationDeadlineSweepHonorsDateAndSource(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	ids := map[string]int64{}
	for _, tc := range []struct {
		name, source string
		days         int
	}{
		{"expired", "eures", -1}, {"today", "eures", 0}, {"other", "ess", -1},
	} {
		p := ingestParams("deadline:"+tc.name, "Engineer")
		p.Source = tc.source
		row, err := ingestUpsert(ctx, q, p)
		if err != nil {
			t.Fatal(err)
		}
		ids[tc.name] = row.ID
		_, err = pool.Exec(ctx, `UPDATE jobs SET application_deadline = (now() AT TIME ZONE 'UTC')::date + $2::int WHERE id=$1`, row.ID, tc.days)
		if err != nil {
			t.Fatal(err)
		}
	}
	count, err := q.CloseJobsPastApplicationDeadline(ctx, "eures")
	if err != nil || count != 1 {
		t.Fatalf("sweep count=%d err=%v, want 1", count, err)
	}
	for name, id := range ids {
		if got := isClosed(t, pool, id); got != (name == "expired") {
			t.Errorf("%s closed=%v", name, got)
		}
	}
	count, err = q.CloseJobsPastApplicationDeadline(ctx, "eures")
	if err != nil || count != 0 {
		t.Fatalf("repeated sweep count=%d err=%v, want 0", count, err)
	}
	count, err = q.CloseJobsPastApplicationDeadline(ctx, "")
	if err != nil || count != 1 {
		t.Fatalf("unfiltered sweep count=%d err=%v, want 1", count, err)
	}
	var queued int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM search_delete_outbox`).Scan(&queued); err != nil {
		t.Fatal(err)
	}
	if queued != 2 {
		t.Errorf("queued=%d, want 2", queued)
	}
}

func TestApplicationDeadlineUpsertQueuesExpiredExistingPosting(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	p := ingestParams("deadline:existing", "Engineer")
	row, err := ingestUpsert(ctx, q, p)
	if err != nil {
		t.Fatal(err)
	}
	p.ApplicationDeadline = pgtype.Date{Time: time.Now().UTC().AddDate(0, 0, -1), Valid: true}
	row, err = ingestUpsert(ctx, q, p)
	if err != nil {
		t.Fatal(err)
	}
	if !row.ClosedAt.Valid {
		t.Fatal("existing posting stayed open past its deadline")
	}
	var queued bool
	if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT FROM search_delete_outbox WHERE job_id=$1)`, row.ID).Scan(&queued); err != nil {
		t.Fatal(err)
	}
	if !queued {
		t.Error("expired existing posting was not queued for search removal")
	}
	p.ContentHash = pgtype.Text{String: "unchanged-content", Valid: true}
	if _, err := ingestUpsert(ctx, q, p); err != nil {
		t.Fatal(err)
	}
	p.ApplicationDeadline = pgtype.Date{Time: time.Now().UTC().AddDate(0, 0, 1), Valid: true}
	reopened, err := q.UpsertJob(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.Job.ClosedAt.Valid || !reopened.Changed {
		t.Error("deadline extension must reopen and signal a search update even with identical content")
	}
	if countDeletionQueue(t, pool) != 0 {
		t.Error("deadline extension left a stale expiry deletion queued")
	}
}

func TestApplicationDeadlineChangeBypassesCheapRefresh(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	seedForRefresh(t, pool, q, []string{"berlin"})
	p := unchangedParams()
	p.ApplicationDeadline = pgtype.Date{Time: time.Now().UTC().AddDate(0, 0, 1), Valid: true}
	if _, err := q.RefreshUnchangedJob(ctx, p); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("changed deadline used cheap refresh: %v", err)
	}
}

func TestApplicationDeadlineChangesSignalSearchUpdate(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	p := ingestParams("deadline:search-update", "Engineer")
	p.ContentHash = pgtype.Text{String: "unchanged-content", Valid: true}
	if _, err := q.UpsertJob(ctx, p); err != nil {
		t.Fatal(err)
	}

	first := pgtype.Date{Time: time.Now().UTC().AddDate(0, 0, 1), Valid: true}
	extended := pgtype.Date{Time: time.Now().UTC().AddDate(0, 0, 2), Valid: true}
	for _, tc := range []struct {
		name     string
		incoming pgtype.Date
		stored   pgtype.Date
		changed  bool
	}{
		{"first deadline", first, first, true},
		{"same deadline", first, first, false},
		{"extended while open", extended, extended, true},
		{"missing preserves deadline", pgtype.Date{}, extended, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p.ApplicationDeadline = tc.incoming
			row, err := q.UpsertJob(ctx, p)
			if err != nil {
				t.Fatal(err)
			}
			if row.Changed != tc.changed {
				t.Errorf("Changed = %v, want %v", row.Changed, tc.changed)
			}
			if row.Job.ClosedAt.Valid {
				t.Error("posting with a future deadline must stay open")
			}
			if !row.Job.ApplicationDeadline.Valid || row.Job.ApplicationDeadline.Time.Format("2006-01-02") != tc.stored.Time.Format("2006-01-02") {
				t.Errorf("stored deadline = %v, want %v", row.Job.ApplicationDeadline, tc.stored)
			}
		})
	}
}

func TestApplicationDeadlineExpiryDoesNotQueuePrivatePosting(t *testing.T) {
	for _, write := range []string{"touch", "upsert"} {
		t.Run(write, func(t *testing.T) {
			pool := startPostgres(t)
			q := New(pool)
			ctx := context.Background()
			p := ingestParams("deadline:private", "Engineer")
			row, err := ingestUpsert(ctx, q, p)
			if err != nil {
				t.Fatal(err)
			}
			_, err = pool.Exec(ctx, `UPDATE jobs SET is_private = true,
				application_deadline = (now() AT TIME ZONE 'UTC')::date - 1 WHERE id = $1`, row.ID)
			if err != nil {
				t.Fatal(err)
			}
			if write == "touch" {
				_, err = q.TouchJob(ctx, TouchJobParams{Source: p.Source, ExternalID: p.ExternalID})
			} else {
				p.Title = "Updated private posting"
				row, err = ingestUpsert(ctx, q, p)
				if err == nil && (!row.IsPrivate || row.Title != p.Title) {
					t.Error("private posting's own update must still apply without changing its visibility")
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			if countDeletionQueue(t, pool) != 0 {
				t.Error("private posting queued for public search removal")
			}
		})
	}
}
