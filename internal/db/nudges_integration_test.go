//go:build integration

// Integration tests for the internal/nudge decision layer's queries — the MATCH
// candidate scans, the idempotent record, the claim CTE, and the delivery-context
// join can only be verified against a real Postgres.
// Run with: go test -tags=integration ./internal/db/
package db

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// resetNudgeTables truncates everything the nudge queries touch, leaving users and
// jobs at identity 1 so test fixtures have predictable ids.
func resetNudgeTables(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		"TRUNCATE application_nudges, application_events, applications, notification_settings, emails, jobs, users RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}
}

// insertApplication inserts a minimal applications row: an application with no
// posting-independent fields the nudge queries don't read.
func insertApplication(t *testing.T, pool *pgxpool.Pool, uid, jid int64, appliedAt time.Time, stage string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO applications (user_id, job_id, applied_at, stage) VALUES ($1, $2, $3, NULLIF($4, ''))`,
		uid, jid, appliedAt, stage); err != nil {
		t.Fatalf("insert application: %v", err)
	}
}

// insertStageSetEvent inserts a minimal application_events row for a stage_set
// transition.
func insertStageSetEvent(t *testing.T, pool *pgxpool.Pool, uid, jid int64, stage string, occurredAt time.Time, retracted bool) {
	t.Helper()
	var retractedAt any
	if retracted {
		retractedAt = time.Now()
	}
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO application_events (user_id, job_id, kind, signal, occurred_at, source, retracted_at)
		 VALUES ($1, $2, 'stage_set', $3, $4, 'user', $5)`,
		uid, jid, stage, occurredAt, retractedAt); err != nil {
		t.Fatalf("insert stage_set event: %v", err)
	}
}

func TestListFollowUpCandidates(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	resetNudgeTables(t, pool)

	uidEnabled := insertUser(t, pool, "enabled@example.test")
	uidDisabled := insertUser(t, pool, "disabled@example.test")
	if _, err := q.UpsertNotificationSettings(ctx, UpsertNotificationSettingsParams{UserID: uidEnabled, Enabled: true, DigestFrequency: "instant", Channels: []string{"email"}}); err != nil {
		t.Fatalf("upsert settings enabled: %v", err)
	}
	if _, err := q.UpsertNotificationSettings(ctx, UpsertNotificationSettingsParams{UserID: uidDisabled, Enabled: false, DigestFrequency: "instant", Channels: []string{"email"}}); err != nil {
		t.Fatalf("upsert settings disabled: %v", err)
	}

	// Truncated to microsecond precision: timestamptz's own resolution, so the
	// value read back from Postgres compares equal to the one written.
	withinWindow := time.Now().Add(-25 * 24 * time.Hour).Truncate(time.Microsecond)
	outsideWindow := time.Now().Add(-40 * 24 * time.Hour).Truncate(time.Microsecond)

	openJob := insertJob(t, pool, "within-window")
	insertApplication(t, pool, uidEnabled, openJob, withinWindow, "applied")

	staleJob := insertJob(t, pool, "outside-window")
	insertApplication(t, pool, uidEnabled, staleJob, outsideWindow, "applied")

	disabledJob := insertJob(t, pool, "disabled-user")
	insertApplication(t, pool, uidDisabled, disabledJob, withinWindow, "applied")

	closedJob := insertJob(t, pool, "closed-job")
	insertApplication(t, pool, uidEnabled, closedJob, withinWindow, "applied")
	if _, err := pool.Exec(ctx, "UPDATE jobs SET closed_at = now() WHERE id = $1", closedJob); err != nil {
		t.Fatalf("close job: %v", err)
	}

	rows, err := q.ListFollowUpCandidates(ctx, 30)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("candidates = %d, want 1 (only the enabled user's open, in-window application)", len(rows))
	}
	got := rows[0]
	if got.UserID != uidEnabled || !got.JobID.Valid || got.JobID.Int64 != openJob {
		t.Errorf("candidate = %+v, want user=%d job=%d", got, uidEnabled, openJob)
	}
	if !got.Stage.Valid || got.Stage.String != "applied" {
		t.Errorf("stage = %+v, want applied", got.Stage)
	}
	if !got.LastActivityAt.Valid || !got.LastActivityAt.Time.Equal(withinWindow) {
		t.Errorf("last_activity_at = %v, want %v", got.LastActivityAt, withinWindow)
	}
}

