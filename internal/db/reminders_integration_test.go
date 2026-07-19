//go:build integration

// Integration tests for the saved-job reminder queries — the partial-unique
// upsert arbiter, the due-scan claim CTE, and the status transitions can only be
// verified against a real Postgres. Run with: go test -tags=integration ./internal/db/
// Requires Docker (testcontainers spins up a throwaway Postgres with the migrations).
package db

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func ts(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }

func TestReminderSettings(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	if _, err := pool.Exec(ctx, "TRUNCATE reminder_settings, users RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	uid := insertUser(t, pool, "rules@example.test")

	// No row yet -> ErrNoRows (the repo maps this to the off-by-default state).
	if _, err := q.GetReminderSettings(ctx, uid); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("GetReminderSettings empty: err = %v, want pgx.ErrNoRows", err)
	}

	saved, err := q.UpsertReminderSettings(ctx, UpsertReminderSettingsParams{
		UserID: uid, Enabled: true, DefaultDelayDays: 3, Channels: []string{"email"},
	})
	if err != nil {
		t.Fatalf("upsert settings: %v", err)
	}
	if !saved.Enabled || saved.DefaultDelayDays != 3 || len(saved.Channels) != 1 {
		t.Errorf("saved settings = %+v", saved)
	}

	// Upsert replaces in place (one row per user).
	if _, err := q.UpsertReminderSettings(ctx, UpsertReminderSettingsParams{
		UserID: uid, Enabled: false, DefaultDelayDays: 7, Channels: []string{"telegram", "email"},
	}); err != nil {
		t.Fatalf("re-upsert settings: %v", err)
	}
	got, err := q.GetReminderSettings(ctx, uid)
	if err != nil {
		t.Fatalf("get settings: %v", err)
	}
	if got.Enabled || got.DefaultDelayDays != 7 || len(got.Channels) != 2 {
		t.Errorf("updated settings = %+v, want disabled/7/2 channels", got)
	}
}

