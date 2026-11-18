//go:build integration

// Integration tests for the report's SQL against a real Postgres. Rows are inserted with the
// timestamps each case needs; nothing here runs the letters.
package cvlifecycle

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

// enrolled inserts an account and its enrollment. paidAfter is when it started paying,
// counted from the upload; zero means never.
func enrolled(t *testing.T, pool *pgxpool.Pool, label string, uploadedAgo, enrollAfterUpload time.Duration, holdout bool, paidAfter time.Duration) int64 {
	t.Helper()
	id := newUser(t, pool, label, uploadedAgo)
	uploaded := time.Now().Add(-uploadedAgo)
	var paid *time.Time
	if paidAfter > 0 {
		p := uploaded.Add(paidAfter)
		paid = &p
	}
	exec(t, pool, `
		INSERT INTO cv_lifecycle_enrollments (user_id, uploaded_at, enrolled_at, holdout, paid_at)
		VALUES ($1, $2, $3, $4, $5)`, id, uploaded, uploaded.Add(enrollAfterUpload), holdout, paid)
	return id
}

func sendStep(t *testing.T, pool *pgxpool.Pool, id int64, step int, sentAgo time.Duration, errText string) {
	t.Helper()
	exec(t, pool, `INSERT INTO cv_lifecycle_sends (user_id, step, sent_at, error) VALUES ($1, $2, $3, $4)`,
		id, step, time.Now().Add(-sentAgo), errText)
}

func TestReport_LetterPaymentsCountOnlyClosedWindowsAndSentLetters(t *testing.T) {
	pool := testdb.Pool(t)
	q := db.New(pool)

	// Letters sent 20 days ago: a payer inside 14 days of the letter, one outside it.
	// paid_at is counted from the upload, which was 21 days ago, so the letter fell on day 1.
	inside := enrolled(t, pool, "pay-in", 21*day, 0, false, 6*day)
	outside := enrolled(t, pool, "pay-out", 21*day, 0, false, 17*day)
	burned := enrolled(t, pool, "burned", 21*day, 0, false, 6*day)
	open := enrolled(t, pool, "open", 6*day, 0, false, 3*day)
	sendStep(t, pool, inside, 1, 20*day, "")
	sendStep(t, pool, outside, 1, 20*day, "")
	sendStep(t, pool, burned, 1, 20*day, "ses refused")
	sendStep(t, pool, open, 1, 5*day, "")
	early := enrolled(t, pool, "pay-early", 21*day, 0, false, 5*day)
	sendStep(t, pool, early, 2, 15*day, "")

	got, err := q.CVLetterPayments(context.Background(), db.CVLetterPaymentsParams{WindowDays: 14, Step: 1})
	if err != nil {
		t.Fatal(err)
	}
	// A burned step is not a letter; the open window is sent but not yet readable.
	if got.Sent != 3 || got.Matured != 2 || got.Paid != 1 {
		t.Errorf("step 1 = %+v, want sent 3, matured 2, paid 1", got)
	}
	got2, err := q.CVLetterPayments(context.Background(), db.CVLetterPaymentsParams{WindowDays: 14, Step: 2})
	if err != nil {
		t.Fatal(err)
	}
	// Letter 2 went 15 days ago, on day 6; the payment on day 5 is before it.
	if got2.Sent != 1 || got2.Matured != 1 || got2.Paid != 0 {
		t.Errorf("step 2 = %+v, want sent 1, matured 1, paid 0 (paid before the letter)", got2)
	}
}