func TestListInterviewPrepCandidates(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	resetNudgeTables(t, pool)

	uid := insertUser(t, pool, "interview@example.test")
	if _, err := q.UpsertNotificationSettings(ctx, UpsertNotificationSettingsParams{UserID: uid, Enabled: true, DigestFrequency: "instant", Channels: []string{"email"}}); err != nil {
		t.Fatalf("upsert settings: %v", err)
	}

	// Truncated to microsecond precision — see the same note in TestListFollowUpCandidates.
	recent := time.Now().Add(-2 * time.Hour).Truncate(time.Microsecond)
	old := time.Now().Add(-10 * 24 * time.Hour).Truncate(time.Microsecond)

	freshJob := insertJob(t, pool, "fresh-interview")
	insertStageSetEvent(t, pool, uid, freshJob, "interview", recent, false)

	retractedJob := insertJob(t, pool, "retracted-interview")
	insertStageSetEvent(t, pool, uid, retractedJob, "interview", recent, true)

	staleJob := insertJob(t, pool, "stale-interview")
	insertStageSetEvent(t, pool, uid, staleJob, "interview", old, false)

	otherStageJob := insertJob(t, pool, "not-interview")
	insertStageSetEvent(t, pool, uid, otherStageJob, "screening", recent, false)

	rows, err := q.ListInterviewPrepCandidates(ctx, 7)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("candidates = %d, want 1 (only the fresh, live, in-window interview transition)", len(rows))
	}
	got := rows[0]
	if got.UserID != uid || !got.JobID.Valid || got.JobID.Int64 != freshJob {
		t.Errorf("candidate = %+v, want user=%d job=%d", got, uid, freshJob)
	}
	if !got.OccurredAt.Valid || !got.OccurredAt.Time.Equal(recent) {
		t.Errorf("occurred_at = %v, want %v", got.OccurredAt, recent)
	}
}

func TestListJobClosedCandidates(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	resetNudgeTables(t, pool)

	uid := insertUser(t, pool, "closed@example.test")
	if _, err := q.UpsertNotificationSettings(ctx, UpsertNotificationSettingsParams{UserID: uid, Enabled: true, DigestFrequency: "instant", Channels: []string{"email"}}); err != nil {
		t.Fatalf("upsert settings: %v", err)
	}

	// Truncated to microsecond precision — see the same note in TestListFollowUpCandidates.
	recentClose := time.Now().Add(-2 * 24 * time.Hour).Truncate(time.Microsecond)
	staleClose := time.Now().Add(-10 * 24 * time.Hour).Truncate(time.Microsecond)
	appliedAt := time.Now().Add(-30 * 24 * time.Hour)

	activeJob := insertJob(t, pool, "closed-active")
	insertApplication(t, pool, uid, activeJob, appliedAt, "screening")
	if _, err := pool.Exec(ctx, "UPDATE jobs SET closed_at = $1 WHERE id = $2", recentClose, activeJob); err != nil {
		t.Fatalf("close active job: %v", err)
	}

	settledJob := insertJob(t, pool, "closed-settled")
	insertApplication(t, pool, uid, settledJob, appliedAt, "withdrawn")
	if _, err := pool.Exec(ctx, "UPDATE jobs SET closed_at = $1 WHERE id = $2", recentClose, settledJob); err != nil {
		t.Fatalf("close settled job: %v", err)
	}

	staleJob := insertJob(t, pool, "closed-stale")
	insertApplication(t, pool, uid, staleJob, appliedAt, "screening")
	if _, err := pool.Exec(ctx, "UPDATE jobs SET closed_at = $1 WHERE id = $2", staleClose, staleJob); err != nil {
		t.Fatalf("close stale job: %v", err)
	}

	openJob := insertJob(t, pool, "still-open")
	insertApplication(t, pool, uid, openJob, appliedAt, "screening")

	rows, err := q.ListJobClosedCandidates(ctx, 7)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("candidates = %d, want 2 (both in-window closures; the Go-side active-stage filter drops the settled one)", len(rows))
	}
	byJob := map[int64]ListJobClosedCandidatesRow{}
	for _, r := range rows {
		byJob[r.JobID.Int64] = r
	}
	if _, ok := byJob[activeJob]; !ok {
		t.Errorf("missing the active application's closed job, got %+v", rows)
	}
	if _, ok := byJob[settledJob]; !ok {
		t.Errorf("missing the settled application's closed job (SQL doesn't filter stage; Go does), got %+v", rows)
	}
	if got := byJob[activeJob]; !got.ClosedAt.Valid || !got.ClosedAt.Time.Equal(recentClose) {
		t.Errorf("closed_at = %v, want %v", got.ClosedAt, recentClose)
	}
}

