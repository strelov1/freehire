//go:build integration

package db

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

func digestTime(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }
func digestAccount(t *testing.T, pool *pgxpool.Pool, now time.Time, zone string) int64 {
	t.Helper()
	uid := insertUser(t, pool, fmt.Sprintf("digest-%s@example.test", zone))
	ctx := context.Background()
	if _, err := pool.Exec(ctx, "UPDATE users SET email_verified=true,last_active_at=now(),timezone=$2 WHERE id=$1", uid, zone); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO notification_settings(user_id,enabled,channels,digest_frequency,digest_time,inbox_digest_enabled,inbox_digest_enabled_at) VALUES($1,true,'{email}','daily','09:00',true,$2)`, uid, now.Add(-2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	return uid
}
func digestMail(t *testing.T, pool *pgxpool.Pool, uid int64, ext, thread, signal, direction string, at time.Time) int64 {
	t.Helper()
	var id int64
	err := pool.QueryRow(context.Background(), `INSERT INTO emails(user_id,source,external_id,thread_id,subject,body_text,status_signal,classified_at,direction,received_at) VALUES($1,'gmail',$2,$3,$2,'Original excerpt',$4,now(),$5,$6) RETURNING id`, uid, ext, thread, signal, direction, at).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestInboxDigestGroupsAndRetiresOverflow(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	now := time.Date(2030, 1, 2, 12, 0, 0, 0, time.UTC)
	uid := digestAccount(t, pool, now, "UTC")
	first := digestMail(t, pool, uid, "first", "thread", "screening", "incoming", now.Add(-time.Hour))
	latest := digestMail(t, pool, uid, "latest", "thread", "info_request", "incoming", now.Add(-30*time.Minute))
	digestMail(t, pool, uid, "reply", "thread", "other", "outgoing", now.Add(-10*time.Minute))
	digestMail(t, pool, uid, "noise", "noise", "other", "incoming", now.Add(-time.Hour))
	digestMail(t, pool, uid, "historical", "old", "screening", "incoming", now.Add(-3*time.Hour))
	deleted := digestMail(t, pool, uid, "deleted", "deleted", "offer", "incoming", now.Add(-time.Hour))
	if _, err := pool.Exec(ctx, "UPDATE emails SET deleted_at=now() WHERE id=$1", deleted); err != nil {
		t.Fatal(err)
	}
	for i := range 12 {
		digestMail(t, pool, uid, fmt.Sprintf("new-%d", i), fmt.Sprintf("thread-%d", i), "acknowledgement", "incoming", now.Add(-time.Hour))
	}
	snapshot, err := q.GetInboxDigestSnapshot(ctx, GetInboxDigestSnapshotParams{UserID: uid, EnabledAt: digestTime(now.Add(-2 * time.Hour)), NowAt: digestTime(now)})
	if err != nil {
		t.Fatal(err)
	}
	var items []struct {
		ID      int64 `json:"id"`
		Replied bool  `json:"replied"`
	}
	if err := json.Unmarshal(snapshot.Messages, &items); err != nil {
		t.Fatal(err)
	}
	if snapshot.Total != 13 || len(items) != 10 || len(snapshot.EmailIds) != 14 {
		t.Fatalf("conversations=%d shown=%d covered=%d", snapshot.Total, len(items), len(snapshot.EmailIds))
	}
	if items[0].ID != latest || !items[0].Replied {
		t.Fatalf("thread latest/reply evidence lost: %+v", items[0])
	}
	_ = first
	candidates, err := q.ListInboxDigestCandidates(ctx, ListInboxDigestCandidatesParams{NowAt: digestTime(now), BatchSize: 1})
	if err != nil || len(candidates) != 1 {
		t.Fatalf("candidates=%v err=%v", candidates, err)
	}
	c := candidates[0]
	claim := ClaimInboxDigestParams{UserID: uid, Email: c.Email, Timezone: c.Timezone, EnabledAt: c.EnabledAt, EmailIds: snapshot.EmailIds, NowAt: digestTime(now), LocalDay: pgtype.Date{Time: now, Valid: true}}
	var wg sync.WaitGroup
	results := make(chan int64, 2)
	errs := make(chan error, 2)
	for range 2 {
		wg.Go(func() { n, err := q.ClaimInboxDigest(ctx, claim); results <- n; errs <- err })
	}
	wg.Wait()
	close(results)
	close(errs)
	var claimed int64
	for n := range results {
		claimed += n
	}
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if claimed != 1 {
		t.Fatalf("concurrent day claimed %d times", claimed)
	}
	if _, err := q.MarkInboxDigestSent(ctx, MarkInboxDigestSentParams{UserID: uid, LocalDay: claim.LocalDay, SentAt: digestTime(now)}); err != nil {
		t.Fatal(err)
	}
	next, err := q.GetInboxDigestSnapshot(ctx, GetInboxDigestSnapshotParams{UserID: uid, EnabledAt: c.EnabledAt, NowAt: digestTime(now.Add(24 * time.Hour))})
	if err != nil {
		t.Fatal(err)
	}
	if next.Total != 0 || len(next.EmailIds) != 0 {
		t.Fatal("overflow or delivered mail was requeued")
	}
}

func TestInboxDigestTimezoneAndLiveConsent(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	now := time.Date(2030, 1, 2, 14, 0, 0, 0, time.UTC)
	uid := digestAccount(t, pool, now, "America/New_York")
	digestMail(t, pool, uid, "timed", "thread", "screening", "incoming", now.Add(-time.Hour))
	before, err := q.ListInboxDigestCandidates(ctx, ListInboxDigestCandidatesParams{NowAt: digestTime(now.Add(-time.Minute)), BatchSize: 10})
	if err != nil || len(before) != 0 {
		t.Fatalf("sent before local time: %v %v", before, err)
	}
	due, err := q.ListInboxDigestCandidates(ctx, ListInboxDigestCandidatesParams{NowAt: digestTime(now), BatchSize: 10})
	if err != nil || len(due) != 1 {
		t.Fatalf("not due at local nine: %v %v", due, err)
	}
	s, err := q.GetInboxDigestSnapshot(ctx, GetInboxDigestSnapshotParams{UserID: uid, EnabledAt: due[0].EnabledAt, NowAt: digestTime(now)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "UPDATE notification_settings SET enabled=false WHERE user_id=$1", uid); err != nil {
		t.Fatal(err)
	}
	n, err := q.ClaimInboxDigest(ctx, ClaimInboxDigestParams{UserID: uid, Email: due[0].Email, Timezone: due[0].Timezone, EnabledAt: due[0].EnabledAt, EmailIds: s.EmailIds, NowAt: digestTime(now), LocalDay: pgtype.Date{Time: now, Valid: true}})
	if err != nil || n != 0 {
		t.Fatalf("unsubscribed account claimed: n=%d err=%v", n, err)
	}
}

func TestInboxDigestDoesNotResurrectSupersededRequest(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	now := time.Date(2030, 1, 2, 12, 0, 0, 0, time.UTC)
	uid := digestAccount(t, pool, now, "UTC")
	if _, err := pool.Exec(ctx, "UPDATE notification_settings SET inbox_digest_enabled_at=$2 WHERE user_id=$1", uid, now.Add(-48*time.Hour)); err != nil {
		t.Fatal(err)
	}
	older := digestMail(t, pool, uid, "old-interview", "same", "interview_invitation", "incoming", now.Add(-26*time.Hour))
	newer := digestMail(t, pool, uid, "new-rejection", "same", "rejection", "incoming", now.Add(-25*time.Hour))
	if _, err := pool.Exec(ctx, `INSERT INTO inbox_digest_deliveries(user_id,local_day,email_ids,claimed_at,sent_at) VALUES($1,$2,ARRAY[$3]::bigint[],$4,$4)`, uid, now.Add(-24*time.Hour), newer, now.Add(-24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	snapshot, err := q.GetInboxDigestSnapshot(ctx, GetInboxDigestSnapshotParams{UserID: uid, EnabledAt: digestTime(now.Add(-48 * time.Hour)), NowAt: digestTime(now)})
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Total != 0 {
		t.Fatalf("late-classified old message %d resurrected an outdated request: %s", older, snapshot.Messages)
	}
	candidates, err := q.ListInboxDigestCandidates(ctx, ListInboxDigestCandidatesParams{NowAt: digestTime(now), BatchSize: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 0 {
		t.Fatal("superseded messages kept an empty account eligible")
	}
}
