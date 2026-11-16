//go:build integration

package db

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestApplicationDigestRequiresConsent(t *testing.T) {
	pool := startPostgres(t)
	uid := insertUser(t, pool, "weekly-digest@example.test")
	var enabled bool
	if err := pool.QueryRow(context.Background(), "SELECT coalesce((SELECT application_digest_enabled FROM notification_settings WHERE user_id=$1),false)", uid).Scan(&enabled); err != nil {
		t.Fatal(err)
	}
	if enabled {
		t.Fatal("weekly application digest must require explicit consent")
	}
}

func TestApplicationDigestSnapshotExcludesCorrectionsAndOldEvents(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	uid := insertUser(t, pool, "weekly-events@example.test")
	now := time.Now().UTC()
	for _, event := range []struct {
		kind, signal string
		days         int
		retracted    bool
	}{
		{"applied", "", 1, false}, {"employer_reply", "", 2, false}, {"stage_set", "interview", 3, false}, {"stage_set", "offer", 4, false},
		{"employer_reply", "", 1, true}, {"applied", "", 8, false},
	} {
		_, err := pool.Exec(ctx, `INSERT INTO application_events(user_id,company_slug,kind,signal,occurred_at,source,retracted_at) VALUES($1,'example',$2,$3,$4,'user',CASE WHEN $5 THEN now() END)`, uid, event.kind, event.signal, now.AddDate(0, 0, -event.days), event.retracted)
		if err != nil {
			t.Fatal(err)
		}
	}
	got, err := New(pool).GetApplicationDigestSnapshot(ctx, GetApplicationDigestSnapshotParams{UserID: uid, NowAt: pgtype.Timestamptz{Time: now, Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	if got.Applied != 1 || got.Replies != 1 || got.Interviews != 1 || got.Offers != 1 {
		t.Fatalf("snapshot=%+v", got)
	}
}

func TestApplicationDigestClaimIsWeeklyAndRechecksConsent(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	uid := insertUser(t, pool, "weekly-claim@example.test")
	monday := time.Now().UTC().Truncate(24 * time.Hour).Add(12 * time.Hour)
	for monday.Weekday() != time.Monday {
		monday = monday.AddDate(0, 0, 1)
	}
	_, err := pool.Exec(ctx, `UPDATE users SET email_verified=true,last_active_at=now(),pro_until_granted=$2,timezone='UTC' WHERE id=$1`, uid, monday.AddDate(0, 1, 0))
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO notification_settings(user_id,enabled,channels,digest_time,application_digest_enabled,application_digest_enabled_at) VALUES($1,true,ARRAY['email'],'09:00',true,$2)`, uid, monday.Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	q := New(pool)
	stamp := pgtype.Timestamptz{Time: monday, Valid: true}
	candidates, err := q.ListApplicationDigestCandidates(ctx, ListApplicationDigestCandidatesParams{NowAt: stamp, BatchSize: 100})
	if err != nil || len(candidates) != 1 {
		t.Fatalf("candidates=%+v err=%v", candidates, err)
	}
	claim := ClaimApplicationDigestParams{UserID: uid, NowAt: stamp, LocalWeek: pgtype.Date{Time: monday, Valid: true}, Email: candidates[0].Email, Timezone: candidates[0].Timezone, EnabledAt: candidates[0].EnabledAt}
	for i, want := range []int64{1, 0} {
		n, err := q.ClaimApplicationDigest(ctx, claim)
		if err != nil || n != want {
			t.Fatalf("claim %d rows=%d err=%v", i, n, err)
		}
	}
	if err := q.ReleasePausedApplicationDigest(ctx, ReleasePausedApplicationDigestParams{UserID: uid, LocalWeek: claim.LocalWeek}); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "UPDATE notification_settings SET application_digest_enabled=false WHERE user_id=$1", uid); err != nil {
		t.Fatal(err)
	}
	n, err := q.ClaimApplicationDigest(ctx, claim)
	if err != nil || n != 0 {
		t.Fatalf("revoked consent claim=%d err=%v", n, err)
	}
}