func TestRecordNudge_IsIdempotentPerEpisode(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	resetNudgeTables(t, pool)

	uid := insertUser(t, pool, "episode@example.test")
	jid := insertJob(t, pool, "episode-job")
	episode1 := ts(time.Now().Add(-25 * 24 * time.Hour))
	episode2 := ts(time.Now().Add(-1 * time.Hour))

	affected, err := q.RecordNudge(ctx, RecordNudgeParams{UserID: uid, JobID: jid, Kind: "follow_up", EpisodeKey: episode1})
	if err != nil || affected != 1 {
		t.Fatalf("first record: affected=%d err=%v, want 1", affected, err)
	}
	affected, err = q.RecordNudge(ctx, RecordNudgeParams{UserID: uid, JobID: jid, Kind: "follow_up", EpisodeKey: episode1})
	if err != nil || affected != 0 {
		t.Fatalf("re-scan of the same episode: affected=%d err=%v, want 0 (idempotent)", affected, err)
	}
	affected, err = q.RecordNudge(ctx, RecordNudgeParams{UserID: uid, JobID: jid, Kind: "follow_up", EpisodeKey: episode2})
	if err != nil || affected != 1 {
		t.Fatalf("new episode: affected=%d err=%v, want 1 (a changed episode key is a new nudge)", affected, err)
	}
}

func TestNudgeClaimAndDelivery(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	resetNudgeTables(t, pool)

	uid := insertUser(t, pool, "deliver@example.test")
	jid := insertJob(t, pool, "deliver-job")
	if _, err := q.UpsertNotificationSettings(ctx, UpsertNotificationSettingsParams{UserID: uid, Enabled: true, DigestFrequency: "instant", Channels: []string{"email"}}); err != nil {
		t.Fatalf("upsert settings: %v", err)
	}
	appliedAt := time.Now().Add(-25 * 24 * time.Hour)
	insertApplication(t, pool, uid, jid, appliedAt, "applied")

	if _, err := q.RecordNudge(ctx, RecordNudgeParams{UserID: uid, JobID: jid, Kind: "follow_up", EpisodeKey: ts(appliedAt)}); err != nil {
		t.Fatalf("record: %v", err)
	}

	claimed, err := q.ClaimDueNudges(ctx, ClaimDueNudgesParams{LeaseSeconds: 600, BatchSize: 50})
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if len(claimed) != 1 {
		t.Fatalf("claimed = %d, want 1", len(claimed))
	}
	id := claimed[0]

	info, err := q.GetNudgeForDelivery(ctx, id)
	if err != nil {
		t.Fatalf("delivery context: %v", err)
	}
	if info.Kind != "follow_up" || !info.JobOpen || !info.NotificationsEnabled || info.AccountEmail == "" {
		t.Errorf("delivery context = %+v, want follow_up/open/enabled/with account email", info)
	}
	if !info.Stage.Valid || info.Stage.String != "applied" {
		t.Errorf("stage = %+v, want applied", info.Stage)
	}

	n, err := q.MarkNudgeDelivered(ctx, id)
	if err != nil || n != 1 {
		t.Fatalf("mark delivered: n=%d err=%v, want 1", n, err)
	}
	// A delivered nudge is terminal: a second claim finds nothing new.
	again, err := q.ClaimDueNudges(ctx, ClaimDueNudgesParams{LeaseSeconds: 600, BatchSize: 50})
	if err != nil {
		t.Fatalf("re-claim: %v", err)
	}
	if len(again) != 0 {
		t.Errorf("re-claim = %d, want 0 (delivered nudge never re-fires)", len(again))
	}
}

func TestCancelNudgeAtFire_IsTerminal(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	resetNudgeTables(t, pool)

	uid := insertUser(t, pool, "cancel@example.test")
	jid := insertJob(t, pool, "cancel-job")
	if _, err := q.RecordNudge(ctx, RecordNudgeParams{UserID: uid, JobID: jid, Kind: "interview_prep", EpisodeKey: ts(time.Now())}); err != nil {
		t.Fatalf("record: %v", err)
	}
	claimed, err := q.ClaimDueNudges(ctx, ClaimDueNudgesParams{LeaseSeconds: 600, BatchSize: 50})
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claim: %v (n=%d)", err, len(claimed))
	}
	n, err := q.CancelNudgeAtFire(ctx, claimed[0])
	if err != nil || n != 1 {
		t.Fatalf("cancel: n=%d err=%v, want 1", n, err)
	}
	// Cancelled is terminal too.
	again, err := q.ClaimDueNudges(ctx, ClaimDueNudgesParams{LeaseSeconds: 600, BatchSize: 50})
	if err != nil {
		t.Fatalf("re-claim: %v", err)
	}
	if len(again) != 0 {
		t.Errorf("re-claim = %d, want 0 (cancelled nudge never re-fires)", len(again))
	}
}