func TestJobReminderLifecycle(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	reset := func(t *testing.T) {
		t.Helper()
		if _, err := pool.Exec(ctx, "TRUNCATE job_reminders, user_jobs, users, jobs RESTART IDENTITY CASCADE"); err != nil {
			t.Fatalf("truncate: %v", err)
		}
	}
	countPending := func(t *testing.T, uid, jid int64) int {
		t.Helper()
		var n int
		if err := pool.QueryRow(ctx,
			`SELECT count(*) FROM job_reminders WHERE user_id=$1 AND job_id=$2 AND status='pending'`, uid, jid).Scan(&n); err != nil {
			t.Fatalf("count pending: %v", err)
		}
		return n
	}

	t.Run("upsert schedules one pending reminder and replaces it in place", func(t *testing.T) {
		reset(t)
		uid := insertUser(t, pool, "sched@example.test")
		jid := insertJob(t, pool, "sched-job")

		if _, err := q.UpsertJobReminder(ctx, UpsertJobReminderParams{
			UserID: uid, JobID: jid, FireAt: ts(time.Now().Add(72 * time.Hour)), Channels: []string{"email"},
		}); err != nil {
			t.Fatalf("first upsert: %v", err)
		}
		// A second upsert (re-save with a new choice) must replace, not duplicate.
		row, err := q.UpsertJobReminder(ctx, UpsertJobReminderParams{
			UserID: uid, JobID: jid, FireAt: ts(time.Now().Add(24 * time.Hour)), Channels: []string{"telegram"},
		})
		if err != nil {
			t.Fatalf("second upsert: %v", err)
		}
		if countPending(t, uid, jid) != 1 {
			t.Errorf("pending rows = %d, want 1 (partial-unique replace)", countPending(t, uid, jid))
		}
		if len(row.Channels) != 1 || row.Channels[0] != "telegram" {
			t.Errorf("channels = %v, want [telegram] after replace", row.Channels)
		}
	})

	t.Run("cancel is idempotent", func(t *testing.T) {
		reset(t)
		uid := insertUser(t, pool, "cancel@example.test")
		jid := insertJob(t, pool, "cancel-job")
		if _, err := q.UpsertJobReminder(ctx, UpsertJobReminderParams{
			UserID: uid, JobID: jid, FireAt: ts(time.Now().Add(48 * time.Hour)), Channels: []string{"email"},
		}); err != nil {
			t.Fatalf("upsert: %v", err)
		}
		n, err := q.CancelJobReminder(ctx, CancelJobReminderParams{UserID: uid, JobID: jid})
		if err != nil || n != 1 {
			t.Fatalf("cancel: n=%d err=%v, want 1", n, err)
		}
		n, err = q.CancelJobReminder(ctx, CancelJobReminderParams{UserID: uid, JobID: jid})
		if err != nil || n != 0 {
			t.Fatalf("re-cancel: n=%d err=%v, want 0 (idempotent)", n, err)
		}
		if countPending(t, uid, jid) != 0 {
			t.Error("cancel left a pending row")
		}
	})

	t.Run("reschedule reports no row when none is pending", func(t *testing.T) {
		reset(t)
		uid := insertUser(t, pool, "resched@example.test")
		jid := insertJob(t, pool, "resched-job")
		_, err := q.RescheduleJobReminder(ctx, RescheduleJobReminderParams{
			FireAt: ts(time.Now().Add(24 * time.Hour)), UserID: uid, JobID: jid,
		})
		if !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("reschedule with no pending: err = %v, want pgx.ErrNoRows", err)
		}
	})

	t.Run("claim leases only due reminders, delivery marks terminal", func(t *testing.T) {
		reset(t)
		uid := insertUser(t, pool, "claim@example.test")
		dueJob := insertJob(t, pool, "due-job")
		futureJob := insertJob(t, pool, "future-job")

		if _, err := q.SaveJob(ctx, SaveJobParams{UserID: uid, JobID: dueJob}); err != nil {
			t.Fatalf("save due: %v", err)
		}
		// One reminder already due, one in the future.
		if _, err := q.UpsertJobReminder(ctx, UpsertJobReminderParams{
			UserID: uid, JobID: dueJob, FireAt: ts(time.Now().Add(-time.Minute)), Channels: []string{"email"},
		}); err != nil {
			t.Fatalf("upsert due: %v", err)
		}
		if _, err := q.UpsertJobReminder(ctx, UpsertJobReminderParams{
			UserID: uid, JobID: futureJob, FireAt: ts(time.Now().Add(72 * time.Hour)), Channels: []string{"email"},
		}); err != nil {
			t.Fatalf("upsert future: %v", err)
		}

		claimed, err := q.ClaimDueReminders(ctx, ClaimDueRemindersParams{LeaseSeconds: 600, BatchSize: 50})
		if err != nil {
			t.Fatalf("claim: %v", err)
		}
		if len(claimed) != 1 {
			t.Fatalf("claimed = %d, want 1 (only the due reminder)", len(claimed))
		}
		id := claimed[0]

		info, err := q.GetReminderForDelivery(ctx, id)
		if err != nil {
			t.Fatalf("delivery context: %v", err)
		}
		if !info.JobOpen || !info.StillActionable || info.AccountEmail == "" {
			t.Errorf("delivery flags: open=%v actionable=%v email=%q, want open+actionable+email",
				info.JobOpen, info.StillActionable, info.AccountEmail)
		}

		n, err := q.MarkReminderDelivered(ctx, id)
		if err != nil || n != 1 {
			t.Fatalf("mark delivered: n=%d err=%v, want 1", n, err)
		}
		// A delivered reminder is terminal: a second claim finds nothing new.
		again, err := q.ClaimDueReminders(ctx, ClaimDueRemindersParams{LeaseSeconds: 600, BatchSize: 50})
		if err != nil {
			t.Fatalf("re-claim: %v", err)
		}
		if len(again) != 0 {
			t.Errorf("re-claim = %d, want 0 (delivered reminder never re-fires)", len(again))
		}
	})

	t.Run("delivery context reports a non-actionable reminder for an applied job", func(t *testing.T) {
		reset(t)
		uid := insertUser(t, pool, "applied@example.test")
		jid := insertJob(t, pool, "applied-job")
		if _, err := q.SaveJob(ctx, SaveJobParams{UserID: uid, JobID: jid}); err != nil {
			t.Fatalf("save: %v", err)
		}
		if _, err := q.UpsertJobReminder(ctx, UpsertJobReminderParams{
			UserID: uid, JobID: jid, FireAt: ts(time.Now().Add(-time.Minute)), Channels: []string{"email"},
		}); err != nil {
			t.Fatalf("upsert: %v", err)
		}
		if _, err := q.MarkJobApplied(ctx, MarkJobAppliedParams{UserID: uid, JobID: jid}); err != nil {
			t.Fatalf("apply: %v", err)
		}
		claimed, err := q.ClaimDueReminders(ctx, ClaimDueRemindersParams{LeaseSeconds: 600, BatchSize: 50})
		if err != nil || len(claimed) != 1 {
			t.Fatalf("claim: %v (n=%d)", err, len(claimed))
		}
		info, err := q.GetReminderForDelivery(ctx, claimed[0])
		if err != nil {
			t.Fatalf("delivery context: %v", err)
		}
		if info.StillActionable {
			t.Error("still_actionable must be false once the job is applied — the worker cancels-and-skips")
		}
	})

	t.Run("ListUserJobs projects the pending reminder fire time", func(t *testing.T) {
		reset(t)
		uid := insertUser(t, pool, "list@example.test")
		jid := insertJob(t, pool, "list-job")
		if _, err := q.SaveJob(ctx, SaveJobParams{UserID: uid, JobID: jid}); err != nil {
			t.Fatalf("save: %v", err)
		}
		if _, err := q.UpsertJobReminder(ctx, UpsertJobReminderParams{
			UserID: uid, JobID: jid, FireAt: ts(time.Now().Add(72 * time.Hour)), Channels: []string{"email"},
		}); err != nil {
			t.Fatalf("upsert: %v", err)
		}
		rows, err := q.ListUserJobs(ctx, ListUserJobsParams{UserID: uid, Filter: "saved", Limit: 10, Offset: 0})
		if err != nil {
			t.Fatalf("list saved: %v", err)
		}
		if len(rows) != 1 || !rows[0].ReminderFireAt.Valid {
			t.Fatalf("saved listing must carry a reminder_fire_at, got %d rows (valid=%v)", len(rows), len(rows) == 1 && rows[0].ReminderFireAt.Valid)
		}

		// After cancel, the projection goes back to null.
		if _, err := q.CancelJobReminder(ctx, CancelJobReminderParams{UserID: uid, JobID: jid}); err != nil {
			t.Fatalf("cancel: %v", err)
		}
		rows, err = q.ListUserJobs(ctx, ListUserJobsParams{UserID: uid, Filter: "saved", Limit: 10, Offset: 0})
		if err != nil {
			t.Fatalf("list saved after cancel: %v", err)
		}
		if len(rows) != 1 || rows[0].ReminderFireAt.Valid {
			t.Errorf("cancelled reminder must project null fire_at, got valid=%v", len(rows) == 1 && rows[0].ReminderFireAt.Valid)
		}
	})
}