func TestReport_HoldoutControlIsOnTheLettersOwnClock(t *testing.T) {
	pool := testdb.Pool(t)
	q := db.New(pool)

	enrolled(t, pool, "h-paid", 30*day, 0, true, 10*day)
	enrolled(t, pool, "h-unpaid", 30*day, 0, true, 0)
	enrolled(t, pool, "h-late", 30*day, 0, true, 20*day)
	// Paid before the letter fell due: not at risk when it did.
	enrolled(t, pool, "h-early", 30*day, 0, true, 12*time.Hour)
	// Window not closed yet; and a mailed account, which is no control.
	enrolled(t, pool, "h-young", 10*day, 0, true, 0)
	enrolled(t, pool, "m-paid", 30*day, 0, false, 10*day)

	got, err := q.CVLetterHoldoutAtStep(context.Background(), db.CVLetterHoldoutAtStepParams{AfterDays: 1, WindowDays: 14})
	if err != nil {
		t.Fatal(err)
	}
	if got.Accounts != 3 || got.Paid != 1 {
		t.Errorf("control = %+v, want 3 accounts, 1 paid", got)
	}

	// Letter 3 falls on day 13: the window runs to day 27, so the payer on day 20 counts
	// and the one on day 10 had already paid.
	got, err = q.CVLetterHoldoutAtStep(context.Background(), db.CVLetterHoldoutAtStepParams{AfterDays: 13, WindowDays: 14})
	if err != nil {
		t.Fatal(err)
	}
	if got.Accounts != 2 || got.Paid != 1 {
		t.Errorf("control for letter 3 = %+v, want 2 accounts, 1 paid", got)
	}
}

func TestReport_GroupsFromEnrollmentLeaveOutOpenWindows(t *testing.T) {
	pool := testdb.Pool(t)
	q := db.New(pool)

	enrolled(t, pool, "g-held-paid", 40*day, 0, true, 3*day)
	enrolled(t, pool, "g-held-none", 40*day, 0, true, 0)
	enrolled(t, pool, "g-mail-paid", 40*day, 0, false, 14*day)
	enrolled(t, pool, "g-mail-slow", 40*day, 0, false, 20*day)
	enrolled(t, pool, "g-mail-none", 40*day, 0, false, 0)
	// Enrolled a week after the upload: the window counts from enrollment, not the upload.
	enrolled(t, pool, "g-mail-late", 40*day, 7*day, false, 20*day)
	enrolled(t, pool, "g-young", 5*day, 0, false, 0)

	rows, err := q.CVLetterGroupsFromEnrollment(context.Background(), 14)
	if err != nil {
		t.Fatal(err)
	}
	got := map[bool]Cohort{}
	for _, r := range rows {
		got[r.Holdout] = Cohort{Accounts: r.Accounts, Paid: r.Paid}
	}
	if got[true] != (Cohort{Accounts: 2, Paid: 1}) {
		t.Errorf("holdout = %+v, want 2 accounts, 1 paid", got[true])
	}
	if got[false] != (Cohort{Accounts: 4, Paid: 2}) {
		t.Errorf("mailed = %+v, want 4 accounts, 2 paid", got[false])
	}
}

func TestReport_EngagementAddsTheLedgerAndItsDailyCounts(t *testing.T) {
	pool := testdb.Pool(t)
	q := db.New(pool)

	exec(t, pool, `
		INSERT INTO email_events (message_id, type, campaign, occurred_at) VALUES
		('m1', 'send',  'cv-letter-1', now()),
		('m1', 'click', 'cv-letter-1', now()),
		('m2', 'send',  'cv-letter-1', now()),
		('m3', 'send',  'alerts-digest', now())`)
	exec(t, pool, `
		INSERT INTO email_event_daily (day, campaign, variant, type, count) VALUES
		(current_date - 120, 'cv-letter-1', '', 'send', 40),
		(current_date - 120, 'cv-letter-1', '', 'click', 3),
		(current_date - 120, 'broadcast-x', '', 'send', 99)`)

	rows, err := q.CVLetterEngagement(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]int64{}
	for _, r := range rows {
		got[r.Campaign+"/"+r.Type] = r.Events
	}
	want := map[string]int64{"cv-letter-1/send": 42, "cv-letter-1/click": 4}
	if len(got) != len(want) {
		t.Fatalf("rows = %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %d, want %d", k, got[k], v)
		}
	}
}
